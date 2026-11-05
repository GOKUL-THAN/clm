# Database Verification Report

**Execution date:** 2026-11-05
**Database engine:** PostgreSQL 18.6
**Database:** Disposable local scratch database; not the demo/application database
**Status:** Live SQL and Go repository verification passed on PostgreSQL 18.6; supported-version CI remains unverified.

## Scope and setup

- Applied the ten numbered schema migrations `001_initial.sql` through
  `010_certificates_rls.sql` in filename order, excluding the container-only
  `000_keycloak_db.sql`.
- Created the scratch database with `clm_table_owner` as owner and applied
  migrations under `SET ROLE clm_table_owner`.
- Ran the SQL suites with `ON_ERROR_STOP=1`.
- Executed the application-role suite using a temporary password-authenticated
  `clm_app_role` connection. The table-owner suite used `SET ROLE
  clm_table_owner` from the local PostgreSQL administrator connection.
- Go repository tests were run with `CLM_TEST_DB_REQUIRED=1` and a live
  app-role DSN. The configured-credential `NewDB` connection test passed.
- This verifies repository-level database access, not a deployed application's
  runtime behavior.
- Confirmed `approval_requests`, `approval_rules`, and `certificates` each
  reported both `relrowsecurity=true` and `relforcerowsecurity=true`.

## Results

| Check | Result | Evidence |
|---|---|---|
| Ordered migrations 001–010 | PASS | All ten migration commands completed on the clean scratch DB |
| Application-role live RLS suite | PASS | Connected directly as `clm_app_role`; role reported `rolsuper=false`, `rolbypassrls=false`; tenant reads, cross-tenant denial, no-context denial, writes, and audit checks completed |
| Table-owner FORCE RLS suite | PASS | Table owner reported `rolsuper=false`, `rolbypassrls=false`; no-context, positive-control, cross-tenant read and write checks completed |
| Superuser negative control | PASS | Confirmed the documented PostgreSQL behavior that a superuser bypasses RLS |
| Go PostgreSQL repository tests | PASS | SaaS Go module `go test ./... -count=1` with required live PostgreSQL app-role DSN |
| PostgreSQL 15 CI service | NOT RUN | Local test used PostgreSQL 18.6; CI has separate PostgreSQL 15 configuration |
| Configured application DB connection | PASS | `NewDB` connected using the configured app-role credentials; this was a Go test, not a deployed service |

## Conclusions and limits

The migration syntax and SQL-level RLS behavior for approvals and certificates
were verified on PostgreSQL 18.6. The SaaS Go module compiled, passed its test
suite with a required live app-role DSN, and connected through the configured
`NewDB` path. This does not establish that the same behavior has passed on
PostgreSQL 15 or in a deployed service. It is not a production, staging,
compliance, or independent security certification.

The disposable database `clm_verify_20261105_001` and temporary `clm_app_role`
were removed and absence was verified. The pre-existing `clm_table_owner` role
was left untouched. See `EXECUTION_CHECKPOINT.md` and
`MASTER_REQUIREMENT_TRACKER.md` for remaining gates.
