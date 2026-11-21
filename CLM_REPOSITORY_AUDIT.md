# CLM Repository Audit

## Scope

- Repository revision audited: `5a1d55d3daa2cf579c37b73edbd14b9e1e9c5cf0` (baseline).
- Remediation branch: `copilot/clm-remediation`.
- Repository root: `D:\CLM`.

## Initial inventory

- Tracked 2,026 files.
- Primary stack: Node.js 18+ demonstration server, Go 1.26 modules, PostgreSQL migrations, Vercel configuration, Docker and Kubernetes.
- Three Go modules: SaaS control plane, satellite gateway, vSatellite.
- Seven AppleDouble artifacts directories were represented by 319 tracked `._*` files. Every file had the AppleDouble magic header `00 05 16 07`; they were removed and `.gitignore` now excludes `._*`.
- The README incorrectly referenced `GOKUL-THAN/cmd`; it was corrected to `GOKUL-THAN/clm`.

## Baseline failures

- Direct `go test ./...` in component modules failed because those modules are incomplete outside the repository CI staging script.
- `npm run demo:health` failed because `.env.demo.local` was not configured.
- Initial `govulncheck` used Go 1.27.3 and found GO-2026-6859 in `net/http`.

## Remediation state

- Five source security findings were fixed.
- Go modules, Docker builders and CI now use Go 1.27.4 or Go 1.26.11 as appropriate.
- All declared Node suites pass locally.

