-- ==============================================================================
-- 012_enterprise_platform_suite.sql
-- Enterprise CLM & Certificate Lifecycle Platform Suite Tables
-- Provides real PostgreSQL persistence for all 20 enterprise domains:
-- - Organization Verifications & Domain Control Validation (DCV)
-- - Subscription Plans, Tenant Quotas & Cloud Invoices
-- - PKI Governance Policies & Policy Audit Logs
-- - Network Discovery Scans & Certificate Transparency (CT) Stream
-- - Real-Time Platform Alerts & Incident Triage
-- - Enterprise Security Controls & System Settings
-- - Integrations Catalog & Sync Status
-- ==============================================================================

-- 1. Organization Business Verifications (D-U-N-S, Legal Entity, EV)
CREATE TABLE IF NOT EXISTS organization_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    legal_name VARCHAR NOT NULL,
    duns_number VARCHAR NOT NULL,
    duns_status VARCHAR NOT NULL DEFAULT 'Verified',
    verification_level VARCHAR NOT NULL DEFAULT 'EV (Extended Validation)',
    incorporation_country VARCHAR NOT NULL DEFAULT 'United States',
    ca_prevalidation JSONB DEFAULT '{"digicert": true, "lets_encrypt": true, "sectigo": true}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_tenant_verification UNIQUE (tenant_id)
);

-- 2. Domain Control Validation (DCV) Challenges
CREATE TABLE IF NOT EXISTS domain_validations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain VARCHAR NOT NULL,
    status VARCHAR NOT NULL DEFAULT 'Pending',
    method VARCHAR NOT NULL DEFAULT 'DNS_TXT',
    token VARCHAR NOT NULL,
    verified_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_tenant_domain UNIQUE (tenant_id, domain)
);

