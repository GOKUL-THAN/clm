#!/bin/bash
# ==============================================================================
# CLM vSatellite Endpoint Agent Installation Script
# Supports: Ubuntu, Debian, RHEL, Rocky, AlmaLinux, Amazon Linux 2 & 2023
# ==============================================================================
set -euo pipefail

GATEWAY_ENDPOINT="${1:-http://127.0.0.1:9090}"
ENROLL_TOKEN="${2:-}"

INSTALL_DIR="/opt/clm-vsatellite"
CONFIG_DIR="/etc/clm"
SERVICE_FILE="/etc/systemd/system/clm-vsatellite.service"

echo "=== Installing CLM vSatellite Endpoint Agent ==="

# Create directories
mkdir -p "$INSTALL_DIR/bin" "$CONFIG_DIR"

# Copy binary
if [ -f "bin/clm-vsatellite" ]; then
    cp bin/clm-vsatellite "$INSTALL_DIR/bin/"
    chmod 755 "$INSTALL_DIR/bin/clm-vsatellite"
elif [ -f "../bin/clm-vsatellite" ]; then
    cp ../bin/clm-vsatellite "$INSTALL_DIR/bin/"
    chmod 755 "$INSTALL_DIR/bin/clm-vsatellite"
fi

# Copy config if not already present
if [ ! -f "$CONFIG_DIR/vsatellite.yaml" ]; then
    if [ -f "configs/vsatellite.yaml" ]; then
        cp configs/vsatellite.yaml "$CONFIG_DIR/"
    elif [ -f "../configs/vsatellite.yaml" ]; then
        cp ../configs/vsatellite.yaml "$CONFIG_DIR/"
    fi
    # Update endpoint and token
    sed -i "s|gateway_endpoint: .*|gateway_endpoint: \"$GATEWAY_ENDPOINT\"|" "$CONFIG_DIR/vsatellite.yaml"
    if [ -n "$ENROLL_TOKEN" ]; then
        sed -i "s|token: .*|token: \"$ENROLL_TOKEN\"|" "$CONFIG_DIR/vsatellite.yaml"
    fi
    chmod 600 "$CONFIG_DIR/vsatellite.yaml"
fi

# Install systemd service
if [ -f "deploy/clm-vsatellite.service" ]; then
    cp deploy/clm-vsatellite.service "$SERVICE_FILE"
elif [ -f "clm-vsatellite.service" ]; then
    cp clm-vsatellite.service "$SERVICE_FILE"
fi

systemctl daemon-reload
systemctl enable clm-vsatellite.service
echo "=== Installation complete. Start with: systemctl start clm-vsatellite ==="
