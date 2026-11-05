#!/bin/bash
set -euo pipefail

# ============================================================================
# CLM SaaS Control Plane + Admin Portal — Test Profile Deployment to Lightsail
# ============================================================================
#
# This script deploys the CLM SaaS Control Plane and Admin Portal
# to an AWS Lightsail VM using docker compose with the test profile.
#
# Prerequisites:
#   - AWS CLI configured with Lightsail permissions
#   - SSH key pair for Lightsail instance access
#
# Usage:
#   ./deploy-test.sh                    # Deploy to the existing test VM
#   ./deploy-test.sh --create-vm        # Create new Lightsail VM + deploy
#   ./deploy-test.sh --ssh-only         # SSH into the test VM
#
# Environment overrides:
#   AWS_PROFILE, TEST_VM_IP, SSH_KEY_PATH, SSH_USER
#
# ============================================================================

# --- Configuration ---
AWS_PROFILE="${AWS_PROFILE:-test}"
INSTANCE_NAME="clm-saas-test"
REGION="ap-south-1"
AZ="${REGION}a"
BUNDLE_ID="${BUNDLE_ID:-xlarge_3_1}" # 4 vCPU, 16GB RAM, 320GB SSD
BLUEPRINT_ID="ubuntu_22_04"
STATIC_IP_NAME="clm-saas-test-ip"
KEY_PAIR_NAME="${KEY_PAIR_NAME:-clm-saas-deploy}"
SSH_KEY_PATH="${SSH_KEY_PATH:-$HOME/.ssh/clm-saas-deploy.pem}"
DOCKER_VERSION="${DOCKER_VERSION:-29.1.3-0ubuntu3~22.04.2}"
COMPOSE_VERSION="${COMPOSE_VERSION:-2.40.3+ds1-0ubuntu1~22.04.1}"

# Current isolated test VM from the component runbook. Override TEST_VM_IP when
# the Lightsail static IP changes; --create-vm refreshes it from AWS.
TEST_VM_IP="${TEST_VM_IP:-35.154.6.28}"
SSH_USER="${SSH_USER:-ubuntu}"

# Project paths
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPONENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Remote deployment directory
REMOTE_DIR="/opt/clm"

# --- Color output ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()   { echo -e "${GREEN}[CLM]${NC} $1"; }
warn()  { echo -e "${YELLOW}[CLM]${NC} $1"; }
error() { echo -e "${RED}[CLM]${NC} $1" >&2; }

aws_test() {
    command aws --profile "$AWS_PROFILE" "$@"
}

ssh_options() {
    local options="-o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10"
    if [ -f "$SSH_KEY_PATH" ]; then
        options="$options -i $SSH_KEY_PATH"
    fi
    printf '%s' "$options"
}

# --- Functions ---

