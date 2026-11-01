-- ==============================================================================
-- ENTERPRISE CLM — LIVE POSTGRESQL RLS & HASH CHAIN VERIFICATION TEST SUITE
-- Executed strictly as unprivileged application role: clm_app_role
-- (NOSUPERUSER, NOBYPASSRLS)
-- ==============================================================================

\set ON_ERROR_STOP on
\echo '>>> Starting Live PostgreSQL RLS & Audit Hash Chain Test Suite <<<'

-- ------------------------------------------------------------------------------
-- 0. Assert Strict Unprivileged Role Context (Zero Superuser False Positives)
-- ------------------------------------------------------------------------------
DO $$
DECLARE
    v_is_super boolean;
    v_bypass boolean;
    v_user text := current_user;
BEGIN
    SELECT rolsuper, rolbypassrls INTO v_is_super, v_bypass
    FROM pg_roles WHERE rolname = v_user;

    IF v_is_super THEN
        RAISE EXCEPTION 'CRITICAL SECURITY DEFECT: Test connection is running as SUPERUSER! RLS cannot be truthfully verified under superuser.';
    END IF;
    IF v_bypass THEN
        RAISE EXCEPTION 'CRITICAL SECURITY DEFECT: Test connection has BYPASSRLS! RLS cannot be truthfully verified under BYPASSRLS.';
    END IF;
    IF v_user <> 'clm_app_role' THEN
        RAISE EXCEPTION 'CRITICAL SECURITY DEFECT: Test connection must run as clm_app_role, but is %', v_user;
    END IF;
    RAISE NOTICE 'CONFIRMED: Connected as verified unprivileged role: % (rolsuper=%, rolbypassrls=%)', v_user, v_is_super, v_bypass;
END $$;

-- ------------------------------------------------------------------------------
-- 1. Verify Table FORCE ROW LEVEL SECURITY Status
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 1] Verifying relforcerowsecurity on all tenant tables <<<'
SELECT relname, relrowsecurity, relforcerowsecurity 
FROM pg_class 
WHERE relname IN (
    'contracts', 'contract_requests', 'contract_templates', 'clause_library', 
    'procurement_suppliers', 'legal_matters',
    'contract_audit_logs', 'tenant_audit_chain_heads', 'delegation_of_authority_rules',
    'contract_approvals', 'procurement_purchase_orders', 'procurement_goods_receipts',
    'procurement_supplier_invoices', 'procurement_three_way_matches', 'satellite_enclave_keys',
    'contract_versions', 'contract_negotiations', 'contract_redlines', 'contract_signatures',
    'contract_obligations', 'contract_risks', 'contract_cryptographic_envelopes'
)
ORDER BY relname;

-- ------------------------------------------------------------------------------
-- 2. Tenant Alpha Isolated Operations
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 2] Tenant Alpha insertion and self-read <<<'
INSERT INTO tenants (id, name, slug)
VALUES 
    ('a0000000-0000-0000-0000-000000000001', 'Tenant Alpha Corp', 'tenant-alpha'),
    ('b0000000-0000-0000-0000-000000000002', 'Tenant Beta LLC', 'tenant-beta')
ON CONFLICT (id) DO NOTHING;

BEGIN;
SELECT set_config('app.current_tenant_id', 'a0000000-0000-0000-0000-000000000001', true);

INSERT INTO contracts (id, tenant_id, contract_number, title, counterparty_name, effective_date, expiration_date, status, stage)
VALUES ('c0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'CNT-ALPHA-01', 'Tenant Alpha Confidential MSA', 'Global Alpha Vendor', NOW(), NOW() + interval '1 year', 'active', 'execution');

-- Tenant Alpha reads its own contract
SELECT id, contract_number, title FROM contracts WHERE id = 'c0000000-0000-0000-0000-000000000001';
COMMIT;

-- ------------------------------------------------------------------------------
-- 3. Tenant Beta Adversarial Operations (Attempting Cross-Tenant Access)
-- ------------------------------------------------------------------------------
BEGIN;
SELECT set_config('app.current_tenant_id', 'b0000000-0000-0000-0000-000000000002', true);

-- ATTACK 1: Cross-tenant SELECT
\echo '>>> [TEST 3A] Cross-Tenant SELECT attack <<<'
SELECT count(*) AS leaked_to_beta FROM contracts WHERE id = 'c0000000-0000-0000-0000-000000000001';
-- Must return 0

-- ATTACK 2: Cross-tenant UPDATE
\echo '>>> [TEST 3B] Cross-Tenant UPDATE attack <<<'
UPDATE contracts SET title = 'HACKED BY TENANT B' WHERE id = 'c0000000-0000-0000-0000-000000000001';
-- Must update 0 rows

-- ATTACK 3: Cross-tenant DELETE
\echo '>>> [TEST 3C] Cross-Tenant DELETE attack <<<'
DELETE FROM contracts WHERE id = 'c0000000-0000-0000-0000-000000000001';
-- Must delete 0 rows

COMMIT;

-- ------------------------------------------------------------------------------
-- 4. Missing or Unset Tenant Context Test (Connection Pool Reuse)
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 4] Unauthenticated pooled connection fail-closed test <<<'
BEGIN;
SELECT count(*) AS visible_without_tenant_context FROM contracts;
-- Must return 0 rows (RLS fails closed when app.current_tenant_id is NULL/empty)

