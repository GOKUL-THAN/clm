-- ==============================================================================
-- 007_immutable_audit_hash_chaining_and_truncate_protection.sql
-- Enterprise CLM Immutable Audit Trail Hardening:
-- 1. Table-level TRUNCATE protection trigger (closes unhandled truncate vector)
-- 2. Row-level UPDATE and DELETE rejection triggers (enforces append-only ledger)
-- 3. Dedicated tenant_audit_chain_heads table for atomic row-locked chain advancement
-- 4. Monotonic sequence_num and cryptographic hash chaining columns
-- 5. Automatic SHA-256 Linear Cryptographic Hash Chain trigger with clock_timestamp()
-- 6. Role privilege separation (REVOKE UPDATE, DELETE, TRUNCATE)
-- ==============================================================================

-- 1. STATEMENT-LEVEL TRUNCATE EXCEPTION TRIGGER
-- In PostgreSQL, TRUNCATE does not fire FOR EACH ROW triggers.
-- A dedicated statement-level trigger is mandatory to prevent table wiping.
CREATE OR REPLACE FUNCTION reject_contract_audit_truncate()
RETURNS trigger
SECURITY DEFINER
SET search_path = pg_catalog
AS $$
BEGIN
    RAISE EXCEPTION 'TRUNCATE operation strictly prohibited: contract_audit_logs is an immutable compliance ledger under SOX 404 / SOC 2 CC6.8 / ISO 27001 A.8.15';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS contract_audit_no_truncate ON contract_audit_logs;
CREATE TRIGGER contract_audit_no_truncate
BEFORE TRUNCATE ON contract_audit_logs
FOR EACH STATEMENT EXECUTE FUNCTION reject_contract_audit_truncate();

-- 2. ROW-LEVEL MUTATION EXCEPTION TRIGGER (BLOCK UPDATE & DELETE)
CREATE OR REPLACE FUNCTION reject_contract_audit_mutation()
RETURNS trigger
SECURITY DEFINER
SET search_path = pg_catalog
AS $$
BEGIN
    RAISE EXCEPTION 'UPDATE and DELETE operations are strictly prohibited: contract_audit_logs is an immutable compliance ledger under SOX 404 / SOC 2 CC6.8 / ISO 27001 A.8.15';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS contract_audit_no_mutation ON contract_audit_logs;
CREATE TRIGGER contract_audit_no_mutation
BEFORE UPDATE OR DELETE ON contract_audit_logs
FOR EACH ROW EXECUTE FUNCTION reject_contract_audit_mutation();

-- 3. CRYPTOGRAPHIC HASH CHAINING & SEQUENCE COLUMNS
ALTER TABLE contract_audit_logs ADD COLUMN IF NOT EXISTS previous_hash VARCHAR(128) DEFAULT 'GENESIS_0000000000000000000000000000000000000000000000000000000000000000';
ALTER TABLE contract_audit_logs ADD COLUMN IF NOT EXISTS record_hash VARCHAR(128);
ALTER TABLE contract_audit_logs ADD COLUMN IF NOT EXISTS sequence_num BIGINT DEFAULT 1;

-- 4. DEDICATED TENANT AUDIT CHAIN HEADS TABLE
-- Maintains single-row head pointer per tenant with transactional row locking (SELECT FOR UPDATE)
-- to prevent chain forks under REPEATABLE READ and READ COMMITTED snapshot isolation.
CREATE TABLE IF NOT EXISTS tenant_audit_chain_heads (
    tenant_id UUID PRIMARY KEY,
    last_hash VARCHAR(128) NOT NULL,
    sequence_num BIGINT NOT NULL DEFAULT 0,
    last_log_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

-- Enforce Row Level Security on chain-head table
ALTER TABLE tenant_audit_chain_heads ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_audit_chain_heads FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_audit_chain_heads_isolation_policy ON tenant_audit_chain_heads;
CREATE POLICY tenant_audit_chain_heads_isolation_policy ON tenant_audit_chain_heads
FOR ALL
USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- Backfill chain heads for existing tenants if any audit logs already exist
INSERT INTO tenant_audit_chain_heads (tenant_id, last_hash, sequence_num, last_log_id, updated_at)
SELECT DISTINCT ON (tenant_id)
    tenant_id,
    COALESCE(record_hash, 'GENESIS_0000000000000000000000000000000000000000000000000000000000000000'),
    COALESCE(sequence_num, 1),
    id,
    created_at
FROM contract_audit_logs
ORDER BY tenant_id, created_at DESC, id DESC
ON CONFLICT (tenant_id) DO NOTHING;

-- 5. AUTOMATIC SHA-256 LINEAR HASH CHAIN TRIGGER WITH ROW LOCKING
CREATE OR REPLACE FUNCTION compute_contract_audit_hash_chain()
RETURNS trigger
SECURITY DEFINER
SET search_path = pg_catalog
AS $$
DECLARE
    v_prev_hash VARCHAR(128);
    v_seq BIGINT;
    payload_text TEXT;
BEGIN
    -- Advisory lock as defense-in-depth serialization
    PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtext(NEW.tenant_id::text));

    -- Lock the tenant's chain-head row using SELECT FOR UPDATE
    SELECT last_hash, sequence_num
    INTO v_prev_hash, v_seq
    FROM public.tenant_audit_chain_heads
    WHERE tenant_id = NEW.tenant_id
    FOR UPDATE;

    -- Genesis initialization for new tenant
    IF NOT FOUND THEN
        INSERT INTO public.tenant_audit_chain_heads (tenant_id, last_hash, sequence_num, last_log_id, updated_at)
        VALUES (
            NEW.tenant_id,
            'GENESIS_0000000000000000000000000000000000000000000000000000000000000000',
            0,
            NEW.id,
            pg_catalog.clock_timestamp()
        )
        ON CONFLICT (tenant_id) DO UPDATE
        SET updated_at = pg_catalog.clock_timestamp()
        RETURNING last_hash, sequence_num INTO v_prev_hash, v_seq;
    END IF;

    -- Monotonic sequence advancement
    NEW.sequence_num := v_seq + 1;
    NEW.previous_hash := v_prev_hash;
    NEW.created_at := pg_catalog.clock_timestamp();

    -- Compute SHA-256 over canonical forensic fields
    payload_text := NEW.previous_hash || ':' || NEW.sequence_num::text || ':' || NEW.tenant_id::text || ':' || NEW.actor_email || ':' || NEW.action_type || ':' || NEW.created_at::text || ':' || COALESCE(NEW.description, '');
    NEW.record_hash := pg_catalog.encode(pg_catalog.sha256(payload_text::bytea), 'hex');

    -- Advance chain head atomically
    UPDATE public.tenant_audit_chain_heads
    SET last_hash = NEW.record_hash,
        sequence_num = NEW.sequence_num,
        last_log_id = NEW.id,
        updated_at = NEW.created_at
    WHERE tenant_id = NEW.tenant_id;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS contract_audit_hash_chain_trigger ON contract_audit_logs;
CREATE TRIGGER contract_audit_hash_chain_trigger
BEFORE INSERT ON contract_audit_logs
FOR EACH ROW EXECUTE FUNCTION compute_contract_audit_hash_chain();

-- 6. DATABASE ROLE PRIVILEGE SEPARATION
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'clm_app_role') THEN
        REVOKE UPDATE, DELETE, TRUNCATE ON contract_audit_logs FROM clm_app_role;
        GRANT SELECT, INSERT ON contract_audit_logs TO clm_app_role;
        GRANT SELECT, INSERT, UPDATE ON tenant_audit_chain_heads TO clm_app_role;
    END IF;
END $$;
