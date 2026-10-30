-- CLM Migration: Add SSO fields to tenants and users
-- Run: psql -U clm -d clm_db -f 002_sso_fields.sql

-- Add SSO config JSON column to tenants
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS sso_config JSONB DEFAULT '{}';

-- Add SSO auth provider tracking to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_provider VARCHAR(50) NOT NULL DEFAULT 'local';
ALTER TABLE users ADD COLUMN IF NOT EXISTS sso_subject VARCHAR(500);

-- Allow nullable password_hash for SSO-only users (they never set a local password)
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
ALTER TABLE users ALTER COLUMN password_hash SET DEFAULT '';

-- Index for SSO subject lookups
CREATE INDEX IF NOT EXISTS idx_users_sso_subject ON users(auth_provider, sso_subject) WHERE sso_subject IS NOT NULL;

-- Index for SSO-enabled tenants
CREATE INDEX IF NOT EXISTS idx_tenants_sso ON tenants(sso_enabled) WHERE sso_enabled = true;
