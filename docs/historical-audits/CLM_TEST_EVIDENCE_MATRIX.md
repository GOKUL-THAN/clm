# CLM Test Evidence Matrix

**Assessment date:** 2026-11-10  
**Local repository HEAD:** `360f3b95d64d60fcf07e17ae4f429a6b533f93ad`  
**Latest successful CI:** [42257224350](https://github.com/GOKUL-THAN/clm/actions/runs/42257224350), same commit

## Evidence-state vocabulary

`VERIFIED_IN_SOURCE`, `VERIFIED_BY_UNIT_TEST`,
`VERIFIED_BY_INTEGRATION_TEST`, `VERIFIED_IN_CI`,
`VERIFIED_IN_STAGING`, `VERIFIED_IN_PRODUCTION`,
`INDEPENDENTLY_ASSESSED`, `CERTIFIED`, `NOT_VERIFIED`, `BLOCKED`, and
`NOT_APPLICABLE` are not interchangeable. The status in this matrix applies
only to the named command, source, environment, and commit.

## Local commands rerun

| Command | Result | What it establishes | What it does not establish | Evidence state |
|---|---|---|---|---|
| `npm test` | PASS — 40/40 scenarios | Script-level scenarios including selected tenant, self-approval, signature/tamper, AI guardrail, and audit-trigger assertions | Deployed API, real crypto provider, all auth paths, or database runtime | `VERIFIED_BY_UNIT_TEST` |
| `npm run test:lifecycle` | PASS — 26/26 stages | Local happy-path lifecycle harness | Rejection of every illegal transition, persistence, concurrency, or external provider behavior | `VERIFIED_BY_UNIT_TEST` |
| `npm run test:400` | PASS — 400/400 registry wrapper checks | Registry metadata/validation and registry-dispatch plumbing | 400 implemented business capabilities; the registry invokes domain simulations instead of each feature module's `execute` method | `VERIFIED_BY_UNIT_TEST` |
| `npm run test:corporate` | PASS — 10 suites | Local scenario harness checks | Live corporate systems, authorization, or data exchange | `VERIFIED_BY_UNIT_TEST` |
| `npm run test:workflows` | PASS — 3 modes | Test-mode workflow engine behavior | Durable backend workflow state or production authorization | `VERIFIED_BY_UNIT_TEST` |
| `npm run test:global` | PASS — 15/15 checks | In-process scenarios, including local signature checks | HSM/KMS, external trust services, provider behavior, or deployed key custody | `VERIFIED_BY_UNIT_TEST` |
| Go tests, vet, and server/CLI builds | PASS on a temporary staged SaaS copy on 2026-11-10 | Compile/test coverage for the SaaS module after the canonical module-path rewrite | Database-backed integration tests, PostgreSQL migration execution, provider behavior, or deployed services | `VERIFIED_BY_UNIT_TEST` |
| PostgreSQL migration/integration, container, and deployment validation | BLOCKED / not run | None in this run | No migration 011 execution or live infrastructure evidence | `BLOCKED` |

The scripts are declared in [`package.json`](./package.json#L9). Local run
outputs were reproduced on 2026-11-06; these results are from separate harnesses,
not a coverage percentage.

### 2026-11-10 continuation validation

The six declared root Node suites were rerun and passed: scenarios 40/40,
registry wrappers 400/400, corporate 10/10, lifecycle 26/26, workflows 3/3,
and global 15/15. The registry result remains wrapper/smoke evidence only.

The current SaaS control-plane worktree was copied to a temporary directory,
the canonical module path rewrite was applied, and `go test ./...`, `go vet ./...`,
and server/CLI builds passed. PostgreSQL integration tests were skipped:
`CLM_TEST_DB_DSN` was unset. Consequently, migration 011 and the newly added
catalog and cross-tenant database tests were not executed. These results do not
verify SQL execution or production behavior.

### Signup tenant-context follow-up

A source trace found that forced tenant/platform contexts had blocked the
public registration flow. Registration now uses a dedicated context that
permits tenant-row insertion only, and the initial tenant administrator is
created under the new tenant's context. The staged SaaS `go test ./...`, `go vet
./...`, and server/CLI builds passed after this fix. The PostgreSQL test for
tenant provisioning and duplicate-slug handling was compiled but skipped
because `CLM_TEST_DB_DSN` is unset; the migration policy was not executed
against PostgreSQL.

### Feature registry limitation

The read-only registry audit found that `features/registry.js` labels feature
evidence as `NOT_ASSESSED` / `NOT_VERIFIED` with no proof
([registry.js](./features/registry.js#L4016)); it loads modules
([registry.js](./features/registry.js#L4032)) but registry execution validates a
module then dispatches through `domainEngine.execute` rather than the
feature-specific `mod.execute` ([registry.js](./features/registry.js#L4046)).
The domain engine labels its output as smoke evidence and marks feature
acceptance/business capability verification false
([index.js](./services/domain-engine/index.js#L70)). The harness explicitly
warns that it does not verify business capability
([test_400_features.js](./scripts/test_400_features.js#L2)).

Therefore the evidenced count from this harness is **400 registry wrappers
checked and 0 individually verified business capabilities established by that
harness**. This does not imply separate services are unimplemented. The
requested per-feature mapping of business need, backend/API, data model,
authorization, UI, integration, and acceptance test has not been completed;
that detailed inventory remains `BLOCKED`, not inferred from filenames.

## CI evidence

`gh run view 42257224350 --repo GOKUL-THAN/clm --json status,conclusion,headSha,headBranch,jobs`
reported a successful run on
`360f3b95d64d60fcf07e17ae4f429a6b533f93ad`. The nine jobs and results were:

| CI job | Result | Evidence scope |
|---|---|---|
| Filesystem, IaC, and SBOM | PASS | Configured scan and SBOM workflow steps |
| Terraform formatting and validation | PASS | Formatting/provider validation; no cloud plan/apply |
| Go and dependencies — vSatellite | PASS | Configured module checks |
| Script and configuration syntax | PASS | Configured syntax checks only |
| Go and dependencies — satellite-gateway | PASS | Configured module checks |
| PostgreSQL 15 RLS and Immutable Audit Verification | PASS | Migrations and selected app-role/table-owner/superuser role tests |
| Container image scan | PASS | Configured image builds/scans and non-root assertions |
| Go and dependencies — saas-control-plane | PASS | Configured module checks |
| Secret scan | PASS | Configured repository scan |

PostgreSQL CI logs confirmed `clm_app_role` had `rolsuper=f` and
`rolbypassrls=f`; selected cross-tenant approval/certificate writes and writes
without tenant context were blocked. Table-owner cross-tenant certificate
write was blocked under FORCE RLS. Audit UPDATE/DELETE and TRUNCATE attempts
were blocked by configured protections. The superuser test is a negative
control showing PostgreSQL's expected superuser bypass, not proof that
superusers are constrained.

At the run's exact commit, the Go repository selector included approval and
certificate tests, but not the newer `TestUserRepositoryEnforcesTenantScope`.
That test and its selector remain absent from that historical CI run. The
current local staged run did execute the Go suite, but skipped its database
tests because no test DSN was configured. See
[`.github/workflows/ci.yml`](./.github/workflows/ci.yml#L247) and
[`repo_test.go`](./components/saas-control-plane/internal/store/postgres/repo_test.go#L91).

## Unexecuted requirements

| Requirement | Result | Missing evidence |
|---|---|---|
| Identity API role privilege ceiling | Unit tests PASS in the current staged Go suite | The suite ran handler regression tests locally; latest CI predates the fix and live identity-provider behavior remains untested. |
| Complete JWT/OIDC/SSO negative cases | BLOCKED | No full entry-point test sweep or live IdP |
| All tenant-owned Go repositories and user role changes | BLOCKED | Only selected repositories in CI; new user test unrun |
| Satellite enrollment, mTLS lifecycle, command authorization/replay | BLOCKED | No integrated gateway/vSatellite deployment test |
| Real CA issuance/renewal/revocation | BLOCKED | No real CA/provider execution |
| Tenant-isolated AI retrieval, data egress, tool authorization | BLOCKED | No provider/runtime retrieval test |
| Full SSRF/DNS rebinding and network egress corpus | BLOCKED | No current end-to-end replay |
| Browser end-to-end and accessibility | BLOCKED | Snapshots are not complete workflow tests |
| Backup restoration and disaster recovery | BLOCKED | No restore/failover exercise or measured RPO/RTO |
| Performance/load/concurrency/fuzz/race on full supported matrix | BLOCKED | No reproducible complete suite in this assessment |
| Each of the 400 feature business capabilities | BLOCKED | Registry audit established 400 wrappers, not 400 feature-specific business implementations; per-feature acceptance mapping absent |
| Staging/production runtime controls | BLOCKED | No deployment or production access |
| Formal certifications | BLOCKED | No accredited assessment/certificate evidence |
