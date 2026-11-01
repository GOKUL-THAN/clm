#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "usage: $0 <module-path> <destination>" >&2
    exit 2
fi

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
MODULE=$1
DESTINATION=$2
CONTROL="$ROOT/components/saas-control-plane"
GATEWAY="$ROOT/components/satellite-gateway"
VSATELLITE="$ROOT/components/vsatellite"

rm -rf "$DESTINATION"
mkdir -p "$DESTINATION"

stage_shared_gateway_source() {
    mkdir -p "$DESTINATION/internal/satellite" "$DESTINATION/internal/core"
    for source in "$GATEWAY"/internal/*; do
        [[ -e "$source" ]] || continue
        name=$(basename "$source")
        case "$name" in
            runtime)
                cp "$source"/*.go "$DESTINATION/internal/satellite/"
                ;;
            vsatellite-coordinator)
                cp -R "$source" "$DESTINATION/internal/satellite/vsatellite"
                ;;
            *)
                cp -R "$source" "$DESTINATION/internal/satellite/$name"
                ;;
        esac
    done
    cp "$CONTROL/internal/core/interfaces.go" "$DESTINATION/internal/core/"
    if [[ ! -d "$DESTINATION/pkg" ]]; then
        cp -R "$GATEWAY/pkg" "$DESTINATION/"
    fi
}

case "$MODULE" in
    components/saas-control-plane)
        cp -R "$CONTROL"/. "$DESTINATION/"
        ;;
    components/satellite-gateway)
        cp -R "$GATEWAY"/. "$DESTINATION/"
        rm -rf "$DESTINATION/internal"
        stage_shared_gateway_source
        ;;
    components/vsatellite)
        cp -R "$VSATELLITE"/. "$DESTINATION/"
        rm -rf "$DESTINATION/internal" "$DESTINATION/pkg"
        mkdir -p "$DESTINATION/internal/satellite" "$DESTINATION/internal/core"
        cp -R "$GATEWAY/internal/deployment" "$DESTINATION/internal/satellite/"
        cp -R "$GATEWAY/internal/discovery" "$DESTINATION/internal/satellite/"
        rm -rf "$DESTINATION/internal/satellite/deployment/deployers/cloud"
        rm -rf "$DESTINATION/internal/satellite/discovery/scanners/cloud"
        cp "$CONTROL/internal/core/interfaces.go" "$DESTINATION/internal/core/"
        cp -R "$GATEWAY/pkg" "$DESTINATION/"
        cp "$GATEWAY/go.mod" "$DESTINATION/go.mod"
        ;;
    *)
        echo "unsupported module path: $MODULE" >&2
        exit 2
        ;;
esac