create_lightsail_vm() {
    log "Creating Lightsail instance: $INSTANCE_NAME"

    # Check if instance already exists
    if aws_test lightsail get-instance --instance-name "$INSTANCE_NAME" --region "$REGION" &>/dev/null; then
        warn "Instance '$INSTANCE_NAME' already exists. Skipping creation."
        return 0
    fi

    # Create key pair if not exists
    if ! aws_test lightsail get-key-pair --key-pair-name "$KEY_PAIR_NAME" --region "$REGION" &>/dev/null; then
        log "Creating SSH key pair: $KEY_PAIR_NAME"
        aws_test lightsail create-key-pair \
            --key-pair-name "$KEY_PAIR_NAME" \
            --region "$REGION" \
            --query 'privateKeyBase64' --output text | base64 -d > "$SSH_KEY_PATH"
        chmod 600 "$SSH_KEY_PATH"
        log "SSH key saved to: $SSH_KEY_PATH"
    fi

    # Create instance with bootstrap user-data
    log "Creating instance ($BUNDLE_ID in $AZ)..."
    aws_test lightsail create-instances \
        --instance-names "$INSTANCE_NAME" \
        --availability-zone "$AZ" \
        --blueprint-id "$BLUEPRINT_ID" \
        --bundle-id "$BUNDLE_ID" \
        --key-pair-name "$KEY_PAIR_NAME" \
        --region "$REGION" \
        --tags "key=managed-by,value=clm" "key=environment,value=test" "key=component,value=saas-control-plane" \
        --user-data "$(cat <<USERDATA
#!/bin/bash
# Lightsail may invoke user data through /bin/sh even when a shebang is present.
set -eu
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y \
    "docker.io=$DOCKER_VERSION" \
    "docker-compose-v2=$COMPOSE_VERSION" \
    curl ca-certificates
systemctl enable --now docker
usermod -aG docker ubuntu
mkdir -p /opt/clm
chown ubuntu:ubuntu /opt/clm
USERDATA
)"

    # Wait for instance to be running
    log "Waiting for instance to start..."
    for i in $(seq 1 30); do
        STATE=$(aws_test lightsail get-instance --instance-name "$INSTANCE_NAME" --region "$REGION" \
            --query 'instance.state.name' --output text 2>/dev/null || echo "pending")
        if [ "$STATE" = "running" ]; then
            log "Instance is running."
            break
        fi
        sleep 10
    done

    # Allocate and attach static IP
    if ! aws_test lightsail get-static-ip --static-ip-name "$STATIC_IP_NAME" --region "$REGION" &>/dev/null; then
        log "Allocating static IP..."
        aws_test lightsail allocate-static-ip --static-ip-name "$STATIC_IP_NAME" --region "$REGION"
    fi

    log "Attaching static IP to instance..."
    aws_test lightsail attach-static-ip \
        --static-ip-name "$STATIC_IP_NAME" \
        --instance-name "$INSTANCE_NAME" \
        --region "$REGION"

    # Get the IP
    TEST_VM_IP=$(aws_test lightsail get-static-ip --static-ip-name "$STATIC_IP_NAME" --region "$REGION" \
        --query 'staticIp.ipAddress' --output text)
    log "Static IP: $TEST_VM_IP"

    # Open ports
    log "Configuring firewall (ports 80 and 22 only)..."
    aws_test lightsail put-instance-public-ports \
        --instance-name "$INSTANCE_NAME" \
        --region "$REGION" \
        --port-infos \
            "fromPort=80,toPort=80,protocol=tcp" \
            "fromPort=22,toPort=22,protocol=tcp"

    log "Lightsail VM created successfully!"
    log "IP: $TEST_VM_IP"
    log "SSH: ssh -i $SSH_KEY_PATH $SSH_USER@$TEST_VM_IP"

    # Wait for SSH to be ready
    log "Waiting for SSH to be ready..."
    for i in $(seq 1 20); do
        if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -i "$SSH_KEY_PATH" "$SSH_USER@$TEST_VM_IP" "echo ok" &>/dev/null; then
            log "SSH is ready."
            break
        fi
        sleep 5
    done
}

