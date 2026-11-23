-- Complete tenant isolation for core and CLM data. The application must enter
-- clm_app_role and set a transaction-local tenant context for tenant queries.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_roles
        WHERE rolname = 'clm_app_role'
            AND NOT rolcanlogin
            AND NOT rolsuper
            AND NOT rolbypassrls
            AND NOT rolcreatedb
            AND NOT rolcreaterole
            AND NOT rolreplication
    ) THEN
        RAISE EXCEPTION 'clm_app_role must be provisioned as a restricted NOLOGIN role before migrations';
    END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO clm_app_role;
ALTER TABLE public.tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.tenants FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS clm_tenant_isolation ON public.tenants;
CREATE POLICY clm_tenant_isolation ON public.tenants
    FOR SELECT
    USING (
        id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        OR current_setting('app.platform_access', true) = 'on'
    );
CREATE POLICY clm_tenant_write_isolation ON public.tenants
    FOR UPDATE
    USING (
        id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        OR current_setting('app.platform_access', true) = 'on'
    )
    WITH CHECK (
        id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        OR current_setting('app.platform_access', true) = 'on'
    );
CREATE POLICY clm_tenant_delete_isolation ON public.tenants
    FOR DELETE
    USING (
        id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        OR current_setting('app.platform_access', true) = 'on'
    );
DROP POLICY IF EXISTS clm_tenant_registration_insert ON public.tenants;
CREATE POLICY clm_tenant_registration_insert ON public.tenants
    FOR INSERT
    WITH CHECK (
        current_setting('app.tenant_provisioning', true) = 'on'
        AND current_setting('app.platform_access', true) != 'on'
    );
GRANT SELECT, INSERT, UPDATE, DELETE ON public.tenants TO clm_app_role;

-- Contract risks were previously permitted to have no parent contract, and
-- the old RLS policy explicitly exposed those unowned rows to every tenant.
ALTER TABLE contract_risks ADD COLUMN IF NOT EXISTS tenant_id UUID;
UPDATE contract_risks r
SET tenant_id = c.tenant_id
FROM contracts c
WHERE r.contract_id = c.id
  AND r.tenant_id IS NULL;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM contract_risks WHERE tenant_id IS NULL) THEN
        RAISE EXCEPTION 'cannot enable tenant RLS: contract_risks contains rows without an owning tenant; resolve these rows before retrying migration 011';
    END IF;
END
$$;

ALTER TABLE contract_risks ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE contract_risks
    ADD CONSTRAINT contract_risks_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_contracts_tenant_id_id
    ON contracts(tenant_id, id);
ALTER TABLE contract_risks
    ADD CONSTRAINT contract_risks_contract_tenant_fkey
    FOREIGN KEY (tenant_id, contract_id) REFERENCES contracts(tenant_id, id);

DO $$
DECLARE
    target_table TEXT;
    policy_name TEXT;
    direct_tables TEXT[] := ARRAY[
        'users', 'teams', 'satellites', 'certificates',
        'approval_requests', 'approval_rules', 'purchase_orders', 'ca_accounts',
        'audit_log', 'control_plane_audit_events', 'cloud_accounts',
        'cloud_account_idempotency', 'contracts', 'contract_requests',
        'contract_templates', 'clause_library', 'procurement_suppliers',
        'legal_matters', 'contract_audit_logs', 'delegation_of_authority_rules',
        'contract_approvals', 'procurement_purchase_orders',
        'procurement_goods_receipts', 'procurement_supplier_invoices',
        'procurement_three_way_matches', 'satellite_enclave_keys',
        'tenant_audit_chain_heads', 'contract_risks'
    ];
    child_tables TEXT[] := ARRAY[
        'contract_versions', 'contract_negotiations', 'contract_redlines',
        'contract_signatures', 'contract_obligations', 'contract_financials',
        'ai_contract_analyses', 'contract_cryptographic_envelopes'
    ];
    satellite_child_tables TEXT[] := ARRAY['satellite_commands'];
