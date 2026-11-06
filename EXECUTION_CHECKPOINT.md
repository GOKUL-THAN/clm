# Execution Checkpoint

**Checkpoint date:** 2026-11-06
**Active scope:** CI and security hardening on `audit/phase0-ci-workflow`.
**Production readiness:** Not established.

## Supplemental checkpoint — 2026-11-06

- Created audit branch `audit/phase0-ci-workflow` from `main` at `2ae097c`.
- Removed the tracked binary AppleDouble path `.github/workflows/._ci.yml`
  (`25b0abb`); GitHub had enumerated it as an invalid second workflow.
- Fixed the actual workflow YAML parse error by quoting the step name containing
  `Table Ownership: clm_table_owner` (`9e90667`).
- Pushed the audit branch to `origin/audit/phase0-ci-workflow`. No merge to
  `main` was performed.
- Hardened vSatellite install-command generation and URL validation
  (`a9003bd`), removed committed test-secret/password placeholders and
  narrowed intentional private-key serialization findings (`d9d056b`), and
  enforced gateway JWT expiry, issuer, and audience validation (`62b8ada`).
- CI run `42151433656` completed on `62b8ada` with seven of eight jobs passing:
  PostgreSQL 15 RLS, vSatellite Go, SaaS Go, syntax, filesystem/IaC/SBOM,
  secret scan, and container scan passed. The gateway job stopped at its
  formatting gate before gateway tests, build, and security scans could run.
- Pushed commit `21f2e64`, which corrected the two reported formatting
  differences, enabled full-history Gitleaks scanning, and recorded the
  preceding CI evidence.
- CI run `42152346744` completed with seven of eight jobs passing. Gateway Go
  formatting, vet, tests, race detector, build, Gosec, and dependency scan
  passed; SaaS and vSatellite Go jobs, PostgreSQL 15 RLS, syntax, filesystem/
  IaC/SBOM, and container scan also passed.
- The full-history Gitleaks job reported six detections in earlier commits:
  removed test-only signing literals, replaced Terraform password placeholders,
  and a rephrased prose false positive. Values were not copied into this record.
- Added six exact historical fingerprints to `.gitleaksignore` in commit
  `01e3ec7`; CI run `42152893086` then passed all eight jobs, including the
  full-history secret scan.
- No history rewrite, merge to `main`, or deployment was performed.
- Re-ran `npm test`, `npm run test:lifecycle`, `npm run test:400`,
  `npm run test:corporate`, `npm run test:workflows`, and `npm run test:global`;
  all six commands exited 0. Registry and lifecycle checks remain script-level
  evidence, not proof of persistent production workflows.
- Local Node.js `v24.19.0` is available. Go, `psql`, and Docker were not found
  on the current PowerShell `PATH`; previous Go/PostgreSQL evidence remains
  historical and was not reproduced in this checkpoint.
- Current structural count: 2,020 tracked paths, 319 of which use an
  AppleDouble `._` filename prefix. Only the workflow sidecar was removed;
  unrelated metadata, the edited root README, and untracked `CLM/` were left
  untouched.
- Worktree evidence and tracker documents are being updated with the current
  terminal CI results. Root `README.md` edits and untracked `CLM/` remain
  unstaged and preserved.

The CI and frontend deployment results below are time-scoped. The prior
2026-11-05 verification table records that day's status and is distinct from
the completed CI run `42151433656` summarized above. The 2026-11-06 Vercel
production deploy updated the Node.js demo portal only and did not deploy or
validate Go services.

## Verification recorded on 2026-11-05

## Last completed work

- Existing approval tenant-isolation changes were retained.
- Added certificate forced-RLS migration 010, tenant-scoped certificate repository operations, explicit trusted scan interface, and internal monitoring/renewal wiring.
- Added certificate handler/repository tests, pagination overflow guard, CI/deployment wiring, and SQL assertions for app-role and table-owner behavior.
- Updated tenant-isolation/API documentation.
- Corrected top-level README, execution-status, and QA-report claims so local Node checks are not presented as production certification.
- Created the phase tracker, implementation plan, and structural repository inventory.

## Verification in this execution

| Check | Result | Scope / limitation |
|---|---|---|
| `npm test` | PASS, 40 checks | Node scenario harness |
| `npm run test:400` | PASS, 400 checks | Registry wrapper smoke checks only |
| `npm run test:lifecycle` | PASS, 26 checks | Node lifecycle harness |
| `npm run test:corporate` | PASS, 10 checks | Node corporate harness |
| `npm run test:workflows` | PASS, 3 checks | Workflow-engine harness |
| `npm run test:global` | PASS, 15 checks | Node advanced-capability harness |
| Git Bash `bash -n` for deploy scripts | PASS | Shell syntax only |
| SaaS Go module: `go mod tidy`, `go vet ./...`, `go test ./... -count=1`, `go build ./...` | PASS | Go 1.22.12 staged module; tests required the live PostgreSQL app-role DSN |
| Satellite gateway Go module: tidy, vet, tests, build | PASS | Staged monorepo module; all package tests passed |
| vSatellite Go module: tidy, vet, tests, build | PASS | Staged monorepo module; all package tests passed |
| `gofmt` | PASS | Changed Go files formatted |
| `go test -race ./...` | BLOCKED | `CGO_ENABLED=0`; no C compiler available |
| PostgreSQL migrations 001–010 | PASS | PostgreSQL 18.6; applied in order to a unique scratch database as `clm_table_owner` |
| `scripts/test_postgres_rls_live.sql` | PASS | Connected as password-authenticated `clm_app_role`; role has `rolsuper=f`, `rolbypassrls=f` |
| `scripts/test_postgres_table_owner_rls.sql` | PASS | Executed with `SET ROLE clm_table_owner`; confirmed FORCE RLS and cross-tenant write denial |
| `scripts/test_postgres_superuser_negative_control.sql` | PASS | Confirmed expected superuser RLS bypass behavior |
| Scratch DB and temporary application-role cleanup | PASS | `clm_verify_20261105_001` and `clm_app_role` absent after cleanup |
| `git diff --check` | PASS | Worktree whitespace |
| CI / staging / production | NOT RUN | No remote run or deployment authorization/evidence |

The six Node suite counts are from separate harnesses, are not a coverage
percentage, and do not verify the Go/PostgreSQL implementation. In particular,
the 400 registry checks do not prove 400 independent business features. The
Go checks used staged copies of the monorepo components. SaaS repository tests
ran with `CLM_TEST_DB_REQUIRED=1` against PostgreSQL 18.6 using the configured
temporary app role; the live `NewDB` connection test passed. PostgreSQL 15 CI,
race detection, external provider integrations, staging, and production were
not verified.

## Current worktree state

The latest evidence-document updates remain uncommitted. The root `README.md`
has separate existing local edits, and untracked `CLM/` remains untouched. Run
`git diff --check` before committing. Do not stage the root README or nested
`CLM/` as part of the audit.

## Next actions

1. Broaden tenant-isolation tests to other tenant-owned repositories and API
   handlers.
2. Execute authenticated API, provider, frontend, performance, recovery, and
   staging checks where suitable credentials and environments are available.
3. Continue remaining phases in `MASTER_REQUIREMENT_TRACKER.md`; do not
   declare production readiness until every applicable release gate has
   evidence.
