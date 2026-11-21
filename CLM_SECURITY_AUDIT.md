# CLM Security Audit

## High-risk fixes

1. **SAML signature and assertion validation:** IdP certificates are mandatory. Response or assertion signatures must validate using the configured certificate; ACS destination, audience and subject-confirmation conditions are checked.
2. **OIDC SSRF protection:** Issuer, authorization, token and userinfo endpoints must use HTTPS and resolve only to public addresses. Redirects are blocked and upstream error bodies are redacted.
3. **SSO role restriction:** Tenant SSO auto-provisioning cannot assign `super_admin` or `admin`.
4. **SSO state binding:** State is server-side, tenant-bound, expires after 10 minutes and is atomically consumed once.
5. **Tenant inventory isolation:** Gateway inventory and detail access enforce tenant ownership unless a platform-admin role is explicit.
6. **Incident isolation:** ITSM resolution now requires a tenant context and verifies incident ownership.

## Static findings

- Gosec staged control plane: 0 issues across 88 files.
- Gosec staged satellite gateway: 0 issues across 77 files.
- Gosec staged vSatellite: 0 issues across 26 files.
- `govulncheck`: no vulnerabilities in all three staged modules on Go 1.27.4.

## Not verified

- Deployed identity-provider behavior and certificate trust.
- Live database role architecture and RLS.
- Production secrets, TLS/mTLS and external CA credentials.
- Cloud IAM and deployed private-key storage.

