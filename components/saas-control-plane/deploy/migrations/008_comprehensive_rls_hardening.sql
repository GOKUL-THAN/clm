-- ==============================================================================
-- 008_comprehensive_rls_hardening.sql
-- Enterprise CLM Comprehensive Row-Level Security & Privilege Hardening:
-- 1. Enforces ENABLE and FORCE ROW LEVEL SECURITY across all secondary tenant tables
-- 2. Enforces RLS on contract child tables via relational JOIN to contracts
-- 3. Hardens SECURITY DEFINER function execute privileges (revokes from PUBLIC)
-- 4. Grants explicit execution permissions strictly to clm_app_role & clm_table_owner
-- ==============================================================================

-- ------------------------------------------------------------------------------
-- 1. DIRECT TENANT-ID TABLES: ENABLE AND FORCE ROW-LEVEL SECURITY
-- ------------------------------------------------------------------------------

-- Delegation of Authority Rules
ALTER TABLE delegation_of_authority_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE delegation_of_authority_rules FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_doa_rules ON delegation_of_authority_rules;
CREATE POLICY tenant_isolation_doa_rules ON delegation_of_authority_rules
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- Contract Approvals
ALTER TABLE contract_approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_approvals FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_contract_approvals ON contract_approvals;
CREATE POLICY tenant_isolation_contract_approvals ON contract_approvals
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- Procurement Purchase Orders
ALTER TABLE procurement_purchase_orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE procurement_purchase_orders FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_procurement_pos ON procurement_purchase_orders;
CREATE POLICY tenant_isolation_procurement_pos ON procurement_purchase_orders
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- Procurement Goods Receipts
ALTER TABLE procurement_goods_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE procurement_goods_receipts FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_goods_receipts ON procurement_goods_receipts;
CREATE POLICY tenant_isolation_goods_receipts ON procurement_goods_receipts
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- Procurement Supplier Invoices
ALTER TABLE procurement_supplier_invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE procurement_supplier_invoices FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_supplier_invoices ON procurement_supplier_invoices;
CREATE POLICY tenant_isolation_supplier_invoices ON procurement_supplier_invoices
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- Procurement Three-Way Matches
ALTER TABLE procurement_three_way_matches ENABLE ROW LEVEL SECURITY;
ALTER TABLE procurement_three_way_matches FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_three_way_matches ON procurement_three_way_matches;
CREATE POLICY tenant_isolation_three_way_matches ON procurement_three_way_matches
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- Satellite Enclave Keys
ALTER TABLE satellite_enclave_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE satellite_enclave_keys FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_satellite_enclave_keys ON satellite_enclave_keys;
CREATE POLICY tenant_isolation_satellite_enclave_keys ON satellite_enclave_keys
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- ------------------------------------------------------------------------------
-- 2. CHILD TABLES LINKED VIA contract_id: ENABLE AND FORCE ROW-LEVEL SECURITY
-- ------------------------------------------------------------------------------

-- Contract Versions
ALTER TABLE contract_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_versions FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_contract_versions ON contract_versions;
CREATE POLICY tenant_isolation_contract_versions ON contract_versions
    FOR ALL
    USING (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_versions.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ))
    WITH CHECK (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_versions.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ));

-- Contract Negotiations
ALTER TABLE contract_negotiations ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_negotiations FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_contract_negotiations ON contract_negotiations;
CREATE POLICY tenant_isolation_contract_negotiations ON contract_negotiations
    FOR ALL
    USING (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_negotiations.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ))
    WITH CHECK (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_negotiations.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ));

-- Contract Redlines
ALTER TABLE contract_redlines ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_redlines FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_contract_redlines ON contract_redlines;
CREATE POLICY tenant_isolation_contract_redlines ON contract_redlines
    FOR ALL
    USING (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_redlines.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ))
    WITH CHECK (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_redlines.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ));

-- Contract Signatures
ALTER TABLE contract_signatures ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_signatures FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_contract_signatures ON contract_signatures;
CREATE POLICY tenant_isolation_contract_signatures ON contract_signatures
    FOR ALL
    USING (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_signatures.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ))
    WITH CHECK (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_signatures.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ));

-- Contract Obligations
ALTER TABLE contract_obligations ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_obligations FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_contract_obligations ON contract_obligations;
CREATE POLICY tenant_isolation_contract_obligations ON contract_obligations
    FOR ALL
    USING (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_obligations.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ))
    WITH CHECK (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_obligations.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ));

-- Contract Risks
ALTER TABLE contract_risks ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_risks FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_contract_risks ON contract_risks;
CREATE POLICY tenant_isolation_contract_risks ON contract_risks
    FOR ALL
    USING (contract_id IS NULL OR EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_risks.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ))
    WITH CHECK (contract_id IS NULL OR EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_risks.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ));

-- Contract Cryptographic Envelopes
ALTER TABLE contract_cryptographic_envelopes ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_cryptographic_envelopes FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_cryptographic_envelopes ON contract_cryptographic_envelopes;
CREATE POLICY tenant_isolation_cryptographic_envelopes ON contract_cryptographic_envelopes
    FOR ALL
    USING (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_cryptographic_envelopes.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ))
    WITH CHECK (EXISTS (
        SELECT 1 FROM contracts c 
        WHERE c.id = contract_cryptographic_envelopes.contract_id 
        AND c.tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    ));

-- ------------------------------------------------------------------------------
-- 3. HARDEN SECURITY DEFINER FUNCTION EXECUTION PRIVILEGES
-- ------------------------------------------------------------------------------
REVOKE EXECUTE ON FUNCTION reject_contract_audit_truncate() FROM PUBLIC;
REVOKE EXECUTE ON FUNCTION reject_contract_audit_mutation() FROM PUBLIC;
REVOKE EXECUTE ON FUNCTION compute_contract_audit_hash_chain() FROM PUBLIC;

DO $$
BEGIN
    -- Least privilege enforcement: Trigger functions are executed by PostgreSQL engine
    -- on behalf of table ownership. Application roles do not require direct EXECUTE
    -- privileges, preventing direct execution outside of trigger context.
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'clm_app_role') THEN
        REVOKE EXECUTE ON FUNCTION reject_contract_audit_truncate() FROM clm_app_role;
        REVOKE EXECUTE ON FUNCTION reject_contract_audit_mutation() FROM clm_app_role;
        REVOKE EXECUTE ON FUNCTION compute_contract_audit_hash_chain() FROM clm_app_role;
    END IF;
    GRANT EXECUTE ON FUNCTION reject_contract_audit_truncate() TO clm_table_owner;
    GRANT EXECUTE ON FUNCTION reject_contract_audit_mutation() TO clm_table_owner;
    GRANT EXECUTE ON FUNCTION compute_contract_audit_hash_chain() TO clm_table_owner;
END $$;

