# Repository Baseline Report

**Assessment date:** 2026-11-06
**Repository:** `GOKUL-THAN/clm`
**Assessment branch:** `audit/phase0-ci-workflow`
**Production readiness:** Not established

## Scope and evidence limits

This is a Phase 0 snapshot, not a complete file-by-file audit. It records
reproducible repository metadata, current tool availability, the declared
Node.js suites, and the GitHub Actions workflow issue found during this
assessment. Historical reports are treated as leads, not independent evidence.

The assessment did not inspect every source file, every dependency, or every
deployment path. No production credentials, customer networks, or paid cloud
resources were used.

## Repository state

- The audit branch was created from `main` at commit `2ae097c`, which matches
  `origin/main`.
- Audit commit `25b0abb` removes the tracked AppleDouble artifact
  `.github/workflows/._ci.yml`; GitHub had registered this binary metadata file
  as a second workflow.
- Audit commit `9e90667` quotes a workflow step name whose unquoted colon broke
  YAML parsing.
- The audit branch was pushed to `origin/audit/phase0-ci-workflow`. It has not
  been merged to `main`.
- The root `README.md` has pre-existing local edits and the untracked `CLM/`
  directory is present. Neither was staged, modified, or included in the audit
  commits.
- There are 2,020 tracked paths in the current checkout, including 319 paths
  whose names begin with the AppleDouble `._` convention. Only the invalid
  workflow sidecar was removed; the remaining metadata files were left
  untouched pending a separate impact review.

## Local environment and test baseline

Node.js `v24.19.0` and npm are available. Go, `psql`, and Docker were not found
on the current PowerShell `PATH`; therefore Go compilation, current-host live
PostgreSQL validation, and local container validation were not performed in
this assessment.

All six declared Node.js commands completed with exit code 0 on 2026-11-06:

| Command | Result | Scope limitation |
|---|---|---|
| `npm test` | PASS, 40 checks | Local scenario/mock harness |
| `npm run test:lifecycle` | PASS, 26 checks | Script checks; not proof of persistent production workflows |
| `npm run test:400` | PASS, 400 checks | Registry wrapper smoke checks, not 400 independent capabilities |
| `npm run test:corporate` | PASS, 10 checks | Local corporate-module harness |
| `npm run test:workflows` | PASS, 3 checks | Local workflow-engine harness |
| `npm run test:global` | PASS, 15 checks | Local advanced-capability harness |

These are separate harnesses, not a coverage percentage. The test output
contains simulated provider, signature, identity, and lifecycle results; those
results are not live integration evidence.

## CI finding and remediation

The initial workflow parsing defects were corrected on the audit branch: the
tracked AppleDouble sidecar `.github/workflows/._ci.yml` was removed, and the
step name containing `Table Ownership: clm_table_owner` was quoted. Subsequent
commits hardened vSatellite install-command quoting, replaced Terraform
password placeholders with protected inputs, removed test-secret literals,
and added gateway JWT expiration, issuer, and audience validation.

GitHub Actions run `42151433656` tested commit `62b8ada` and completed with
**failure**. Seven of eight jobs passed:

| Job | Result | Evidence / limitation |
|---|---|---|
| PostgreSQL 15 RLS and immutable audit | PASS | Migration and role-isolation CI checks completed |
| vSatellite Go | PASS | CI job completed successfully |
| Filesystem, IaC, and SBOM | PASS | Configured Trivy/IaC/SBOM job completed |
| Script and configuration syntax | PASS | CI syntax job completed |
| SaaS control-plane Go | PASS | Formatting, vet, tests, race detector, build, Gosec, and dependency scan completed |
| Secret scan | PASS | This run used the then-current working-tree scan configuration; it did not scan Git history |
| Container image scan | PASS | Both configured images built and the critical-severity scan completed |
| Satellite gateway Go | FAIL | Formatting gate stopped the job before gateway vet/tests/build/security scans; it identified formatting differences in `internal/api/auth_test.go` and `internal/secrets/store.go` |

Those two Go formatting differences were corrected in commit `21f2e64` and
validated by its CI run. The gateway JWT tests ran successfully. A
full-history Gitleaks workflow adjustment was also pushed in that commit. Its
CI run `42152346744` passed the
gateway formatting, vet, tests, race detector, build, Gosec, and dependency
scan, along with the other non-secret-scan jobs. The full-history secret scan
reported six detections, all in earlier commits: test-only signing literals,
Terraform password placeholders since replaced with validated variables, and
a prose false positive since rephrased. No detected value is reproduced here.
Six exact Gitleaks fingerprints have been added to `.gitleaksignore` for these
reviewed historical false positives; the full-history scan must pass again
before the secret-scan gate is considered verified. No Git history rewrite or
deployment was performed, and production readiness is not established.

## Product and architecture verification

The earlier browser check for the tenant-scoped Cloud Accounts, Gateways, and
Deploy Jobs flows passed against the local demo. The Vercel production portal
was also confirmed to serve the updated tenant-selection script after the
authorized deployment of commit `2ae097c`. These checks verify the specific
frontend behavior only; they do not establish production API, identity,
database, or infrastructure readiness.

The current source and previous verification records show separate Node.js
demo and Go service paths. Historical reports dated 2026-10-31 contain
unsubstantiated completion language and outdated absolute paths; for example,
they claim comprehensive feature completion, hardware/HSM verification, and
staging readiness. Those statements conflict with the active
`MASTER_REQUIREMENT_TRACKER.md` and are not accepted as current evidence.

## Open baseline work

1. Push and verify the gateway formatting corrections and full-history secret
   scan configuration; investigate any history-only findings without exposing
   secret values or rewriting history without authorization.
2. Reconcile stale reports against present source and reproducible evidence,
   retaining historical reports but clearly marking unsupported claims.
3. Inventory the 319 remaining AppleDouble paths and determine which are
   metadata artifacts before proposing any cleanup.
4. Complete module dependency, API-to-UI, migration-to-runtime, and trust-boundary
   inventories.
5. Re-run Go, PostgreSQL, container, integration, and security checks in
   environments that provide the required toolchains and isolated dependencies.
