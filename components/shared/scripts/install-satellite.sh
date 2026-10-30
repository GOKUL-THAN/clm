#!/usr/bin/env bash
# Install a pre-provisioned CLM satellite on a supported Linux host.
#
# Registration is deliberately not performed here. First register the satellite
# through the authenticated control-plane API, then pass the UUID and the
# one-time plaintext credential returned by that response. Supply a client
# certificate whose URI/DNS SAN binds the same UUID, its private key, and the CA
# bundle used to verify the control-plane server.
set -euo pipefail

CLM_VERSION="${CLM_VERSION:-latest}"
CLM_ENV="${CLM_ENV:-production}"
SAAS_URL="${SAAS_URL:-}"
SATELLITE_CREDENTIAL="${CLM_SATELLITE_TOKEN:-${SATELLITE_CREDENTIAL:-}}"
SATELLITE_ID="${SATELLITE_ID:-}"
SATELLITE_NAME="${SATELLITE_NAME:-$(hostname -s)}"
CONNECTOR_MODE="${CONNECTOR_MODE:-direct}"
CLIENT_CERT_SOURCE="${CLIENT_CERT_SOURCE:-}"
CLIENT_KEY_SOURCE="${CLIENT_KEY_SOURCE:-}"
SERVER_CA_SOURCE="${SERVER_CA_SOURCE:-}"
TLS_SERVER_NAME="${TLS_SERVER_NAME:-}"
SECRETS_BACKEND="${SECRETS_BACKEND:-vault}"
AWS_REGION="${AWS_REGION:-ap-south-1}"
AWS_KMS_KEY="${AWS_KMS_KEY:-}"
GCP_PROJECT="${GCP_PROJECT:-}"
VAULT_ADDR="${VAULT_ADDR:-http://127.0.0.1:8200}"
DATA_DIR="${DATA_DIR:-/var/lib/clm-satellite}"
CONFIG_DIR="${CONFIG_DIR:-/etc/clm}"
LOG_DIR="${LOG_DIR:-/var/log/clm}"
BIN_DIR="${BIN_DIR:-/usr/local/bin}"
SKIP_START=false

info() { printf '[INFO] %s\n' "$1"; }
die() { printf '[ERROR] %s\n' "$1" >&2; exit 1; }

usage() {
    cat <<'EOF'
Usage: sudo install-satellite.sh [OPTIONS]

Required for connected production installs:
  --saas-url URL          HTTPS control-plane URL
  --id UUID               Satellite UUID returned by registration
  --credential VALUE      One-time satellite credential returned by registration
  --client-cert PATH      CA-issued client certificate with matching URI/DNS SAN
  --client-key PATH       Private key for the client certificate
  --server-ca PATH        CA bundle used to verify the control-plane server

Optional:
  --server-name NAME      Expected control-plane TLS server name (URL host by default)
  --name NAME             Satellite name (default: hostname)
  --mode direct           Only direct mode is currently supported by this installer
  --secrets-backend NAME  aws, gcp, azure, or vault (default: vault)
  --aws-region REGION
  --aws-kms-key KEY
  --gcp-project PROJECT
  --vault-addr URL
  --data-dir DIR
  --config-dir DIR
  --version VERSION
  --skip-start

The credential is written only to a root-owned service environment file. This
installer never logs it and never sends it to a registration/activation route.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --saas-url) SAAS_URL="$2"; shift 2 ;;
        --id) SATELLITE_ID="$2"; shift 2 ;;
        --credential|--token) SATELLITE_CREDENTIAL="$2"; shift 2 ;;
        --client-cert) CLIENT_CERT_SOURCE="$2"; shift 2 ;;
        --client-key) CLIENT_KEY_SOURCE="$2"; shift 2 ;;
        --server-ca) SERVER_CA_SOURCE="$2"; shift 2 ;;
        --server-name) TLS_SERVER_NAME="$2"; shift 2 ;;
        --name) SATELLITE_NAME="$2"; shift 2 ;;
        --mode) CONNECTOR_MODE="$2"; shift 2 ;;
        --secrets-backend) SECRETS_BACKEND="$2"; shift 2 ;;
        --aws-region) AWS_REGION="$2"; shift 2 ;;
        --aws-kms-key) AWS_KMS_KEY="$2"; shift 2 ;;
        --gcp-project) GCP_PROJECT="$2"; shift 2 ;;
        --vault-addr) VAULT_ADDR="$2"; shift 2 ;;
        --data-dir) DATA_DIR="$2"; shift 2 ;;
        --config-dir) CONFIG_DIR="$2"; shift 2 ;;
        --version) CLM_VERSION="$2"; shift 2 ;;
        --skip-start) SKIP_START=true; shift ;;
        --help|-h) usage; exit 0 ;;
        *) die "unknown option: $1" ;;
    esac
done

