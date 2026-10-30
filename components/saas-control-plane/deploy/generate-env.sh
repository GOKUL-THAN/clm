#!/usr/bin/env bash
# ==============================================================================
# Generate a complete deploy/.env with cryptographically secure random secrets
# ==============================================================================
set -euo pipefail

ENV_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/.env"

if [ -f "$ENV_FILE" ]; then
    echo "deploy/.env already exists. Preserving existing secrets."
    exit 0
fi

echo "Generating deploy/.env with new random secrets..."

# Generate random hex strings
rand_hex() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex "$1"
    else
        head -c "$1" /dev/urandom | xxd -p | tr -d '\n'
    fi
}

POSTGRES_PASS=$(rand_hex 16)
JWT_SEC=$(rand_hex 32)
MASTER_KEY=$(rand_hex 16) # 16 bytes = 32 hex chars
KEYCLOAK_PASS=$(rand_hex 16)
CLIENT_SEC=$(rand_hex 16)

cat > "$ENV_FILE" <<EOF
POSTGRES_PASSWORD=$POSTGRES_PASS
REDIS_PASSWORD=
JWT_SECRET=$JWT_SEC
CLM_JWKS_URL=http://keycloak:8180/auth/realms/clm/protocol/openid-connect/certs
CLM_EXTERNAL_JWT_ISSUER=http://localhost/auth/realms/clm
CLM_EXTERNAL_JWT_AUDIENCE=clm-api
CLM_CLOUD_ACCOUNT_MASTER_KEY=$MASTER_KEY

KEYCLOAK_ADMIN_USER=admin
KEYCLOAK_ADMIN_PASS=$KEYCLOAK_PASS
CLM_SERVER_CLIENT_SECRET=$CLIENT_SEC

PUBLIC_URL=http://localhost
CLM_ADMIN_PORTAL_URL=http://localhost
CLM_GATEWAY_PORTAL_URL=http://localhost

CLM_SATELLITE_TOKEN=$(rand_hex 16)
CLM_SATELLITE_ID=sat-local-01
CLM_SATELLITE_NAME=Local-Satellite-Gateway
CLM_SAAS_ENDPOINT=http://clm-server:8080
CLM_TLS_SERVER_NAME=localhost
EOF

chmod 600 "$ENV_FILE"
echo "Generated $ENV_FILE successfully (mode 600)."

