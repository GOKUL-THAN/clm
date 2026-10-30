#!/usr/bin/env bash
#
# CLM CLI Tool - Installation Script
# Installs the CLM command-line interface for managing certificates, satellites, and deployments.
# Supports: Linux (amd64, arm64), macOS (amd64, arm64), Windows (via WSL)
#
# Usage:
#   curl -sSL https://clm.example.com/install-cli.sh | bash
#   OR
#   ./scripts/install-cli.sh [OPTIONS]
#
# Options:
#   --version       Version to install (default: latest)
#   --install-dir   Installation directory (default: /usr/local/bin or ~/.local/bin)
#   --api-url       Default SaaS API URL to configure
#   --no-profile    Don't update shell profile
#
set -euo pipefail

# ============================================================
# Configuration
# ============================================================
CLM_VERSION="${CLM_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-}"
API_URL="${API_URL:-}"
UPDATE_PROFILE=true
BINARY_NAME="clm"

# GitHub release URL (update with actual repo)
GITHUB_REPO="m2p/clm"
DOWNLOAD_BASE="https://github.com/${GITHUB_REPO}/releases"

# ============================================================
# Colors and helpers
# ============================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

log_info()  { echo -e "${BLUE}[INFO]${NC} $1"; }
log_ok()    { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

die() { log_error "$1"; exit 1; }

# ============================================================
# Parse arguments
# ============================================================
while [[ $# -gt 0 ]]; do
    case $1 in
        --version)      CLM_VERSION="$2"; shift 2 ;;
        --install-dir)  INSTALL_DIR="$2"; shift 2 ;;
        --api-url)      API_URL="$2"; shift 2 ;;
        --no-profile)   UPDATE_PROFILE=false; shift ;;
        --help|-h)
            echo "CLM CLI Installer"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --version VER       Version to install (default: latest)"
            echo "  --install-dir DIR   Install directory (default: auto-detect)"
            echo "  --api-url URL       Set default CLM SaaS API URL"
            echo "  --no-profile        Don't modify shell profile"
            echo ""
            echo "Environment Variables:"
            echo "  CLM_VERSION         Same as --version"
            echo "  CLM_API_URL         Same as --api-url"
            echo ""
            echo "Examples:"
            echo "  curl -sSL https://clm.example.com/install-cli.sh | bash"
            echo "  ./install-cli.sh --version 1.2.0 --api-url https://clm.company.com"
            echo "  ./install-cli.sh --install-dir ~/bin"
            exit 0
            ;;
        *) die "Unknown option: $1. Use --help for usage." ;;
    esac
done

# ============================================================
# Banner
# ============================================================
echo ""
echo -e "${BOLD}╔══════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║     CLM CLI - Installation Script        ║${NC}"
echo -e "${BOLD}║   Certificate Lifecycle Management       ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════╝${NC}"
echo ""

# ============================================================
# Detect platform
# ============================================================
detect_platform() {
    OS_TYPE="$(uname -s)"
    ARCH="$(uname -m)"

    case "$OS_TYPE" in
        Linux)   PLATFORM="linux" ;;
        Darwin)  PLATFORM="darwin" ;;
        MINGW*|MSYS*|CYGWIN*) PLATFORM="windows" ;;
        *) die "Unsupported OS: $OS_TYPE" ;;
    esac

    case "$ARCH" in
        x86_64)       ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        armv7l)       ARCH="armv7" ;;
        i386|i686)    ARCH="386" ;;
        *) die "Unsupported architecture: $ARCH" ;;
    esac
}

detect_platform
log_info "Platform: ${PLATFORM}/${ARCH}"

# ============================================================
# Determine install directory
# ============================================================
determine_install_dir() {
    if [[ -n "$INSTALL_DIR" ]]; then
        return
    fi

    # If running as root, use system path
    if [[ $EUID -eq 0 ]]; then
        INSTALL_DIR="/usr/local/bin"
    else
        # Non-root: use user-local directory
        if [[ -d "$HOME/.local/bin" ]]; then
            INSTALL_DIR="$HOME/.local/bin"
        elif [[ -d "$HOME/bin" ]]; then
            INSTALL_DIR="$HOME/bin"
        else
            INSTALL_DIR="$HOME/.local/bin"
            mkdir -p "$INSTALL_DIR"
        fi
    fi
}

determine_install_dir
log_info "Install directory: $INSTALL_DIR"

# ============================================================
# Check for existing installation
# ============================================================
check_existing() {
    if command -v clm &>/dev/null; then
        EXISTING_PATH=$(command -v clm)
        EXISTING_VERSION=$(clm --version 2>/dev/null || echo "unknown")
        log_info "Existing installation found: $EXISTING_PATH ($EXISTING_VERSION)"
        log_info "Will be upgraded to: $CLM_VERSION"
    fi
}