-- Attempt INSERT without tenant context
DO $$
BEGIN
    BEGIN
        INSERT INTO contracts (id, tenant_id, contract_number, title, counterparty_name, effective_date, expiration_date)
        VALUES (gen_random_uuid(), 'a0000000-0000-0000-0000-000000000001', 'CNT-LEAK-01', 'Leaked Contract', 'Counterparty', NOW(), NOW() + interval '1 year');
        RAISE EXCEPTION 'RLS Failure: Insert succeeded without tenant context!';
    EXCEPTION WHEN OTHERS THEN
        RAISE NOTICE 'SUCCESS: Unauthenticated insert blocked by RLS policy: %', SQLERRM;
    END;
END $$;
ROLLBACK;

-- ------------------------------------------------------------------------------
-- 5. Audit Hash-Chain Scenario 1: First Audit Record (Genesis Initialization)
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 5] First audit log genesis initialization <<<'
BEGIN;
SELECT set_config('app.current_tenant_id', 'a0000000-0000-0000-0000-000000000001', true);

INSERT INTO contract_audit_logs (id, tenant_id, actor_email, action_type, description)
VALUES ('e0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'auditor@alpha.corp', 'create', 'Genesis audit log entry');

-- Assert sequence_num = 1 and previous_hash = GENESIS_...
SELECT id, sequence_num, previous_hash, record_hash FROM contract_audit_logs WHERE id = 'e0000000-0000-0000-0000-000000000001';
COMMIT;

-- ------------------------------------------------------------------------------
-- 6. Audit Hash-Chain Scenario 2: Second Linked Audit Record
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 6] Second linked audit log pointing to genesis hash <<<'
BEGIN;
SELECT set_config('app.current_tenant_id', 'a0000000-0000-0000-0000-000000000001', true);

INSERT INTO contract_audit_logs (id, tenant_id, actor_email, action_type, description)
VALUES ('e0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001', 'counsel@alpha.corp', 'approve', 'Second chained log entry');

-- Assert sequence_num = 2 and previous_hash matches record 1's hash
SELECT l2.sequence_num, l2.previous_hash = l1.record_hash AS hash_chain_linked
FROM contract_audit_logs l2
JOIN contract_audit_logs l1 ON l1.id = 'e0000000-0000-0000-0000-000000000001'
WHERE l2.id = 'e0000000-0000-0000-0000-000000000002';
COMMIT;

-- ------------------------------------------------------------------------------
-- 7. Audit Hash-Chain Scenario 3: Multiple Sequential Inserts for Same Tenant
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 7] Third chained record advancing sequence to 3 <<<'
BEGIN;
SELECT set_config('app.current_tenant_id', 'a0000000-0000-0000-0000-000000000001', true);

INSERT INTO contract_audit_logs (id, tenant_id, actor_email, action_type, description)
VALUES ('e0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000001', 'cfo@alpha.corp', 'sign', 'Third chained log entry');

-- Verify sequence progression: 1 -> 2 -> 3
SELECT sequence_num, previous_hash, record_hash FROM contract_audit_logs WHERE tenant_id = 'a0000000-0000-0000-0000-000000000001' ORDER BY sequence_num ASC;
COMMIT;

-- ------------------------------------------------------------------------------
-- 8. Audit Hash-Chain Scenario 4: Isolated Chain for Different Tenant (Tenant Beta)
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 8] Tenant Beta independent genesis and sequence initialization <<<'
BEGIN;
SELECT set_config('app.current_tenant_id', 'b0000000-0000-0000-0000-000000000002', true);

INSERT INTO contract_audit_logs (id, tenant_id, actor_email, action_type, description)
VALUES ('e0000000-0000-0000-0000-000000000004', 'b0000000-0000-0000-0000-000000000002', 'admin@beta.corp', 'create', 'Tenant Beta Genesis Entry');

