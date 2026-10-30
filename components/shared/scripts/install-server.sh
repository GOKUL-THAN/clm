#!/usr/bin/env bash
#
# CLM SaaS Control Plane - Installation Script
# Supports: Ubuntu 20.04+, Debian 11+, RHEL 8+, CentOS 8+, Amazon Linux 2023
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/m2p/clm/main/scripts/install-server.sh | bash
#   OR
#   ./scripts/install-server.sh [OPTIONS]
#
# Options:
#   --db-host       PostgreSQL host (default: localhost)
#   --db-port       PostgreSQL port (default: 5432)
#   --db-user       PostgreSQL user (default: clm)
#   --db-password   PostgreSQL password (default: auto-generated)
#   --db-name       Database name (default: clm_db)
#   --db-sslmode    PostgreSQL TLS mode (required in production)
#   --redis-addr    Redis address (default: localhost:6379)
#   --port          Server listen port (default: 8080)
#   --domain        Server domain for TLS (optional)
#   --tls-cert      CA-issued HTTPS certificate (required in production)
#   --tls-key       HTTPS private key (required in production)
#   --gateway-client-ca  CA bundle for gateway client certificates (required in production)
#   --install-deps  Install PostgreSQL and Redis locally (default: false)
#   --data-dir      Data directory (default: /var/lib/clm)
#   --config-dir    Config directory (default: /etc/clm)
#   --skip-db       Skip database setup (use existing)
#   --admin-email   Initial admin email (default: admin@localhost)
#   --admin-pass    Initial admin password (default: auto-generated)
#   --version       CLM version to install (default: latest)
#
set -euo pipefail

# ============================================================
# Configuration (override via flags or environment variables)
# ============================================================
CLM_VERSION="${CLM_VERSION:-latest}"
CLM_PORT="${CLM_PORT:-8080}"
CLM_GRPC_PORT="${CLM_GRPC_PORT:-9090}"
CLM_HOST="${CLM_HOST:-0.0.0.0}"
CLM_DOMAIN="${CLM_DOMAIN:-}"
CLM_ENV="${CLM_ENV:-production}"
TLS_CERT_SOURCE="${CLM_TLS_CERT_PATH:-}"
TLS_KEY_SOURCE="${CLM_TLS_KEY_PATH:-}"
GATEWAY_CLIENT_CA_SOURCE="${CLM_GATEWAY_CLIENT_CA_PATH:-}"
CLOUD_ACCOUNT_MASTER_KEY="${CLM_CLOUD_ACCOUNT_MASTER_KEY:-}"

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-clm}"
DB_PASSWORD="${DB_PASSWORD:-}"
DB_NAME="${DB_NAME:-clm_db}"
DB_SSLMODE="${DB_SSLMODE:-}"

REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"

INSTALL_DEPS=false
SKIP_DB=false
DATA_DIR="/var/lib/clm"
CONFIG_DIR="/etc/clm"
LOG_DIR="/var/log/clm"
BIN_DIR="/usr/local/bin"

ADMIN_EMAIL="${ADMIN_EMAIL:-admin@localhost}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
JWT_SECRET="${JWT_SECRET:-}"