check_existing

# ============================================================
# Download binary
# ============================================================
download_binary() {
    log_info "Downloading CLM CLI ($CLM_VERSION)..."

    # Build download URL
    local download_url=""
    local filename="${BINARY_NAME}-${PLATFORM}-${ARCH}"

    if [[ "$PLATFORM" == "windows" ]]; then
        filename="${filename}.exe"
    fi

    if [[ "$CLM_VERSION" == "latest" ]]; then
        download_url="${DOWNLOAD_BASE}/latest/download/${filename}"
    else
        download_url="${DOWNLOAD_BASE}/download/v${CLM_VERSION}/${filename}"
    fi

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT

    local tmp_binary="$TMP_DIR/$BINARY_NAME"

    # Try downloading
    local http_code=""
    if command -v curl &>/dev/null; then
        http_code=$(curl -fsSL -w "%{http_code}" -o "$tmp_binary" "$download_url" 2>/dev/null) || true
    elif command -v wget &>/dev/null; then
        wget -q -O "$tmp_binary" "$download_url" 2>/dev/null && http_code="200" || true
    else
        die "Neither curl nor wget found. Install one and retry."
    fi

    # If download failed, try building from source
    if [[ ! -f "$tmp_binary" || ! -s "$tmp_binary" || "$http_code" == "404" ]]; then
        log_warn "Download failed (release may not exist yet). Attempting build from source..."
        build_from_source "$tmp_binary"
    fi

    # Verify binary
    if [[ ! -f "$tmp_binary" || ! -s "$tmp_binary" ]]; then
        die "Failed to obtain CLM binary"
    fi

    # Install
    chmod +x "$tmp_binary"
    mv "$tmp_binary" "$INSTALL_DIR/$BINARY_NAME"

    log_ok "Binary installed at $INSTALL_DIR/$BINARY_NAME"
}

build_from_source() {
    local output="$1"

    if ! command -v go &>/dev/null; then
        die "Go is not installed. Install Go 1.22+ or download a pre-built release."
    fi

    log_info "Building from source with Go..."

    # Find source
    local source_dir=""
    if [[ -f "$(pwd)/go.mod" && -d "$(pwd)/cmd/clm-cli" ]]; then
        source_dir="$(pwd)"
    elif [[ -f "$HOME/clm/go.mod" ]]; then
        source_dir="$HOME/clm"
    elif [[ -f "/opt/clm/go.mod" ]]; then
        source_dir="/opt/clm"
    else
        # Try go install
        log_info "Running: go install github.com/${GITHUB_REPO}/cmd/clm-cli@latest"
        GOBIN="$INSTALL_DIR" go install "github.com/${GITHUB_REPO}/cmd/clm-cli@latest" 2>/dev/null && {
            # go install puts binary as clm-cli, rename to clm
            if [[ -f "$INSTALL_DIR/clm-cli" ]]; then
                mv "$INSTALL_DIR/clm-cli" "$output"
            fi
            return
        }
        die "Cannot find CLM source. Clone it first: git clone https://github.com/${GITHUB_REPO}.git"
    fi

    # Build
    (cd "$source_dir" && go build -ldflags="-s -w" -o "$output" ./cmd/clm-cli)
    log_ok "Built from source successfully"
}

download_binary

# ============================================================
# Verify installation
# ============================================================
verify_install() {
    log_info "Verifying installation..."

    if [[ -x "$INSTALL_DIR/$BINARY_NAME" ]]; then
        log_ok "Binary is executable"
    else
        die "Binary is not executable at $INSTALL_DIR/$BINARY_NAME"
    fi

    # Check if install dir is in PATH
    if echo "$PATH" | tr ':' '\n' | grep -q "^${INSTALL_DIR}$"; then
        log_ok "$INSTALL_DIR is in PATH"
    else
        log_warn "$INSTALL_DIR is NOT in your PATH"
        NEEDS_PATH_UPDATE=true
    fi
}

NEEDS_PATH_UPDATE=false
verify_install

