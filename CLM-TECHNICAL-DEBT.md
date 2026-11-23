# CLM Technical Debt — Initial Baseline

**Audit date:** 2026-11-10  
**Scope:** Tracked repository at `D:\400 clm\CLM`

This list contains evidence-backed engineering and validation gaps identified during the initial architecture pass. It is not a complete code-quality or security assessment.

| ID | Priority | Debt | Evidence | Impact | Suggested disposition |
|---|---|---|---|---|---|
| TD-001 | High | Migration 011 and tenant isolation need database integration validation | The ordered runner embeds `deploy/migrations/001–011`. Migration 011 and cross-tenant catalog/data tests are present; staged Go tests, vet, and builds pass, but no `CLM_TEST_DB_DSN` was configured. | SQL application, legacy baseline adoption, upgrade compatibility, RLS, and audit-trigger behavior are not proven by this run. | Run fresh-database and representative legacy-database tests; verify policies, triggers, and cross-tenant isolation before rollout. |
| TD-002 | Medium | Feature catalog completion is not measured by current smoke checks | `scripts/test_400_features.js` asserts registry-level success while asserting capability verification and readiness are false. | A 400-entry count can be mistaken for 400 complete business features. | Preserve explicit status metadata and add requirement-linked behavioral acceptance tests. |
| TD-003 | Low | vSatellite README contradicts tracked source | README says “Not Implemented” and packages are empty; Go source files exist under `cmd/` and `internal/`. | Confusing implementation and release status. | Update docs only after canonical tests and protocol checks establish what is operational. |
| TD-004 | Medium | Local Go validation helper depends on Bash/WSL | Component Makefiles call `components/shared/scripts/go-staged.sh`; its launcher could not attach the configured WSL disk. A temporary PowerShell staging copy ran tests, vet, and builds successfully, but is not a maintained developer command. | Windows developers may be unable to use the documented component Makefile targets even though the Go code validated through an equivalent temporary staging workflow. | Verify the helper in CI and document or provide a supported Windows staging workflow if Windows is a supported environment. |
| TD-005 | Medium | Demo evidence and production evidence share a repository but are distinct execution paths | Root Node tests call `dev-server.js`; Go backend and satellite are separate modules. README notes that demo screens/fixtures are not production proof. | Test results can be misapplied to the production API or persistence layer. | Keep test reports labeled by runtime and add integration tests for the Go service path. |

## Change-control note

The worktree was already dirty before this audit. Existing user changes were not modified. The migration runner follow-up is documented in `CLM-ARCHITECTURE-AUDIT.md` and `CLM-IMPLEMENTATION-STATUS.md`; integration behavior remains unverified.
