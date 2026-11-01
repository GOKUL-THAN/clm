-- ==============================================================================
-- scripts/setup_ci_postgres_roles.sql
-- Enterprise CLM Isolated Role Architecture Setup
-- Executed strictly by PostgreSQL superuser once at CI initialization
-- ==============================================================================

\set ON_ERROR_STOP on

\echo '>>> Initializing Enterprise CLM Role Architecture (Superuser Context) <<<'

-- 1. Table Owner Role: Owns all domain tables. NOSUPERUSER, NOBYPASSRLS.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'clm_table_owner') THEN
        CREATE ROLE clm_table_owner NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS LOGIN PASSWORD 'owner_ci_secret';
        RAISE NOTICE 'Role clm_table_owner created successfully.';
    END IF;
END $$;

-- 2. Migration Owner Role: Applies migrations. Member of clm_table_owner. NOSUPERUSER, NOBYPASSRLS.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'clm_migration_owner') THEN
        CREATE ROLE clm_migration_owner NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS LOGIN PASSWORD 'migration_ci_secret';
        RAISE NOTICE 'Role clm_migration_owner created successfully.';
    END IF;
END $$;
GRANT clm_table_owner TO clm_migration_owner;

-- 3. Application Runtime Role: Used by clm-server and tests. NOSUPERUSER, NOBYPASSRLS.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'clm_app_role') THEN
        CREATE ROLE clm_app_role NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS LOGIN PASSWORD 'app_ci_secret';
        RAISE NOTICE 'Role clm_app_role created successfully.';
    END IF;
END $$;

-- 4. Database & Schema Grants
GRANT CONNECT ON DATABASE clm_test TO clm_table_owner, clm_migration_owner, clm_app_role;
GRANT USAGE, CREATE ON SCHEMA public TO clm_table_owner, clm_migration_owner;
GRANT USAGE ON SCHEMA public TO clm_app_role;

-- 5. Default Privileges: Objects created by clm_table_owner grant minimal privileges to clm_app_role
ALTER DEFAULT PRIVILEGES FOR ROLE clm_table_owner IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO clm_app_role;

ALTER DEFAULT PRIVILEGES FOR ROLE clm_migration_owner IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO clm_app_role;

-- 6. Enable cryptographic and UUID extensions via superuser
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

\echo '>>> Enterprise CLM Role Setup Complete <<<'