[[ "$(uname -s)" == "Linux" ]] || die "this production installer currently supports Linux only"
[[ $EUID -eq 0 ]] || die "this script must run as root"
[[ "$CONNECTOR_MODE" == "direct" ]] || die "connector mode '$CONNECTOR_MODE' is not implemented by this installer; use direct"
[[ "$SAAS_URL" == https://* ]] || die "--saas-url must be an absolute HTTPS URL"
[[ "$SATELLITE_ID" =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$ ]] || die "--id must be a valid non-nil satellite UUID"
[[ "$SATELLITE_ID" != "00000000-0000-0000-0000-000000000000" ]] || die "--id must not be the nil UUID"
[[ "$SATELLITE_NAME" =~ ^[A-Za-z0-9._-]{1,128}$ ]] || die "--name may contain only letters, digits, dot, underscore, and hyphen"
(( ${#SATELLITE_CREDENTIAL} >= 24 && ${#SATELLITE_CREDENTIAL} <= 256 )) || die "--credential must be between 24 and 256 characters"
[[ "$SATELLITE_CREDENTIAL" != *$'\n'* && "$SATELLITE_CREDENTIAL" != *$'\r'* ]] || die "--credential must not contain newlines"
for required_file in "$CLIENT_CERT_SOURCE" "$CLIENT_KEY_SOURCE" "$SERVER_CA_SOURCE"; do
    [[ -f "$required_file" && -r "$required_file" ]] || die "required identity file is missing or unreadable: $required_file"
done
case "$SECRETS_BACKEND" in aws|gcp|azure|vault) ;; *) die "unsupported secrets backend: $SECRETS_BACKEND" ;; esac

command -v openssl >/dev/null || die "openssl is required"
openssl x509 -in "$CLIENT_CERT_SOURCE" -noout -checkend 0 >/dev/null || die "client certificate is invalid or expired"
openssl crl2pkcs7 -nocrl -certfile "$SERVER_CA_SOURCE" 2>/dev/null | openssl pkcs7 -print_certs -noout >/dev/null || die "server CA bundle does not contain a usable certificate"
cert_pub=$(openssl x509 -in "$CLIENT_CERT_SOURCE" -pubkey -noout | openssl pkey -pubin -outform DER 2>/dev/null | openssl dgst -sha256)
key_pub=$(openssl pkey -in "$CLIENT_KEY_SOURCE" -pubout -outform DER 2>/dev/null | openssl dgst -sha256)
[[ -n "$cert_pub" && "$cert_pub" == "$key_pub" ]] || die "client certificate and private key do not match"
san_text=$(openssl x509 -in "$CLIENT_CERT_SOURCE" -noout -ext subjectAltName 2>/dev/null || true)
if ! grep -Eq "DNS:(satellite-)?${SATELLITE_ID}([,[:space:]]|$)|URI:.*?/satellites/${SATELLITE_ID}([,[:space:]]|$)" <<<"$san_text"; then
    die "client certificate SAN is not bound to satellite UUID $SATELLITE_ID"
fi

if [[ -z "$TLS_SERVER_NAME" ]]; then
    endpoint_host=${SAAS_URL#https://}
    endpoint_host=${endpoint_host%%/*}
    TLS_SERVER_NAME=${endpoint_host%%:*}
fi
[[ "$TLS_SERVER_NAME" =~ ^[A-Za-z0-9.-]+$ ]] || die "--server-name is invalid"

if [[ -f /etc/os-release ]]; then . /etc/os-release; else die "unsupported Linux distribution"; fi
case "$ID" in
    ubuntu|debian)
        export DEBIAN_FRONTEND=noninteractive
        apt-get update -qq
        apt-get install -y -qq ca-certificates curl openssl
        ;;
    rhel|centos|rocky|alma|amzn|fedora)
        yum install -y -q ca-certificates curl openssl
        ;;
    *) die "unsupported Linux distribution: $ID" ;;
esac

id -u clm-satellite >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin clm-satellite
CERTS_DIR="$CONFIG_DIR/certs"
STATE_DIR="$DATA_DIR/state"
install -d -m 0750 -o clm-satellite -g clm-satellite "$DATA_DIR" "$STATE_DIR" "$LOG_DIR"
install -d -m 0750 -o root -g clm-satellite "$CONFIG_DIR" "$CERTS_DIR"
install -m 0640 -o root -g clm-satellite "$CLIENT_CERT_SOURCE" "$CERTS_DIR/satellite.crt"
install -m 0640 -o root -g clm-satellite "$CLIENT_KEY_SOURCE" "$CERTS_DIR/satellite.key"
install -m 0640 -o root -g clm-satellite "$SERVER_CA_SOURCE" "$CERTS_DIR/control-plane-ca.crt"

ARCH=$(uname -m)
case "$ARCH" in x86_64) ARCH=amd64 ;; aarch64|arm64) ARCH=arm64 ;; armv7l) ARCH=armv7 ;; *) die "unsupported architecture: $ARCH" ;; esac
BINARY_NAME=clm-satellite
if command -v go >/dev/null 2>&1 && [[ -f "$(pwd)/go.mod" && -f "$(pwd)/cmd/clm-satellite/main.go" ]]; then
    info "building satellite from local source"
    go build -ldflags="-s -w" -o "$BIN_DIR/$BINARY_NAME" ./cmd/clm-satellite
else
    if [[ "$CLM_VERSION" == "latest" ]]; then
        DOWNLOAD_URL="https://github.com/m2p/clm/releases/latest/download/${BINARY_NAME}-linux-${ARCH}"
    else
        DOWNLOAD_URL="https://github.com/m2p/clm/releases/download/v${CLM_VERSION}/${BINARY_NAME}-linux-${ARCH}"
    fi
    info "downloading signed release location $DOWNLOAD_URL"
    curl -fsSL "$DOWNLOAD_URL" -o "$BIN_DIR/$BINARY_NAME" || die "failed to download satellite binary"
fi
chmod 0755 "$BIN_DIR/$BINARY_NAME"

case "$SECRETS_BACKEND" in
    aws) SECRETS_CONFIG="  provider: \"aws\"\n  region: \"${AWS_REGION}\"\n  kms_key_id: \"${AWS_KMS_KEY}\"\n  prefix: \"clm/\"" ;;
    gcp) SECRETS_CONFIG="  provider: \"gcp\"\n  gcp_project: \"${GCP_PROJECT}\"\n  prefix: \"clm-\"" ;;
    azure) SECRETS_CONFIG="  provider: \"azure\"\n  azure_vault_url: \"${VAULT_ADDR}\"\n  prefix: \"clm-\"" ;;
    vault) SECRETS_CONFIG="  provider: \"vault\"\n  vault_addr: \"${VAULT_ADDR}\"\n  prefix: \"clm/\"" ;;
esac

cat > "$CONFIG_DIR/satellite.yaml" <<EOF
id: "${SATELLITE_ID}"
name: "${SATELLITE_NAME}"
env: "${CLM_ENV}"
saas_endpoint: "${SAAS_URL}"
connector_mode: "direct"
tls_cert_path: "${CERTS_DIR}/satellite.crt"
tls_key_path: "${CERTS_DIR}/satellite.key"
tls_ca_path: "${CERTS_DIR}/control-plane-ca.crt"
tls_server_name: "${TLS_SERVER_NAME}"
insecure_skip_verify: false
log_level: "info"

state:
  dir: "${STATE_DIR}"

commands:
  poll_interval: "5s"
  timeout: "15m"
  lease_renewal: "45s"
  max_result_bytes: 262144
  network_retries: 5

crypto:
  default_algorithm: "ECDSA"
  ecdsa_curve: "P-256"
  rsa_bits: 4096
  hsm_enabled: false

secrets:
$(printf '%b' "$SECRETS_CONFIG")

discovery:
  enabled: false
  networks: []
  domains: []
  ports: [443, 8443, 636, 993, 6443]
  concurrency: 100
  timeout: "5s"
EOF
chmod 0640 "$CONFIG_DIR/satellite.yaml"
chown root:clm-satellite "$CONFIG_DIR/satellite.yaml"

RUNTIME_ENV="$CONFIG_DIR/satellite.env"
escaped_credential=${SATELLITE_CREDENTIAL//\'/\'\\\'\'}
umask 027
printf "CLM_SATELLITE_TOKEN='%s'\n" "$escaped_credential" > "$RUNTIME_ENV"
chmod 0640 "$RUNTIME_ENV"
chown root:clm-satellite "$RUNTIME_ENV"
unset SATELLITE_CREDENTIAL escaped_credential

cat > /etc/systemd/system/clm-satellite.service <<EOF
[Unit]
Description=CLM Satellite Runtime
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=clm-satellite
Group=clm-satellite
ExecStart=${BIN_DIR}/${BINARY_NAME}
Environment=CLM_SATELLITE_CONFIG=${CONFIG_DIR}/satellite.yaml
EnvironmentFile=${RUNTIME_ENV}
WorkingDirectory=${DATA_DIR}
Restart=always
RestartSec=5
LimitNOFILE=65535
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR} ${LOG_DIR}
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable clm-satellite
if [[ "$SKIP_START" != true ]]; then
    systemctl restart clm-satellite
    sleep 2
    systemctl is-active --quiet clm-satellite || die "satellite failed to start; inspect journalctl -u clm-satellite"
fi

info "satellite installed with UUID $SATELLITE_ID"
info "configuration: $CONFIG_DIR/satellite.yaml"
info "credential: stored only in $RUNTIME_ENV; remove the provisioning copy"
info "no live mTLS handshake was asserted by the installer; confirm heartbeat and server logs operationally"
