# Repository Inventory

**Workspace:** `D:\400 clm\CLM`
**Git root:** `D:\400 clm\CLM`
**Inventory date:** `2026-11-04`

This is a structural inventory derived from the checked-out workspace and
`git ls-files`; it is not a claim that every source file or business workflow
has been audited. Existing discovery reports reference a different historical
path and contain architecture claims that require reconciliation.

## Top-level areas

| Path | Observed content | Tracked files |
|---|---|---:|
| `components/saas-control-plane/` | Go SaaS backend, REST API, domain services, Postgres stores/migrations, deployment assets, admin portal | 303 |
| `components/satellite-gateway/` | Go gateway, local crypto/CA/deployment services, configuration and gateway UI | 220 |
| `components/shared/` | Shared components, scripts and support code | 16 |
| `components/vsatellite/` | Go virtual satellite connector | 24 |
| `features/` | Feature registry and feature definitions | 1,201 |
| `scripts/` | Node, SQL and shell verification/development scripts | 30 |
| `docs/` | Architecture, API, operations and feature documentation | 43 |
| `compliance/` | Compliance and standards material | 22 |
| `.github/` | CI workflows and helper scripts | 6 |
| `deploy/` | Deployment configuration | 9 |
| `services/` | Service-level artifacts | 13 |
| `api/` | API-level artifacts | 1 |
| `semantic-review/` | Review artifacts | 4 |

Tracked language/configuration file counts include 366 Go files, 848 JavaScript
files, 28 SQL files, 19 shell scripts, 15 YAML files, 6 YML files, and 15 JSON
files. These counts include test and generated/support code where present; they
are not module-quality or feature-completion metrics.

## Root-level implementation and evidence

- `dev-server.js` and `package.json`: Node.js demo runtime and declared npm
  scripts. The demo harness must not be conflated with the Go backend.
- `README.md`: user-facing architecture, demo, and test documentation; its
  current status and test-count language was corrected in this execution.
- `components/saas-control-plane/`: primary Go service. Its module includes
  configuration, REST handlers, tenant and approval services, certificate
  storage, monitoring, renewal, and database migrations.
- `components/satellite-gateway/` and `components/vsatellite/`: customer-side
  components whose runtime and hardware/provider paths have not been verified
  in this execution.
- `.github/workflows/ci.yml`: Go matrix, syntax/security checks, and
  PostgreSQL RLS workflow configuration; remote workflow execution was not
  observed here.
- `deploy/`, component `deploy/`, and component `configs/`: deployment and
  runtime configuration; deployment readiness is not established by file
  presence.
- `features/` and the feature traceability documents: registry content; the
  400-entry test is a registry smoke check, not independent proof of each
  capability.
- Root-level audit, readiness, security, crypto, compliance, and test reports:
  historical claims requiring evidence reconciliation. See
  `MASTER_REQUIREMENT_TRACKER.md`.

## Tooling observed

- Node.js and npm are available.
- Go, `gofmt`, and Docker are unavailable on PATH.
- PostgreSQL 18.6 client/server are installed and the local service reports
  ready. The current OS role `gokul` has no matching database role; a local
  `postgres` connection was available and used only for the uniquely named
  scratch database documented in `DATABASE_VERIFICATION_REPORT.md`.
- The Go toolchain and Docker are unavailable. Go compilation and repository
  integration tests remain blocked, but the live PostgreSQL SQL RLS suites
  were executed against that disposable database using a temporary
  password-authenticated application role.

## Inventory limitations

- File counts are from tracked paths; the worktree also contains a pre-existing
  untracked `CLM\` directory, which was deliberately excluded and left
  untouched.
- Some tracked AppleDouble-style `._*` metadata entries are present, so no
  single aggregate source-file count is reported.
- A complete dependency graph, API-to-UI trace, migration-to-runtime inventory,
  and every-file code review remain open work.
