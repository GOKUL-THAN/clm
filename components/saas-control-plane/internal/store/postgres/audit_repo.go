package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
)

type AuditRepository struct{ db *DB }

func NewAuditRepository(db *DB) *AuditRepository { return &AuditRepository{db: db} }

func (r *AuditRepository) Append(ctx context.Context, e *models.AuditEvent) error {
	return r.db.WithTenantContext(ctx, e.TenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO control_plane_audit_events (id,tenant_id,request_id,actor_id,actor_role,action,resource_type,resource_id,outcome,source_ip,metadata,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, e.ID, e.TenantID, e.RequestID, e.ActorID, e.ActorRole, e.Action, e.ResourceType, e.ResourceID, e.Outcome, e.SourceIP, e.Metadata, e.CreatedAt)
		return err
	})
}

func (r *AuditRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, f models.AuditFilter) ([]*models.AuditEvent, error) {
	query := `SELECT id,tenant_id,request_id,actor_id,actor_role,action,resource_type,resource_id,outcome,source_ip,metadata,created_at FROM control_plane_audit_events WHERE tenant_id=$1`
	args := []any{tenantID}
	add := func(clause string, value any) { args = append(args, value); query += fmt.Sprintf(clause, len(args)) }
	if f.Action != "" {
		add(" AND action=$%d", f.Action)
	}
	if f.ResourceType != "" {
		add(" AND resource_type=$%d", f.ResourceType)
	}
	if f.ActorID != nil {
		add(" AND actor_id=$%d", *f.ActorID)
	}
	if f.Outcome != "" {
		add(" AND outcome=$%d", f.Outcome)
	}
	if f.From != nil {
		add(" AND created_at >= $%d", *f.From)
	}
	if f.To != nil {
		add(" AND created_at <= $%d", *f.To)
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit, f.Offset)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	var events []*models.AuditEvent
	err := r.db.WithTenantContext(ctx, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			e := &models.AuditEvent{}
			if err := rows.Scan(&e.ID, &e.TenantID, &e.RequestID, &e.ActorID, &e.ActorRole, &e.Action, &e.ResourceType, &e.ResourceID, &e.Outcome, &e.SourceIP, &e.Metadata, &e.CreatedAt); err != nil {
				return err
			}
			events = append(events, e)
		}
		return rows.Err()
	})
	return events, err
}
