package organization

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap/zaptest"
)

func TestDepartmentController(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewDepartmentController(logger)
	tenantID := uuid.New()
	ctx := context.Background()

	// 1. Create department
	dept, err := ctrl.CreateDepartment(ctx, tenantID, &Department{
		Name:       "Payments & Settlements",
		Code:       "DEPT-PAY-01",
		CostCenter: "CC-FIN-7892",
		LeadEmail:  "payments-lead@m2pfintech.com",
		BudgetUSD:  1000.00,
		CertQuota:  2,
	})
	if err != nil {
		t.Fatalf("failed to create department: %v", err)
	}

	// 2. Test duplicate code
	_, err = ctrl.CreateDepartment(ctx, tenantID, &Department{
		Name: "Duplicate",
		Code: "DEPT-PAY-01",
	})
	if err != ErrDuplicateDeptCode {
		t.Fatalf("expected ErrDuplicateDeptCode, got %v", err)
	}

	// 3. Test Spend within budget
	err = ctrl.CheckAndRecordSpend(ctx, dept.ID, 400.00)
	if err != nil {
		t.Fatalf("spend should succeed: %v", err)
	}

	// 4. Test Budget Exceeded
	err = ctrl.CheckAndRecordSpend(ctx, dept.ID, 800.00)
	if err != ErrBudgetExceeded {
		t.Fatalf("expected ErrBudgetExceeded, got %v", err)
	}

	// 5. Test Cert Quota
	if err := ctrl.IncrementCertCount(ctx, dept.ID); err != nil {
		t.Fatalf("first cert increment failed: %v", err)
	}
	if err := ctrl.IncrementCertCount(ctx, dept.ID); err != nil {
		t.Fatalf("second cert increment failed: %v", err)
	}
	// Third cert should exceed quota
	if err := ctrl.IncrementCertCount(ctx, dept.ID); err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}
}
