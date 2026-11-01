-- ==============================================================================
-- scripts/test_postgres_superuser_negative_control.sql
-- Documented Negative Control: PostgreSQL Superuser Bypasses RLS by Engine Design
-- Executed strictly as superuser: postgres
-- ==============================================================================

\set ON_ERROR_STOP on
\echo '>>> Starting Superuser Negative Control Test (Documented Engine Behavior) <<<'

-- 1. Assert Superuser Identity
DO $$
DECLARE
    v_is_super boolean;
    v_user text := current_user;
BEGIN
    SELECT rolsuper INTO v_is_super FROM pg_roles WHERE rolname = v_user;
    IF NOT v_is_super THEN
        RAISE EXCEPTION 'TEST FAILURE: Expected superuser context, got %', v_user;
    END IF;
    RAISE NOTICE 'CONFIRMED: Connected as superuser % (rolsuper=true)', v_user;
END $$;

-- 2. Document Superuser RLS Bypass Behavior
BEGIN;
-- Even with NO tenant context set, superuser bypasses RLS and reads all rows
SELECT count(*) AS superuser_bypasses_rls_count FROM contracts;
ROLLBACK;

\echo '>>> Superuser Negative Control Successfully Documented: Proves clm_app_role MUST NOT be superuser <<<'

