#!/usr/bin/env bash
# ==============================================================================
# CLM Automated Runner & Orchestrator (Linux / macOS)
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo -e "\033[36m==========================================================\033[0m"
echo -e "\033[33m       CLM AUTOMATED RUNNER & ORCHESTRATOR               \033[0m"
echo -e "\033[36m==========================================================\033[0m"

# Step 1: Ensure Runtime Environment Secrets (.env)
ENV_GEN="$PROJECT_ROOT/components/saas-control-plane/deploy/generate-env.sh"
if [ -f "$ENV_GEN" ]; then
    echo -e "\033[32m[1/4] Ensuring runtime environment secrets...\033[0m"
    bash "$ENV_GEN"
fi

# Step 2: Check Node.js Runtime
echo -e "\033[32m[2/4] Verifying execution runtime...\033[0m"
if ! command -v node &>/dev/null; then
    echo "Error: Node.js is required. Please install Node.js." >&2
    exit 1
fi
echo "      Node.js version: $(node --version)"

# Step 3: Launch Unified Development & API Server
echo -e "\033[32m[3/4] Launching CLM Unified Portals and Background APIs...\033[0m"
cd "$PROJECT_ROOT"
node dev-server.js &
SERVER_PID=$!
trap 'kill $SERVER_PID 2>/dev/null || true' EXIT

sleep 2

# Step 4: Display Automated Automation Status
echo -e "\033[32m[4/4] Automated Background Pipelines Active:\033[0m"
echo "      [+] Automated TLS Network Discovery: Sweeping 10.125.0.0/16 on ports 443, 8443, 636, 6443"
echo "      [+] Automated Certificate Expiry Alerts: Scanning every 1h (Warning: 30d, Critical: 7d)"
echo "      [+] Automated ACME Renewal Engine: Sweeping every 12h for certs expiring within 30 days"
echo "      [+] Automated vSatellite Fleet Coordinator: Polling leased agent commands every 30s"

echo ""
echo -e "\033[36m==========================================================\033[0m"
echo -e "\033[33m  CLM PLATFORM READY AND RUNNING AUTOMATICALLY\033[0m"
echo -e "\033[36m==========================================================\033[0m"
echo -e "\033[32m  * SaaS Admin Portal:         http://localhost:3000\033[0m"
echo -e "\033[32m  * Satellite Gateway Portal:  http://localhost:8080\033[0m"
echo -e "\033[32m  * vSatellite API:            http://localhost:8080/vsatellite/agents\033[0m"
echo -e "\033[36m==========================================================\033[0m"
echo "Press Ctrl+C to stop the automated services."

wait $SERVER_PID

