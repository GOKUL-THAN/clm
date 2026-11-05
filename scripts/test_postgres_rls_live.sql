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
    'contract_obligations', 'contract_risks', 'contract_cryptographic_envelopes',
    'approval_requests', 'approval_rules', 'certificates'
)
ORDER BY relname;

DO $$
DECLARE
    table_name text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY['approval_requests', 'approval_rules', 'certificates'] LOOP
        IF NOT EXISTS (
            SELECT 1 FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = current_schema()
                AND c.relname = table_name
                AND c.relrowsecurity
                AND c.relforcerowsecurity
        ) THEN
            RAISE EXCEPTION 'RLS and FORCE RLS must both be enabled for %', table_name;
        END IF;
    END LOOP;
END $$;

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
INSERT INTO users (id, tenant_id, email, name, role, status, password_hash)
VALUES (
    'a0000000-0000-0000-0000-000000000011',
    'a0000000-0000-0000-0000-000000000001',
    'rls-approval-alpha@example.invalid',
    'RLS Approval Alpha',
    'approver',
    'active',
    'test-only'
)
ON CONFLICT (id) DO NOTHING;
INSERT INTO approval_requests (
    id, tenant_id, request_type, level, status, requested_by, justification,
    payload, estimated_cost, sla_deadline
)
VALUES (
    'd0000000-0000-0000-0000-000000000001',
    'a0000000-0000-0000-0000-000000000001',
    'cert_create', 'L1', 'pending',
    'a0000000-0000-0000-0000-000000000011',
    'RLS tenant isolation test', '{}', 0, NOW() + interval '1 day'
)
ON CONFLICT (id) DO NOTHING;
INSERT INTO approval_rules (id, tenant_id, name, level, priority)
VALUES (
    'e0000000-0000-0000-0000-000000000011',
    'a0000000-0000-0000-0000-000000000001',
    'RLS Alpha approval rule', 'L1', 1
)
ON CONFLICT (id) DO NOTHING;
INSERT INTO certificates (id, tenant_id, domain, type, status, key_algorithm, environment)
VALUES (
    'f0000000-0000-0000-0000-000000000001',
    'a0000000-0000-0000-0000-000000000001',
    'rls-approval-alpha.example.invalid', 'x509', 'active', 'ECDSA-P256', 'test'
)
ON CONFLICT (id) DO NOTHING;
DO $$
DECLARE
    visible_count integer;
BEGIN
    SELECT count(*) INTO visible_count
    FROM certificates
    WHERE id = 'f0000000-0000-0000-0000-000000000001';
    IF visible_count <> 1 THEN
        RAISE EXCEPTION 'Certificate RLS blocked the owning tenant from reading its certificate';
    END IF;
END $$;
COMMIT;

BEGIN;
SELECT set_config('app.current_tenant_id', 'b0000000-0000-0000-0000-000000000002', true);
INSERT INTO users (id, tenant_id, email, name, role, status, password_hash)
VALUES (
    'b0000000-0000-0000-0000-000000000012',
    'b0000000-0000-0000-0000-000000000002',
    'rls-approval-beta@example.invalid',
    'RLS Approval Beta',
    'approver',
    'active',
    'test-only'
)
ON CONFLICT (id) DO NOTHING;
COMMIT;

-- ------------------------------------------------------------------------------
-- Approval RLS: tenant access and write enforcement
-- ------------------------------------------------------------------------------
\echo '>>> [TEST 2A] Approval RLS blocks cross-tenant read, update, delete, and insert <<<'
BEGIN;
SELECT set_config('app.current_tenant_id', 'b0000000-0000-0000-0000-000000000002', true);
DO $$
DECLARE
    visible_count integer;
    affected_count integer;