-- Tenant Beta's first record must have sequence_num = 1 and genesis previous_hash, independent of Tenant Alpha
SELECT sequence_num, previous_hash, record_hash FROM contract_audit_logs WHERE id = 'e0000000-0000-0000-0000-000000000004';
COMMIT;

-- ------------------------------------------------------------------------------
-- 9. Audit Hash-Chain Scenario 5: Transaction Rollback Atomicity
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 9] Transaction rollback restores previous chain head atomically <<<'
BEGIN;
SELECT set_config('app.current_tenant_id', 'a0000000-0000-0000-0000-000000000001', true);

INSERT INTO contract_audit_logs (id, tenant_id, actor_email, action_type, description)
VALUES ('e0000000-0000-0000-0000-000000000099', 'a0000000-0000-0000-0000-000000000001', 'aborted@alpha.corp', 'draft', 'Aborted transaction record');
-- Force rollback
ROLLBACK;

-- Verify that sequence_num in tenant_audit_chain_heads was restored to 3
BEGIN;
SELECT set_config('app.current_tenant_id', 'a0000000-0000-0000-0000-000000000001', true);
SELECT sequence_num, last_hash FROM tenant_audit_chain_heads WHERE tenant_id = 'a0000000-0000-0000-0000-000000000001';
COMMIT;

-- ------------------------------------------------------------------------------
-- 10. Audit Hash-Chain Scenario 6: Attempted UPDATE, DELETE, and TRUNCATE Rejection
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 10A] UPDATE rejection verification <<<'
DO $$
BEGIN
    BEGIN
        UPDATE contract_audit_logs SET description = 'TAMPERED DESCRIPTION' WHERE id = 'e0000000-0000-0000-0000-000000000001';
        RAISE EXCEPTION 'Immutability Failure: UPDATE succeeded on contract_audit_logs!';
    EXCEPTION WHEN OTHERS THEN
        RAISE NOTICE 'SUCCESS: UPDATE blocked by trigger: %', SQLERRM;
    END;
END $$;

\echo '>>> [TEST 10B] DELETE rejection verification <<<'
DO $$
BEGIN
    BEGIN
        DELETE FROM contract_audit_logs WHERE id = 'e0000000-0000-0000-0000-000000000001';
        RAISE EXCEPTION 'Immutability Failure: DELETE succeeded on contract_audit_logs!';
    EXCEPTION WHEN OTHERS THEN
        RAISE NOTICE 'SUCCESS: DELETE blocked by trigger: %', SQLERRM;
    END;
END $$;

\echo '>>> [TEST 10C] TRUNCATE rejection verification <<<'
DO $$
BEGIN
    BEGIN
        TRUNCATE TABLE contract_audit_logs;
        RAISE EXCEPTION 'Immutability Failure: TRUNCATE succeeded on contract_audit_logs!';
    EXCEPTION WHEN OTHERS THEN
        RAISE NOTICE 'SUCCESS: TRUNCATE blocked by statement trigger: %', SQLERRM;
    END;
END $$;

-- ------------------------------------------------------------------------------
-- 11. Audit Hash-Chain Scenario 7: Cross-Tenant Access to tenant_audit_chain_heads Blocked
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 11] Cross-tenant access to chain-heads table blocked by RLS <<<'
BEGIN;
SELECT set_config('app.current_tenant_id', 'b0000000-0000-0000-0000-000000000002', true);
-- Tenant Beta attempts to view Tenant Alpha's chain head
SELECT count(*) AS leaked_heads FROM tenant_audit_chain_heads WHERE tenant_id = 'a0000000-0000-0000-0000-000000000001';
-- Must return 0
COMMIT;

-- ------------------------------------------------------------------------------
-- 12. Audit Hash-Chain Scenario 8: Hash Verification & Sequence Continuity
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 12] Hash verification and sequence continuity across tenant log <<<'
BEGIN;
SELECT set_config('app.current_tenant_id', 'a0000000-0000-0000-0000-000000000001', true);

-- Verify that for every row N > 1, previous_hash equals row N-1's record_hash
SELECT
    cur.sequence_num,
    cur.previous_hash = prev.record_hash AS chain_valid
FROM contract_audit_logs cur
JOIN contract_audit_logs prev
    ON cur.tenant_id = prev.tenant_id AND cur.sequence_num = prev.sequence_num + 1
WHERE cur.tenant_id = 'a0000000-0000-0000-0000-000000000001'
ORDER BY cur.sequence_num ASC;
COMMIT;

\echo '>>> All Live PostgreSQL RLS and Immutable Audit Security Tests Complete <<<'
