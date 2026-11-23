package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
)

// ErrUserNotFound is returned when no user matches the lookup. It wraps
// core.ErrNotFound so service code can distinguish a genuine miss from a
// database failure rather than reporting every error as "not found".
var ErrUserNotFound = fmt.Errorf("user: %w", core.ErrNotFound)

// UserRepository implements core.UserRepository with PostgreSQL.
type UserRepository struct {
	db *DB
}

// NewUserRepository creates a new user repository.
func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

// userColumns is shared by the single-row reads so the SELECT list and the Scan
// targets cannot drift apart.
//
// password_hash is nullable — an SSO-provisioned user has no local password — and
// models.User.PasswordHash is a plain string, so it is coalesced to an empty
// string. Without this, reading any passwordless user fails with "converting NULL
// to string is unsupported", which on the login path surfaces as invalid
// credentials rather than as the database error it actually is.
const userColumns = `id, tenant_id, email, name, role, status, mfa_enabled,
		COALESCE(password_hash, ''), last_login_at, created_at, updated_at`

// scanUser reads one user row.
func scanUser(row interface{ Scan(...any) error }) (*models.User, error) {
	u := &models.User{}
	err := row.Scan(
		&u.ID, &u.TenantID, &u.Email, &u.Name, &u.Role,
		&u.Status, &u.MFAEnabled, &u.PasswordHash, &u.LastLoginAt,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		// Never return a half-populated user alongside an error; callers that
		// only nil-check would treat it as a real record.
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	query := `INSERT INTO users (id, tenant_id, email, name, role, status, mfa_enabled, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	return r.db.WithTenantContext(ctx, user.TenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query,
			user.ID, user.TenantID, user.Email, user.Name, user.Role,
			user.Status, user.MFAEnabled, user.PasswordHash, user.CreatedAt, user.UpdatedAt,
		)
		return err
	})
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1`
	if tenantID, ok := core.TenantIDFromContext(ctx); ok {
		return r.GetByIDForTenant(ctx, tenantID, id)
	}
	if !core.HasPlatformAccess(ctx) {
		return nil, errors.New("tenant context is required for user lookup")
	}
	var user *models.User
	err := r.db.WithPlatformContext(ctx, func(tx *sql.Tx) error {
		var err error
		user, err = scanUser(tx.QueryRowContext(ctx, query, id))
		return err
	})
	return user, err
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE email = $1`
	var user *models.User
	err := r.db.WithUserAuthLookup(ctx, func(tx *sql.Tx) error {
		var err error
		user, err = scanUser(tx.QueryRowContext(ctx, query, email))
		return err
	})
	return user, err
}

func (r *UserRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*models.User, error) {
	query := `SELECT id, tenant_id, email, name, role, status, mfa_enabled, last_login_at, created_at, updated_at
		FROM users WHERE tenant_id = $1 ORDER BY created_at DESC`

	var usersList []*models.User
	err := r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			u := &models.User{}
			if err := rows.Scan(
				&u.ID, &u.TenantID, &u.Email, &u.Name, &u.Role,
				&u.Status, &u.MFAEnabled, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
			); err != nil {
				return err
			}
			usersList = append(usersList, u)
		}
		return rows.Err()
	})
	return usersList, err
}

func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	query := `UPDATE users SET name=$1, role=$2, status=$3, mfa_enabled=$4, last_login_at=$5, updated_at=$6
		WHERE id = $7`

	return r.db.WithTenantContext(ctx, user.TenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query,
			user.Name, user.Role, user.Status, user.MFAEnabled, user.LastLoginAt, user.UpdatedAt, user.ID,
		)
		return err
	})
}

func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if tenantID, ok := core.TenantIDFromContext(ctx); ok {
		return r.DeleteForTenant(ctx, tenantID, id)
	}
	if !core.HasPlatformAccess(ctx) {
		return errors.New("tenant context is required for user deletion")
	}
	return r.db.WithPlatformContext(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
		return err
	})
}

func (r *UserRepository) GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE tenant_id = $1 AND id = $2`
	var user *models.User
	err := r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		var err error
		user, err = scanUser(tx.QueryRowContext(ctx, query, tenantID, id))
		return err
	})
	return user, err
}

func (r *UserRepository) ListAll(ctx context.Context) ([]*models.User, error) {
	query := `SELECT id, tenant_id, email, name, role, status, mfa_enabled, last_login_at, created_at, updated_at FROM users ORDER BY created_at DESC`
	var result []*models.User
	err := r.db.WithPlatformContext(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			u := &models.User{}
			if err := rows.Scan(&u.ID, &u.TenantID, &u.Email, &u.Name, &u.Role, &u.Status, &u.MFAEnabled, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
				return err
			}
			result = append(result, u)
		}
		return rows.Err()
	})
	return result, err
}

func (r *UserRepository) UpdateForTenant(ctx context.Context, tenantID uuid.UUID, user *models.User) error {
	return r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE users SET name=$1, role=$2, status=$3, mfa_enabled=$4, last_login_at=$5, updated_at=$6 WHERE tenant_id=$7 AND id=$8`, user.Name, user.Role, user.Status, user.MFAEnabled, user.LastLoginAt, user.UpdatedAt, tenantID, user.ID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrUserNotFound
		}
		return nil
	})
}

func (r *UserRepository) DeleteForTenant(ctx context.Context, tenantID, id uuid.UUID) error {
	return r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE tenant_id=$1 AND id=$2`, tenantID, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrUserNotFound
		}
		return nil
	})
}