# ============================================================
# Colors and helpers
# ============================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info()  { echo -e "${BLUE}[INFO]${NC} $1"; }
log_ok()    { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

die() { log_error "$1"; exit 1; }

generate_password() {
    # Fixed-length output avoids Base64 character filtering shortening a secret
    # below the production validator's minimum.
    openssl rand -hex 32
}

# ============================================================
# Parse arguments
# ============================================================
while [[ $# -gt 0 ]]; do
    case $1 in
        --db-host)       DB_HOST="$2"; shift 2 ;;
        --db-port)       DB_PORT="$2"; shift 2 ;;
        --db-user)       DB_USER="$2"; shift 2 ;;
        --db-password)   DB_PASSWORD="$2"; shift 2 ;;
        --db-name)       DB_NAME="$2"; shift 2 ;;
        --db-sslmode)    DB_SSLMODE="$2"; shift 2 ;;
        --redis-addr)    REDIS_ADDR="$2"; shift 2 ;;
        --port)          CLM_PORT="$2"; shift 2 ;;
        --domain)        CLM_DOMAIN="$2"; shift 2 ;;
        --tls-cert)      TLS_CERT_SOURCE="$2"; shift 2 ;;
        --tls-key)       TLS_KEY_SOURCE="$2"; shift 2 ;;
        --gateway-client-ca) GATEWAY_CLIENT_CA_SOURCE="$2"; shift 2 ;;
        --install-deps)  INSTALL_DEPS=true; shift ;;
        --data-dir)      DATA_DIR="$2"; shift 2 ;;
        --config-dir)    CONFIG_DIR="$2"; shift 2 ;;
        --skip-db)       SKIP_DB=true; shift ;;
        --admin-email)   ADMIN_EMAIL="$2"; shift 2 ;;
        --admin-pass)    ADMIN_PASSWORD="$2"; shift 2 ;;
        --version)       CLM_VERSION="$2"; shift 2 ;;
        --help|-h)
            echo "CLM SaaS Control Plane Installer"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --db-host HOST        PostgreSQL host (default: localhost)"
            echo "  --db-port PORT        PostgreSQL port (default: 5432)"
            echo "  --db-user USER        PostgreSQL user (default: clm)"
            echo "  --db-password PASS    PostgreSQL password (auto-generated if empty)"
            echo "  --db-name NAME        Database name (default: clm_db)"
            echo "  --db-sslmode MODE     PostgreSQL TLS mode (production: require/verify-ca/verify-full)"
            echo "  --redis-addr ADDR     Redis address (default: localhost:6379)"
            echo "  --port PORT           Server port (default: 8080)"
            echo "  --domain DOMAIN       Domain for TLS (optional)"
            echo "  --tls-cert PATH       CA-issued HTTPS server certificate (required in production)"
            echo "  --tls-key PATH        Private key for the HTTPS certificate (required in production)"
            echo "  --gateway-client-ca PATH  CA bundle for gateway client certificates (required in production)"
            echo "  --install-deps        Install PostgreSQL & Redis locally"
            echo "  --data-dir DIR        Data directory (default: /var/lib/clm)"
            echo "  --config-dir DIR      Config directory (default: /etc/clm)"
            echo "  --skip-db             Skip database creation"
            echo "  --admin-email EMAIL   Admin email (default: admin@localhost)"
            echo "  --admin-pass PASS     Admin password (auto-generated if empty)"
            echo "  --version VER         Version to install (default: latest)"
            exit 0
            ;;
        *) die "Unknown option: $1" ;;
    esac
done

# ============================================================
# Pre-flight checks
# ============================================================
echo ""
echo "=========================================="
echo "  CLM SaaS Control Plane Installer"
echo "=========================================="
echo ""

# Must run as root
if [[ $EUID -ne 0 ]]; then
    die "This script must be run as root (use sudo)"
fi

# Detect OS
detect_os() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS=$ID
        OS_VERSION=$VERSION_ID
    else
        die "Unsupported operating system"
    fi
}
detect_os

log_info "Detected OS: $OS $OS_VERSION"
log_info "CLM Version: $CLM_VERSION"

# Generate secrets if not provided
if [[ -z "$DB_PASSWORD" ]]; then
    DB_PASSWORD=$(generate_password)
    log_info "Generated database password"
fi

if [[ -z "$JWT_SECRET" ]]; then
    JWT_SECRET=$(generate_password)
    log_info "Generated JWT secret"
fi

if [[ -z "$ADMIN_PASSWORD" ]]; then
    ADMIN_PASSWORD=$(generate_password)
    log_info "Generated admin password"
fi

if [[ -z "$CLOUD_ACCOUNT_MASTER_KEY" ]]; then
    CLOUD_ACCOUNT_MASTER_KEY=$(openssl rand -hex 16)
    log_info "Generated cloud-account encryption master key"
