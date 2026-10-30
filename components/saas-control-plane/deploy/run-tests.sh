#!/bin/bash
#
# Runs the Go test suite against a throwaway PostgreSQL database.
#
# There is no local Go toolchain on the development machines, so tests compile
# and run inside a golang container. The container joins the compose network so
# it can reach the postgres service by name.
#
# A dedicated `clm_test` database is created and dropped each run: the tests
# insert and delete rows, and pointing them at clm_db would put live tenant and
# user data at risk.
#
# Usage (from the host running the compose stack):
#   cd /opt/clm/deploy && ./run-tests.sh
set -euo pipefail

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.test.yml}"
SRC_ROOT="${SRC_ROOT:-/opt/clm}"
TEST_DB="clm_test"
PGUSER="clm"

# Docker network the compose stack runs on, so the test container can resolve
# the postgres hostname.
NETWORK="$(docker network ls --format '{{.Name}}' | grep -E 'deploy_default|clm.*default' | head -1)"
if [ -z "$NETWORK" ]; then
    echo "Could not determine the compose network; is the stack running?" >&2
    exit 1
fi
echo "network: $NETWORK"

# Read the runtime-generated password from the isolated database container
# rather than carrying a tracked test credential.
PGPASS=$(docker compose -f "$COMPOSE_FILE" exec -T postgres sh -c 'printf %s "$POSTGRES_PASSWORD"')
if [ -z "$PGPASS" ]; then
    echo "PostgreSQL runtime password is unavailable" >&2
    exit 1
fi

psql_admin() {
    docker compose -f "$COMPOSE_FILE" exec -T postgres psql -U "$PGUSER" -d clm_db "$@"
}
psql_test() {
    docker compose -f "$COMPOSE_FILE" exec -T postgres psql -U "$PGUSER" -d "$TEST_DB" "$@"
}

cleanup() {
    echo "=== dropping $TEST_DB ==="
    psql_admin -c "DROP DATABASE IF EXISTS $TEST_DB;" > /dev/null 2>&1 || true
}
trap cleanup EXIT

echo "=== creating a clean $TEST_DB ==="
psql_admin -c "DROP DATABASE IF EXISTS $TEST_DB;" > /dev/null
psql_admin -c "CREATE DATABASE $TEST_DB OWNER $PGUSER;" > /dev/null

echo "=== applying schema ==="
for f in migrations/001_initial.sql migrations/002_sso_fields.sql migrations/003_tenant_security.sql migrations/004_secure_command_transport.sql; do
    if [ -f "$f" ]; then
        echo "  $f"
        psql_test -v ON_ERROR_STOP=1 -q < "$f" > /dev/null
    fi
done
echo "  tables: $(psql_test -tAc "select count(*) from information_schema.tables where table_schema='public'")"

echo "=== running go test ==="
# The module path is rewritten the same way the production image does it, so the
# github.com/m2p/clm/internal/... imports resolve.
docker run --rm \
    --network "$NETWORK" \
    -v "$SRC_ROOT":/src \
    -w /src \
    -e CLM_TEST_DB_DSN="postgres://$PGUSER:$PGPASS@postgres:5432/$TEST_DB?sslmode=disable" \
    -e GOFLAGS=-buildvcs=false \
    golang:1.22-alpine \
    sh -c '
        apk add --no-cache git > /dev/null 2>&1
        cp -r /src /build && cd /build
        sed -i "s|module github.com/m2p/clm/saas-control-plane|module github.com/m2p/clm|" go.mod
        go mod tidy > /dev/null 2>&1
        echo "--- go vet ---"
        go vet ./...
        echo "--- go test ---"
        # CLM_TEST_DB_REQUIRED makes a missing database a hard failure rather
        # than a silent "ok" with zero tests run. Keep the test process as the
        # final command so its exit status cannot be hidden by output filters.
        CLM_TEST_DB_REQUIRED=1 go test ./... -count=1 -v
    '
