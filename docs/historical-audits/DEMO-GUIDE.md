# CLM Local Demonstration Guide

## What this demo runs

The database-backed demo uses the existing Go SaaS control plane, PostgreSQL, and
the existing Node portal server. The portal proxies `/api/v1/*` to the Go API.
Contract lifecycle records, users, approvals, and control-plane audit events are
stored in the isolated local `clm_demo` database.

The Satellite Gateway portal is still a local preview with in-memory APIs. This
setup does not start Redis, Keycloak, Docker, the Satellite Gateway service, or
vSatellite agents. External e-signature, cloud, and AI providers are not
configured or represented as tested integrations.

`npm start` and `node dev-server.js` remain the separate in-memory preview mode.
They do not use PostgreSQL or the Go API.

## Prerequisites

- Node.js 18 or newer and npm.
- Go, available as `go` on PATH (or set `CLM_GO_PATH`).
- A local PostgreSQL server listening on `127.0.0.1:5432`, plus `psql` (or set
  `PSQL_PATH`).
- A PostgreSQL administrator login able to create a role and database. The
  default is the local `postgres` role; if password authentication is enabled,
  set `CLM_DEMO_PG_ADMIN_PASSWORD` in the current terminal before setup.
- A modern browser.

Docker and Redis are not required by the current Go control-plane runtime.

## Setup

From PowerShell:

```powershell
Set-Location 'D:\400 clm\CLM'
npm run demo:setup
```

Setup creates the dedicated `clm_demo` role/database only if neither already
exists. It generates local-only secrets in the ignored `.env.demo.local` file,
applies migrations `001` through `008` in order, skips the optional Keycloak
database migration, runs focused Go tests including PostgreSQL repository
integration tests against the demo database, and builds the Go server.
Migration `008` also requires the PostgreSQL cluster role `clm_table_owner`;
setup creates it only when absent and only with `NOLOGIN`, `NOSUPERUSER`, and
`NOBYPASSRLS`. It refuses to modify an existing role with different privileges.

Setup refuses to reuse an existing `clm_demo` database unless its ownership
marker matches `.env.demo.local`. It never reads or overwrites the existing
control-plane `deploy/.env` file.

If PostgreSQL administrator authentication requires a password, provide it for
the setup command in the current PowerShell session:

```powershell
$env:CLM_DEMO_PG_ADMIN_USER = 'postgres'
$env:CLM_DEMO_PG_ADMIN_PASSWORD = Read-Host 'Local PostgreSQL administrator password'
npm run demo:setup
Remove-Item Env:\CLM_DEMO_PG_ADMIN_PASSWORD
```

## Start and stop

Start the database-backed demo in a foreground terminal:

```powershell
npm run demo:start
```

The startup script verifies the database-backed API, creates the demo
organization/admin/approver accounts if needed, and ensures one demo MSA template
and contract exist.

- Admin portal: <http://127.0.0.1:3000>
- Gateway portal preview: <http://127.0.0.1:8080>
- Go API health: <http://127.0.0.1:8081/health>
- Portal-to-Go API health: <http://127.0.0.1:3000/api/v1/health>

Stop from another terminal with `npm run demo:stop`, or press Ctrl+C in the
startup terminal. The stop script signals only the managed demo session; it does
not terminate processes by name.

The demo binds to loopback addresses only. If any of ports 3000, 8080, or 8081
are already in use, startup stops rather than terminating or reusing an
unverified process.

## Demo credentials

These are public, demo-only credentials and must never be reused outside this
local demonstration:

| Role | Email | Password |
|---|---|---|
| Tenant administrator | `admin@enterprise.internal` | `Demo@2024!` |
| Approver | `approver@enterprise.internal` | `Demo@2024!` |

The first successful startup creates the organization and both users through
the Go API. The administrator is a tenant administrator, not a platform
super-admin.

## Demonstration workflow

1. Open the admin portal and sign in with the tenant administrator credentials.
2. Open the Contracts page and inspect the persisted `CLM Demo MSA`.
3. Use the template library to inspect `CLM Demo MSA Template`.
4. Open Approvals to review workflow records after running the live test below.
5. Run `npm run demo:test` from another terminal to exercise persisted contract
   creation, version history, template assembly, negotiation redlines, internal
   risk analysis, a separate-user approval, demo signature evidence, obligation
   fulfillment, audit history, and cross-tenant isolation through the Go API.
6. Refresh the portal and inspect the persisted records.

The signature operation is a local SHA-256 demo attestation; it is not a
legally-binding electronic signature and does not use customer private keys or
production certificates. AI analysis uses the repository's local engine; no
external model was connected.

## Health and tests

```powershell
npm run demo:health
npm run demo:test
```

The health check verifies the control-plane health endpoint, both portals, the
Go API proxy, demo login, tenant-scoped contracts and analytics, approval API,
and audit API. The test command runs the existing Node preview suites, Go tests
with PostgreSQL integration tests enabled against `clm_demo`, and the live API
workflow.

## Reset and troubleshooting

The live workflow intentionally writes demo records and audit events. To remove
the isolated demo database, stop the demo and use the explicit confirmation
flag:

```powershell
npm run demo:stop
npm run demo:reset -- --confirm
npm run demo:setup
```

Reset checks the database ownership marker and removes only the `clm_demo`
database and role. It preserves `.env.demo.local`; it does not touch other
databases or production configuration.

Common issues:

- **Go or `psql` not found:** install the tools or set `CLM_GO_PATH` and
  `PSQL_PATH` for the setup command.
- **PostgreSQL connection refused:** start the local PostgreSQL service and
  confirm it listens on `127.0.0.1:5432`.
- **Administrator authentication failed:** set
  `CLM_DEMO_PG_ADMIN_USER` and `CLM_DEMO_PG_ADMIN_PASSWORD` in the setup
  terminal.
- **A demo port is occupied:** identify and stop that service yourself, then
  restart the demo. CLM will not kill unrelated processes.
- **A demo ownership marker mismatch:** stop and inspect the database and
  `.env.demo.local`; setup/reset intentionally refuse to guess ownership.
- **An API route returns 404:** the database-backed mode sends `/api/v1/*` to
  the real Go API; routes that exist only in the mock preview are not silently
  served from mock data.
