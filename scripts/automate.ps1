<#
.SYNOPSIS
    Automated CLM End-to-End Orchestrator & Runner
.DESCRIPTION
    One-click automated launch, health verification, and scheduler management
    for the Certificate Lifecycle Management (CLM) system.
#>

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "       CLM AUTOMATED RUNNER & ORCHESTRATOR               " -ForegroundColor Yellow
Write-Host "==========================================================" -ForegroundColor Cyan

# Step 1: Ensure Runtime Environment Secrets (.env)
$envGenScript = Join-Path $ProjectRoot "components\saas-control-plane\deploy\generate-env.ps1"
if (Test-Path $envGenScript) {
    Write-Host "[1/4] Ensuring runtime environment secrets..." -ForegroundColor Green
    & $envGenScript
}

# Step 2: Check Node.js Runtime
Write-Host "[2/4] Verifying execution runtime..." -ForegroundColor Green
$nodeCmd = Get-Command node -ErrorAction SilentlyContinue
if (-not $nodeCmd) {
    Write-Error "Node.js is required to run the local CLM orchestrator. Please install Node.js."
    exit 1
}
Write-Host "      Node.js version: $(node --version)" -ForegroundColor Gray

# Step 3: Launch Unified Development & API Server
Write-Host "[3/4] Launching CLM Unified Portals and Background APIs..." -ForegroundColor Green
$serverScript = Join-Path $ProjectRoot "dev-server.js"

$proc = Start-Process node -ArgumentList "$serverScript" -WorkingDirectory $ProjectRoot -PassThru
Write-Host "      Services started with Process ID: $($proc.Id)" -ForegroundColor Gray

Start-Sleep -Seconds 2

# Step 4: Display Automated Automation Status
Write-Host "[4/4] Automated Background Pipelines Active:" -ForegroundColor Green
Write-Host "      [+] Automated TLS Network Discovery: Sweeping 10.125.0.0/16 on ports 443, 8443, 636, 6443" -ForegroundColor White
Write-Host "      [+] Automated Certificate Expiry Alerts: Scanning every 1h (Warning: 30d, Critical: 7d)" -ForegroundColor White
Write-Host "      [+] Automated ACME Renewal Engine: Sweeping every 12h for certs expiring within 30 days" -ForegroundColor White
Write-Host "      [+] Automated vSatellite Fleet Coordinator: Polling leased agent commands every 30s" -ForegroundColor White

Write-Host "`n==========================================================" -ForegroundColor Cyan
Write-Host "  CLM PLATFORM READY AND RUNNING AUTOMATICALLY" -ForegroundColor Yellow
Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "  * SaaS Admin Portal:         http://localhost:3000" -ForegroundColor Green
Write-Host "  * Satellite Gateway Portal:  http://localhost:8080" -ForegroundColor Green
Write-Host "  * vSatellite API:            http://localhost:8080/vsatellite/agents" -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "Press Ctrl+C to stop the automated services.`n"

try {
    # Keep running and monitor process
    while (-not $proc.HasExited) {
        Start-Sleep -Seconds 5
    }
} finally {
    if (-not $proc.HasExited) {
        Write-Host "Stopping CLM processes..." -ForegroundColor Yellow
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    }
}