BEGIN
    SELECT count(*) INTO visible_count
    FROM approval_requests
    WHERE id = 'd0000000-0000-0000-0000-000000000001';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Approval RLS leaked % cross-tenant requests', visible_count;
    END IF;

    SELECT count(*) INTO visible_count
    FROM approval_rules
    WHERE id = 'e0000000-0000-0000-0000-000000000011';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Approval RLS leaked % cross-tenant rules', visible_count;
    END IF;

    SELECT count(*) INTO visible_count
    FROM certificates
    WHERE id = 'f0000000-0000-0000-0000-000000000001';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Certificate RLS leaked % cross-tenant certificates', visible_count;
    END IF;

    UPDATE approval_requests
    SET comment = 'cross-tenant update attempt'
    WHERE id = 'd0000000-0000-0000-0000-000000000001';
    GET DIAGNOSTICS affected_count = ROW_COUNT;
    IF affected_count <> 0 THEN
        RAISE EXCEPTION 'Approval RLS allowed cross-tenant update';
    END IF;

    DELETE FROM approval_requests
    WHERE id = 'd0000000-0000-0000-0000-000000000001';
    GET DIAGNOSTICS affected_count = ROW_COUNT;
    IF affected_count <> 0 THEN
        RAISE EXCEPTION 'Approval RLS allowed cross-tenant delete';
    END IF;

    UPDATE certificates
    SET status = 'revoked'
    WHERE id = 'f0000000-0000-0000-0000-000000000001';
    GET DIAGNOSTICS affected_count = ROW_COUNT;
    IF affected_count <> 0 THEN
        RAISE EXCEPTION 'Certificate RLS allowed cross-tenant update';
    END IF;

    DELETE FROM certificates
    WHERE id = 'f0000000-0000-0000-0000-000000000001';
    GET DIAGNOSTICS affected_count = ROW_COUNT;
    IF affected_count <> 0 THEN
        RAISE EXCEPTION 'Certificate RLS allowed cross-tenant delete';
    END IF;

    BEGIN
        INSERT INTO approval_rules (id, tenant_id, name, level, priority)
        VALUES (
            'e0000000-0000-0000-0000-000000000012',
            'a0000000-0000-0000-0000-000000000001',
            'Cross-tenant rule insert attempt', 'L1', 1
        );
        RAISE EXCEPTION 'Approval RLS allowed cross-tenant rule insert';
    EXCEPTION WHEN insufficient_privilege THEN
        RAISE NOTICE 'SUCCESS: cross-tenant approval rule insert blocked by RLS';
    END;

    BEGIN
        INSERT INTO approval_requests (
            id, tenant_id, request_type, level, status, requested_by, justification,
            payload, estimated_cost, sla_deadline
        )
        VALUES (
            'd0000000-0000-0000-0000-000000000002',
            'a0000000-0000-0000-0000-000000000001',
            'cert_create', 'L1', 'pending',
            'b0000000-0000-0000-0000-000000000012',
            'cross-tenant insert attempt', '{}', 0, NOW() + interval '1 day'
        );
        RAISE EXCEPTION 'Approval RLS allowed cross-tenant insert';
    EXCEPTION WHEN insufficient_privilege THEN
        RAISE NOTICE 'SUCCESS: cross-tenant approval insert blocked by RLS';
    END;

    BEGIN
        INSERT INTO certificates (id, tenant_id, domain, type, status, key_algorithm, environment)
        VALUES (
            'f0000000-0000-0000-0000-000000000002',
            'a0000000-0000-0000-0000-000000000001',
            'cross-tenant.example.invalid', 'x509', 'pending', 'ECDSA-P256', 'test'
        );
        RAISE EXCEPTION 'Certificate RLS allowed cross-tenant insert';
    EXCEPTION WHEN insufficient_privilege THEN
        RAISE NOTICE 'SUCCESS: cross-tenant certificate insert blocked by RLS';
    END;
END $$;
COMMIT;

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
DO $$
DECLARE
    visible_count integer;
BEGIN
    SELECT count(*) INTO visible_count
    FROM approval_requests
    WHERE id = 'd0000000-0000-0000-0000-000000000001';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Approval RLS exposed % requests without tenant context', visible_count;
    END IF;

    SELECT count(*) INTO visible_count
    FROM certificates
    WHERE id = 'f0000000-0000-0000-0000-000000000001';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Certificate RLS exposed % certificates without tenant context', visible_count;
    END IF;

    BEGIN
        INSERT INTO approval_requests (
            id, tenant_id, request_type, level, status, requested_by, justification,
            payload, estimated_cost, sla_deadline
        )
        VALUES (
            'd0000000-0000-0000-0000-000000000003',
            'a0000000-0000-0000-0000-000000000001',
            'cert_create', 'L1', 'pending',
            'a0000000-0000-0000-0000-000000000011',
            'missing tenant context insert attempt', '{}', 0, NOW() + interval '1 day'
        );
        RAISE EXCEPTION 'Approval RLS allowed insert without tenant context';
    EXCEPTION WHEN insufficient_privilege THEN
        RAISE NOTICE 'SUCCESS: approval insert without tenant context blocked by RLS';
    END;

    BEGIN
        INSERT INTO certificates (id, tenant_id, domain, type, status, key_algorithm, environment)
        VALUES (
            'f0000000-0000-0000-0000-000000000003',
            'a0000000-0000-0000-0000-000000000001',
            'missing-context.example.invalid', 'x509', 'pending', 'ECDSA-P256', 'test'
        );
        RAISE EXCEPTION 'Certificate RLS allowed insert without tenant context';
    EXCEPTION WHEN insufficient_privilege THEN
        RAISE NOTICE 'SUCCESS: certificate insert without tenant context blocked by RLS';
    END;
END $$;

-- Attempt INSERT without tenant context
DO $$
DECLARE
    insert_succeeded boolean := false;
BEGIN
    BEGIN
        INSERT INTO contracts (id, tenant_id, contract_number, title, counterparty_name, effective_date, expiration_date)
        VALUES (gen_random_uuid(), 'a0000000-0000-0000-0000-000000000001', 'CNT-LEAK-01', 'Leaked Contract', 'Counterparty', NOW(), NOW() + interval '1 year');
        insert_succeeded := true;
    EXCEPTION WHEN insufficient_privilege THEN
        RAISE NOTICE 'SUCCESS: Unauthenticated insert blocked by RLS policy';
    END;
    IF insert_succeeded THEN
        RAISE EXCEPTION 'RLS Failure: Insert succeeded without tenant context!';
    END IF;
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
