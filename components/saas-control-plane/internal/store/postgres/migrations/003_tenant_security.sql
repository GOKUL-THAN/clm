-- Task 2: tenant-scoped append-only audits and encrypted cloud accounts.

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_valid;
ALTER TABLE users ADD CONSTRAINT users_role_valid CHECK (role IN (
    'super_admin', 'admin', 'tenant_admin', 'cert_manager', 'approver',
    'operator', 'auditor', 'viewer', 'finance', 'service_account'
));

CREATE TABLE IF NOT EXISTS control_plane_audit_events (
    id UUID PRIMARY KEY,
    -- Kept as an immutable tenant identifier without a foreign key so tenant
    -- deletion cannot erase or invalidate the security trail.
    tenant_id UUID NOT NULL,
    request_id VARCHAR(128) NOT NULL,
    actor_id UUID NOT NULL,
    actor_role VARCHAR(64) NOT NULL,
    action VARCHAR(128) NOT NULL,
    resource_type VARCHAR(128) NOT NULL,
    resource_id VARCHAR(255) NOT NULL DEFAULT '',
    outcome VARCHAR(32) NOT NULL CHECK (outcome IN ('success', 'failure')),
    source_ip VARCHAR(64) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_control_plane_audit_tenant_time
    ON control_plane_audit_events(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_control_plane_audit_filters
    ON control_plane_audit_events(tenant_id, action, resource_type, outcome);

CREATE OR REPLACE FUNCTION reject_control_plane_audit_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'control-plane audit events are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS control_plane_audit_no_update ON control_plane_audit_events;
CREATE TRIGGER control_plane_audit_no_update
BEFORE UPDATE ON control_plane_audit_events
FOR EACH ROW EXECUTE FUNCTION reject_control_plane_audit_mutation();

DROP TRIGGER IF EXISTS control_plane_audit_no_delete ON control_plane_audit_events;
CREATE TRIGGER control_plane_audit_no_delete
BEFORE DELETE ON control_plane_audit_events
FOR EACH ROW EXECUTE FUNCTION reject_control_plane_audit_mutation();

CREATE TABLE IF NOT EXISTS cloud_accounts (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    provider VARCHAR(32) NOT NULL CHECK (provider IN ('aws', 'gcp', 'azure')),
    encryption_version INTEGER NOT NULL,
    credential_version INTEGER NOT NULL DEFAULT 1,
    credential_nonce BYTEA NOT NULL,
    credential_ciphertext BYTEA NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_cloud_accounts_tenant ON cloud_accounts(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS cloud_account_idempotency (
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    idempotency_key VARCHAR(255) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    cloud_account_id UUID NOT NULL REFERENCES cloud_accounts(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, idempotency_key)
);

-- Reapplying this migration upgrades deployments created before the cascade
-- was added, so deleting an account cannot be blocked by its replay record.
ALTER TABLE cloud_account_idempotency
    DROP CONSTRAINT IF EXISTS cloud_account_idempotency_cloud_account_id_fkey;
ALTER TABLE cloud_account_idempotency
    ADD CONSTRAINT cloud_account_idempotency_cloud_account_id_fkey
    FOREIGN KEY (cloud_account_id) REFERENCES cloud_accounts(id) ON DELETE CASCADE;

-- Existing deployments may not yet persist deployment jobs. Where the table is
-- present, operators should add cloud_account_id as a NOT NULL tenant-owned FK
-- after backfilling. Runtime deploy requests require the reference immediately.