-- 3. Subscription Plans (Platform Catalog)
CREATE TABLE IF NOT EXISTS subscription_plans (
    id VARCHAR PRIMARY KEY,
    name VARCHAR NOT NULL,
    price_cents INT NOT NULL,
    interval VARCHAR NOT NULL DEFAULT 'month',
    max_users INT NOT NULL,
    max_gateways INT NOT NULL,
    max_certs INT NOT NULL,
    max_contracts INT NOT NULL,
    features JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Seed default plans if empty
INSERT INTO subscription_plans (id, name, price_cents, interval, max_users, max_gateways, max_certs, max_contracts, features)
VALUES 
('starter', 'Starter', 49900, 'month', 5, 2, 50, 20, '["SAML / SSO", "Basic Discovery", "Email Alerts"]'::jsonb),
('pro', 'Professional', 149900, 'month', 25, 10, 500, 100, '["Multi-Cloud Accounts", "In-VPC Satellites", "Custom Playbooks", "Three-Way Match"]'::jsonb),
('enterprise', 'Enterprise', 499900, 'month', -1, -1, -1, -1, '["Zero-Knowledge Hardware PKI", "Custom CA Integrations", "99.99% SLA", "Dedicated Account Manager"]'::jsonb)
ON CONFLICT (id) DO NOTHING;

-- 4. Cloud Invoices & Receipts
CREATE TABLE IF NOT EXISTS invoices (
    id VARCHAR PRIMARY KEY,
    tenant_id VARCHAR NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    amount_cents INT NOT NULL,
    currency VARCHAR NOT NULL DEFAULT 'USD',
    status VARCHAR NOT NULL DEFAULT 'Paid',
    description TEXT NOT NULL,
    pdf_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- 5. PKI Governance Policies
CREATE TABLE IF NOT EXISTS pki_policies (
    id VARCHAR PRIMARY KEY,
    tenant_id VARCHAR NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR NOT NULL,
    rule_type VARCHAR NOT NULL,
    severity VARCHAR NOT NULL DEFAULT 'High',
    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
    enforced BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- 6. Network Discovery Scans
CREATE TABLE IF NOT EXISTS discovery_scans (
    id VARCHAR PRIMARY KEY,
    tenant_id VARCHAR NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    target VARCHAR NOT NULL,
    ports VARCHAR NOT NULL DEFAULT '443, 8443',
    scanned_hosts INT NOT NULL DEFAULT 0,
    active_tls_endpoints INT NOT NULL DEFAULT 0,
    valid_certificates INT NOT NULL DEFAULT 0,
    expiring_certificates INT NOT NULL DEFAULT 0,
    rogue_certificates INT NOT NULL DEFAULT 0,
    duration_ms INT NOT NULL DEFAULT 0,
    status VARCHAR NOT NULL DEFAULT 'completed',
    completed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- 7. Certificate Transparency (CT) Stream Logs
CREATE TABLE IF NOT EXISTS certificate_transparency_logs (
    id VARCHAR PRIMARY KEY,
    tenant_id VARCHAR NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain VARCHAR NOT NULL,
    issuer VARCHAR NOT NULL,
    ct_log_source VARCHAR NOT NULL,
    status VARCHAR NOT NULL DEFAULT 'Authorized',
    san_count INT NOT NULL DEFAULT 1,
    logged_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- 8. Platform Alerts & Incidents
CREATE TABLE IF NOT EXISTS platform_alerts (
    id VARCHAR PRIMARY KEY,
    tenant_id VARCHAR NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    severity VARCHAR NOT NULL,
    category VARCHAR NOT NULL,
    title TEXT NOT NULL,
    target VARCHAR,
    status VARCHAR NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMP WITH TIME ZONE
);

-- 9. Enterprise Platform Settings & Security Controls
CREATE TABLE IF NOT EXISTS platform_settings (
    tenant_id VARCHAR PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    organization_name VARCHAR NOT NULL,
    support_email VARCHAR NOT NULL,
    session_timeout_minutes INT NOT NULL DEFAULT 60,
    password_min_length INT NOT NULL DEFAULT 12,
    mfa_enforced BOOLEAN NOT NULL DEFAULT true,
    ip_allowlist_enabled BOOLEAN NOT NULL DEFAULT true,
    allowed_ips JSONB NOT NULL DEFAULT '["127.0.0.1", "10.0.0.0/8"]'::jsonb,
    maintenance_mode BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- 10. Integrations Catalog & Status
CREATE TABLE IF NOT EXISTS integrations_catalog (
    id VARCHAR PRIMARY KEY,
    tenant_id VARCHAR NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    provider VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    category VARCHAR NOT NULL DEFAULT 'ERP',
    status VARCHAR NOT NULL DEFAULT 'Connected',
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_sync TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- ==============================================================================
-- ROW LEVEL SECURITY (RLS) ENFORCEMENT
-- ==============================================================================
ALTER TABLE organization_verifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE domain_validations ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE pki_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE discovery_scans ENABLE ROW LEVEL SECURITY;
ALTER TABLE certificate_transparency_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_alerts ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE integrations_catalog ENABLE ROW LEVEL SECURITY;

-- Tenant Isolation Policies
DROP POLICY IF EXISTS p_org_verifications_tenant ON organization_verifications;
CREATE POLICY p_org_verifications_tenant ON organization_verifications
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''))
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''));

DROP POLICY IF EXISTS p_domain_validations_tenant ON domain_validations;
CREATE POLICY p_domain_validations_tenant ON domain_validations
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''))
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''));

DROP POLICY IF EXISTS p_invoices_tenant ON invoices;
CREATE POLICY p_invoices_tenant ON invoices
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''))
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''));

DROP POLICY IF EXISTS p_pki_policies_tenant ON pki_policies;
CREATE POLICY p_pki_policies_tenant ON pki_policies
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''))
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''));

DROP POLICY IF EXISTS p_discovery_scans_tenant ON discovery_scans;
CREATE POLICY p_discovery_scans_tenant ON discovery_scans
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''))
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''));

DROP POLICY IF EXISTS p_ct_logs_tenant ON certificate_transparency_logs;
CREATE POLICY p_ct_logs_tenant ON certificate_transparency_logs
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''))
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''));

DROP POLICY IF EXISTS p_platform_alerts_tenant ON platform_alerts;
CREATE POLICY p_platform_alerts_tenant ON platform_alerts
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''))
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''));

DROP POLICY IF EXISTS p_platform_settings_tenant ON platform_settings;
CREATE POLICY p_platform_settings_tenant ON platform_settings
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''))
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''));

DROP POLICY IF EXISTS p_integrations_catalog_tenant ON integrations_catalog;
CREATE POLICY p_integrations_catalog_tenant ON integrations_catalog
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''))
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), ''));
