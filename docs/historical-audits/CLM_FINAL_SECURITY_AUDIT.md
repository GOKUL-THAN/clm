# CLM Final Security Audit

**Review date:** 2026-11-06  
**Repository:** `GOKUL-THAN/clm`  
**Branch / HEAD:** `audit/phase0-ci-workflow` / `360f3b95d64d60fcf07e17ae4f429a6b533f93ad`  
**Assessment type:** Partial, evidence-bounded repository and CI review  
**Decision:** Full security audit incomplete; production readiness not established

## Scope and evidence

This is not a certification, penetration-test report, or production audit. The
current worktree contains pre-existing user changes. No files were staged,
committed, pushed, or deployed by this assessment. GitHub Actions run
[42257224350](https://github.com/GOKUL-THAN/clm/actions/runs/42257224350)
completed successfully on the exact HEAD listed above with nine successful
jobs. The run did not include worktree-only changes.

The six locally rerun Node harnesses passed, but each proves only its
script-defined behavior. In particular, `test:400` is a registry-wrapper
smoke test. The requested broad source, runtime, integration, and operational
review is not complete. A report of an untested domain as `NOT_VERIFIED` or
`BLOCKED` is not a finding that its implementation is absent.

## Evidence-state definitions

These states are distinct:

- `VERIFIED_IN_SOURCE`: code/configuration was directly inspected; behavior is
  not thereby verified.
- `VERIFIED_BY_UNIT_TEST`: a unit test exercised the behavior.
- `VERIFIED_BY_INTEGRATION_TEST`: a relevant integration test executed.
- `VERIFIED_IN_CI`: the specified check passed in the referenced CI run and
  exact commit.
- `VERIFIED_IN_STAGING` / `VERIFIED_IN_PRODUCTION`: the specified deployed
  environment was exercised.
- `INDEPENDENTLY_ASSESSED`: an independent assessment covered the stated scope;
  this does not imply certification.
- `CERTIFIED`: an applicable formal certification was evidenced.
- `NOT_VERIFIED`, `BLOCKED`, and `NOT_APPLICABLE`: the claim was not
  established, the required evidence/environment was unavailable, or the
  requirement does not apply.

## Domain assessment

| Domain | Evidence reviewed | Current status | Material gaps |
|---|---|---|---|
| Repository, branch, and audit history | Current Git branch, HEAD, staged/unstaged/untracked state and requested audit documents inspected | `VERIFIED_IN_SOURCE` (state only) | Full repository-wide source and report reconciliation remains incomplete |
| User identity and tenant isolation | Existing PostgreSQL 15 selected app-role and certificate/approval tests passed in CI; a new user-repository cross-tenant test exists in the worktree | `BLOCKED` for full boundary | Worktree test was not in CI and was not run locally; complete user routes, role changes, tenant forgery, concurrency, and pool-reuse scenarios remain unverified |
| Identity role authorization | Identity handlers previously forwarded client-selected Keycloak roles without applying the existing actor privilege ceiling. Checks are now added to direct user/group role assignment and group membership paths in both source trees. | `VERIFIED_IN_SOURCE`; regression test `NOT_VERIFIED` | Go tests could not run locally; latest CI predates the fix. Live Keycloak token-role behavior is untested |
| JWT, OIDC, SSO, sessions | Configured gateway JWT checks are part of CI; no live identity provider was exercised | `BLOCKED` | Full entry-point inventory, keys/JWKS rotation, session revocation, MFA, cookies/CSRF, rate limits, and negative token cases |
| PostgreSQL RLS and database roles | PostgreSQL 15 CI applied migrations and exercised selected checks as `clm_app_role`, `clm_table_owner`, and `postgres` | `VERIFIED_IN_CI` for selected checks only | Not all tenant tables, repositories, joins, permissions, role grants, or deployed DB settings were reviewed |
| Migrations | CI applied repository migrations in PostgreSQL 15 | `VERIFIED_IN_CI` for this path | Upgrade/rollback from every supported version and production backup restoration not tested |
| Satellite gateway and vSatellite | Configured Go checks and image scan passed in CI | `VERIFIED_IN_CI` for configured checks only | mTLS lifecycle, enrollment/revocation, replay, tenant binding, command authorization, recovery, and deployment not exercised end-to-end |
| Contract lifecycle and workflow authorization | Local Node harnesses reported 26 lifecycle and three workflow checks passed | `VERIFIED_BY_UNIT_TEST` for harness scope only | Persisted backend state machine, unauthorized transitions, races, retries, stale versions, and signature invalidation |
| Certificate lifecycle | Selected certificate repository/RLS test passed in CI | `VERIFIED_IN_CI` for selected repository behavior | No real CA, issuance, renewal, revocation propagation, deployment rollback, or provider-specific integration |
| Cryptography and signatures | In-process Node suites reported signature verification and tamper checks | `VERIFIED_BY_UNIT_TEST` for those cases only | No HSM/KMS, private-key custody, FIPS module validation, production certificate chain, or trusted timestamp service |
| AI and retrieval | Local harness reported selected prompt-injection cases passed | `VERIFIED_BY_UNIT_TEST` for harness scope only | Cross-tenant retrieval, model/network egress, tools, adversarial corpus, logging, citations, and provider runtime |
| HTTP integrations, webhooks, and SSRF | Historical reports make claims; current review did not replay the full SSRF/DNS/replay corpus | `NOT_VERIFIED` | DNS rebinding, redirect, internal DNS, IPv6 variants, HMAC/timestamps, network egress controls, and provider integrations |
| Immutable audit | CI executed selected PostgreSQL mutation protections and negative controls | `VERIFIED_IN_CI` for selected DB behavior | External Object Lock, retention, legal hold, export verification, privileged bypass, and recovery |
| Docker, Kubernetes, Terraform, CI | Nine configured CI jobs passed, including images, scans, infrastructure validation, and Kubernetes policy assertions | `VERIFIED_IN_CI` for configured checks only | No cloud plan/apply, live cluster/CNI, IAM review, branch protection, production deployment gate, or image-signing verification |
| Secrets, SBOM, provenance | Secret scan and SBOM jobs passed; prior SBOM attestation verification is recorded separately | `VERIFIED_IN_CI` for configured checks | Current run artifact provenance was not independently verified here; secret rotation and runtime consumption not exercised |
| Backup, disaster recovery, performance, monitoring | No restore, DR exercise, load test, live alert, or production telemetry was reproduced | `BLOCKED` | Measured RPO/RTO, restore integrity, throughput/latency, alert delivery, and incident-response exercise |
| Frontend workflows | Browser snapshots are not full end-to-end tests; this assessment did not reproduce complete workflows | `BLOCKED` | Authentication, CRUD, authorization errors, accessibility, and deployed backend integration |
| 400 feature claims | The registry smoke harness checks 400 entries/wrappers; the registry invokes domain simulations rather than each feature module's `execute` method | `BLOCKED` for feature implementation | The harness establishes no individually verified business capability; per-feature behavior, persistence, authorization, UI, integration, and acceptance remain unmapped |
| Standards and compliance | Existing mappings were inspected; no full control-by-control assessment conducted | `NOT_VERIFIED` | Applicability, authoritative control mapping, organizational evidence, auditor, and formal certification |

## Findings and risk disposition

No comprehensive vulnerability conclusion is made. Historical “resolved”
labels and completion percentages are not accepted as current evidence. The
current actionable gaps and remediation status are maintained in
[CLM_REMEDIATION_TRACKER.md](./CLM_REMEDIATION_TRACKER.md). Any bounded source
review result must be read only within its explicitly stated scope.

An independent diff-level platform review inspected the changed satellite
gateway authentication/installer/secrets files, Kubernetes manifests,
AWS/Azure/GCP Terraform, CI workflow and selected build scripts. It found no
actionable concerns in that bounded changed-file scope. That review explicitly
excluded unchanged identity/JWT and tenant-repository code, and did not execute
tests or assess live gateway, Kubernetes, cloud, AI, PKI, or audit behavior.
This limited result does not offset the identity role-escalation finding below
or establish whole-product security.

### Confirmed high-severity issue fixed in source

The bounded auth/tenant review identified that the identity administration
handlers did not enforce the privilege ceiling already implemented for local
user roles. An `admin` could request `super_admin` assignment via Keycloak user
role operations; user creation and group role/membership operations exposed
equivalent paths. This could elevate a user to the highest mapped platform role.
Both the tracked source and the pre-existing untracked `CLM/` copy now call the
shared role-assignment policy before the relevant Keycloak mutation, including
inherited group roles. Tracked entry points are `CreateUser` (line 102),
`SetUserRole` (line 350), `CreateGroup` (line 646), `SetGroupRole` (line 683),
and `AddUserToGroup` (line 714); the shared policy adapter is at line 370 and
the role-ceiling policy is `users.CanAssignRole` in
`components/saas-control-plane/internal/saas/users/service.go` (line 48).
Regression tests are `TestIdentityRoleHandlersRejectAdminSuperAdminAssignment`
(line 13), `TestIdentityRoleAssignmentAuthorization` (line 68), and
`TestIdentityGroupMembershipEnforcesInheritedRoleCeiling` (line 105) in
`identity_role_test.go`; the equivalent source/test changes exist in the
pre-existing nested tree. Attempts to run the focused Go tests in each module
were blocked because `go` is not installed/on PATH. No new CI run includes
these edits, so the fix is `VERIFIED_IN_SOURCE` only; tests remain
`NOT_VERIFIED`.

## Release recommendation

Do not promote to production and do not claim staging verification from these
results. Keep production satellite deployment disabled until the security,
identity, certificate, command authorization, and operational controls are
verified in the intended environment.
