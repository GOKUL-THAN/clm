-- Certificate inventory contains tenant-owned metadata and must remain isolated
-- even when queries run as the table owner.
ALTER TABLE certificates ENABLE ROW LEVEL SECURITY;
ALTER TABLE certificates FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_certificates ON certificates;
CREATE POLICY tenant_isolation_certificates ON certificates
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);
