package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
)

// CertificateRepository implements core.CertificateRepository.
type CertificateRepository struct {
	db *DB
}

// NewCertificateRepository creates a new certificate repository.
func NewCertificateRepository(db *DB) *CertificateRepository {
	return &CertificateRepository{db: db}
}

const certificateColumns = `id, tenant_id, domain, sans, type, status, key_algorithm,
	COALESCE(issuer, ''), COALESCE(serial_number, ''), COALESCE(fingerprint, ''),
	not_before, not_after, environment, COALESCE(satellite_id, ''), COALESCE(secret_ref, ''),
	tags, created_by, renewal_count, last_deployed_at, approval_id, purchase_order_id, created_at, updated_at`

func scanCertificate(row rowScanner) (*models.Certificate, error) {
	cert := &models.Certificate{}
	var notBefore, notAfter *time.Time
	var createdBy *uuid.UUID
	err := row.Scan(
		&cert.ID, &cert.TenantID, &cert.Domain, pq.Array(&cert.SANs), &cert.Type, &cert.Status,
		&cert.KeyAlgorithm, &cert.Issuer, &cert.SerialNumber, &cert.Fingerprint,
		&notBefore, &notAfter, &cert.Environment, &cert.SatelliteID,
		&cert.SecretRef, pq.Array(&cert.Tags), &createdBy, &cert.RenewalCount,
		&cert.LastDeployedAt, &cert.ApprovalID, &cert.PurchaseOrderID, &cert.CreatedAt, &cert.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if notBefore != nil {
		cert.NotBefore = *notBefore
	}
	if notAfter != nil {
		cert.NotAfter = *notAfter
	}
	if createdBy != nil {
		cert.CreatedBy = *createdBy
	}
	return cert, nil
}

func (r *CertificateRepository) Create(ctx context.Context, cert *models.Certificate) error {
	query := `INSERT INTO certificates (id, tenant_id, domain, sans, type, status, key_algorithm, issuer, serial_number, fingerprint, not_before, not_after, environment, satellite_id, secret_ref, tags, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`

	_, err := r.db.ExecContext(ctx, query,
		cert.ID, cert.TenantID, cert.Domain, pq.Array(cert.SANs), cert.Type, cert.Status,
		cert.KeyAlgorithm, cert.Issuer, cert.SerialNumber, cert.Fingerprint,
		cert.NotBefore, cert.NotAfter, cert.Environment, cert.SatelliteID,
		cert.SecretRef, pq.Array(cert.Tags), cert.CreatedBy, cert.CreatedAt, cert.UpdatedAt,
	)
	return err
}

func (r *CertificateRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Certificate, error) {
	cert, err := scanCertificate(r.db.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificates WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrNotFound
	}
	return cert, err
}

func (r *CertificateRepository) GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.Certificate, error) {
	cert, err := scanCertificate(r.db.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificates WHERE tenant_id = $1 AND id = $2`, tenantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrNotFound
	}
	return cert, err
}

func (r *CertificateRepository) GetByDomain(ctx context.Context, tenantID uuid.UUID, domain string) ([]*models.Certificate, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+certificateColumns+` FROM certificates WHERE tenant_id = $1 AND domain = $2 ORDER BY created_at DESC`, tenantID, domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []*models.Certificate
	for rows.Next() {
		cert, err := scanCertificate(rows)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	return certs, rows.Err()
}

func (r *CertificateRepository) List(ctx context.Context, tenantID uuid.UUID, filter *core.CertFilter) ([]*models.Certificate, int, error) {
	query := `SELECT ` + certificateColumns + ` FROM certificates WHERE TRUE`
	countQuery := `SELECT COUNT(*) FROM certificates WHERE TRUE`
	args := make([]any, 0, 6)
	addFilter := func(condition string, value any) {
		condition = fmt.Sprintf(condition, len(args)+1)
		query += ` AND ` + condition
		countQuery += ` AND ` + condition
		args = append(args, value)
	}

	// uuid.Nil is reserved for trusted internal callers that need a cross-tenant view.
	if tenantID != uuid.Nil {
		addFilter(`tenant_id = $%d`, tenantID)
	}
	if filter != nil {
		if len(filter.Status) > 0 {
			statuses := make([]string, len(filter.Status))
			for i, status := range filter.Status {
				statuses[i] = string(status)
			}
			addFilter(`status = ANY($%d)`, pq.Array(statuses))
		}
		if filter.Environment != "" {
			addFilter(`environment = $%d`, filter.Environment)
		}
		if filter.SatelliteID != "" {
			addFilter(`satellite_id = $%d`, filter.SatelliteID)
		}
		if filter.ExpiringIn > 0 {
			addFilter(`not_after IS NOT NULL AND not_after <= NOW() + ($%d * INTERVAL '1 day')`, filter.ExpiringIn)
		}
	}

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += ` ORDER BY created_at DESC`
	queryArgs := append([]any(nil), args...)
	if filter != nil && filter.PageSize > 0 {
		page := filter.Page
		if page < 1 {
			page = 1
		}
		query += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(queryArgs)+1, len(queryArgs)+2)
		queryArgs = append(queryArgs, filter.PageSize, (page-1)*filter.PageSize)
	}

	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var certs []*models.Certificate
	for rows.Next() {
		cert, err := scanCertificate(rows)
		if err != nil {
			return nil, 0, err
		}
		certs = append(certs, cert)
	}
	return certs, total, rows.Err()
}

func (r *CertificateRepository) Update(ctx context.Context, cert *models.Certificate) error {
	query := `UPDATE certificates SET status=$1, issuer=$2, serial_number=$3, fingerprint=$4, not_before=$5, not_after=$6, secret_ref=$7,
		renewal_count=$8, last_deployed_at=$9, approval_id=$10, purchase_order_id=$11, updated_at=$12
		WHERE id=$13 AND tenant_id=$14`

	return requireAffected(r.db.ExecContext(ctx, query,
		cert.Status, cert.Issuer, cert.SerialNumber, cert.Fingerprint,
		cert.NotBefore, cert.NotAfter, cert.SecretRef, cert.RenewalCount,
		cert.LastDeployedAt, cert.ApprovalID, cert.PurchaseOrderID, cert.UpdatedAt,
		cert.ID, cert.TenantID,
	))
}

func (r *CertificateRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM certificates WHERE id = $1`, id)
	return err
}

func (r *CertificateRepository) CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM certificates WHERE tenant_id = $1`, tenantID).Scan(&count)
	return count, err
}
