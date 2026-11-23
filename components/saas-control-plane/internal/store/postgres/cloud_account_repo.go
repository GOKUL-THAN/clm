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

var ErrCloudAccountNotFound = fmt.Errorf("cloud account: %w", core.ErrNotFound)

type CloudAccountRepository struct{ db *DB }

func NewCloudAccountRepository(db *DB) *CloudAccountRepository {
	return &CloudAccountRepository{db: db}
}

const cloudAccountColumns = `id,tenant_id,name,provider,encryption_version,credential_version,credential_nonce,credential_ciphertext,created_at,updated_at`

func scanCloudAccount(row interface{ Scan(...any) error }) (*models.EncryptedCloudCredentials, error) {
	a := &models.EncryptedCloudCredentials{}
	err := row.Scan(&a.ID, &a.TenantID, &a.Name, &a.Provider, &a.EncryptionVersion, &a.CredentialVersion, &a.Nonce, &a.Ciphertext, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCloudAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *CloudAccountRepository) Create(ctx context.Context, a *models.EncryptedCloudCredentials) error {
	return r.db.WithTenantContext(ctx, a.TenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO cloud_accounts (`+cloudAccountColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, a.ID, a.TenantID, a.Name, a.Provider, a.EncryptionVersion, a.CredentialVersion, a.Nonce, a.Ciphertext, a.CreatedAt, a.UpdatedAt)
		return err
	})
}
func (r *CloudAccountRepository) GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.EncryptedCloudCredentials, error) {
	var account *models.EncryptedCloudCredentials
	err := r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		var err error
		account, err = scanCloudAccount(tx.QueryRowContext(ctx, `SELECT `+cloudAccountColumns+` FROM cloud_accounts WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		return err
	})
	return account, err
}
func (r *CloudAccountRepository) GetByNameForTenant(ctx context.Context, tenantID uuid.UUID, name string) (*models.EncryptedCloudCredentials, error) {
	var account *models.EncryptedCloudCredentials
	err := r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		var err error
		account, err = scanCloudAccount(tx.QueryRowContext(ctx, `SELECT `+cloudAccountColumns+` FROM cloud_accounts WHERE tenant_id=$1 AND name=$2`, tenantID, name))
		return err
	})
	return account, err
}
func (r *CloudAccountRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*models.CloudAccount, error) {
	var out []*models.CloudAccount
	err := r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id,tenant_id,name,provider,credential_version,created_at,updated_at FROM cloud_accounts WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a := &models.CloudAccount{}
			if err := rows.Scan(&a.ID, &a.TenantID, &a.Name, &a.Provider, &a.CredentialVersion, &a.CreatedAt, &a.UpdatedAt); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}
func (r *CloudAccountRepository) RenameForTenant(ctx context.Context, tenantID, id uuid.UUID, name string) error {
	return r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE cloud_accounts SET name=$1,updated_at=NOW() WHERE tenant_id=$2 AND id=$3`, name, tenantID, id)
		return affectedOrNotFound(res, err, ErrCloudAccountNotFound)
	})
}
func (r *CloudAccountRepository) RotateForTenant(ctx context.Context, tenantID, id uuid.UUID, version int, nonce, ciphertext []byte) error {
	return r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE cloud_accounts SET encryption_version=$1,credential_version=credential_version+1,credential_nonce=$2,credential_ciphertext=$3,updated_at=NOW() WHERE tenant_id=$4 AND id=$5`, version, nonce, ciphertext, tenantID, id)
		return affectedOrNotFound(res, err, ErrCloudAccountNotFound)
	})
}
func (r *CloudAccountRepository) DeleteForTenant(ctx context.Context, tenantID, id uuid.UUID) error {
	return r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM cloud_accounts WHERE tenant_id=$1 AND id=$2`, tenantID, id)
		return affectedOrNotFound(res, err, ErrCloudAccountNotFound)
	})
}
func affectedOrNotFound(res sql.Result, err, errorNotFound error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errorNotFound
	}
	return nil
}
func (r *CloudAccountRepository) CreateIdempotent(ctx context.Context, account *models.EncryptedCloudCredentials, key, requestHash string) (*models.EncryptedCloudCredentials, string, error) {
	var existing *models.EncryptedCloudCredentials
	var existingHash string
	err := r.db.WithTenantContext(ctx, account.TenantID, func(tx *sql.Tx) error {
		// Serialize requests for a tenant/key so replay checks and inserts are atomic.
		lockKey := account.TenantID.String() + "/" + key
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
			return err
		}
		existing = &models.EncryptedCloudCredentials{}
		err := tx.QueryRowContext(ctx, `SELECT a.`+strings.ReplaceAll(cloudAccountColumns, ",", ",a.")+`,i.request_hash FROM cloud_account_idempotency i JOIN cloud_accounts a ON a.id=i.cloud_account_id WHERE i.tenant_id=$1 AND i.idempotency_key=$2`, account.TenantID, key).Scan(&existing.ID, &existing.TenantID, &existing.Name, &existing.Provider, &existing.EncryptionVersion, &existing.CredentialVersion, &existing.Nonce, &existing.Ciphertext, &existing.CreatedAt, &existing.UpdatedAt, &existingHash)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		existing = nil
		if _, err := tx.ExecContext(ctx, `INSERT INTO cloud_accounts (`+cloudAccountColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, account.ID, account.TenantID, account.Name, account.Provider, account.EncryptionVersion, account.CredentialVersion, account.Nonce, account.Ciphertext, account.CreatedAt, account.UpdatedAt); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_account_idempotency (tenant_id,idempotency_key,request_hash,cloud_account_id) VALUES ($1,$2,$3,$4)`, account.TenantID, key, requestHash, account.ID)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	if existing != nil {
		return existing, existingHash, nil
	}
	return nil, "", nil
}
