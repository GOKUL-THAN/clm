package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
)

// TenantRepository implements core.TenantRepository with PostgreSQL.
type TenantRepository struct {
	db *DB
}

// NewTenantRepository creates a new tenant repository.
func NewTenantRepository(db *DB) *TenantRepository {
	return &TenantRepository{db: db}
}

func (r *TenantRepository) Create(ctx context.Context, t *models.Tenant) error {
	query := `INSERT INTO tenants (id, name, slug, plan, status, max_certs, max_satellites, max_users, sso_enabled, data_region, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err := r.db.ExecContext(ctx, query,
		t.ID, t.Name, t.Slug, t.Plan, t.Status, t.MaxCerts, t.MaxSatellites, t.MaxUsers,
		t.SSOEnabled, t.DataRegion, t.CreatedAt, t.UpdatedAt,
	)
	return err
}

// ErrTenantNotFound is returned when no tenant matches the lookup. It wraps
// core.ErrNotFound so service code can test for either.
var ErrTenantNotFound = fmt.Errorf("tenant: %w", core.ErrNotFound)

// tenantColumns is shared by every single-row read so the SELECT list and the
// Scan targets cannot drift apart.
//
// sso_provider is nullable and models.Tenant.SSOProvider is a plain string, so
// it is coalesced to an empty string. Without this, scanning any tenant that has
// no SSO provider fails with "converting NULL to string is unsupported".
const tenantColumns = `id, name, slug, plan, status, max_certs, max_satellites, max_users,
		sso_enabled, COALESCE(sso_provider, ''), data_region, created_at, updated_at`

// scanTenant reads one tenant row.
func scanTenant(row interface{ Scan(...any) error }) (*models.Tenant, error) {
	t := &models.Tenant{}
	err := row.Scan(
		&t.ID, &t.Name, &t.Slug, &t.Plan, &t.Status, &t.MaxCerts, &t.MaxSatellites, &t.MaxUsers,
		&t.SSOEnabled, &t.SSOProvider, &t.DataRegion, &t.CreatedAt, &t.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTenantNotFound
	}
	if err != nil {
		// Never hand back a half-populated tenant alongside an error; callers
		// that only check for nil would treat it as a real record.
		return nil, err
	}
	return t, nil
}

func (r *TenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Tenant, error) {
	query := `SELECT ` + tenantColumns + ` FROM tenants WHERE id = $1`
	return scanTenant(r.db.QueryRowContext(ctx, query, id))
}

func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*models.Tenant, error) {
	query := `SELECT ` + tenantColumns + ` FROM tenants WHERE slug = $1`
	return scanTenant(r.db.QueryRowContext(ctx, query, slug))
}

func (r *TenantRepository) List(ctx context.Context) ([]*models.Tenant, error) {
	query := `SELECT id, name, slug, plan, status, max_certs, max_satellites, max_users, sso_enabled, data_region, created_at, updated_at
		FROM tenants ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []*models.Tenant
	for rows.Next() {
		t := &models.Tenant{}
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Slug, &t.Plan, &t.Status, &t.MaxCerts, &t.MaxSatellites, &t.MaxUsers,
			&t.SSOEnabled, &t.DataRegion, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (r *TenantRepository) Update(ctx context.Context, t *models.Tenant) error {
	query := `UPDATE tenants SET name=$1, plan=$2, status=$3, max_certs=$4, max_satellites=$5, max_users=$6, sso_enabled=$7, updated_at=$8
		WHERE id = $9`

	_, err := r.db.ExecContext(ctx, query,
		t.Name, t.Plan, t.Status, t.MaxCerts, t.MaxSatellites, t.MaxUsers, t.SSOEnabled, t.UpdatedAt, t.ID,
	)
	return err
}

// tenantChildTables are every table with a foreign key onto tenants(id). All of
// them are declared NO ACTION, so a tenant cannot be removed until its rows in
// these tables are gone. Order matters: children before parents.
var tenantChildTables = []string{
	"cloud_account_idempotency",
	"cloud_accounts",
	"approval_requests",
	"approval_rules",
	"purchase_orders",
	"ca_accounts",
	"certificates",
	"satellites",
	"audit_log",
	"teams",
	"users",
}

// CountDependents reports how many rows in each child table belong to a tenant.
// Tables absent from this deployment are skipped rather than failing the count.
func (r *TenantRepository) CountDependents(ctx context.Context, id uuid.UUID) (map[string]int, error) {
	counts := make(map[string]int)
	for _, table := range tenantChildTables {
		// table comes from the fixed list above, never from user input.
		var n int
		err := r.db.QueryRowContext(ctx,
			`SELECT count(*) FROM `+table+` WHERE tenant_id = $1`, id).Scan(&n)
		if err != nil {
			if isUndefinedTable(err) {
				continue
			}
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		if n > 0 {
			counts[table] = n
		}
	}
	return counts, nil
}

// isUndefinedTable reports whether the error is Postgres 42P01 (undefined table).
func isUndefinedTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "42P01")
}

// Delete removes a tenant. It fails if any dependent rows remain, which is the
// safe default: the foreign keys are NO ACTION, so this surfaces a clear error
// instead of a raw constraint violation.
func (r *TenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrTenantNotFound
	}
	return nil
}

// DeleteCascade removes a tenant together with every row that references it, in
// a single transaction so a partial teardown cannot be left behind.
func (r *TenantRepository) DeleteCascade(ctx context.Context, id uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	for _, table := range tenantChildTables {
		// #nosec G202 -- table names come only from the package-private constant allow-list.
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM `+table+` WHERE tenant_id = $1`, id); err != nil {
			if isUndefinedTable(err) {
				continue
			}
			return fmt.Errorf("delete from %s: %w", table, err)
		}
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrTenantNotFound
	}

	return tx.Commit()
}
