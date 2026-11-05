-- ==============================================================================
-- scripts/test_postgres_table_owner_rls.sql
-- Verifies Table Owner behavior under FORCE ROW LEVEL SECURITY
-- Executed strictly as table owner: clm_table_owner (NOSUPERUSER, NOBYPASSRLS)
-- ==============================================================================

\set ON_ERROR_STOP on
\echo '>>> Starting Table Owner FORCE RLS Verification (clm_table_owner) <<<'

-- 1. Assert Table Owner Identity and Lack of Superuser/BypassRLS privileges
DO $$
DECLARE
    v_is_super boolean;
    v_bypass boolean;
    v_user text := current_user;
BEGIN
    SELECT rolsuper, rolbypassrls INTO v_is_super, v_bypass
    FROM pg_roles WHERE rolname = v_user;

    IF v_is_super THEN
        RAISE EXCEPTION 'TEST SECURITY FAILURE: Current user % is a SUPERUSER!', v_user;
    END IF;
    IF v_bypass THEN
        RAISE EXCEPTION 'TEST SECURITY FAILURE: Current user % has BYPASSRLS!', v_user;
    END IF;
    IF v_user <> 'clm_table_owner' THEN
        RAISE EXCEPTION 'TEST SECURITY FAILURE: Current user is % instead of clm_table_owner!', v_user;
    END IF;
    RAISE NOTICE 'CONFIRMED: Connected as table owner: % (rolsuper=%, rolbypassrls=%)', v_user, v_is_super, v_bypass;
END $$;

-- Normalize any connection-level setting so the missing-context checks are deterministic.
SELECT set_config('app.current_tenant_id', '', false);

-- 2. Verify FORCE ROW LEVEL SECURITY protects against Table Owner without tenant context
DO $$
DECLARE
    table_name text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY['contracts', 'approval_requests', 'approval_rules', 'certificates'] LOOP
        IF NOT EXISTS (
            SELECT 1
            FROM pg_class c
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

BEGIN;
-- Without setting app.current_tenant_id, even table owner sees 0 rows
DO $$
DECLARE
    visible_count integer;
BEGIN
    SELECT count(*) INTO visible_count
    FROM contracts
    WHERE id = 'c0000000-0000-0000-0000-000000000001';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Table owner saw % contracts without tenant context', visible_count;
    END IF;

    SELECT count(*) INTO visible_count
    FROM approval_requests
    WHERE id = 'd0000000-0000-0000-0000-000000000001';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Table owner saw % approval requests without tenant context', visible_count;
    END IF;

    SELECT count(*) INTO visible_count
    FROM approval_rules
    WHERE id = 'e0000000-0000-0000-0000-000000000011';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Table owner saw % approval rules without tenant context', visible_count;
    END IF;

    SELECT count(*) INTO visible_count
    FROM certificates
    WHERE id = 'f0000000-0000-0000-0000-000000000001';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Table owner saw % certificates without tenant context', visible_count;
    END IF;
END $$;
ROLLBACK;

-- A positive control ensures the owner can see data in the selected tenant.
BEGIN;
SELECT set_config('app.current_tenant_id', 'a0000000-0000-0000-0000-000000000001', true);
DO $$
DECLARE
    visible_count integer;
BEGIN
    SELECT count(*) INTO visible_count
    FROM contracts
    WHERE id = 'c0000000-0000-0000-0000-000000000001';
    IF visible_count <> 1 THEN
        RAISE EXCEPTION 'Table owner could not read the selected tenant contract';
    END IF;

    SELECT count(*) INTO visible_count
    FROM approval_requests
    WHERE id = 'd0000000-0000-0000-0000-000000000001';
    IF visible_count <> 1 THEN
        RAISE EXCEPTION 'Table owner could not read the selected tenant approval';
    END IF;

    SELECT count(*) INTO visible_count
    FROM approval_rules
    WHERE id = 'e0000000-0000-0000-0000-000000000011';
    IF visible_count <> 1 THEN
        RAISE EXCEPTION 'Table owner could not read the selected tenant approval rule';
    END IF;

    SELECT count(*) INTO visible_count
    FROM certificates
    WHERE id = 'f0000000-0000-0000-0000-000000000001';
    IF visible_count <> 1 THEN
        RAISE EXCEPTION 'Table owner could not read the selected tenant certificate';
    END IF;
END $$;
ROLLBACK;

-- 3. Verify Table Owner under Tenant Beta context CANNOT see Tenant Alpha data
BEGIN;
SELECT set_config('app.current_tenant_id', 'b0000000-0000-0000-0000-000000000002', true);

DO $$
DECLARE
    visible_count integer;
    affected_count integer;
BEGIN
    SELECT count(*) INTO visible_count
    FROM contracts
    WHERE id = 'c0000000-0000-0000-0000-000000000001';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Table owner cross-tenant contract leak: % rows', visible_count;
    END IF;

    SELECT count(*) INTO visible_count
    FROM approval_requests
    WHERE id = 'd0000000-0000-0000-0000-000000000001';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Table owner cross-tenant approval leak: % rows', visible_count;
    END IF;

    SELECT count(*) INTO visible_count
    FROM approval_rules
    WHERE id = 'e0000000-0000-0000-0000-000000000011';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Table owner cross-tenant approval rule leak: % rows', visible_count;
    END IF;

    SELECT count(*) INTO visible_count
    FROM certificates
    WHERE id = 'f0000000-0000-0000-0000-000000000001';
    IF visible_count <> 0 THEN
        RAISE EXCEPTION 'Table owner cross-tenant certificate leak: % rows', visible_count;
    END IF;

    UPDATE certificates
    SET status = 'revoked'
    WHERE id = 'f0000000-0000-0000-0000-000000000001';
    GET DIAGNOSTICS affected_count = ROW_COUNT;
    IF affected_count <> 0 THEN
        RAISE EXCEPTION 'Table owner RLS allowed cross-tenant certificate update';
    END IF;

    DELETE FROM certificates
    WHERE id = 'f0000000-0000-0000-0000-000000000001';
    GET DIAGNOSTICS affected_count = ROW_COUNT;
    IF affected_count <> 0 THEN
        RAISE EXCEPTION 'Table owner RLS allowed cross-tenant certificate delete';
    END IF;

    BEGIN
        INSERT INTO certificates (id, tenant_id, domain, type, status, key_algorithm, environment)
        VALUES (
            'f0000000-0000-0000-0000-000000000004',
            'a0000000-0000-0000-0000-000000000001',
            'table-owner-cross-tenant.example.invalid', 'x509', 'pending', 'ECDSA-P256', 'test'
        );
        RAISE EXCEPTION 'Table owner RLS allowed cross-tenant certificate insert';
    EXCEPTION WHEN insufficient_privilege THEN
        RAISE NOTICE 'SUCCESS: table owner cross-tenant certificate insert blocked by RLS';
    END;
END $$;

ROLLBACK;

\echo '>>> Table Owner FORCE RLS Verification Successfully Defended <<<'