# ============================================================
# Update shell profile (add to PATH if needed)
# ============================================================
update_profile() {
    if [[ "$UPDATE_PROFILE" != true ]]; then
        return
    fi

    if [[ "$NEEDS_PATH_UPDATE" != true && -z "$API_URL" ]]; then
        return
    fi

    # Determine shell profile file
    local profile_file=""
    local shell_name=$(basename "${SHELL:-/bin/bash}")

    case "$shell_name" in
        zsh)  profile_file="$HOME/.zshrc" ;;
        bash)
            if [[ -f "$HOME/.bash_profile" ]]; then
                profile_file="$HOME/.bash_profile"
            else
                profile_file="$HOME/.bashrc"
            fi
            ;;
        fish) profile_file="$HOME/.config/fish/config.fish" ;;
        *)    profile_file="$HOME/.profile" ;;
    esac

    if [[ -z "$profile_file" ]]; then
        return
    fi

    local additions=""

    # Add PATH update
    if [[ "$NEEDS_PATH_UPDATE" == true ]]; then
        if [[ "$shell_name" == "fish" ]]; then
            additions="set -gx PATH \$PATH $INSTALL_DIR"
        else
            additions="export PATH=\"\$PATH:$INSTALL_DIR\""
        fi
    fi

    # Add API URL
    if [[ -n "$API_URL" ]]; then
        if [[ "$shell_name" == "fish" ]]; then
            additions="${additions}\nset -gx CLM_API_URL \"$API_URL\""
        else
            additions="${additions}\nexport CLM_API_URL=\"$API_URL\""
        fi
    fi

    if [[ -n "$additions" ]]; then
        # Check if already added
        if ! grep -q "CLM_API_URL\|$INSTALL_DIR" "$profile_file" 2>/dev/null; then
            echo "" >> "$profile_file"
            echo "# CLM CLI (added by install-cli.sh)" >> "$profile_file"
            echo -e "$additions" >> "$profile_file"
            log_ok "Updated $profile_file"
            log_info "Run: source $profile_file  (or open a new terminal)"
        fi
    fi
}

update_profile

# ============================================================
# Create CLI config directory
# ============================================================
setup_cli_config() {
    local config_dir="$HOME/.clm"
    mkdir -p "$config_dir"

    # Create default config if it doesn't exist
    if [[ ! -f "$config_dir/config.yaml" ]]; then
        cat > "$config_dir/config.yaml" <<EOF
# CLM CLI Configuration
# Generated by install-cli.sh on $(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Default SaaS API endpoint
api_url: "${API_URL:-http://localhost:8080}"

# Authentication (set via 'clm login' or export CLM_TOKEN)
# token: ""

# Default output format: json, table, yaml
output: "json"

# Default satellite (set to skip --satellite flag)
# default_satellite: ""

# Default environment
default_environment: "prod"
EOF
        chmod 600 "$config_dir/config.yaml"
        log_ok "CLI config created at $config_dir/config.yaml"
    fi
}

setup_cli_config

# ============================================================
# Shell completions
# ============================================================
setup_completions() {
    log_info "Setting up shell completions..."

    local shell_name=$(basename "${SHELL:-/bin/bash}")
    local clm_bin="$INSTALL_DIR/$BINARY_NAME"

    case "$shell_name" in
        bash)
            local comp_dir="/etc/bash_completion.d"
            if [[ $EUID -ne 0 ]]; then
                comp_dir="$HOME/.local/share/bash-completion/completions"
                mkdir -p "$comp_dir"
            fi
            # Generate completions (if binary supports it)
            "$clm_bin" completion bash > "$comp_dir/clm" 2>/dev/null || true
            ;;
        zsh)
            local comp_dir="${fpath[1]:-$HOME/.zsh/completions}"
            mkdir -p "$comp_dir" 2>/dev/null || true
            "$clm_bin" completion zsh > "$comp_dir/_clm" 2>/dev/null || true
            ;;
        fish)
            local comp_dir="$HOME/.config/fish/completions"
            mkdir -p "$comp_dir"
            "$clm_bin" completion fish > "$comp_dir/clm.fish" 2>/dev/null || true
            ;;
    esac

    log_ok "Shell completions configured for $shell_name"
}

setup_completions 2>/dev/null || true

# ============================================================
# Print summary
# ============================================================
echo ""
echo -e "${BOLD}╔══════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║       CLM CLI - Installation Complete    ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════╝${NC}"
echo ""
echo "  Binary:    $INSTALL_DIR/$BINARY_NAME"
echo "  Config:    $HOME/.clm/config.yaml"
if [[ -n "$API_URL" ]]; then
echo "  API URL:   $API_URL"
fi
echo ""
echo -e "  ${CYAN}Quick Start:${NC}"
echo ""
echo "    # Authenticate"
echo "    clm login --email admin@company.com --password <password>"
echo ""
echo "    # List certificates"
echo "    clm cert list"
echo ""
echo "    # Register a satellite"
echo "    clm satellite register --name my-satellite --mode direct --secrets-backend aws"
echo ""
echo "    # Create a certificate"
echo "    clm cert create --domain api.example.com --satellite <id> --ca letsencrypt"
echo ""
echo "    # Get help"
echo "    clm --help"
echo "    clm cert --help"
echo "    clm satellite --help"
echo ""
if [[ "$NEEDS_PATH_UPDATE" == true ]]; then
echo -e "  ${YELLOW}NOTE:${NC} Run this to add CLM to your PATH:"
echo "    source ~/.$(basename $SHELL)rc"
echo ""
fi
echo "  Documentation: https://github.com/${GITHUB_REPO}"
echo ""
