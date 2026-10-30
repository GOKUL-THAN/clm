package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/m2p/clm/pkg/models"
)

// PurchaseRepository implements core.PurchaseRepository.
type PurchaseRepository struct {
	db *DB
}

// NewPurchaseRepository creates a new purchase repository.
func NewPurchaseRepository(db *DB) *PurchaseRepository {
	return &PurchaseRepository{db: db}
}

func (r *PurchaseRepository) Create(ctx context.Context, order *models.PurchaseOrder) error {
	query := `INSERT INTO purchase_orders (id, tenant_id, cert_request_id, ca_provider, ca_order_id, product_id, domain, sans, cert_type, validity, price, currency, status, approval_id, requested_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

	_, err := r.db.ExecContext(ctx, query,
		order.ID, order.TenantID, order.CertRequestID, order.CAProvider, order.CAOrderID,
		order.ProductID, order.Domain, pq.Array(order.SANs), order.CertType, order.Validity,
		order.Price, order.Currency, order.Status, order.ApprovalID, order.RequestedBy,
		order.CreatedAt, order.UpdatedAt,
	)
	return err
}

func (r *PurchaseRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.PurchaseOrder, error) {
	query := `SELECT id, tenant_id, cert_request_id, ca_provider, ca_order_id, product_id, domain, sans, cert_type, validity, price, currency, status, approval_id, requested_by, created_at, updated_at, issued_at
		FROM purchase_orders WHERE id = $1`

	order := &models.PurchaseOrder{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&order.ID, &order.TenantID, &order.CertRequestID, &order.CAProvider, &order.CAOrderID,
		&order.ProductID, &order.Domain, pq.Array(&order.SANs), &order.CertType, &order.Validity,
		&order.Price, &order.Currency, &order.Status, &order.ApprovalID, &order.RequestedBy,
		&order.CreatedAt, &order.UpdatedAt, &order.IssuedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("purchase order not found")
	}
	return order, err
}

func (r *PurchaseRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, status *models.PurchaseStatus) ([]*models.PurchaseOrder, error) {
	query := `SELECT id, tenant_id, ca_provider, domain, cert_type, price, currency, status, created_at
		FROM purchase_orders WHERE tenant_id = $1`
	args := []interface{}{tenantID}

	if status != nil {
		query += ` AND status = $2`
		args = append(args, *status)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*models.PurchaseOrder
	for rows.Next() {
		o := &models.PurchaseOrder{}
		if err := rows.Scan(
			&o.ID, &o.TenantID, &o.CAProvider, &o.Domain, &o.CertType, &o.Price, &o.Currency, &o.Status, &o.CreatedAt,
		); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func (r *PurchaseRepository) Update(ctx context.Context, order *models.PurchaseOrder) error {
	query := `UPDATE purchase_orders SET status=$1, ca_order_id=$2, issued_at=$3, updated_at=$4
		WHERE id = $5`

	_, err := r.db.ExecContext(ctx, query,
		order.Status, order.CAOrderID, order.IssuedAt, order.UpdatedAt, order.ID,
	)
	return err
}

func (r *PurchaseRepository) GetCAAccounts(ctx context.Context, tenantID uuid.UUID) ([]*models.CAAccountConfig, error) {
	query := `SELECT id, tenant_id, provider, name, environment, cred_ref, is_default, budget, spent_month, active, created_at
		FROM ca_accounts WHERE tenant_id = $1 AND active = true`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*models.CAAccountConfig
	for rows.Next() {
		a := &models.CAAccountConfig{}
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.Provider, &a.Name, &a.Environment, &a.CredRef,
			&a.IsDefault, &a.Budget, &a.SpentMonth, &a.Active, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}
