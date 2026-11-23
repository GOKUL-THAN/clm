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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (slug) DO NOTHING`

	create := func(run func(context.Context, func(*sql.Tx) error) error) error {
		return run(ctx, func(tx *sql.Tx) error {
			result, err := tx.ExecContext(ctx, query,
				t.ID, t.Name, t.Slug, t.Plan, t.Status, t.MaxCerts, t.MaxSatellites, t.MaxUsers,
				t.SSOEnabled, t.DataRegion, t.CreatedAt, t.UpdatedAt,
			)
			if err != nil {
				return err
			}
			affected, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if affected == 0 {
				return core.ErrTenantExists
			}
			return nil
		})
	}

	if core.HasPlatformAccess(ctx) {
		return create(r.db.WithPlatformContext)
	}
	if tenantID, ok := core.TenantIDFromContext(ctx); ok {
		if tenantID != t.ID {
			return errors.New("tenant isolation violation: tenant context does not match new tenant")
		}
		return create(func(ctx context.Context, fn func(*sql.Tx) error) error {
			return r.db.WithTenantContext(ctx, t.ID, fn)
		})
	}
	if core.HasTenantProvisioning(ctx) {
		return create(r.db.WithTenantProvisioningContext)
	}
	return errors.New("tenant isolation violation: tenant or platform context is required")
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
	var tenant *models.Tenant
	if tenantID, ok := core.TenantIDFromContext(ctx); ok {
		if tenantID != id {
			return nil, ErrTenantNotFound
		}
		err := r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
			var err error
			tenant, err = scanTenant(tx.QueryRowContext(ctx, query, id))
			return err
		})
		return tenant, err
	}
	err := r.db.WithPlatformContext(ctx, func(tx *sql.Tx) error {
		var err error
		tenant, err = scanTenant(tx.QueryRowContext(ctx, query, id))
		return err
	})
	return tenant, err
}

func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*models.Tenant, error) {
	query := `SELECT ` + tenantColumns + ` FROM tenants WHERE slug = $1`
	var tenant *models.Tenant
	err := r.db.WithPlatformContext(ctx, func(tx *sql.Tx) error {
		var err error
		tenant, err = scanTenant(tx.QueryRowContext(ctx, query, slug))
		return err
	})
	return tenant, err
}

func (r *TenantRepository) List(ctx context.Context) ([]*models.Tenant, error) {
	query := `SELECT id, name, slug, plan, status, max_certs, max_satellites, max_users, sso_enabled, data_region, created_at, updated_at
		FROM tenants ORDER BY created_at DESC`

	var tenants []*models.Tenant
	err := r.db.WithPlatformContext(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t := &models.Tenant{}
			if err := rows.Scan(
				&t.ID, &t.Name, &t.Slug, &t.Plan, &t.Status, &t.MaxCerts, &t.MaxSatellites, &t.MaxUsers,
				&t.SSOEnabled, &t.DataRegion, &t.CreatedAt, &t.UpdatedAt,
			); err != nil {
				return err
			}
			tenants = append(tenants, t)
		}
		return rows.Err()
	})
	return tenants, err
}

func (r *TenantRepository) Update(ctx context.Context, t *models.Tenant) error {
	query := `UPDATE tenants SET name=$1, plan=$2, status=$3, max_certs=$4, max_satellites=$5, max_users=$6, sso_enabled=$7, updated_at=$8
		WHERE id = $9`

	if tenantID, ok := core.TenantIDFromContext(ctx); ok {
		if tenantID != t.ID {
			return ErrTenantNotFound
		}
		return r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, query,
				t.Name, t.Plan, t.Status, t.MaxCerts, t.MaxSatellites, t.MaxUsers, t.SSOEnabled, t.UpdatedAt, t.ID,
			)
			return err
		})
	}
	return r.db.WithPlatformContext(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query,
			t.Name, t.Plan, t.Status, t.MaxCerts, t.MaxSatellites, t.MaxUsers, t.SSOEnabled, t.UpdatedAt, t.ID,
		)
		return err
	})
}

// tenantChildTables are mutable tenant tables with a foreign key onto tenants.
// Order matters: children before parents.
var tenantChildTables = []string{
	"cloud_account_idempotency",
	"cloud_accounts",
	"approval_requests",
	"approval_rules",
	"purchase_orders",
	"ca_accounts",
	"certificates",
	"satellites",
	"teams",
	"users",
}

var tenantImmutableChildTables = []string{"audit_log"}

// CountDependents reports how many rows in each child table belong to a tenant.
// Tables absent from this deployment are skipped rather than failing the count.
func (r *TenantRepository) CountDependents(ctx context.Context, id uuid.UUID) (map[string]int, error) {
	counts := make(map[string]int)
	err := r.db.WithPlatformContext(ctx, func(tx *sql.Tx) error {
		for _, table := range append(append([]string(nil), tenantChildTables...), tenantImmutableChildTables...) {
			// table comes from the fixed list above, never from user input.
			var n int
			err := tx.QueryRowContext(ctx,
				`SELECT count(*) FROM `+table+` WHERE tenant_id = $1`, id).Scan(&n)
			if err != nil {
				if isUndefinedTable(err) {
					continue
				}
				return fmt.Errorf("count %s: %w", table, err)
			}
			if n > 0 {
				counts[table] = n
			}
		}
		return nil
	})
	return counts, err
}

// isUndefinedTable reports whether the error is Postgres 42P01 (undefined table).
func isUndefinedTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "42P01")
}

// Delete removes a tenant. It fails if any dependent rows remain, which is the
// safe default: the foreign keys are NO ACTION, so this surfaces a clear error
// instead of a raw constraint violation.
func (r *TenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithPlatformContext(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, id)
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
	})
}

// DeleteCascade removes a tenant together with every row that references it, in
// a single transaction so a partial teardown cannot be left behind.
func (r *TenantRepository) DeleteCascade(ctx context.Context, id uuid.UUID) error {
	return r.db.WithPlatformContext(ctx, func(tx *sql.Tx) error {
		for _, table := range tenantImmutableChildTables {
			var count int
			if err := tx.QueryRowContext(ctx,
				`SELECT count(*) FROM `+table+` WHERE tenant_id = $1`, id).Scan(&count); err != nil {
				if isUndefinedTable(err) {
					continue
				}
				return fmt.Errorf("check immutable dependent rows in %s: %w", table, err)
			}
			if count > 0 {
				return fmt.Errorf("tenant has immutable audit records and cannot be hard-deleted")
			}
		}

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

		return nil
	})
}
