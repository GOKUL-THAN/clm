package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
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

	return r.db.WithTenantContext(ctx, cert.TenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query,
			cert.ID, cert.TenantID, cert.Domain, pq.Array(cert.SANs), cert.Type, cert.Status,
			cert.KeyAlgorithm, cert.Issuer, cert.SerialNumber, cert.Fingerprint,
			cert.NotBefore, cert.NotAfter, cert.Environment, cert.SatelliteID,
			cert.SecretRef, pq.Array(cert.Tags), cert.CreatedBy, cert.CreatedAt, cert.UpdatedAt,
		)
		return err
	})
}

func (r *CertificateRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Certificate, error) {
	return r.GetByIDForTenant(ctx, tenantID, id)
}

func (r *CertificateRepository) GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.Certificate, error) {
	var cert *models.Certificate
	err := r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		var err error
		cert, err = scanCertificate(tx.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificates WHERE tenant_id = $1 AND id = $2`, tenantID, id))
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrNotFound
	}
	return cert, err
}

func (r *CertificateRepository) GetByDomain(ctx context.Context, tenantID uuid.UUID, domain string) ([]*models.Certificate, error) {
	var certs []*models.Certificate
	err := r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT `+certificateColumns+` FROM certificates WHERE tenant_id = $1 AND domain = $2 ORDER BY created_at DESC`, tenantID, domain)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			cert, err := scanCertificate(rows)
			if err != nil {
				return err
			}
			certs = append(certs, cert)
		}
		return rows.Err()
	})
	return certs, err
}

func (r *CertificateRepository) List(ctx context.Context, tenantID uuid.UUID, filter *core.CertFilter) ([]*models.Certificate, int, error) {
	if tenantID == uuid.Nil {
		return nil, 0, errors.New("tenant ID is required; use ListAllTenants for trusted system scans")
	}

	return r.listForTenant(ctx, tenantID, filter)
}

func (r *CertificateRepository) ListAllTenants(ctx context.Context, filter *core.CertFilter) ([]*models.Certificate, int, error) {
	return r.listAcrossTenants(ctx, filter)
}

func (r *CertificateRepository) listAcrossTenants(ctx context.Context, filter *core.CertFilter) ([]*models.Certificate, int, error) {
	page, pageSize, offset, err := certificatePagination(filter)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `SELECT id FROM tenants ORDER BY id`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tenantIDs []uuid.UUID
	for rows.Next() {
		var tenantID uuid.UUID
		if err := rows.Scan(&tenantID); err != nil {
			return nil, 0, err
		}
		tenantIDs = append(tenantIDs, tenantID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	scanPageSize := pageSize * page
	scanFilter := filter
	if filter != nil && pageSize > 0 {
		scanFilter = &core.CertFilter{
			Status:      filter.Status,
			Environment: filter.Environment,
			SatelliteID: filter.SatelliteID,
			ExpiringIn:  filter.ExpiringIn,
			Page:        1,
			PageSize:    scanPageSize,
		}
	}

	var certificates []*models.Certificate
	total := 0
	for _, tenantID := range tenantIDs {
		tenantCertificates, tenantTotal, err := r.listForTenant(ctx, tenantID, scanFilter)
		if err != nil {
			return nil, 0, err
		}
		total += tenantTotal
		certificates = append(certificates, tenantCertificates...)
	}

	sort.Slice(certificates, func(i, j int) bool {
		if certificates[i].CreatedAt.Equal(certificates[j].CreatedAt) {
			return certificates[i].ID.String() < certificates[j].ID.String()
		}
		return certificates[i].CreatedAt.After(certificates[j].CreatedAt)
	})
	if pageSize > 0 {
		start := offset
		if start >= len(certificates) {
			return []*models.Certificate{}, total, nil
		}
		end := start + pageSize
		if end > len(certificates) {
			end = len(certificates)
		}
		certificates = certificates[start:end]
	}
	return certificates, total, nil
}

func certificatePagination(filter *core.CertFilter) (page, pageSize, offset int, err error) {
	page = 1
	if filter == nil || filter.PageSize <= 0 {
		return page, 0, 0, nil
	}
	page = filter.Page
	if page < 1 {
		page = 1
	}
	pageSize = filter.PageSize
	maxInt := int(^uint(0) >> 1)
	if page > maxInt/pageSize {
		return 0, 0, 0, errors.New("certificate pagination overflows int")
	}
	offset = (page - 1) * pageSize
	return page, pageSize, offset, nil
}

func (r *CertificateRepository) listForTenant(ctx context.Context, tenantID uuid.UUID, filter *core.CertFilter) ([]*models.Certificate, int, error) {
	_, pageSize, offset, err := certificatePagination(filter)
	if err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + certificateColumns + ` FROM certificates WHERE TRUE`
	countQuery := `SELECT COUNT(*) FROM certificates WHERE TRUE`
	args := make([]any, 0, 6)
	addFilter := func(condition string, value any) {
		condition = fmt.Sprintf(condition, len(args)+1)
		query += ` AND ` + condition
		countQuery += ` AND ` + condition
		args = append(args, value)
	}

	addFilter(`tenant_id = $%d`, tenantID)
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

	var certs []*models.Certificate
	var total int
	err = r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		query += ` ORDER BY created_at DESC, id ASC`
		queryArgs := append([]any(nil), args...)
		if pageSize > 0 {
			// #nosec G202 -- placeholders are generated from the argument count; values remain bound parameters.
			query += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(queryArgs)+1, len(queryArgs)+2)
			queryArgs = append(queryArgs, pageSize, offset)
		}

		rows, err := tx.QueryContext(ctx, query, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			cert, err := scanCertificate(rows)
			if err != nil {
				return err
			}
			certs = append(certs, cert)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return certs, total, nil
}

func (r *CertificateRepository) Update(ctx context.Context, cert *models.Certificate) error {
	query := `UPDATE certificates SET status=$1, issuer=$2, serial_number=$3, fingerprint=$4, not_before=$5, not_after=$6, secret_ref=$7,
		renewal_count=$8, last_deployed_at=$9, approval_id=$10, purchase_order_id=$11, updated_at=$12
		WHERE id=$13 AND tenant_id=$14`

	return r.db.WithTenantContext(ctx, cert.TenantID, func(tx *sql.Tx) error {
		return requireCertificateAffected(tx.ExecContext(ctx, query,
			cert.Status, cert.Issuer, cert.SerialNumber, cert.Fingerprint,
			cert.NotBefore, cert.NotAfter, cert.SecretRef, cert.RenewalCount,
			cert.LastDeployedAt, cert.ApprovalID, cert.PurchaseOrderID, cert.UpdatedAt,
			cert.ID, cert.TenantID,
		))
	})
}

func (r *CertificateRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		return requireCertificateAffected(tx.ExecContext(ctx, `DELETE FROM certificates WHERE tenant_id = $1 AND id = $2`, tenantID, id))
	})
}

func requireCertificateAffected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *CertificateRepository) CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var count int
	err := r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM certificates WHERE tenant_id = $1`, tenantID).Scan(&count)
	})
	return count, err
}
