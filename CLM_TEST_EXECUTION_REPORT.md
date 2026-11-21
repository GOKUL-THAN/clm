# CLM Test Execution Report

## Node.js

| Command | Result |
|---|---|
| `npm test` | 40/40 passed |
| `npm run test:400` | 400/400 passed |
| `npm run test:corporate` | 10/10 passed |
| `npm run test:lifecycle` | 26/26 passed |
| `npm run test:workflows` | 3/3 passed |
| `npm run test:global` | 15/15 passed |
| `npm run test:v410` | 20/20 passed |

## Staged Go

- Control plane: vet passed, tests passed, build passed.
- Satellite gateway: vet passed, tests passed, build passed.
- vSatellite: vet passed, tests passed, build passed.
- Added focused security regressions for SSO state, SSO role restrictions, SAML certificates/signatures, OIDC destination rejection, ITSM tenant scope and Gateway inventory tenant scope.

## Blocked

- Live PostgreSQL RLS suite.
- Database-backed demo health.
- Docker/terraform/container scans: Docker unavailable or not configured locally.
- Go race suite: CGO race support unavailable in the local Windows environment.

