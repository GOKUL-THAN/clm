package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/config"
)

// DB wraps the sql.DB connection.
type DB struct {
	*sql.DB
}

// NewDB creates a new PostgreSQL connection.
func NewDB(cfg *config.DatabaseConfig) (*DB, error) {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &DB{db}, nil
}

// WithTenantContext executes a query or transaction scoped strictly to the current tenant session
// using transaction-local set_config('app.current_tenant_id', $1, true) with parameterized execution
// to guarantee zero connection-pool leakage across tenant transactions.
func (db *DB) WithTenantContext(ctx context.Context, tenantID uuid.UUID, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTenantTransaction(ctx, tenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// BeginTenantTransaction starts a transaction with the restricted database role
// and transaction-local tenant context. The caller must commit or roll it back.
func (db *DB) BeginTenantTransaction(ctx context.Context, tenantID uuid.UUID) (*sql.Tx, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant isolation violation: tenantID cannot be nil")
	}
	requestTenant, hasTenant := core.TenantIDFromContext(ctx)
	if !hasTenant && !core.HasPlatformAccess(ctx) {
		return nil, fmt.Errorf("tenant isolation violation: authenticated tenant context is required")
	}
	if hasTenant && requestTenant != tenantID && !core.HasPlatformAccess(ctx) {
		return nil, fmt.Errorf("tenant isolation violation: request tenant %s does not match query tenant %s", requestTenant, tenantID)
	}

	return db.beginLocalTransaction(ctx, "app.current_tenant_id", tenantID.String())
}

func (db *DB) WithPlatformContext(ctx context.Context, fn func(tx *sql.Tx) error) error {
	if !core.HasPlatformAccess(ctx) {
		return fmt.Errorf("tenant isolation violation: platform database access is not authorized")
	}
	return db.withLocalContext(ctx, "app.platform_access", "on", fn)
}

func (db *DB) WithTenantProvisioningContext(ctx context.Context, fn func(tx *sql.Tx) error) error {
	if !core.HasTenantProvisioning(ctx) {
		return fmt.Errorf("tenant isolation violation: tenant provisioning is not authorized")
	}
	return db.withLocalContext(ctx, "app.tenant_provisioning", "on", fn)
}

func (db *DB) WithUserAuthLookup(ctx context.Context, fn func(tx *sql.Tx) error) error {
	return db.withLocalContext(ctx, "app.user_auth_lookup", "on", fn)
}

func (db *DB) WithSatelliteAuthLookup(ctx context.Context, fn func(tx *sql.Tx) error) error {
	return db.withLocalContext(ctx, "app.satellite_auth_lookup", "on", fn)
}

func (db *DB) withLocalContext(ctx context.Context, setting, value string, fn func(tx *sql.Tx) error) error {
	tx, err := db.beginLocalTransaction(ctx, setting, value)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) beginLocalTransaction(ctx context.Context, setting, value string) (*sql.Tx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting tenant-scoped transaction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "SET LOCAL ROLE clm_app_role"); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("setting restricted application database role: %w", err)
	}
	var setVal string
	if setting == "app.current_tenant_id" {
		if err := tx.QueryRowContext(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", value).Scan(&setVal); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("setting transaction-local tenant context: %w", err)
		}
	} else {
		if err := tx.QueryRowContext(ctx, "SELECT set_config($1, $2, true)", setting, value).Scan(&setVal); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("setting transaction-local database context: %w", err)
		}
	}
	return tx, nil
}

func (db *DB) VerifyApplicationRole(ctx context.Context) error {
	var exists, appIsSuperuser, appBypassRLS, sessionIsSuperuser, sessionBypassRLS, member, privilegedMembership bool
	err := db.QueryRowContext(ctx, `
		WITH session_role AS (
			SELECT oid, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = session_user
		)
		SELECT EXISTS (
			SELECT 1 FROM pg_roles WHERE rolname = 'clm_app_role'
		),
		COALESCE((SELECT rolsuper FROM pg_roles WHERE rolname = 'clm_app_role'), true),
		COALESCE((SELECT rolbypassrls FROM pg_roles WHERE rolname = 'clm_app_role'), true),
		COALESCE((SELECT rolsuper FROM session_role), true),
		COALESCE((SELECT rolbypassrls FROM session_role), true),
		pg_has_role(session_user, 'clm_app_role', 'MEMBER'),
		EXISTS (
			SELECT 1 FROM pg_roles r
			WHERE (r.rolsuper OR r.rolbypassrls)
				AND pg_has_role(session_user, r.oid, 'MEMBER')
		)
	`).Scan(&exists, &appIsSuperuser, &appBypassRLS, &sessionIsSuperuser, &sessionBypassRLS, &member, &privilegedMembership)
	if err != nil {
		return fmt.Errorf("checking tenant-restricted database role: %w", err)
	}
	if !exists || appIsSuperuser || appBypassRLS || !member ||
		sessionIsSuperuser || sessionBypassRLS || privilegedMembership {
		return fmt.Errorf("database session is missing clm_app_role membership or has superuser/BYPASSRLS privileges")
	}
	return nil
}