fi
if [[ ${#CLOUD_ACCOUNT_MASTER_KEY} -ne 32 ]]; then
    die "CLM_CLOUD_ACCOUNT_MASTER_KEY must be exactly 32 characters"
fi
if [[ ! "$DB_USER" =~ ^[A-Za-z_][A-Za-z0-9_]*$ || ! "$DB_NAME" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    die "Database user and name must be safe PostgreSQL identifiers"
fi

if [[ "$CLM_ENV" == "production" || "$CLM_ENV" == "prod" ]]; then
    for tls_file in "$TLS_CERT_SOURCE" "$TLS_KEY_SOURCE" "$GATEWAY_CLIENT_CA_SOURCE"; do
        [[ -f "$tls_file" && -r "$tls_file" ]] || die "Production requires readable --tls-cert, --tls-key, and --gateway-client-ca files"
    done
    cert_pub=$(openssl x509 -in "$TLS_CERT_SOURCE" -pubkey -noout | openssl pkey -pubin -outform DER 2>/dev/null | openssl dgst -sha256)
    key_pub=$(openssl pkey -in "$TLS_KEY_SOURCE" -pubout -outform DER 2>/dev/null | openssl dgst -sha256)
    [[ -n "$cert_pub" && "$cert_pub" == "$key_pub" ]] || die "TLS server certificate and key do not match"
    openssl x509 -in "$TLS_CERT_SOURCE" -noout -checkend 0 >/dev/null || die "TLS server certificate is invalid or expired"
fi

if [[ "$CLM_ENV" == "production" || "$CLM_ENV" == "prod" ]]; then
    case "$DB_SSLMODE" in
        require|verify-ca|verify-full) ;;
        "") die "DB_SSLMODE is required in production (require, verify-ca, or verify-full)" ;;
        *) die "Unsafe DB_SSLMODE '$DB_SSLMODE' for production; use require, verify-ca, or verify-full" ;;
    esac
elif [[ -z "$DB_SSLMODE" ]]; then
    DB_SSLMODE="disable"
fi

# ============================================================
# Install system dependencies
# ============================================================
install_system_deps() {
    log_info "Installing system dependencies..."

    case $OS in
        ubuntu|debian)
            export DEBIAN_FRONTEND=noninteractive
            apt-get update -qq
            apt-get install -y -qq curl wget gnupg2 lsb-release ca-certificates openssl jq
            ;;
        rhel|centos|rocky|alma|amzn|fedora)
            yum install -y -q curl wget gnupg2 ca-certificates openssl jq
            ;;
        *)
            die "Unsupported OS: $OS"
            ;;
    esac

    log_ok "System dependencies installed"
}

install_system_deps

# ============================================================
# Install PostgreSQL (optional)
# ============================================================
install_postgresql() {
    if [[ "$INSTALL_DEPS" != true ]]; then
        return
    fi

    log_info "Installing PostgreSQL 16..."

    case $OS in
        ubuntu|debian)
            # Add PostgreSQL APT repo
            curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /usr/share/keyrings/postgresql.gpg
            echo "deb [signed-by=/usr/share/keyrings/postgresql.gpg] http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list
            apt-get update -qq
            apt-get install -y -qq postgresql-16
            ;;
        rhel|centos|rocky|alma|fedora)
            yum install -y -q https://download.postgresql.org/pub/repos/yum/reporpms/EL-$(rpm -E %{rhel})-x86_64/pgdg-redhat-repo-latest.noarch.rpm || true
            yum install -y -q postgresql16-server postgresql16
            /usr/pgsql-16/bin/postgresql-16-setup initdb
            systemctl enable postgresql-16
            systemctl start postgresql-16
            ;;
        amzn)
            amazon-linux-extras install postgresql16 -y 2>/dev/null || yum install -y -q postgresql16-server
            postgresql-setup --initdb
            systemctl enable postgresql
            systemctl start postgresql
            ;;
    esac

    # Ensure PostgreSQL is running
    systemctl start postgresql 2>/dev/null || systemctl start postgresql-16 2>/dev/null || true

    log_ok "PostgreSQL installed and running"
}

install_postgresql

# ============================================================
# Install Redis (optional)
# ============================================================
install_redis() {
    if [[ "$INSTALL_DEPS" != true ]]; then
        return
    fi

    log_info "Installing Redis..."

    case $OS in
        ubuntu|debian)
            apt-get install -y -qq redis-server
            systemctl enable redis-server
            systemctl start redis-server
            ;;
        rhel|centos|rocky|alma|amzn|fedora)
            yum install -y -q redis
            systemctl enable redis
            systemctl start redis
            ;;
    esac

    log_ok "Redis installed and running"
}

install_redis

