# TENANT ISOLATION & ACCESS CONTROL VERIFICATION

**Document Reference:** SEC-TIV-2026-FINAL  
**Lead Auditor:** Principal Database Security Architect & IAM Specialist  
**Standard:** OWASP ASVS v4.0.3 (V4 Access Control), NIST SP 800-53 Rev. 5 (AC-3, SC-4), ISO/IEC 27017  
**Date of Audit:** 2026-10-31  

---

## 1. ARCHITECTURAL TENANT ISOLATION MODEL

The Enterprise CLM implements a defense-in-depth, three-tier tenant isolation boundary:

```mermaid
graph TD
    subgraph "Tier 1: API & Gateway Boundary"
        Token["Keycloak / OIDC Bearer Token"] --> MW["AuthMiddleware + GetTenantID(c)"]
        MW --> AuthCheck{"Token TenantID == Target TenantID?"}
        AuthCheck -->|No| Abort403["403 Forbidden: Cross-Tenant Denied"]
        AuthCheck -->|Yes| Handler["Execute Handler with Scoped UUID"]
    end
    subgraph "Tier 2: Transaction Session Context"
        Handler --> Pool["PostgreSQL Connection Pool"]
        Pool --> Tx["Begin Transaction"]
        Tx --> SetConfig["SELECT set_config('app.current_tenant_id', $1, true)"]
    end
    subgraph "Tier 3: Database Engine (Row-Level Security)"
        SetConfig --> Table["Table Query (contracts, etc.)"]
        Table --> RLS{"PostgreSQL FORCE RLS Policy"}
        RLS -->|tenant_id == current_setting| AuthorizedRows["Tenant Rows Returned"]
        RLS -->|mismatch or unauthenticated| ZeroRows["0 Rows / Access Denied"]
    end
```

---

## 2. INDEPENDENT VERIFICATION OF MISSION 1 MANDATES

### 1. Removal of Hardcoded Fallback Tenant Identities
* **Inspection Target:** `components/saas-control-plane/internal/api/rest/handlers/contracts.go`.
* **Previous Implementation:** Line 621 returned `uuid.MustParse("a0000000-0000-0000-0000-000000000001")` when caller tenant context was missing.
* **Remediation Executed:** Deleted fallback UUID entirely. If `callerID == uuid.Nil`, the handler calls:
  ```go
  c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated tenant context"})
  return uuid.Nil
  ```
* **Verification Proof:** `independent_adversarial_test_suite.js` ATK-012 confirmed zero fallback identities in Go codebase.

### 2. Rejection of Missing Tenant Authentication
* **Verification Proof:** Unauthenticated requests attempting to mutate contracts (`POST /api/v1/contracts`) without an `Authorization` header are rejected with `401 Unauthorized` (verified in ATK-003).

### 3. Verification of Authenticated Tenant Identity & Parameter Tampering
* **Attack Scenario:** Attacker authenticated under Tenant A attempts to supply `?tenant_id=tenant-enterprise-uat` (Tenant B).
* **Enforcement:** `middleware.RequireTenantAccess(c, target)` verifies whether the caller token's tenant claim matches the requested parameter. If mismatched and caller lacks `super_admin`, the request is aborted with `403 Forbidden`.
* **Verification Proof:** Adversarial test ATK-002 confirmed parameter tampering is blocked (`403 Forbidden`).

### 4. PostgreSQL FORCE ROW LEVEL SECURITY
* **Attack Scenario:** Application connects to PostgreSQL as the database user that created the tables (`clm_user`).
* **Technical Reality:** Under standard PostgreSQL behavior, table owners bypass RLS policies even if `ENABLE ROW LEVEL SECURITY` is set.
* **Enforcement:** Added `ALTER TABLE <table_name> FORCE ROW LEVEL SECURITY;` across all tables in `006_contract_security_and_procurement_rls.sql`. This forces PostgreSQL to evaluate policies even for the table owner.
* **Verification Proof:** Adversarial test ATK-010 verified `FORCE ROW LEVEL SECURITY` across all 7 core tables.

### 5. Table-Owner & BYPASSRLS Privileges Analysis
* **PostgreSQL Privilege Hierarchy:**
  1. **Regular Roles:** Subject to RLS policies.
  2. **Table Owners:** Subject to RLS **only if** `FORCE ROW LEVEL SECURITY` is enabled.
  3. **Superusers & Roles with `BYPASSRLS`:** Always exempt from RLS policies.
* **Production Deployment Hardening Rule:** The application connection pool user (`clm_user`) must **never** be granted `SUPERUSER` or `BYPASSRLS`. The migration user and application runtime user should be separated in production environments.

### 6. Connection-Pool Transaction Isolation & `set_config()`
* **Leakage Threat:** When database connections are returned to a pool (e.g. `pgx`, `database/sql`, PgBouncer), session-level variables (`SET app.current_tenant_id = ...`) persist on the connection and can leak to the next tenant's query!
* **Remediation Executed:** Implemented parameterized transaction-local `set_config` in `components/saas-control-plane/internal/store/postgres/db.go`:
  ```go
  var setVal string
  err = tx.QueryRowContext(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", tenantID.String()).Scan(&setVal)
  ```
* **Guaranteed Isolation:** Setting the third parameter (`is_local = true`) guarantees that the configuration setting applies strictly to the current transaction. When `tx.Commit()` or `tx.Rollback()` is called, PostgreSQL automatically reverts the setting, guaranteeing **zero connection-pool cross-tenant leakage**.

---

## 3. VERIFICATION VERDICT

The tenant isolation subsystem satisfies all technical, architectural, and adversarial criteria:
* **Zero Hardcoded Fallbacks**
* **Strict Rejection of Missing Tenant Context**
* **Zero-Leak Connection-Pool Transaction Scoping**
* **PostgreSQL FORCE ROW LEVEL SECURITY Enforced**

**Status:** **INDEPENDENTLY VERIFIED & HARDENED**.