deploy_to_vm() {
    log "Deploying CLM SaaS Control Plane + Admin Portal to $TEST_VM_IP"

    # Determine SSH key without disabling host verification.
    local ssh_opts
    ssh_opts=$(ssh_options)

    # Ensure remote directory exists and Docker is ready
    log "Preparing remote environment..."
    ssh $ssh_opts "$SSH_USER@$TEST_VM_IP" \
        "DOCKER_VERSION='$DOCKER_VERSION' COMPOSE_VERSION='$COMPOSE_VERSION' bash -s" <<'REMOTE_PREP'
set -e
sudo mkdir -p /opt/clm/deploy/migrations
sudo mkdir -p /opt/clm/deploy/keycloak
sudo mkdir -p /opt/clm/configs
sudo mkdir -p /opt/clm/web/admin-portal
sudo chown -R ubuntu:ubuntu /opt/clm

# Ensure Docker is installed
if ! command -v docker &>/dev/null; then
    sudo apt-get update -y
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y \
        "docker.io=$DOCKER_VERSION" \
        "docker-compose-v2=$COMPOSE_VERSION" \
        curl ca-certificates
    sudo systemctl enable --now docker
    sudo usermod -aG docker ubuntu
fi
REMOTE_PREP

    # Copy project files to remote
    log "Syncing project files..."

    # Copy deployment configs
    scp $ssh_opts "$SCRIPT_DIR/docker-compose.test.yml" "$SSH_USER@$TEST_VM_IP:/opt/clm/deploy/"
    scp $ssh_opts "$SCRIPT_DIR/nginx-test.conf" "$SSH_USER@$TEST_VM_IP:/opt/clm/deploy/"
    scp $ssh_opts "$SCRIPT_DIR/init-seed.sql" "$SSH_USER@$TEST_VM_IP:/opt/clm/deploy/"
    scp $ssh_opts "$SCRIPT_DIR/Dockerfile.server" "$SSH_USER@$TEST_VM_IP:/opt/clm/deploy/"
    scp $ssh_opts "$SCRIPT_DIR/run-tests.sh" "$SSH_USER@$TEST_VM_IP:/opt/clm/deploy/"
    scp $ssh_opts "$SCRIPT_DIR/keycloak/clm-realm.json" "$SSH_USER@$TEST_VM_IP:/opt/clm/deploy/keycloak/"
    ssh $ssh_opts "$SSH_USER@$TEST_VM_IP" "chmod +x /opt/clm/deploy/run-tests.sh"

    # Copy all ordered migrations. New migrations are also applied explicitly
    # below because docker-entrypoint-initdb.d only runs for an empty volume.
    for migration in "$SCRIPT_DIR"/migrations/*.sql; do
        scp $ssh_opts "$migration" "$SSH_USER@$TEST_VM_IP:/opt/clm/deploy/migrations/"
    done

    # Copy server configs. server.yaml is a build input in Dockerfile.server;
    # server-test.yaml is mounted over it at runtime for this isolated profile.
    scp $ssh_opts "$COMPONENT_DIR/configs/server.yaml" "$SSH_USER@$TEST_VM_IP:/opt/clm/configs/"
    scp $ssh_opts "$COMPONENT_DIR/configs/server-test.yaml" "$SSH_USER@$TEST_VM_IP:/opt/clm/configs/"

    # Copy admin portal
    scp $ssh_opts "$COMPONENT_DIR/web/admin-portal/index.html" "$SSH_USER@$TEST_VM_IP:/opt/clm/web/admin-portal/"
    scp $ssh_opts "$COMPONENT_DIR/web/admin-portal/app.js" "$SSH_USER@$TEST_VM_IP:/opt/clm/web/admin-portal/"

    # Copy Go source for build
    log "Syncing Go source code..."
    ssh $ssh_opts "$SSH_USER@$TEST_VM_IP" "mkdir -p /opt/clm/cmd /opt/clm/internal /opt/clm/pkg"

    # Use rsync if available, otherwise tar+scp
    if command -v rsync &>/dev/null; then
        rsync -avz --delete -e "ssh $ssh_opts" \
            "$COMPONENT_DIR/cmd/" "$SSH_USER@$TEST_VM_IP:/opt/clm/cmd/"
        rsync -avz --delete -e "ssh $ssh_opts" \
            "$COMPONENT_DIR/internal/" "$SSH_USER@$TEST_VM_IP:/opt/clm/internal/"
        rsync -avz --delete -e "ssh $ssh_opts" \
            "$COMPONENT_DIR/pkg/" "$SSH_USER@$TEST_VM_IP:/opt/clm/pkg/"
        rsync -avz -e "ssh $ssh_opts" \
            "$COMPONENT_DIR/go.mod" "$COMPONENT_DIR/go.sum" "$SSH_USER@$TEST_VM_IP:/opt/clm/"
    else
        # Fallback: tar and send
        (cd "$COMPONENT_DIR" && tar czf - cmd internal pkg go.mod go.sum 2>/dev/null || \
         cd "$COMPONENT_DIR" && tar czf - cmd internal pkg go.mod) | \
            ssh $ssh_opts "$SSH_USER@$TEST_VM_IP" "cd /opt/clm && tar xzf -"
    fi

    # Create go.sum if missing (needed for Docker build)
    ssh $ssh_opts "$SSH_USER@$TEST_VM_IP" "touch /opt/clm/go.sum"

    # Create the ignored, mode-600 runtime environment without rotating values
    # already used by the persistent databases. Older deployments predate .env,
    # so recover their values from running container configuration without
    # printing them, then generate only newly introduced secrets.
    log "Ensuring remote runtime secrets are present..."
    ssh $ssh_opts "$SSH_USER@$TEST_VM_IP" "TEST_VM_IP='$TEST_VM_IP' bash -s" <<'REMOTE_ENV'
set -euo pipefail
cd /opt/clm/deploy
umask 077

container_env() {
    local service="$1" key="$2" container
    # Do not use `docker compose ps` here: when .env is absent, Compose rejects
    # required substitutions before it can identify the already-running stack.
    container=$(docker ps \
        --filter "label=com.docker.compose.service=$service" \
        --format '{{.ID}}' | head -1)
    [ -n "$container" ] || return 0
    docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$container" \
        | sed -n "s/^${key}=//p" | head -1
}

if [ ! -f .env ]; then
    existing_stack=$(docker ps -aq \
        --filter "label=com.docker.compose.project=deploy" | head -1)
    postgres_password=$(container_env postgres POSTGRES_PASSWORD)
    jwt_secret=$(container_env clm-server JWT_SECRET)
    [ -n "$jwt_secret" ] || jwt_secret=$(container_env clm-server CLM_JWT_SECRET)
    keycloak_admin_user=$(container_env keycloak KEYCLOAK_ADMIN)
    keycloak_admin_password=$(container_env keycloak KEYCLOAK_ADMIN_PASSWORD)
    server_client_secret=$(container_env keycloak CLM_SERVER_CLIENT_SECRET)
    redis_password=$(container_env redis REDIS_PASSWORD)

    if [ -n "$existing_stack" ]; then
        [ -n "$postgres_password" ] || {
            echo "Cannot recover the existing PostgreSQL password" >&2
            exit 1
        }
        [ -n "$keycloak_admin_password" ] || {
            echo "Cannot recover the existing Keycloak admin password" >&2
            exit 1
        }
    else
        # A genuinely empty VM has no values to recover. Generate every secret
        # locally on the host and never print it to deployment output.
        postgres_password=$(openssl rand -hex 32)
        keycloak_admin_password=$(openssl rand -hex 32)
    fi

    [ -n "$jwt_secret" ] || jwt_secret=$(openssl rand -hex 32)
    [ -n "$keycloak_admin_user" ] || keycloak_admin_user=admin
    [ -n "$server_client_secret" ] || server_client_secret=$(openssl rand -hex 32)

    cat > .env <<EOF
POSTGRES_PASSWORD=$postgres_password
REDIS_PASSWORD=$redis_password
JWT_SECRET=$jwt_secret
CLM_CLOUD_ACCOUNT_MASTER_KEY=$(openssl rand -hex 16)
KEYCLOAK_ADMIN_USER=$keycloak_admin_user
KEYCLOAK_ADMIN_PASS=$keycloak_admin_password
CLM_SERVER_CLIENT_SECRET=$server_client_secret
PUBLIC_URL=http://$TEST_VM_IP
CLM_ADMIN_PORTAL_URL=http://$TEST_VM_IP
CLM_GATEWAY_PORTAL_URL=http://$TEST_VM_IP
EOF
else
    if ! grep -q '^CLM_CLOUD_ACCOUNT_MASTER_KEY=.' .env; then
        printf 'CLM_CLOUD_ACCOUNT_MASTER_KEY=%s\n' "$(openssl rand -hex 16)" >> .env
    fi
fi
chmod 600 .env
REMOTE_ENV

    # Validate and deploy. A clean VM starts the data/identity services first so
    # integration tests run against a disposable database on the real Compose
    # network. Existing stacks retain the backup-before-migration path.
    log "Validating and deploying the CLM stack..."
    ssh $ssh_opts "$SSH_USER@$TEST_VM_IP" bash <<'REMOTE_DEPLOY'
set -euo pipefail
cd /opt/clm/deploy
COMPOSE="docker compose -f docker-compose.test.yml"
BACKUP_DIR=/opt/clm/backups
TIMESTAMP=$(date -u +%Y%m%dT%H%M%SZ)

$COMPOSE config --quiet </dev/null

clean_install=false
postgres_container=$(docker ps -aq \
    --filter "label=com.docker.compose.project=deploy" \
    --filter "label=com.docker.compose.service=postgres" | head -1)
if [ -z "$postgres_container" ]; then
    clean_install=true
fi

echo "Ensuring PostgreSQL, Redis, and Keycloak are running..."
$COMPOSE up -d postgres redis keycloak </dev/null

postgres_ready=false
for i in $(seq 1 60); do
    # Redirect stdin: this script itself arrives over SSH stdin, and an attached
    # compose exec can otherwise consume the remaining deployment program.
    if $COMPOSE exec -T postgres pg_isready -U clm \
        </dev/null >/dev/null 2>&1; then
        postgres_ready=true
        break
    fi
    echo "  waiting for PostgreSQL... ($i/60)"
    sleep 5
done
[ "$postgres_ready" = true ] || {
    echo "PostgreSQL failed to become ready" >&2
    $COMPOSE logs --tail=100 postgres >&2
    exit 1
}

keycloak_ready=false
for i in $(seq 1 60); do
    if curl -fsS \
        http://localhost:8180/auth/realms/clm/.well-known/openid-configuration \
        >/dev/null 2>&1; then
        keycloak_ready=true
        break
    fi
    echo "  waiting for Keycloak realm... ($i/60)"
    sleep 5
done
[ "$keycloak_ready" = true ] || {
    echo "Keycloak failed to import and serve the CLM realm" >&2
    $COMPOSE logs --tail=100 keycloak >&2
    exit 1
}

# This isolated test profile has an HTTP-only nginx listener. Keep the realm
# source secure-by-default (`external`) and relax transport only in this test
# deployment; production never runs deploy-test.sh.
docker exec deploy-keycloak-1 sh -lc \
    '/opt/keycloak/bin/kcadm.sh config credentials \
        --server http://127.0.0.1:8180/auth \
        --realm master \
        --user "$KEYCLOAK_ADMIN" \
        --password "$KEYCLOAK_ADMIN_PASSWORD" >/dev/null'
docker exec deploy-keycloak-1 /opt/keycloak/bin/kcadm.sh \
    update realms/clm -s sslRequired=NONE >/dev/null

# Tests use a disposable clm_test database and never mutate clm_db.
./run-tests.sh </dev/null
$COMPOSE build clm-server </dev/null

if [ "$clean_install" = true ]; then
    # Ordered init scripts created both databases and applied every migration.
    # Start the freshly built API and its only public ingress after validation.
    $COMPOSE up -d clm-server nginx </dev/null
else
    # Capture restorable custom-format dumps immediately before mutating an
    # existing schema. These files remain mode-restricted on the VM.
    umask 077
    mkdir -p "$BACKUP_DIR"
    CLM_BACKUP="$BACKUP_DIR/clm_db_${TIMESTAMP}.dump"
    KEYCLOAK_BACKUP="$BACKUP_DIR/keycloak_${TIMESTAMP}.dump"
    $COMPOSE exec -T postgres pg_dump -U clm -d clm_db -Fc < /dev/null > "$CLM_BACKUP"
    $COMPOSE exec -T postgres pg_dump -U clm -d keycloak -Fc < /dev/null > "$KEYCLOAK_BACKUP"
    test -s "$CLM_BACKUP"
    test -s "$KEYCLOAK_BACKUP"
    echo "Backups created: $CLM_BACKUP and $KEYCLOAK_BACKUP"

    # Init scripts do not rerun against an existing volume.
    for migration in migrations/003_tenant_security.sql migrations/004_secure_command_transport.sql migrations/009_approval_requests_rls.sql migrations/010_certificates_rls.sql; do
        echo "Applying $migration"
        $COMPOSE exec -T postgres psql -U clm -d clm_db \
            -v ON_ERROR_STOP=1 --single-transaction < "$migration"
    done
    $COMPOSE up -d --no-deps clm-server </dev/null
fi

# Keep nginx lifecycle explicit for both fresh and in-place deployments. It is
# the only public ingress; API, PostgreSQL, Redis, and Keycloak ports are not
# opened in the Lightsail firewall.
$COMPOSE up -d nginx </dev/null

echo "Waiting for CLM health recovery..."
healthy=false
for i in $(seq 1 60); do
    if curl -fsS http://localhost/health >/dev/null 2>&1; then
        healthy=true
        break
    fi
    echo "  waiting... ($i/60)"
    sleep 5
done
if [ "$healthy" != true ]; then
    echo "CLM server failed to recover health" >&2
    $COMPOSE logs --tail=100 clm-server nginx >&2
    exit 1
fi

# Assert concrete schema outcomes rather than treating process success as proof.
SCHEMA_OK=$($COMPOSE exec -T postgres psql -U clm -d clm_db -tAc "
SELECT
  to_regclass('public.cloud_accounts') IS NOT NULL
  AND to_regclass('public.control_plane_audit_events') IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema='public' AND table_name='satellite_commands' AND column_name='lease_token'
  )
  AND EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema='public' AND table_name='satellites' AND column_name='credential_hash'
  )
  AND EXISTS (
    SELECT 1 FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname='public' AND c.relname='approval_requests'
      AND c.relrowsecurity AND c.relforcerowsecurity
  )
  AND EXISTS (
    SELECT 1 FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname='public' AND c.relname='approval_rules'
      AND c.relrowsecurity AND c.relforcerowsecurity
  )
  AND EXISTS (
    SELECT 1 FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname='public' AND c.relname='certificates'
      AND c.relrowsecurity AND c.relforcerowsecurity
  );" </dev/null)
[ "$SCHEMA_OK" = "t" ] || { echo "Live schema verification failed" >&2; exit 1; }

echo "CLM server is healthy and approval/certificate RLS is enabled and forced."
$COMPOSE ps
$COMPOSE logs --tail=50 clm-server
REMOTE_DEPLOY

    log "Deployment complete!"
    echo ""
    echo "=============================================="
    echo "  CLM SaaS Control Plane + Admin Portal"
    echo "  Profile: test"
    echo "=============================================="
    echo ""
    echo "  URL:      http://$TEST_VM_IP"
    echo "  API:      http://$TEST_VM_IP/api/v1"
    echo "  Health:   http://$TEST_VM_IP/health"
    echo ""
    echo "  Test users: provision through Keycloak at runtime"
    echo "  Runtime credentials: /opt/clm/deploy/.env (mode 600 on the VM)"
    echo ""
    echo "=============================================="
}

ssh_to_vm() {
    local ssh_opts="-o StrictHostKeyChecking=no"
    if [ -f "$SSH_KEY_PATH" ]; then
        ssh_opts="$ssh_opts -i $SSH_KEY_PATH"
    fi
    log "Connecting to $TEST_VM_IP..."
    ssh $ssh_opts "$SSH_USER@$TEST_VM_IP"
}

show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  (no args)      Deploy to existing test VM ($TEST_VM_IP)"
    echo "  --create-vm    Create new Lightsail VM, then deploy"
    echo "  --ssh-only     SSH into the test VM"
    echo "  --status       Check deployment status"
    echo "  --logs         Show clm-server logs"
    echo "  --teardown     Stop and remove containers (data preserved)"
    echo "  -h, --help     Show this help"
}

check_status() {
    local ssh_opts="-o StrictHostKeyChecking=no -o ConnectTimeout=10"
    if [ -f "$SSH_KEY_PATH" ]; then
        ssh_opts="$ssh_opts -i $SSH_KEY_PATH"
    fi

    log "Checking deployment status on $TEST_VM_IP..."
    ssh $ssh_opts "$SSH_USER@$TEST_VM_IP" "cd /opt/clm/deploy && docker compose -f docker-compose.test.yml ps" 2>/dev/null || \
        error "Could not connect to VM or containers not running."

    echo ""
    log "Health check:"
    curl -sf "http://$TEST_VM_IP/health" 2>/dev/null && echo "" || error "Health check failed."
}

show_logs() {
    local ssh_opts="-o StrictHostKeyChecking=no -o ConnectTimeout=10"
    if [ -f "$SSH_KEY_PATH" ]; then
        ssh_opts="$ssh_opts -i $SSH_KEY_PATH"
    fi

    ssh $ssh_opts "$SSH_USER@$TEST_VM_IP" \
        "cd /opt/clm/deploy && docker compose -f docker-compose.test.yml logs --tail=50 clm-server"
}

teardown() {
    local ssh_opts="-o StrictHostKeyChecking=no -o ConnectTimeout=10"
    if [ -f "$SSH_KEY_PATH" ]; then
        ssh_opts="$ssh_opts -i $SSH_KEY_PATH"
    fi

    warn "Stopping containers on $TEST_VM_IP (data volumes preserved)..."
    ssh $ssh_opts "$SSH_USER@$TEST_VM_IP" \
        "cd /opt/clm/deploy && docker compose -f docker-compose.test.yml down"
    log "Containers stopped."
}

# --- Main ---
case "${1:-}" in
    --create-vm)
        create_lightsail_vm
        sleep 30  # Give cloud-init time to install Docker
        deploy_to_vm
        ;;
    --ssh-only)
        ssh_to_vm
        ;;
    --status)
        check_status
        ;;
    --logs)
        show_logs
        ;;
    --teardown)
        teardown
        ;;
    -h|--help)
        show_usage
        ;;
    "")
        deploy_to_vm
        ;;
    *)
        error "Unknown option: $1"
        show_usage
        exit 1
        ;;
esac
