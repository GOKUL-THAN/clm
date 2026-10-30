#!/bin/bash
# Build/test/vet a component in the repository's canonical github.com/m2p/clm
# import layout without mutating source go.mod files.
set -euo pipefail

COMPONENT="${1:?usage: go-staged.sh <control|satellite|vsatellite> <build|test|vet>}"
ACTION="${2:?usage: go-staged.sh <control|satellite|vsatellite> <build|test|vet>}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPONENTS="$(cd "$SCRIPT_DIR/../.." && pwd)"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

case "$COMPONENT" in
  control)
    SOURCE="$COMPONENTS/saas-control-plane"
    cp -R "$SOURCE/." "$STAGE/"
    sed -i.bak 's|module github.com/m2p/clm/saas-control-plane|module github.com/m2p/clm|' "$STAGE/go.mod"
    rm -f "$STAGE/go.mod.bak"
    ;;
  satellite)
    SOURCE="$COMPONENTS/satellite-gateway"
    "$SOURCE/deploy/build-src-bundle.sh" "$STAGE/source.tar.gz" >/dev/null
    tar xzf "$STAGE/source.tar.gz" -C "$STAGE"
    rm -f "$STAGE/source.tar.gz"
    sed -i.bak 's|module github.com/m2p/clm/satellite-gateway|module github.com/m2p/clm|' "$STAGE/go.mod"
    rm -f "$STAGE/go.mod.bak"
    (cd "$STAGE" && go mod tidy)
    ;;
  vsatellite)
    SOURCE="$COMPONENTS/vsatellite"
    GATEWAY="$COMPONENTS/satellite-gateway"
    "$GATEWAY/deploy/build-src-bundle.sh" "$STAGE/source.tar.gz" >/dev/null
    tar xzf "$STAGE/source.tar.gz" -C "$STAGE"
    rm -rf "$STAGE/cmd" "$STAGE/source.tar.gz"
    cp -R "$SOURCE/cmd" "$STAGE/"
    if [ -d "$SOURCE/internal" ]; then
      mkdir -p "$STAGE/vsatellite"
      cp -R "$SOURCE/internal" "$STAGE/vsatellite/"
      cp -R "$SOURCE/internal" "$STAGE/"
    fi
    sed -i.bak 's|module github.com/m2p/clm/satellite-gateway|module github.com/m2p/clm|' "$STAGE/go.mod"
    rm -f "$STAGE/go.mod.bak"
    (cd "$STAGE" && go mod tidy)
    ;;
  *) echo "unknown component: $COMPONENT" >&2; exit 2 ;;
esac

case "$ACTION" in
  test) (cd "$STAGE" && go test ./...) ;;
  vet) (cd "$STAGE" && go vet ./...) ;;
  build)
    mkdir -p "$SOURCE/bin"
    case "$COMPONENT" in
      control)
        (cd "$STAGE" && go build -ldflags='-s -w' -o "$SOURCE/bin/clm-server" ./cmd/clm-server)
        (cd "$STAGE" && go build -ldflags='-s -w' -o "$SOURCE/bin/clm" ./cmd/clm-cli)
        ;;
      satellite) (cd "$STAGE" && go build -ldflags='-s -w' -o "$SOURCE/bin/clm-satellite" ./cmd/clm-satellite) ;;
      vsatellite) (cd "$STAGE" && go build -ldflags='-s -w' -o "$SOURCE/bin/clm-vsatellite" ./cmd/clm-vsatellite) ;;
    esac
    ;;
  *) echo "unknown action: $ACTION" >&2; exit 2 ;;
esac
