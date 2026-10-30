-- CLM Initial Schema
-- Run: psql -U clm -d clm_db -f 001_initial.sql

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Tenants
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    plan VARCHAR(50) NOT NULL DEFAULT 'free',
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    max_certs INTEGER NOT NULL DEFAULT 100,
    max_satellites INTEGER NOT NULL DEFAULT 2,
    max_users INTEGER NOT NULL DEFAULT 5,
    sso_enabled BOOLEAN NOT NULL DEFAULT false,
    sso_provider VARCHAR(100),
    data_region VARCHAR(50) NOT NULL DEFAULT 'ap-south-1',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Users
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'viewer',
    team_id UUID,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    mfa_enabled BOOLEAN NOT NULL DEFAULT false,
    password_hash VARCHAR(255) NOT NULL,
    last_login_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_users_email ON users(email);

-- Teams
CREATE TABLE teams (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Satellites
CREATE TABLE satellites (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'local',
    status VARCHAR(50) NOT NULL DEFAULT 'registering',
    connector_mode VARCHAR(50) NOT NULL DEFAULT 'direct',
    version VARCHAR(50),
    fingerprint VARCHAR(255),
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    ip_address VARCHAR(45),
    cloud VARCHAR(50),
    region VARCHAR(100),
    secrets_backend VARCHAR(50) NOT NULL DEFAULT 'vault',
    agent_count INTEGER NOT NULL DEFAULT 0,
    cert_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_satellites_tenant ON satellites(tenant_id);

-- Satellite Commands
CREATE TABLE satellite_commands (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    satellite_id UUID NOT NULL REFERENCES satellites(id),
    type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(50) NOT NULL DEFAULT 'queued',
    priority INTEGER NOT NULL DEFAULT 0,
    result JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_commands_satellite_status ON satellite_commands(satellite_id, status);

-- Certificates
CREATE TABLE certificates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    domain VARCHAR(255) NOT NULL,
    sans TEXT[] DEFAULT '{}',
    type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    key_algorithm VARCHAR(50) NOT NULL DEFAULT 'ECDSA-P256',
    issuer VARCHAR(255),
    serial_number VARCHAR(255),
    fingerprint VARCHAR(255),
    not_before TIMESTAMP WITH TIME ZONE,
    not_after TIMESTAMP WITH TIME ZONE,
    environment VARCHAR(50) NOT NULL DEFAULT 'prod',
    satellite_id VARCHAR(255),
    secret_ref VARCHAR(500),
    tags TEXT[] DEFAULT '{}',
    created_by UUID REFERENCES users(id),
    renewal_count INTEGER NOT NULL DEFAULT 0,
    last_deployed_at TIMESTAMP WITH TIME ZONE,
    approval_id UUID,
    purchase_order_id UUID,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_certs_tenant ON certificates(tenant_id);
CREATE INDEX idx_certs_domain ON certificates(tenant_id, domain);
CREATE INDEX idx_certs_status ON certificates(tenant_id, status);
CREATE INDEX idx_certs_expiry ON certificates(not_after) WHERE status = 'active';

-- Approval Requests
CREATE TABLE approval_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    request_type VARCHAR(100) NOT NULL,
    level VARCHAR(50) NOT NULL DEFAULT 'L1',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    requested_by UUID NOT NULL REFERENCES users(id),
    justification TEXT,
    payload JSONB NOT NULL DEFAULT '{}',
    estimated_cost DECIMAL(10,2) NOT NULL DEFAULT 0,
    sla_deadline TIMESTAMP WITH TIME ZONE NOT NULL,
    resolved_by UUID REFERENCES users(id),
    resolved_at TIMESTAMP WITH TIME ZONE,
    comment TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_approvals_tenant_status ON approval_requests(tenant_id, status);

-- Approval Rules
CREATE TABLE approval_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    level VARCHAR(50) NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    conditions JSONB NOT NULL DEFAULT '{}',
    approver_roles TEXT[] DEFAULT '{}',
    min_approvals INTEGER NOT NULL DEFAULT 1,
    sla_hours INTEGER NOT NULL DEFAULT 8,
    escalate_to VARCHAR(50),
    active BOOLEAN NOT NULL DEFAULT true
);

-- Purchase Orders
CREATE TABLE purchase_orders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    cert_request_id UUID REFERENCES certificates(id),
    ca_provider VARCHAR(50) NOT NULL,
    ca_order_id VARCHAR(255),
    product_id VARCHAR(255),
    domain VARCHAR(255) NOT NULL,
    sans TEXT[] DEFAULT '{}',
    cert_type VARCHAR(50) NOT NULL,
    validity VARCHAR(50) NOT NULL,
    price DECIMAL(10,2) NOT NULL DEFAULT 0,
    currency VARCHAR(10) NOT NULL DEFAULT 'USD',
    status VARCHAR(50) NOT NULL DEFAULT 'quoted',
    approval_id UUID REFERENCES approval_requests(id),
    requested_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    issued_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_purchases_tenant ON purchase_orders(tenant_id);

-- CA Account Configs
CREATE TABLE ca_accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    provider VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    environment VARCHAR(50) NOT NULL DEFAULT 'production',
    cred_ref VARCHAR(500) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    budget DECIMAL(10,2) NOT NULL DEFAULT 0,
    spent_month DECIMAL(10,2) NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Audit Log
CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    user_id UUID REFERENCES users(id),
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID,
    details JSONB,
    ip_address VARCHAR(45),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_tenant ON audit_log(tenant_id, created_at DESC);
