# CLM-SPEC-21: DATABASE ARCHITECTURE SPECIFICATION
## PostgreSQL 15+ Relational Schema, Multi-Tenant Isolation, Four-Tier Roles, and Immutable Audit Engine

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-DB-021  
**Classification:** Enterprise Database Architecture Standard  
**Release Classification:** STAGING CANDIDATE — LIVE INFRASTRUCTURE VERIFICATION PENDING  

---

### 1. Database Schema Evolution & Migrations

The CLM database schema is managed via forward-only, idempotent SQL migrations in `components/saas-control-plane/deploy/migrations/`:

| Migration File | Primary Responsibility | Key Tables Created / Modified |
| :--- | :--- | :--- |
| `000_keycloak_db.sql` | Keycloak IAM Database | Keycloak internal realm and user credentials tables. |
| `001_initial.sql` | Core Platform Tenancy | `tenants`, `users`, `certificates`, `purchases`. |
| `002_sso_fields.sql` | Enterprise SSO Federation | `sso_providers`, SAML/OIDC metadata bindings. |
| `003_tenant_security.sql` | Immutable Audits & Encryption | `control_plane_audit_events`, `cloud_accounts` (AES-GCM). |
| `004_secure_command_transport.sql` | Satellite Command Transport | `satellite_commands`, `satellite_nodes`. |
| `005_contract_lifecycle_management.sql` | Core CLM Domain Models | `contracts`, `contract_versions`, `contract_requests`, `contract_templates`, `clause_library`, `contract_negotiations`, `contract_redlines`, `contract_signatures`, `contract_obligations`, `contract_risks`, `procurement_suppliers`, `legal_matters`, `contract_financials`, `ai_contract_analyses`, `contract_audit_logs`. |
| `006_contract_security_and_procurement_rls.sql` | Security & Procurement Hardening | Row-Level Security on core tables; initial WORM triggers; `procurement_purchase_orders`, `procurement_goods_receipts`, `procurement_supplier_invoices`, `procurement_three_way_matches`, `delegation_of_authority_rules`, `contract_approvals`, `satellite_enclave_keys`. |
| `007_immutable_audit_hash_chaining_and_truncate_protection.sql` | Concurrency-Safe WORM Ledger | `tenant_audit_chain_heads`, `SELECT ... FOR UPDATE` row-locking, `sequence_num BIGINT`, `clock_timestamp()` sequencing, non-bypassable `BEFORE UPDATE OR DELETE` and `BEFORE TRUNCATE` triggers, `SECURITY DEFINER SET search_path = pg_catalog`, schema-qualified references `public.tenant_audit_chain_heads`. |
| `008_comprehensive_rls_hardening.sql` | Comprehensive RLS & Privilege Lockdown | `FORCE ROW LEVEL SECURITY` across all primary and child tables (DOA rules, approvals, POs, goods receipts, invoices, matches, satellite keys, contract versions/redlines/signatures/obligations); security definer execution privilege revocation from PUBLIC. |

---

### 2. Four-Tier Security Role Architecture

The database enforces strict least-privilege role separation:
* **`clm_table_owner`:** Schema DDL owner; tables are configured with `FORCE ROW LEVEL SECURITY`.
* **`clm_migration_owner`:** CI/CD role executing migration scripts, operating under `SET ROLE clm_table_owner`.
* **`clm_app_role`:** Application runtime connection pool role; possesses only DML (`SELECT`, `INSERT`, `UPDATE`, `DELETE`) with no DDL privileges, no bypass privileges, and strictly forbidden from updating/deleting/truncating audit tables.
* **`postgres`:** Database cluster administrative superuser reserved for initial setup and infrastructure management.

---

### 3. Multi-Tenant Row-Level Security (RLS) & Connection Pooling

All application tables enforce PostgreSQL Row-Level Security:
```sql
ALTER TABLE contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE contracts FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_policy ON contracts
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);
```
Every application connection executes queries within an explicit transaction using `SET LOCAL`:
```sql
BEGIN;
SELECT set_config('app.current_tenant_id', 'tenant-uuid', true);
-- Transaction queries executed here
COMMIT; -- Discards setting automatically upon connection checkin
```

---

### 4. Immutable Audit Trail (Migration 007)

Audit records written to `contract_audit_logs` are cryptographically chained and protected against concurrency issues:
1. **Chain Heads Table:** `tenant_audit_chain_heads` maintains the latest `current_record_hash` and monotonic `sequence_num` per tenant.
2. **Row-Level Locking:** During audit insertion, the trigger executes `pg_advisory_xact_lock` and `SELECT ... FOR UPDATE` on `tenant_audit_chain_heads`, eliminating race-condition forks.
3. **Monotonic Ordering:** Enforces `clock_timestamp()` instead of `NOW()` to prevent timestamp inversion.
4. **Trigger Protection:** Non-bypassable triggers reject any `UPDATE`, `DELETE`, or `TRUNCATE` operations on `contract_audit_logs`.

---

### 5. Indexing & Query Optimization

1. **Multi-Tenant Indexing:** Composite B-tree indexes on `(tenant_id, created_at DESC)` and `(tenant_id, status)` ensure single-tenant partition scans.
2. **JSONB GIN Indexing:** Metadata attributes (`metadata JSONB`, `tags JSONB`, `form_data JSONB`) use Generalized Inverted Indexes (`GIN`) to support arbitrary sub-field key/value lookups:
   ```sql
   CREATE INDEX idx_contracts_metadata_gin ON contracts USING gin(metadata);
   ```
3. **Expiration & SLA Indexes:** `expiration_date` and `due_date` are indexed to power high-speed notification background workers scanning for contracts expiring within 30/60/90 days.
