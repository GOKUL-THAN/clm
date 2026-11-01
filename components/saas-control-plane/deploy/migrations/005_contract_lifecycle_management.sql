-- ==============================================================================
-- 005_contract_lifecycle_management.sql
-- Enterprise Contract Lifecycle Management (CLM) Schema
-- Covers End-to-End Contract Lifecycle, AI Intelligence, Procurement, Legal Ops,
-- Digital Signatures, Risk, Compliance, and Financial Terms.
-- ==============================================================================

-- 1. CONTRACT REPOSITORY & CORE METADATA
CREATE TABLE IF NOT EXISTS contracts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    contract_number VARCHAR(64) NOT NULL UNIQUE,
    title VARCHAR(255) NOT NULL,
    type VARCHAR(64) NOT NULL DEFAULT 'MSA', -- MSA, NDA, SOW, SLA, Vendor, Customer, Lease, Partnership, License, Employment, Procurement
    status VARCHAR(32) NOT NULL DEFAULT 'draft', -- draft, review, approval, negotiation, ready_for_signature, partially_signed, executed, active, amended, renewing, expired, terminated
    stage VARCHAR(32) NOT NULL DEFAULT 'authoring', -- intake, authoring, negotiation, approval, signature, post_execution, renewal, archival
    counterparty_name VARCHAR(255) NOT NULL,
    counterparty_contact VARCHAR(255),
    counterparty_email VARCHAR(255),
    entity_id VARCHAR(64) DEFAULT 'PRIMARY_LEGAL_ENTITY',
    business_unit VARCHAR(128) DEFAULT 'Global Operations',
    department VARCHAR(128) DEFAULT 'Legal & Compliance',
    owner_user_id VARCHAR(64),
    owner_email VARCHAR(255),
    effective_date TIMESTAMPTZ,
    expiration_date TIMESTAMPTZ,
    execution_date TIMESTAMPTZ,
    renewal_type VARCHAR(32) DEFAULT 'manual', -- manual, auto_renewal, evergreen, non_renewing
    renewal_notice_days INT DEFAULT 60,
    notice_period_days INT DEFAULT 30,
    total_value NUMERIC(15, 2) DEFAULT 0.00,
    currency VARCHAR(10) DEFAULT 'USD',
    payment_terms VARCHAR(64) DEFAULT 'Net 30',
    governing_law VARCHAR(128) DEFAULT 'Delaware, USA',
    jurisdiction VARCHAR(128) DEFAULT 'USA',
    risk_score INT DEFAULT 20, -- 0 - 100
    compliance_status VARCHAR(32) DEFAULT 'compliant', -- compliant, under_review, non_compliant
    current_version INT DEFAULT 1,
    content_body TEXT,
    folder_path VARCHAR(255) DEFAULT '/General/2026',
    parent_contract_id UUID REFERENCES contracts(id) ON DELETE SET NULL,
    master_agreement_id UUID REFERENCES contracts(id) ON DELETE SET NULL,
    tags JSONB DEFAULT '[]'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_contracts_tenant ON contracts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_contracts_status ON contracts(status);
CREATE INDEX IF NOT EXISTS idx_contracts_stage ON contracts(stage);
CREATE INDEX IF NOT EXISTS idx_contracts_type ON contracts(type);
CREATE INDEX IF NOT EXISTS idx_contracts_expiration ON contracts(expiration_date);

-- 2. CONTRACT VERSION CONTROL & AUDIT TRAIL
CREATE TABLE IF NOT EXISTS contract_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    version_number INT NOT NULL,
    title VARCHAR(255) NOT NULL,
    content_body TEXT NOT NULL,
    changelog_summary TEXT,
    author_user_id VARCHAR(64),
    author_email VARCHAR(255),
    document_hash VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(contract_id, version_number)
);

-- 3. CONTRACT INTAKE & REQUEST MANAGEMENT
CREATE TABLE IF NOT EXISTS contract_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    request_number VARCHAR(64) NOT NULL UNIQUE,
    title VARCHAR(255) NOT NULL,
    contract_type VARCHAR(64) NOT NULL,
    request_origin VARCHAR(32) DEFAULT 'business_portal', -- self_service, business_portal, procurement, legal
    counterparty_name VARCHAR(255) NOT NULL,
    counterparty_email VARCHAR(255),
    estimated_value NUMERIC(15, 2) DEFAULT 0.00,
    currency VARCHAR(10) DEFAULT 'USD',
    urgency VARCHAR(32) DEFAULT 'medium', -- low, medium, high, critical
    business_justification TEXT,
    department VARCHAR(128),
    requester_email VARCHAR(255) NOT NULL,
    assigned_counsel_email VARCHAR(255),
    status VARCHAR(32) DEFAULT 'submitted', -- submitted, triaged, under_legal_review, approved, converted, rejected, cancelled
    converted_contract_id UUID REFERENCES contracts(id) ON DELETE SET NULL,
    sla_deadline TIMESTAMPTZ,
    form_data JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 4. CONTRACT TEMPLATES & DYNAMIC CLAUSE LIBRARY