# ============================================================
# Setup database
# ============================================================
setup_database() {
    if [[ "$SKIP_DB" == true ]]; then
        log_info "Skipping database setup (--skip-db)"
        return
    fi

    log_info "Setting up database..."

    # Create user and database
    if [[ "$DB_HOST" == "localhost" || "$DB_HOST" == "127.0.0.1" ]]; then
        # Local PostgreSQL — use peer auth via sudo
        sudo -u postgres psql -c "CREATE USER $DB_USER WITH PASSWORD '$DB_PASSWORD';" 2>/dev/null || true
        sudo -u postgres psql -c "CREATE DATABASE $DB_NAME OWNER $DB_USER;" 2>/dev/null || true
        sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER;" 2>/dev/null || true

        # Allow password auth for CLM user
        PG_HBA=$(sudo -u postgres psql -t -c "SHOW hba_file;" | xargs)
        if ! grep -q "$DB_USER" "$PG_HBA" 2>/dev/null; then
            echo "host $DB_NAME $DB_USER 127.0.0.1/32 md5" >> "$PG_HBA"
            echo "host $DB_NAME $DB_USER ::1/128 md5" >> "$PG_HBA"
            systemctl reload postgresql 2>/dev/null || systemctl reload postgresql-16 2>/dev/null || true
        fi
    else
        # Remote PostgreSQL — use psql with password
        PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1;" 2>/dev/null || \
            die "Cannot connect to PostgreSQL at $DB_HOST:$DB_PORT. Ensure the database and user exist."
    fi

    # Run every schema migration in lexical order and stop on the first error.
    # Missing or partially applied secure-transport schema must never be hidden.
    log_info "Running database migrations..."
    for migration in "$CONFIG_DIR"/migrations/*.sql; do
        [[ -f "$migration" ]] || die "No database migrations found in $CONFIG_DIR/migrations"
        log_info "Applying $(basename "$migration")"
        PGPASSWORD="$DB_PASSWORD" psql -v ON_ERROR_STOP=1 -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f "$migration"
    done

    log_ok "Database setup complete"
}

# ============================================================
# Create directories and user
# ============================================================
setup_directories() {
    log_info "Creating directories and service user..."

    # Create system user
    id -u clm &>/dev/null || useradd --system --no-create-home --shell /usr/sbin/nologin clm

    # Create directories
    mkdir -p "$DATA_DIR" "$CONFIG_DIR" "$LOG_DIR" "$CONFIG_DIR/migrations"
    chown -R clm:clm "$DATA_DIR" "$LOG_DIR"

    log_ok "Directories created"
}

setup_directories

# ============================================================
# Download and install CLM binary
# ============================================================
install_clm_binary() {
    log_info "Installing CLM server binary..."

    # Detect architecture
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l)  ARCH="armv7" ;;
        *) die "Unsupported architecture: $ARCH" ;;
    esac

    PLATFORM="linux"
    BINARY_NAME="clm-server"

    if [[ "$CLM_VERSION" == "latest" ]]; then
        # In production: download from GitHub releases
        # DOWNLOAD_URL="https://github.com/m2p/clm/releases/latest/download/clm-server-${PLATFORM}-${ARCH}"
        # For now, build from source if Go is available
        if command -v go &>/dev/null; then
            log_info "Building from source..."
            cd /tmp
            if [[ -d /tmp/clm-build ]]; then rm -rf /tmp/clm-build; fi
            git clone https://github.com/m2p/clm.git /tmp/clm-build 2>/dev/null || true

            # If source is available locally
            if [[ -f /opt/clm/go.mod ]]; then
                cd /opt/clm
            elif [[ -f /tmp/clm-build/go.mod ]]; then
                cd /tmp/clm-build
            else
                die "Cannot find CLM source. Place source at /opt/clm or ensure git clone works."
            fi

            go build -ldflags="-s -w" -o "$BIN_DIR/$BINARY_NAME" ./cmd/clm-server
            go build -ldflags="-s -w" -o "$BIN_DIR/clm" ./cmd/clm-cli
            rm -rf /tmp/clm-build
        else
            # Download pre-built binary
            DOWNLOAD_URL="https://github.com/m2p/clm/releases/latest/download/${BINARY_NAME}-${PLATFORM}-${ARCH}"
            log_info "Downloading from $DOWNLOAD_URL..."
            curl -fsSL "$DOWNLOAD_URL" -o "$BIN_DIR/$BINARY_NAME" || \
                die "Failed to download binary. Ensure the release exists or install Go to build from source."
        fi
    else
        DOWNLOAD_URL="https://github.com/m2p/clm/releases/download/v${CLM_VERSION}/${BINARY_NAME}-${PLATFORM}-${ARCH}"
        curl -fsSL "$DOWNLOAD_URL" -o "$BIN_DIR/$BINARY_NAME" || \
            die "Failed to download version $CLM_VERSION"
    fi

    chmod +x "$BIN_DIR/$BINARY_NAME"
    chmod +x "$BIN_DIR/clm" 2>/dev/null || true

    log_ok "CLM binary installed at $BIN_DIR/$BINARY_NAME"
}

install_clm_binary

# ============================================================
# Generate configuration
# ============================================================
generate_config() {
    log_info "Generating server configuration..."

    local secrets_dir="$CONFIG_DIR/secrets"
    local tls_dir="$CONFIG_DIR/tls"
    local tls_enabled=false
    local client_auth=none
    local installed_tls_cert=""
    local installed_tls_key=""
    local installed_client_ca=""
    install -d -m 0750 -o root -g clm "$secrets_dir" "$tls_dir"
    printf '%s' "$DB_PASSWORD" > "$secrets_dir/database-password"
    printf '%s' "$REDIS_PASSWORD" > "$secrets_dir/redis-password"
    printf '%s' "$JWT_SECRET" > "$secrets_dir/jwt-secret"
    printf '%s' "$CLOUD_ACCOUNT_MASTER_KEY" > "$secrets_dir/cloud-account-master-key"
    chmod 0640 "$secrets_dir/database-password" "$secrets_dir/redis-password" "$secrets_dir/jwt-secret" "$secrets_dir/cloud-account-master-key"
    chown root:clm "$secrets_dir/database-password" "$secrets_dir/redis-password" "$secrets_dir/jwt-secret" "$secrets_dir/cloud-account-master-key"

    if [[ -n "$TLS_CERT_SOURCE" || -n "$TLS_KEY_SOURCE" || -n "$GATEWAY_CLIENT_CA_SOURCE" ]]; then
        [[ -f "$TLS_CERT_SOURCE" && -f "$TLS_KEY_SOURCE" && -f "$GATEWAY_CLIENT_CA_SOURCE" ]] || die "TLS certificate, key, and gateway client CA must be supplied together"
        installed_tls_cert="$tls_dir/server.crt"
        installed_tls_key="$tls_dir/server.key"
        installed_client_ca="$tls_dir/gateway-client-ca.crt"
        install -m 0640 -o root -g clm "$TLS_CERT_SOURCE" "$installed_tls_cert"
        install -m 0640 -o root -g clm "$TLS_KEY_SOURCE" "$installed_tls_key"
        install -m 0640 -o root -g clm "$GATEWAY_CLIENT_CA_SOURCE" "$installed_client_ca"
        tls_enabled=true
        client_auth=required
    fi

    cat > "$CONFIG_DIR/server.yaml" <<EOF
# Generated by install-server.sh on $(date -u +"%Y-%m-%dT%H:%M:%SZ")
port: ${CLM_PORT}
host: "${CLM_HOST}"
env: "${CLM_ENV}"
log_level: "info"

database:
  host: "${DB_HOST}"
  port: ${DB_PORT}
  user: "${DB_USER}"
  password: "file://${secrets_dir}/database-password"
  dbname: "${DB_NAME}"
  sslmode: "${DB_SSLMODE}"

redis:
  addr: "${REDIS_ADDR}"
  password: "file://${secrets_dir}/redis-password"
  db: 0

jwt:
  secret: "file://${secrets_dir}/jwt-secret"
  expiry_minutes: 60
  refresh_days: 7
  issuer: "clm-server"
  audience: "clm-api"
  allowed_algorithms: ["HS256"]
  jwks_cache_ttl: "5m"
  jwks_http_timeout: "5s"

cloud_accounts:
  master_key: "file://${secrets_dir}/cloud-account-master-key"

grpc:
  port: ${CLM_GRPC_PORT}

tls:
  enabled: ${tls_enabled}
  cert_path: "${installed_tls_cert}"
  key_path: "${installed_tls_key}"
  client_auth: "${client_auth}"
  client_ca_path: "${installed_client_ca}"

gateway:
  allow_insecure_legacy: false
  lease_duration: "2m"
  max_pull_commands: 10
  max_result_bytes: 262144
  retry_base: "5s"
EOF

    local script_dir migration_source=""
    script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
    for candidate in \
        "$script_dir/../../saas-control-plane/internal/store/postgres/migrations" \
        "/opt/clm/internal/store/postgres/migrations" \
        "/opt/clm/components/saas-control-plane/internal/store/postgres/migrations"; do
        if [[ -f "$candidate/004_secure_command_transport.sql" ]]; then
            migration_source="$candidate"
            break
        fi
    done
    [[ -n "$migration_source" ]] || die "Could not find the complete migration set including 004_secure_command_transport.sql"
    rm -f "$CONFIG_DIR"/migrations/*.sql
    for migration in 001_initial.sql 002_sso_fields.sql 003_tenant_security.sql 004_secure_command_transport.sql; do
        [[ -f "$migration_source/$migration" ]] || die "Required migration is missing: $migration"
        install -m 0640 -o root -g clm "$migration_source/$migration" "$CONFIG_DIR/migrations/$migration"
    done

    chmod 0640 "$CONFIG_DIR/server.yaml"
    chown root:clm "$CONFIG_DIR/server.yaml"

    log_ok "Configuration written to $CONFIG_DIR/server.yaml; secrets and TLS material are stored separately"
}

generate_config

# ============================================================
# Setup database (after config is ready)
# ============================================================
setup_database

# ============================================================
# Create systemd service
# ============================================================
create_systemd_service() {
    log_info "Creating systemd service..."

    cat > /etc/systemd/system/clm-server.service <<EOF
[Unit]
Description=CLM SaaS Control Plane Server
Documentation=https://github.com/m2p/clm
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=clm
Group=clm
ExecStart=${BIN_DIR}/clm-server
Environment=CLM_CONFIG=${CONFIG_DIR}/server.yaml
WorkingDirectory=${DATA_DIR}
Restart=always
RestartSec=5
LimitNOFILE=65535

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR} ${LOG_DIR}
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=clm-server

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable clm-server

    log_ok "Systemd service created and enabled"
}

create_systemd_service

# ============================================================
# Create initial admin user
# ============================================================
create_admin_user() {
    log_info "Creating initial admin user..."

    # Hash the password using the CLM binary or Python
    PASS_HASH=$(python3 -c "
import hashlib, base64, os
import bcrypt
print(bcrypt.hashpw('${ADMIN_PASSWORD}'.encode(), bcrypt.gensalt()).decode())
" 2>/dev/null || echo "")

    if [[ -z "$PASS_HASH" ]]; then
        # Fallback: insert via SQL with a placeholder (user must reset)
        PASS_HASH='$2a$10$placeholder_hash_reset_required'
        log_warn "bcrypt not available in Python — admin must reset password on first login"
    fi

    # Create a default tenant and admin user
    PGPASSWORD="$DB_PASSWORD" psql -v ON_ERROR_STOP=1 -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" <<EOF
INSERT INTO tenants (id, name, slug, plan, status, max_certs, max_satellites, max_users, data_region, created_at, updated_at)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Default Organization',
    'default',
    'enterprise',
    'active',
    100000, 100, 500,
    'ap-south-1',
    NOW(), NOW()
) ON CONFLICT (slug) DO NOTHING;

INSERT INTO users (id, tenant_id, email, name, role, status, mfa_enabled, password_hash, created_at, updated_at)
VALUES (
    '00000000-0000-0000-0000-000000000002',
    '00000000-0000-0000-0000-000000000001',
    '${ADMIN_EMAIL}',
    'Admin',
    'super_admin',
    'active',
    false,
    '${PASS_HASH}',
    NOW(), NOW()
) ON CONFLICT (email) DO NOTHING;
EOF

    log_ok "Admin user created: $ADMIN_EMAIL"
}

create_admin_user

# ============================================================
# Setup firewall rules (optional)
# ============================================================
setup_firewall() {
    if command -v ufw &>/dev/null; then
        ufw allow "$CLM_PORT"/tcp comment "CLM HTTPS API" 2>/dev/null || true
        ufw allow "$CLM_GRPC_PORT"/tcp comment "CLM gRPC API" 2>/dev/null || true
        log_info "UFW rules added for ports $CLM_PORT and $CLM_GRPC_PORT"
    elif command -v firewall-cmd &>/dev/null; then
        firewall-cmd --permanent --add-port="$CLM_PORT"/tcp 2>/dev/null || true
        firewall-cmd --permanent --add-port="$CLM_GRPC_PORT"/tcp 2>/dev/null || true
        firewall-cmd --reload 2>/dev/null || true
        log_info "Firewalld rules added for ports $CLM_PORT and $CLM_GRPC_PORT"
    fi
}

setup_firewall

# ============================================================
# Setup log rotation
# ============================================================
setup_logrotate() {
    cat > /etc/logrotate.d/clm-server <<EOF
${LOG_DIR}/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    create 0640 clm clm
    postrotate
        systemctl reload clm-server 2>/dev/null || true
    endscript
}
EOF

    log_ok "Log rotation configured"
}

setup_logrotate

# ============================================================
# Start the service
# ============================================================
start_service() {
    log_info "Starting CLM server..."
    systemctl start clm-server

    # Wait for startup
    sleep 3

    if systemctl is-active --quiet clm-server; then
        log_ok "CLM server is running"
    else
        log_error "CLM server failed to start. Check: journalctl -u clm-server -f"
        return 1
    fi

    # Health check. Never disable certificate verification in the installer.
    local health_host="${CLM_DOMAIN:-localhost}"
    local health_scheme="http"
    [[ -n "$TLS_CERT_SOURCE" ]] && health_scheme="https"
    if curl -sf "${health_scheme}://${health_host}:${CLM_PORT}/health" > /dev/null 2>&1; then
        log_ok "Health check passed"
    else
        log_warn "Health check failed — server may still be starting"
    fi
}

start_service

# ============================================================
# Print summary
# ============================================================
SUMMARY_SCHEME="http"
[[ -n "$TLS_CERT_SOURCE" ]] && SUMMARY_SCHEME="https"
echo ""
echo "=========================================="
echo "  CLM SaaS Control Plane - Installed!"
echo "=========================================="
echo ""
echo "  Service:     clm-server (systemd)"
echo "  Status:      systemctl status clm-server"
echo "  Logs:        journalctl -u clm-server -f"
echo "  Config:      $CONFIG_DIR/server.yaml"
echo "  Data:        $DATA_DIR"
echo ""
echo "  API URL:     ${SUMMARY_SCHEME}://${CLM_DOMAIN:-localhost}:${CLM_PORT}"
echo "  Health:      ${SUMMARY_SCHEME}://${CLM_DOMAIN:-localhost}:${CLM_PORT}/health"
echo "  gRPC:        localhost:${CLM_GRPC_PORT}"
echo ""
echo "  Admin Email:    $ADMIN_EMAIL"
echo "  Admin Password: stored in $CONFIG_DIR/.install-credentials"
echo "  JWT Secret:     stored in $CONFIG_DIR/secrets/jwt-secret"
echo "  Cloud key:      stored in $CONFIG_DIR/secrets/cloud-account-master-key"
echo "  TLS material:   stored in $CONFIG_DIR/tls"
echo ""
echo "  Database:"
echo "    Host:     $DB_HOST:$DB_PORT"
echo "    Name:     $DB_NAME"
echo "    User:     $DB_USER"
echo "    Password: stored in $CONFIG_DIR/secrets/database-password"
echo ""
echo "  Quick Start:"
echo "    clm login --email $ADMIN_EMAIL"
echo "    clm satellite register --name my-satellite --mode direct --secrets-backend aws"
echo ""
echo "  Management:"
echo "    systemctl start clm-server"
echo "    systemctl stop clm-server"
echo "    systemctl restart clm-server"
echo ""
echo "=========================================="
echo ""

# Save credentials to a file (for admin reference)
cat > "$CONFIG_DIR/.install-credentials" <<EOF
# CLM Installation Credentials
# Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# DELETE THIS FILE after noting credentials

ADMIN_EMAIL=$ADMIN_EMAIL
ADMIN_PASSWORD=$ADMIN_PASSWORD
DB_USER=$DB_USER
DB_PASSWORD=$DB_PASSWORD
JWT_SECRET=$JWT_SECRET
EOF
chmod 600 "$CONFIG_DIR/.install-credentials"
chown root:root "$CONFIG_DIR/.install-credentials"
log_warn "Credentials saved to $CONFIG_DIR/.install-credentials — DELETE after noting them!"
