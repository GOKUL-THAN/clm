package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
)

// ApprovalRepository implements core.ApprovalRepository.
type ApprovalRepository struct {
	db *DB
}

// NewApprovalRepository creates a new approval repository.
func NewApprovalRepository(db *DB) *ApprovalRepository {
	return &ApprovalRepository{db: db}
}

func (r *ApprovalRepository) Create(ctx context.Context, req *models.ApprovalRequest) error {
	query := `INSERT INTO approval_requests (id, tenant_id, request_type, level, status, requested_by, justification, payload, estimated_cost, sla_deadline, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err := r.db.ExecContext(ctx, query,
		req.ID, req.TenantID, req.RequestType, req.Level, req.Status, req.RequestedBy,
		req.Justification, req.Payload, req.EstimatedCost, req.SLADeadline, req.CreatedAt, req.UpdatedAt,
	)
	return err
}

func (r *ApprovalRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error) {
	query := `SELECT id, tenant_id, request_type, level, status, requested_by, justification, payload, estimated_cost, sla_deadline, resolved_by, resolved_at, comment, created_at, updated_at
		FROM approval_requests WHERE id = $1`

	req := &models.ApprovalRequest{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&req.ID, &req.TenantID, &req.RequestType, &req.Level, &req.Status, &req.RequestedBy,
		&req.Justification, &req.Payload, &req.EstimatedCost, &req.SLADeadline,
		&req.ResolvedBy, &req.ResolvedAt, &req.Comment, &req.CreatedAt, &req.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("approval request not found")
	}
	return req, err
}

func (r *ApprovalRepository) ListPending(ctx context.Context, tenantID uuid.UUID) ([]*models.ApprovalRequest, error) {
	query := `SELECT id, tenant_id, request_type, level, status, requested_by, justification, estimated_cost, sla_deadline, created_at
		FROM approval_requests WHERE tenant_id = $1 AND status = 'pending'
		ORDER BY sla_deadline ASC`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []*models.ApprovalRequest
	for rows.Next() {
		req := &models.ApprovalRequest{}
		if err := rows.Scan(
			&req.ID, &req.TenantID, &req.RequestType, &req.Level, &req.Status,
			&req.RequestedBy, &req.Justification, &req.EstimatedCost, &req.SLADeadline, &req.CreatedAt,
		); err != nil {
			return nil, err
		}
		reqs = append(reqs, req)
	}
	return reqs, rows.Err()
}

func (r *ApprovalRepository) Update(ctx context.Context, req *models.ApprovalRequest) error {
	query := `UPDATE approval_requests SET status=$1, resolved_by=$2, resolved_at=$3, comment=$4, updated_at=$5
		WHERE id = $6`

	_, err := r.db.ExecContext(ctx, query,
		req.Status, req.ResolvedBy, req.ResolvedAt, req.Comment, req.UpdatedAt, req.ID,
	)
	return err
}

func (r *ApprovalRepository) GetRules(ctx context.Context, tenantID uuid.UUID) ([]*models.ApprovalRule, error) {
	// Simplified — in production, parse JSON conditions
	query := `SELECT id, tenant_id, name, level, priority, min_approvals, sla_hours, active
		FROM approval_rules WHERE tenant_id = $1 AND active = true
		ORDER BY priority ASC`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*models.ApprovalRule
	for rows.Next() {
		rule := &models.ApprovalRule{}
		if err := rows.Scan(
			&rule.ID, &rule.TenantID, &rule.Name, &rule.Level, &rule.Priority, &rule.MinApprovals, &rule.SLAHours, &rule.Active,
		); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}
