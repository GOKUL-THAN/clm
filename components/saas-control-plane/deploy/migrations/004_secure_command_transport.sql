-- Task 3: per-satellite credentials and durable leased command delivery.

ALTER TABLE satellites ADD COLUMN IF NOT EXISTS credential_hash TEXT;
ALTER TABLE satellites ADD COLUMN IF NOT EXISTS credential_generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE satellites ADD COLUMN IF NOT EXISTS credential_created_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE satellites ADD COLUMN IF NOT EXISTS credential_last_used_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE satellites ADD COLUMN IF NOT EXISTS credential_revoked_at TIMESTAMP WITH TIME ZONE;

ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id);
UPDATE satellite_commands c SET tenant_id = s.tenant_id FROM satellites s
WHERE c.satellite_id = s.id AND c.tenant_id IS NULL;
ALTER TABLE satellite_commands ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(255);
UPDATE satellite_commands SET idempotency_key = id::text WHERE idempotency_key IS NULL;
ALTER TABLE satellite_commands ALTER COLUMN idempotency_key SET NOT NULL;
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS execution_generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS max_attempts INTEGER NOT NULL DEFAULT 3;
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS available_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS lease_owner VARCHAR(255);
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS lease_token UUID;
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS lease_until TIMESTAMP WITH TIME ZONE;
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS last_error TEXT;
ALTER TABLE satellite_commands ADD COLUMN IF NOT EXISTS terminal_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE satellite_commands ALTER COLUMN result TYPE TEXT USING result::text;

UPDATE satellite_commands SET status = CASE status
    WHEN 'sent' THEN 'queued'
    WHEN 'executing' THEN 'running'
    WHEN 'completed' THEN 'succeeded'
    ELSE status
END;

ALTER TABLE satellite_commands DROP CONSTRAINT IF EXISTS satellite_commands_status_valid;
ALTER TABLE satellite_commands ADD CONSTRAINT satellite_commands_status_valid
    CHECK (status IN ('queued','leased','running','succeeded','failed','dead_letter','cancelled'));
ALTER TABLE satellite_commands DROP CONSTRAINT IF EXISTS satellite_commands_attempts_valid;
ALTER TABLE satellite_commands ADD CONSTRAINT satellite_commands_attempts_valid
    CHECK (attempts >= 0 AND max_attempts > 0 AND attempts <= max_attempts AND execution_generation >= 0);

CREATE UNIQUE INDEX IF NOT EXISTS idx_commands_idempotency
    ON satellite_commands(tenant_id, satellite_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_commands_claimable
    ON satellite_commands(satellite_id, status, available_at, lease_until, priority DESC, created_at);
