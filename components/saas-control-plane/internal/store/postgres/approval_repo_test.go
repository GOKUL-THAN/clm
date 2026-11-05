package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
)

func TestApprovalRepositoryEnforcesTenantContext(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantA := seedTenant(t, ctx, "approval-a-"+uuid.NewString()[:8])
	tenantB := seedTenant(t, ctx, "approval-b-"+uuid.NewString()[:8])
	requestedBy := uuid.New()

	_, err := testDB.ExecContext(ctx,
		`INSERT INTO users (id, tenant_id, email, name, role, status, password_hash)
		 VALUES ($1, $2, $3, 'Approval Requester', 'approver', 'active', 'test-only')`,
		requestedBy, tenantA, uuid.NewString()+"@example.invalid")
	if err != nil {
		t.Fatalf("seed requester: %v", err)
	}

	request := &models.ApprovalRequest{
		ID:            uuid.New(),
		TenantID:      tenantA,
		RequestType:   "cert_create",
		Level:         models.ApprovalL1,
		Status:        models.ApprovalPending,
		RequestedBy:   requestedBy,
		Justification: "tenant isolation test",
		Payload:       "{}",
		SLADeadline:   time.Now().Add(time.Hour),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	repo := NewApprovalRepository(testDB)
	ruleID := uuid.New()
	t.Cleanup(func() {
		_ = testDB.WithTenantContext(context.Background(), tenantA, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(context.Background(), `DELETE FROM approval_rules WHERE id = $1`, ruleID)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(context.Background(), `DELETE FROM approval_requests WHERE id = $1`, request.ID)
			return err
		})
	})
	if err := repo.Create(ctx, request); err != nil {
		t.Fatalf("create approval in owning tenant: %v", err)
	}

	if _, err := repo.GetByID(ctx, tenantA, request.ID); err != nil {
		t.Fatalf("read approval in owning tenant: %v", err)
	}
	if _, err := repo.GetByID(ctx, tenantB, request.ID); err == nil {
		t.Fatal("cross-tenant approval lookup succeeded")
	}
	if _, err := repo.GetByID(ctx, tenantA, uuid.New()); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("missing approval lookup error = %v, want core.ErrNotFound", err)
	}

	request.TenantID = tenantB
	request.Status = models.ApprovalApproved
	if err := repo.Update(ctx, request); err == nil {
		t.Fatal("cross-tenant approval update succeeded")
	}
	missingRequest := *request
	missingRequest.ID = uuid.New()
	missingRequest.TenantID = tenantA
	if err := repo.Update(ctx, &missingRequest); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("missing approval update error = %v, want core.ErrNotFound", err)
	}

	pending, err := repo.ListPending(ctx, tenantB)
	if err != nil {
		t.Fatalf("list approvals for other tenant: %v", err)
	}
	for _, item := range pending {
		if item.ID == request.ID {
			t.Fatal("cross-tenant approval appeared in pending list")
		}
	}

	if err := testDB.WithTenantContext(ctx, tenantA, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO approval_rules (id, tenant_id, name, level, priority)
			 VALUES ($1, $2, 'tenant isolation test rule', 'L1', 1)`,
			ruleID, tenantA)
		return err
	}); err != nil {
		t.Fatalf("seed approval rule: %v", err)
	}
	rules, err := repo.GetRules(ctx, tenantA)
	if err != nil {
		t.Fatalf("list rules in owning tenant: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != ruleID {
		t.Fatalf("owning tenant rules = %v, want only rule %s", rules, ruleID)
	}
	rules, err = repo.GetRules(ctx, tenantB)
	if err != nil {
		t.Fatalf("list rules for other tenant: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("cross-tenant approval rules visible: %d", len(rules))
	}
}
