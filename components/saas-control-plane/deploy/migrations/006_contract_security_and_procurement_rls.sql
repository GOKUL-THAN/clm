-- ==============================================================================
-- 006_contract_security_and_procurement_rls.sql
-- Enterprise CLM Security Hardening:
-- 1. Row-Level Security (RLS) & Multi-Tenant Isolation
-- 2. WORM (Write Once Read Many) Immutable Audit Trail
-- 3. Delegation of Authority (DOA) Multi-Tier Approvals & Separation of Duties
-- 4. Three-Way Matching (PO + Goods Receipt + Invoice) Engine
-- 5. Zero-Knowledge Cryptographic Envelope & In-VPC Key Custody
-- ==============================================================================

-- 1. ROW-LEVEL SECURITY ENFORCEMENT ON ALL CLM CORE TABLES
ALTER TABLE contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE clause_library ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE procurement_suppliers ENABLE ROW LEVEL SECURITY;
ALTER TABLE legal_matters ENABLE ROW LEVEL SECURITY;

-- Enforce RLS even for table owners to prevent privileged connection bypass
ALTER TABLE contracts FORCE ROW LEVEL SECURITY;
ALTER TABLE contract_requests FORCE ROW LEVEL SECURITY;
ALTER TABLE contract_templates FORCE ROW LEVEL SECURITY;
ALTER TABLE clause_library FORCE ROW LEVEL SECURITY;
ALTER TABLE contract_audit_logs FORCE ROW LEVEL SECURITY;
ALTER TABLE procurement_suppliers FORCE ROW LEVEL SECURITY;
ALTER TABLE legal_matters FORCE ROW LEVEL SECURITY;

-- Tenant Isolation Policies
DROP POLICY IF EXISTS tenant_isolation_contracts ON contracts;
CREATE POLICY tenant_isolation_contracts ON contracts
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

DROP POLICY IF EXISTS tenant_isolation_contract_requests ON contract_requests;
CREATE POLICY tenant_isolation_contract_requests ON contract_requests
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

DROP POLICY IF EXISTS tenant_isolation_contract_templates ON contract_templates;
CREATE POLICY tenant_isolation_contract_templates ON contract_templates
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

DROP POLICY IF EXISTS tenant_isolation_clause_library ON clause_library;
CREATE POLICY tenant_isolation_clause_library ON clause_library
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

DROP POLICY IF EXISTS tenant_isolation_procurement_suppliers ON procurement_suppliers;
CREATE POLICY tenant_isolation_procurement_suppliers ON procurement_suppliers
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

DROP POLICY IF EXISTS tenant_isolation_legal_matters ON legal_matters;
CREATE POLICY tenant_isolation_legal_matters ON legal_matters
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

DROP POLICY IF EXISTS tenant_isolation_contract_audit_logs ON contract_audit_logs;
CREATE POLICY tenant_isolation_contract_audit_logs ON contract_audit_logs
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- 2. WORM (WRITE ONCE READ MANY) IMMUTABLE AUDIT TRAIL PROTECTION
CREATE OR REPLACE FUNCTION reject_contract_audit_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'Contract audit log records are immutable and append-only under compliance requirements (SOX / SOC 2 / ISO 27001)';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS contract_audit_no_update ON contract_audit_logs;
CREATE TRIGGER contract_audit_no_update
BEFORE UPDATE ON contract_audit_logs
FOR EACH ROW EXECUTE FUNCTION reject_contract_audit_mutation();

DROP TRIGGER IF EXISTS contract_audit_no_delete ON contract_audit_logs;
CREATE TRIGGER contract_audit_no_delete
BEFORE DELETE ON contract_audit_logs
FOR EACH ROW EXECUTE FUNCTION reject_contract_audit_mutation();

-- 3. DELEGATION OF AUTHORITY (DOA) & MULTI-TIER APPROVAL ENGINE
CREATE TABLE IF NOT EXISTS delegation_of_authority_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    department VARCHAR(128) NOT NULL,
    min_amount NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
    max_amount NUMERIC(15, 2) NOT NULL, -- e.g. 50000, 250000, 1000000, 999999999
    tier_level INT NOT NULL DEFAULT 1,
    required_role VARCHAR(64) NOT NULL, -- dept_manager, director, vp, cfo, ceo, board
    requires_legal_review BOOLEAN DEFAULT true,
    requires_finance_review BOOLEAN DEFAULT true,
    requires_security_review BOOLEAN DEFAULT false,
    enforce_separation_of_duties BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS contract_approvals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    tier_level INT NOT NULL DEFAULT 1,
    approver_role VARCHAR(64) NOT NULL,
    approver_user_id VARCHAR(64),
    approver_email VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending', -- pending, approved, rejected, delegated
    approval_comments TEXT,
    delegated_to_email VARCHAR(255),
    decided_at TIMESTAMPTZ,
    ip_address VARCHAR(64),
    digital_signature_token VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_approvals_contract ON contract_approvals(contract_id);