BEGIN
    FOREACH target_table IN ARRAY direct_tables LOOP
        EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', target_table);
        EXECUTE format('ALTER TABLE public.%I FORCE ROW LEVEL SECURITY', target_table);
        FOR policy_name IN
            SELECT policyname FROM pg_policies
            WHERE pg_policies.schemaname = 'public' AND pg_policies.tablename = target_table
        LOOP
            EXECUTE format('DROP POLICY %I ON public.%I', policy_name, target_table);
        END LOOP;
        EXECUTE format(
            'CREATE POLICY clm_tenant_isolation ON public.%I FOR ALL ' ||
            'USING (tenant_id = NULLIF(current_setting(''app.current_tenant_id'', true), '''')::uuid ' ||
            'OR current_setting(''app.platform_access'', true) = ''on'') ' ||
            'WITH CHECK (tenant_id = NULLIF(current_setting(''app.current_tenant_id'', true), '''')::uuid ' ||
            'OR current_setting(''app.platform_access'', true) = ''on'')',
            target_table
        );
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO clm_app_role', target_table);
    END LOOP;

    FOREACH target_table IN ARRAY satellite_child_tables LOOP
        EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', target_table);
        EXECUTE format('ALTER TABLE public.%I FORCE ROW LEVEL SECURITY', target_table);
        FOR policy_name IN
            SELECT policyname FROM pg_policies
            WHERE pg_policies.schemaname = 'public' AND pg_policies.tablename = target_table
        LOOP
            EXECUTE format('DROP POLICY %I ON public.%I', policy_name, target_table);
        END LOOP;
        EXECUTE format(
            'CREATE POLICY clm_tenant_isolation ON public.%I FOR ALL ' ||
            'USING (current_setting(''app.platform_access'', true) = ''on'' OR EXISTS (' ||
            'SELECT 1 FROM public.satellites s WHERE s.id = %I.satellite_id ' ||
            'AND s.tenant_id = NULLIF(current_setting(''app.current_tenant_id'', true), '''')::uuid)) ' ||
            'WITH CHECK (current_setting(''app.platform_access'', true) = ''on'' OR EXISTS (' ||
            'SELECT 1 FROM public.satellites s WHERE s.id = %I.satellite_id ' ||
            'AND s.tenant_id = NULLIF(current_setting(''app.current_tenant_id'', true), '''')::uuid))',
            target_table, target_table, target_table
        );
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO clm_app_role', target_table);
    END LOOP;

    FOREACH target_table IN ARRAY child_tables LOOP
        EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', target_table);
        EXECUTE format('ALTER TABLE public.%I FORCE ROW LEVEL SECURITY', target_table);
        FOR policy_name IN
            SELECT policyname FROM pg_policies
            WHERE pg_policies.schemaname = 'public' AND pg_policies.tablename = target_table
        LOOP
            EXECUTE format('DROP POLICY %I ON public.%I', policy_name, target_table);
        END LOOP;
        EXECUTE format(
            'CREATE POLICY clm_tenant_isolation ON public.%I FOR ALL ' ||
            'USING (current_setting(''app.platform_access'', true) = ''on'' OR EXISTS (' ||
            'SELECT 1 FROM public.contracts c WHERE c.id = %I.contract_id ' ||
            'AND c.tenant_id = NULLIF(current_setting(''app.current_tenant_id'', true), '''')::uuid)) ' ||
            'WITH CHECK (current_setting(''app.platform_access'', true) = ''on'' OR EXISTS (' ||
            'SELECT 1 FROM public.contracts c WHERE c.id = %I.contract_id ' ||
            'AND c.tenant_id = NULLIF(current_setting(''app.current_tenant_id'', true), '''')::uuid))',
            target_table, target_table, target_table
        );
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO clm_app_role', target_table);
    END LOOP;

    -- Narrow read-only exceptions for pre-authentication identity lookup and
    -- satellite credential verification. Writes remain tenant scoped.
    DROP POLICY IF EXISTS clm_user_auth_lookup ON users;
    CREATE POLICY clm_user_auth_lookup ON users FOR SELECT
        USING (current_setting('app.user_auth_lookup', true) = 'on');
    DROP POLICY IF EXISTS clm_satellite_auth_lookup ON satellites;
    CREATE POLICY clm_satellite_auth_lookup ON satellites FOR SELECT
        USING (current_setting('app.satellite_auth_lookup', true) = 'on');

    GRANT SELECT, INSERT ON public.control_plane_audit_events TO clm_app_role;
    GRANT SELECT, INSERT ON public.contract_audit_logs TO clm_app_role;
    GRANT SELECT, INSERT ON public.audit_log TO clm_app_role;
    GRANT SELECT, INSERT, UPDATE ON public.tenant_audit_chain_heads TO clm_app_role;

END
$$;

GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO clm_app_role;

-- Restore append-only privileges after the broad table grant above.
REVOKE UPDATE, DELETE, TRUNCATE ON public.control_plane_audit_events FROM clm_app_role;
REVOKE UPDATE, DELETE, TRUNCATE ON public.contract_audit_logs FROM clm_app_role;
REVOKE UPDATE, DELETE, TRUNCATE ON public.audit_log FROM clm_app_role;
