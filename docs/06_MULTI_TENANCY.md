# CLM-SPEC-06: MULTI-TENANCY & DATA ISOLATION SPECIFICATION
## PostgreSQL Row-Level Security (RLS), Logical Isolation, and Tenant Scoping

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-TENANT-006  
**Classification:** Enterprise Data Architecture Standard  

---

### 1. Multi-Tenant Architectural Model

The platform utilizes a **Shared Database, Shared Schema with Row-Level Security (RLS)** model:
* **Storage Efficiency:** Simplifies database migration, backup, and connection pooling across thousands of enterprise tenants.
* **Kernel-Enforced Security:** Tenant isolation is enforced at the database engine level via PostgreSQL Row-Level Security, rather than relying solely on application-level `WHERE` clauses.

---

### 2. PostgreSQL Row-Level Security (RLS) Implementation

Every table containing tenant-owned data includes a mandatory `tenant_id UUID NOT NULL` column referencing `tenants(id)`.

```sql
-- Enable RLS on core tables
ALTER TABLE contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_audit_logs ENABLE ROW LEVEL SECURITY;

-- Tenant Isolation Policy
CREATE POLICY tenant_isolation_contracts ON contracts
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);
```

#### Connection Lifecycle & Tenant Context Injection:
1. **JWT Verification:** The API Gateway extracts and cryptographically verifies the `tenant_id` claim from the incoming JWT bearer token.
2. **Session Variable Setting:** Prior to executing any queries in a database transaction, the Go service executes:
   ```sql
   SET LOCAL app.current_tenant_id = 'c4b8e21a-...';
   ```
3. **Database Engine Enforcement:** Even if an application bug omits `WHERE tenant_id = ...`, PostgreSQL filters out all rows belonging to other tenants.
4. **Transaction Termination:** Upon `COMMIT` or `ROLLBACK`, the session setting is cleared, preventing connection pool contamination.

---

### 3. Tenant Boundary Verification

Tenant isolation is verified continuously in CI/CD via automated attack scenarios:
* **Scenario C Verification:** A request authenticated under Tenant B attempting to access contract records belonging to Tenant A returns zero results, guaranteeing absence of cross-tenant leakage.

