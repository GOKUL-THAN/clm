package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
)

func TestTenantTablesHaveForcedRLS(t *testing.T) {
	skipIfNoDB(t)

	rows, err := testDB.QueryContext(context.Background(), `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity,
			EXISTS (SELECT 1 FROM pg_policies p
				WHERE p.schemaname = n.nspname AND p.tablename = c.relname)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
			AND c.relkind IN ('r', 'p')
			AND (
				c.relname = 'tenants'
				OR EXISTS (
					SELECT 1 FROM pg_attribute a
					WHERE a.attrelid = c.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped
				)
				OR c.relname = 'satellite_commands'
				OR EXISTS (
					SELECT 1 FROM pg_constraint fk
					WHERE fk.conrelid = c.oid AND fk.contype = 'f'
						AND fk.confrelid = 'public.contracts'::regclass
				)
			)
		ORDER BY c.relname`)
	if err != nil {
		t.Fatalf("query tenant table RLS inventory: %v", err)
	}
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var table string
		var enabled, forced, hasPolicy bool
		if err := rows.Scan(&table, &enabled, &forced, &hasPolicy); err != nil {
			t.Fatalf("scan tenant table RLS inventory: %v", err)
		}
		checked++
		if !enabled || !forced || !hasPolicy {
			t.Errorf("%s RLS state: enabled=%t forced=%t policy=%t; want all true", table, enabled, forced, hasPolicy)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tenant table RLS inventory: %v", err)
	}
	if checked == 0 {
		t.Fatal("tenant table RLS inventory returned no tables")
	}

	var roleExists, isSuperuser, bypassRLS bool
	if err := testDB.QueryRowContext(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'clm_app_role'),
			COALESCE((SELECT rolsuper FROM pg_roles WHERE rolname = 'clm_app_role'), true),
			COALESCE((SELECT rolbypassrls FROM pg_roles WHERE rolname = 'clm_app_role'), true)
	`).Scan(&roleExists, &isSuperuser, &bypassRLS); err != nil {
		t.Fatalf("check clm_app_role: %v", err)
	}
	if !roleExists || isSuperuser || bypassRLS {
		t.Fatalf("clm_app_role flags: exists=%t superuser=%t bypassrls=%t", roleExists, isSuperuser, bypassRLS)
	}
}

func TestRLSBlocksCrossTenantContractAndUserAccess(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantA := seedTenant(t, ctx, "rls-a-"+uuid.NewString()[:8])
	tenantB := seedTenant(t, ctx, "rls-b-"+uuid.NewString()[:8])

	userA, userB := uuid.New(), uuid.New()
	for _, row := range []struct {
		id       uuid.UUID
		tenantID uuid.UUID
		email    string
	}{
		{userA, tenantA, uuid.NewString() + "@example.invalid"},
		{userB, tenantB, uuid.NewString() + "@example.invalid"},
	} {
		tenantCtx := core.WithTenantID(ctx, row.tenantID)
		if err := testDB.WithTenantContext(tenantCtx, row.tenantID, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(tenantCtx, `
				INSERT INTO users (id, tenant_id, email, name, role, status, password_hash)
				VALUES ($1, $2, $3, 'RLS test', 'viewer', 'active', 'test')`,
				row.id, row.tenantID, row.email)
			return err
		}); err != nil {
			t.Fatalf("seed user for tenant %s: %v", row.tenantID, err)
		}
	}

	contractA, contractB := uuid.New(), uuid.New()
	for _, row := range []struct {
		id       uuid.UUID
		tenantID uuid.UUID
	}{
		{contractA, tenantA},
		{contractB, tenantB},
	} {
		tenantCtx := core.WithTenantID(ctx, row.tenantID)
		if err := testDB.WithTenantContext(tenantCtx, row.tenantID, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(tenantCtx, `
				INSERT INTO contracts (id, tenant_id, contract_number, title, counterparty_name)
				VALUES ($1, $2, $3, 'RLS test contract', 'RLS test counterparty')`,
				row.id, row.tenantID, "rls-"+uuid.NewString())
			return err
		}); err != nil {
			t.Fatalf("seed contract for tenant %s: %v", row.tenantID, err)
		}
	}
	for _, contractID := range []uuid.UUID{contractA, contractB} {
		tenantID := tenantA
		if contractID == contractB {
			tenantID = tenantB
		}
		tenantCtx := core.WithTenantID(ctx, tenantID)
		if err := testDB.WithTenantContext(tenantCtx, tenantID, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(tenantCtx, `
				INSERT INTO contract_versions (id, contract_id, version_number, title, content_body)
				VALUES ($1, $2, 1, 'RLS test version', 'test')`,
				uuid.New(), contractID)
			return err
		}); err != nil {
			t.Fatalf("seed version for contract %s: %v", contractID, err)
		}
	}
	satelliteA, satelliteB := uuid.New(), uuid.New()
	for _, row := range []struct {
		id       uuid.UUID
		tenantID uuid.UUID
	}{
		{satelliteA, tenantA},
		{satelliteB, tenantB},
	} {
		tenantCtx := core.WithTenantID(ctx, row.tenantID)
		if err := testDB.WithTenantContext(tenantCtx, row.tenantID, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(tenantCtx, `
				INSERT INTO satellites (id, tenant_id, name) VALUES ($1, $2, 'RLS test satellite')`,
				row.id, row.tenantID)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(tenantCtx, `
				INSERT INTO satellite_commands (id, tenant_id, satellite_id, type, payload, idempotency_key)
				VALUES ($1, $2, $3, 'RLS test', '{}'::jsonb, $4)`,
				uuid.New(), row.tenantID, row.id, uuid.NewString())
			return err
		}); err != nil {
			t.Fatalf("seed satellite/command for tenant %s: %v", row.tenantID, err)
		}
	}

	tenantCtx := core.WithTenantID(ctx, tenantA)
	err := testDB.WithTenantContext(tenantCtx, tenantA, func(tx *sql.Tx) error {
		var visibleUsers, visibleContracts, visibleVersions, visibleCommands, visibleCrossTenantUser int
		if err := tx.QueryRowContext(tenantCtx, `SELECT count(*) FROM users`).Scan(&visibleUsers); err != nil {
			return err
		}
		if err := tx.QueryRowContext(tenantCtx, `SELECT count(*) FROM users WHERE id=$1`, userB).Scan(&visibleCrossTenantUser); err != nil {
			return err
		}
		if err := tx.QueryRowContext(tenantCtx, `SELECT count(*) FROM contracts`).Scan(&visibleContracts); err != nil {
			return err
		}
		if err := tx.QueryRowContext(tenantCtx, `SELECT count(*) FROM contract_versions`).Scan(&visibleVersions); err != nil {
			return err
		}
		if err := tx.QueryRowContext(tenantCtx, `SELECT count(*) FROM satellite_commands`).Scan(&visibleCommands); err != nil {
			return err
		}
		if visibleUsers != 1 || visibleContracts != 1 || visibleVersions != 1 || visibleCommands != 1 || visibleCrossTenantUser != 0 {
			return fmt.Errorf("tenant A sees users=%d contracts=%d versions=%d commands=%d and tenant B users=%d; want 1, 1, 1, 1, and 0",
				visibleUsers, visibleContracts, visibleVersions, visibleCommands, visibleCrossTenantUser)
		}

		result, err := tx.ExecContext(tenantCtx, `UPDATE contracts SET title='cross-tenant write' WHERE id=$1`, contractB)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 0 {
			return fmt.Errorf("cross-tenant contract update affected %d rows", affected)
		}
		result, err = tx.ExecContext(tenantCtx, `DELETE FROM users WHERE id=$1`, userB)
		if err != nil {
			return err
		}
		affected, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 0 {
			return fmt.Errorf("cross-tenant user delete affected %d rows", affected)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tenant A RLS read/update check: %v", err)
	}

	if err := testDB.WithTenantContext(tenantCtx, tenantA, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(tenantCtx, `
			INSERT INTO contract_risks (id, tenant_id, contract_id, risk_code, category, title)
			VALUES ($1, $2, $3, 'RLS', 'security', 'cross-tenant risk')`,
			uuid.New(), tenantB, contractB)
		return err
	}); err == nil {
		t.Fatal("tenant A inserted a risk owned by tenant B")
	}
}

func TestWithTenantContextRejectsMissingOrMismatchedIdentity(t *testing.T) {
	db := &DB{}
	tenantA, tenantB := uuid.New(), uuid.New()
	called := false
	fn := func(*sql.Tx) error {
		called = true
		return nil
	}
	if err := db.WithTenantContext(context.Background(), tenantA, fn); err == nil {
		t.Fatal("tenant context without authenticated identity was accepted")
	}
	if err := db.WithTenantContext(core.WithTenantID(context.Background(), tenantA), tenantB, fn); err == nil {
		t.Fatal("mismatched authenticated tenant context was accepted")
	}
	if called {
		t.Fatal("database callback ran after a tenant context validation failure")
	}
}

func TestTenantProvisioningRequiresExplicitContext(t *testing.T) {
	db := &DB{}
	called := false
	err := db.WithTenantProvisioningContext(context.Background(), func(*sql.Tx) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("tenant provisioning without explicit registration context was accepted")
	}
	if called {
		t.Fatal("database callback ran without explicit tenant provisioning context")
	}
}

func TestTenantProvisioningCanInsertWithoutTenantReadAccess(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	repo := NewTenantRepository(testDB)
	id := uuid.New()
	slug := "tenant-registration-" + uuid.NewString()
	defer func() {
		if err := testDB.WithPlatformContext(core.WithPlatformAccess(ctx), func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, id)
			return err
		}); err != nil {
			t.Errorf("clean up registration test tenant: %v", err)
		}
	}()

	tenant := &models.Tenant{
		ID: id, Name: "Registration test", Slug: slug, Plan: "free", Status: "active",
		DataRegion: "test", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	registrationCtx := core.WithTenantProvisioning(ctx)
	if err := repo.Create(registrationCtx, tenant); err != nil {
		t.Fatalf("create tenant in registration context: %v", err)
	}
	if _, err := repo.GetByID(registrationCtx, id); err == nil {
		t.Fatal("registration context unexpectedly read a tenant")
	}

	duplicate := *tenant
	duplicate.ID = uuid.New()
	if err := repo.Create(registrationCtx, &duplicate); !errors.Is(err, core.ErrTenantExists) {
		t.Fatalf("duplicate slug error = %v, want core.ErrTenantExists", err)
	}
}
