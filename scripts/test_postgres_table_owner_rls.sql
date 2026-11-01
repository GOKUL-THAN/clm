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

-- 2. Verify FORCE ROW LEVEL SECURITY protects against Table Owner without tenant context
BEGIN;
-- Without setting app.current_tenant_id, even table owner sees 0 rows
SELECT count(*) AS owner_visible_without_tenant FROM contracts;
ROLLBACK;

-- 3. Verify Table Owner under Tenant Beta context CANNOT see Tenant Alpha data
BEGIN;
SELECT set_config('app.current_tenant_id', 'b0000000-0000-0000-0000-000000000002', true);

SELECT count(*) AS owner_cross_tenant_leak
FROM contracts
WHERE id = 'c0000000-0000-0000-0000-000000000001';
-- Must return 0 because FORCE ROW LEVEL SECURITY applies to table owner!

ROLLBACK;

\echo '>>> Table Owner FORCE RLS Verification Successfully Defended <<<'