CREATE TABLE IF NOT EXISTS contract_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(64) NOT NULL, -- MSA, NDA, SOW, SLA, Vendor, Employment
    description TEXT,
    content_body TEXT NOT NULL,
    version INT DEFAULT 1,
    status VARCHAR(32) DEFAULT 'published', -- draft, published, archived
    variables JSONB DEFAULT '[]'::jsonb,
    governing_rules JSONB DEFAULT '{}'::jsonb,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS clause_library (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    category VARCHAR(64) NOT NULL, -- Confidentiality, Liability, Indemnification, Termination, Data Privacy, Dispute
    standard_text TEXT NOT NULL,
    fallback_text TEXT,
    risk_level VARCHAR(32) DEFAULT 'low', -- low, standard, elevated, high
    is_mandatory BOOLEAN DEFAULT false,
    playbook_guidance TEXT,
    jurisdiction_tags JSONB DEFAULT '["global"]'::jsonb,
    approval_role_required VARCHAR(64) DEFAULT 'legal_counsel',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 5. NEGOTIATION WORKSPACE, REDLINES & COLLABORATION
CREATE TABLE IF NOT EXISTS contract_negotiations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    round_number INT DEFAULT 1,
    status VARCHAR(32) DEFAULT 'in_progress', -- in_progress, agreed, disputed, closed
    turnaround_deadline TIMESTAMPTZ,
    external_workspace_token VARCHAR(128) UNIQUE,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS contract_redlines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    negotiation_id UUID REFERENCES contract_negotiations(id) ON DELETE CASCADE,
    author_name VARCHAR(255) NOT NULL,
    author_type VARCHAR(32) DEFAULT 'internal', -- internal, counterparty
    change_type VARCHAR(32) DEFAULT 'modification', -- addition, deletion, modification
    clause_title VARCHAR(255),
    original_text TEXT,
    proposed_text TEXT,
    status VARCHAR(32) DEFAULT 'proposed', -- proposed, accepted, rejected, superseded
    review_comments TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

-- 6. ELECTRONIC & DIGITAL SIGNATURES (PKI)
CREATE TABLE IF NOT EXISTS contract_signatures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    signer_name VARCHAR(255) NOT NULL,
    signer_email VARCHAR(255) NOT NULL,
    signer_role VARCHAR(128) DEFAULT 'Authorized Representative',
    signing_order INT DEFAULT 1,
    status VARCHAR(32) DEFAULT 'pending', -- pending, sent, viewed, signed, declined, expired
    auth_method VARCHAR(32) DEFAULT 'email_otp', -- email_otp, pki_certificate, hardware_token, sso
    ip_address VARCHAR(64),
    user_agent TEXT,
    digital_signature_hash VARCHAR(128),
    certificate_thumbprint VARCHAR(128),
    signed_at TIMESTAMPTZ,
    evidence_bundle_uri VARCHAR(512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 7. POST-EXECUTION OBLIGATIONS & MILESTONES
CREATE TABLE IF NOT EXISTS contract_obligations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    category VARCHAR(64) DEFAULT 'Operational', -- Operational, Regulatory, Financial, Warranty, Deliverable, Audit
    party_responsible VARCHAR(32) DEFAULT 'counterparty', -- internal, counterparty, both
    owner_email VARCHAR(255),
    due_date TIMESTAMPTZ NOT NULL,
    recurrence VARCHAR(32) DEFAULT 'one_time', -- one_time, monthly, quarterly, annual
    status VARCHAR(32) DEFAULT 'in_progress', -- in_progress, fulfilled, overdue, waived, breached
    penalty_details TEXT,
    service_credit_percent NUMERIC(5, 2) DEFAULT 0.00,
    evidence_notes TEXT,
    evidence_document_uri VARCHAR(512),
    completed_at TIMESTAMPTZ,
    verified_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 8. RISK, COMPLIANCE & LEGAL GOVERNANCE
CREATE TABLE IF NOT EXISTS contract_risks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id UUID REFERENCES contracts(id) ON DELETE CASCADE,
    risk_code VARCHAR(64) NOT NULL,
    category VARCHAR(64) NOT NULL, -- Liability, Cyber, Privacy, SLA, Financial, Sanctions, Single_Source
    severity VARCHAR(32) DEFAULT 'MEDIUM', -- LOW, MEDIUM, HIGH, CRITICAL
    score INT DEFAULT 50, -- 1-100
    title VARCHAR(255) NOT NULL,
    description TEXT,
    remediation_plan TEXT,
    assigned_owner VARCHAR(255),
    mitigation_status VARCHAR(32) DEFAULT 'open', -- open, mitigated, accepted_exception, closed
    exception_approved_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 9. CORPORATE PROCUREMENT & SUPPLIER MANAGEMENT
CREATE TABLE IF NOT EXISTS procurement_suppliers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    supplier_name VARCHAR(255) NOT NULL,
    supplier_code VARCHAR(64) UNIQUE,
    category VARCHAR(64) DEFAULT 'Technology Services',
    contact_name VARCHAR(255),
    contact_email VARCHAR(255),
    onboarding_status VARCHAR(32) DEFAULT 'active', -- prequalification, due_diligence, onboarding, active, offboarding, suspended
    risk_tier VARCHAR(32) DEFAULT 'Tier-2', -- Tier-1, Tier-2, Tier-3
    performance_score INT DEFAULT 88, -- 0-100
    diversity_status VARCHAR(64) DEFAULT 'Minority-Owned Business',
    insurance_cert_expires TIMESTAMPTZ,
    annual_spend NUMERIC(15, 2) DEFAULT 0.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 10. LEGAL OPERATIONS & MATTER MANAGEMENT
CREATE TABLE IF NOT EXISTS legal_matters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    matter_number VARCHAR(64) NOT NULL UNIQUE,
    title VARCHAR(255) NOT NULL,
    case_type VARCHAR(64) DEFAULT 'Commercial Contract Dispute', -- Commercial, Litigation, IP, Regulatory, Advisory
    priority VARCHAR(32) DEFAULT 'high', -- low, medium, high, critical
    lead_counsel VARCHAR(255) NOT NULL,
    outside_counsel_firm VARCHAR(255),
    legal_budget NUMERIC(15, 2) DEFAULT 50000.00,
    legal_spend NUMERIC(15, 2) DEFAULT 12500.00,
    litigation_hold_active BOOLEAN DEFAULT false,
    status VARCHAR(32) DEFAULT 'active', -- active, settled, closed, archived
    related_contract_id UUID REFERENCES contracts(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 11. FINANCIAL & COMMERCIAL TERMS MANAGEMENT
CREATE TABLE IF NOT EXISTS contract_financials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    total_committed_amount NUMERIC(15, 2) DEFAULT 0.00,
    billed_to_date NUMERIC(15, 2) DEFAULT 0.00,
    remaining_balance NUMERIC(15, 2) DEFAULT 0.00,
    discount_percent NUMERIC(5, 2) DEFAULT 0.00,
    rebate_terms TEXT,
    tax_treatment VARCHAR(64) DEFAULT 'VAT Excluded',
    cost_center VARCHAR(64),
    three_way_match_verified BOOLEAN DEFAULT true,
    revenue_leakage_detected BOOLEAN DEFAULT false,
    leakage_notes TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 12. AI CONTRACT INTELLIGENCE & AUDIT LOGS
CREATE TABLE IF NOT EXISTS ai_contract_analyses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contract_id UUID NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    analysis_type VARCHAR(64) DEFAULT 'full_audit',
    model_name VARCHAR(128) DEFAULT 'Enterprise Legal-LLM v2.4 (Gemini-Pro)',
    confidence_score NUMERIC(5, 2) DEFAULT 0.95,
    executive_summary TEXT,
    extracted_clauses JSONB DEFAULT '[]'::jsonb,
    extracted_obligations JSONB DEFAULT '[]'::jsonb,
    detected_risks JSONB DEFAULT '[]'::jsonb,
    human_in_loop_verified BOOLEAN DEFAULT false,
    verified_by_user VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS contract_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    contract_id UUID,
    actor_email VARCHAR(255) NOT NULL,
    action_type VARCHAR(64) NOT NULL, -- create, edit, approve, redline_proposed, signed, obligation_fulfilled, risk_flagged
    description TEXT,
    payload_diff JSONB,
    ip_address VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_contract ON contract_audit_logs(contract_id);
CREATE INDEX IF NOT EXISTS idx_audit_tenant ON contract_audit_logs(tenant_id);

