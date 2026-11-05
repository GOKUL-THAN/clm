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

#### Approval and certificate coverage

The approval API scopes detail reads and decision mutations with the authenticated tenant ID in repository queries. Tenant selection for list and create operations is also checked with `RequireTenantAccess`; cross-tenant object IDs return not found. Migration `009_approval_requests_rls.sql` enables and forces RLS on both `approval_requests` and `approval_rules`.

Certificate operations likewise establish tenant context transaction-locally. Migration `010_certificates_rls.sql` enables and forces RLS on `certificates`. The tenant-scoped repository list rejects a nil tenant; only trusted monitoring and renewal workers use the explicit all-tenant scan, which enumerates tenant IDs and queries each tenant in a separate transaction-local context. CI is configured to check the repositories as `clm_app_role`, and SQL suites verify application-role and table-owner visibility boundaries. Live database execution remains required evidence; a passing application mock suite is not proof of RLS behavior.

### Satellite filesystem destinations

The satellite gateway and each vSatellite agent apply a default-deny filesystem policy from `file_storage.allowed_roots` and `file_storage.tenant_roots` in their respective YAML configurations. Roots must already exist, be absolute and canonical, and not group/world writable; the processes never create configured roots. A configured tenant root replaces the gateway-wide roots for that tenant. File deployments require a signed `tenant_id` claim; a caller-supplied path is accepted only when it resolves beneath that tenant's approved roots. Private keys are stored with mode `0600`, and file operations use rooted directory handles to prevent path traversal and symlink escapes. Filesystem ownership and POSIX mode enforcement is supported on Linux; other platforms fail closed at startup until equivalent ACL verification is implemented.

Replacing an existing local certificate or key requires `replace_existing: true` and an authenticated fleet-management role; the API computes the deployer's replacement capability and does not deserialize it from client JSON. The authorization is audited. Production operators must provision and protect the configured roots before startup. The AWS secrets provider uses AWS Secrets Manager and the configured `kms_key_id`; unavailable AWS operations return errors rather than success-shaped references.
