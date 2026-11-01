# CLM-SPEC-05: IDENTITY AND ACCESS MANAGEMENT SPECIFICATION
## Keycloak Federation, OIDC, SAML 2.0, Multi-Factor Auth, and Enterprise RBAC

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-IAM-005  
**Classification:** Enterprise Identity Architecture Standard  

---

### 1. Identity Federation & Single Sign-On (SSO)

The platform integrates **Keycloak 24+** as its centralized identity and access federation layer:
* **OpenID Connect (OIDC):** Primary authentication protocol using Authorization Code Flow with PKCE (Proof Key for Code Exchange) for SPA frontends.
* **SAML 2.0:** Enterprise identity provider (IdP) integration with Microsoft Entra ID (Azure AD), Okta, PingFederate, and Google Workspace.
* **SCIM 2.0 (System for Cross-domain Identity Management):** Automated user lifecycle provisioning, attribute synchronization, and immediate offboarding.

---

### 2. Multi-Factor Authentication (MFA) Policy

All corporate users accessing non-public contract metadata or administrative functions must satisfy mandatory MFA:
* **FIDO2 / WebAuthn:** Hardware security keys (YubiKey) for cryptographic phishing-resistant authentication.
* **Time-Based One-Time Password (TOTP):** RFC 6238 mobile authenticators (Google Authenticator, Microsoft Authenticator) with 30-second token rotation.
* **Conditional Access:** Step-up MFA challenges are dynamically triggered upon anomalous login IP, location change, or contract actions exceeding \$100,000.

---

### 3. Role-Based Access Control (RBAC) Hierarchy

The platform defines granular enterprise roles aligned across corporate departments:

| Role Name | Description & Permissions | Target Corporate Department |
| :--- | :--- | :--- |
| `super_admin` | Global platform configuration, tenant provisioning, system audits. | IT / DevOps |
| `tenant_admin` | Tenant organization settings, user role assignment, SSO realm config. | Administration |
| `legal_counsel` | Authoring, clause playbook editing, redline negotiation, litigation holds. | Legal |
| `procurement_mgr` | Supplier onboarding, purchase agreements, 3-way match exceptions. | Procurement |
| `finance_controller`| Budget approval, payment milestone release, revenue leakage audits. | Finance |
| `deal_desk_lead` | Sales contract review, discounting threshold approval, order forms. | Sales |
| `hr_lead` | Employment agreements, offer letters, contractor NDAs, severance. | HR |
| `security_officer` | Data Processing Agreements (DPAs), SOC 2 reports, vendor risk audits. | Security |
| `auditor` | Read-only global visibility into contracts, audit logs, and compliance proof. | Compliance / Internal Audit |
| `external_signer` | Restricted counterparty portal for review, redlining, and e-signing. | External Counterparties |

