# CLM Bug Register

| ID | Severity | File | Root cause | Impact | Fix | Verification |
|---|---|---|---|---|---|---|
| CLM-001 | HIGH | `internal/saas/sso/oidc.go:172-221` | OIDC issuer/discovered endpoints were not destination-restricted and token errors leaked upstream text. | SSRF and client-secret disclosure. | HTTPS/public-address validation, redirect blocking, endpoint validation and bounded/error-redacted responses. | Staged Go tests, SSRF unit tests. |
| CLM-002 | HIGH | `internal/saas/sso/saml.go:82-115` | SAML callback trusted status/XML without signature or conditions. | Identity forgery and token issuance. | Require IdP certificate, XML signature validation and ACS/audience/confirmation checks; single-use state. | Staged Go tests and unsigned-SAML regression test. |
| CLM-003 | HIGH | `internal/saas/sso/service.go:206-251` | SSO default role copied without tenant-role allowlist. | Tenant-controlled identities could receive platform roles. | Reject `super_admin` and `admin` for SSO auto-provisioning. | Role allowlist regression test. |
| CLM-004 | MEDIUM | `internal/api/rest/handlers/sso.go:453-464` | SSO state contained tenant UUID plus nonce but nonce was never registered or consumed. | Replay/callback misuse. | Server-side 10-minute single-use state store. | Single-use state regression test. |
| CLM-005 | MEDIUM | `components/satellite-gateway/internal/api/inventory_api.go:212-257` | Inventory handler did not filter by tenant. | Cross-tenant certificate metadata disclosure. | Filter certificates and summary by caller tenant; preserve platform-admin scope. | Tenant inventory unit test. |
| CLM-006 | MEDIUM | `internal/api/rest/handlers/enterprise_operations.go:77-95` | Incident resolution used only incident UUID. | Cross-tenant incident modification. | Require tenant context and tenant-scoped service lookup. | Cross-tenant ITSM regression test. |
| CLM-007 | P2 | Go toolchain pins | CI, Docker and modules used vulnerable Go 1.27.3/old CI versions. | GO-2026-6859 HTTP/1 crash exposure. | Pinned CI/Docker to Go 1.27.4 and modules to Go 1.26.11 minimum. | `govulncheck`: no vulnerabilities. |