CREATE INDEX IF NOT EXISTS idx_approvals_tenant ON contract_approvals(tenant_id);

-- 4. PROCUREMENT THREE-WAY MATCHING (PO + GOODS RECEIPT + INVOICE)
CREATE TABLE IF NOT EXISTS procurement_purchase_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    contract_id UUID REFERENCES contracts(id) ON DELETE SET NULL,
    po_number VARCHAR(64) NOT NULL UNIQUE,
    supplier_id UUID REFERENCES procurement_suppliers(id) ON DELETE RESTRICT,
    total_authorized_amount NUMERIC(15, 2) NOT NULL,
    currency VARCHAR(10) DEFAULT 'USD',
    issue_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status VARCHAR(32) DEFAULT 'issued', -- issued, partially_received, fulfilled, cancelled
    line_items JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS procurement_goods_receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    po_id UUID NOT NULL REFERENCES procurement_purchase_orders(id) ON DELETE CASCADE,
    receipt_number VARCHAR(64) NOT NULL UNIQUE,
    received_by_email VARCHAR(255) NOT NULL,
    received_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    accepted_quantity NUMERIC(10, 2) NOT NULL,
    rejected_quantity NUMERIC(10, 2) DEFAULT 0.00,
    inspection_passed BOOLEAN DEFAULT true,
    inspection_notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS procurement_supplier_invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    supplier_id UUID NOT NULL REFERENCES procurement_suppliers(id) ON DELETE RESTRICT,
    po_id UUID REFERENCES procurement_purchase_orders(id) ON DELETE SET NULL,
    invoice_number VARCHAR(64) NOT NULL,
    invoice_date TIMESTAMPTZ NOT NULL,
    invoice_amount NUMERIC(15, 2) NOT NULL,
    currency VARCHAR(10) DEFAULT 'USD',
    tax_amount NUMERIC(15, 2) DEFAULT 0.00,
    payment_due_date TIMESTAMPTZ NOT NULL,
    payment_status VARCHAR(32) DEFAULT 'pending_verification', -- pending_verification, approved, paid, disputed, rejected
    match_status VARCHAR(32) DEFAULT 'unmatched', -- unmatched, matched_2way, matched_3way, variance_exception
    variance_amount NUMERIC(15, 2) DEFAULT 0.00,
    variance_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(supplier_id, invoice_number)
);

CREATE TABLE IF NOT EXISTS procurement_three_way_matches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    contract_id UUID REFERENCES contracts(id) ON DELETE CASCADE,
    po_id UUID NOT NULL REFERENCES procurement_purchase_orders(id) ON DELETE CASCADE,
    receipt_id UUID REFERENCES procurement_goods_receipts(id) ON DELETE SET NULL,
    invoice_id UUID NOT NULL REFERENCES procurement_supplier_invoices(id) ON DELETE CASCADE,
    match_result VARCHAR(32) NOT NULL, -- perfect_match, price_variance, quantity_variance, unauthorized_overage
    variance_amount NUMERIC(15, 2) DEFAULT 0.00,
    tolerance_threshold_percent NUMERIC(5, 2) DEFAULT 2.00,
    approved_for_disbursement BOOLEAN DEFAULT false,
    disbursement_approver_email VARCHAR(255),
    verified_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 5. ZERO-KNOWLEDGE CRYPTOGRAPHIC KEY CUSTODY & IN-VPC ENVELOPE ENCRYPTION
CREATE TABLE IF NOT EXISTS satellite_enclave_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    satellite_id VARCHAR(128) NOT NULL,
    kms_key_arn VARCHAR(255) NOT NULL, -- Customer In-VPC KMS (AWS KMS, GCP Cloud KMS, Azure Key Vault, or PKCS#11 HSM)
    key_algorithm VARCHAR(64) DEFAULT 'AES-256-GCM / RSA-4096 / Ed25519',
    public_cert_pem TEXT NOT NULL, -- Public certificate only - SaaS control plane NEVER holds private keys
    key_fingerprint VARCHAR(128) NOT NULL,
    is_active BOOLEAN DEFAULT true,
    last_health_heartbeat TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS contract_cryptographic_envelopes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    satellite_id VARCHAR(128) NOT NULL,
    encrypted_data_encryption_key BYTEA NOT NULL, -- DEK encrypted by customer In-VPC KMS Master Key
    key_algorithm VARCHAR(64) DEFAULT 'AES-256-GCM',
    content_sha256 VARCHAR(64) NOT NULL,
    ciphertext_sha256 VARCHAR(64) NOT NULL,
    rfc3161_timestamp_token BYTEA, -- Cryptographic timestamp token from authorized TSA
    signature_evidence_manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

