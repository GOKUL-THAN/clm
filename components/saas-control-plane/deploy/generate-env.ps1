# PowerShell script to generate deploy/.env with random cryptographic secrets
$envPath = Join-Path $PSScriptRoot ".env"

if (Test-Path $envPath) {
    Write-Host "deploy/.env already exists. Preserving existing secrets."
    exit 0
}

Write-Host "Generating deploy/.env with secure random secrets..."

function Get-RandomHex([int]$bytes) {
    $buffer = New-Object byte[] $bytes
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($buffer)
    return [System.BitConverter]::ToString($buffer).Replace("-", "").ToLower()
}

$pgPass = Get-RandomHex 16
$jwtSec = Get-RandomHex 32
$masterKey = Get-RandomHex 16
$kcPass = Get-RandomHex 16
$clientSec = Get-RandomHex 16
$satToken = Get-RandomHex 16

$content = @"
POSTGRES_PASSWORD=$pgPass
REDIS_PASSWORD=
JWT_SECRET=$jwtSec
CLM_JWKS_URL=http://keycloak:8180/auth/realms/clm/protocol/openid-connect/certs
CLM_EXTERNAL_JWT_ISSUER=http://localhost/auth/realms/clm
CLM_EXTERNAL_JWT_AUDIENCE=clm-api
CLM_CLOUD_ACCOUNT_MASTER_KEY=$masterKey

KEYCLOAK_ADMIN_USER=admin
KEYCLOAK_ADMIN_PASS=$kcPass
CLM_SERVER_CLIENT_SECRET=$clientSec

PUBLIC_URL=http://localhost
CLM_ADMIN_PORTAL_URL=http://localhost
CLM_GATEWAY_PORTAL_URL=http://localhost

CLM_SATELLITE_TOKEN=$satToken
CLM_SATELLITE_ID=sat-local-01
CLM_SATELLITE_NAME=Local-Satellite-Gateway
CLM_SAAS_ENDPOINT=http://clm-server:8080
CLM_TLS_SERVER_NAME=localhost
"@

Set-Content -Path $envPath -Value $content -Encoding ASCII
Write-Host "Generated $envPath successfully."

