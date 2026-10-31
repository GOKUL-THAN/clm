// Package organization provides enterprise department, cost center, and quota governance.
//
// Patterned after SAP ERP, Workday, and Salesforce Organizational Management, this controller
// structures tenant accounts into corporate business units, tracks departmental certificate quotas,
// attributes commercial CA expenditures to cost centers, and enforces budgetary caps.
package organization

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	ErrDepartmentNotFound = errors.New("department not found")
	ErrDuplicateDeptCode  = errors.New("department code must be unique within tenant")
	ErrBudgetExceeded     = errors.New("commercial certificate cost exceeds departmental budget cap")
	ErrQuotaExceeded      = errors.New("department active certificate quota exceeded")
)

// Department represents a corporate business unit with financial attribution.
type Department struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	Name         string    `json:"name"`        // e.g. "Payments & Gateway"
	Code         string    `json:"code"`        // e.g. "DEPT-PAY-01"
	CostCenter   string    `json:"cost_center"` // e.g. "CC-FIN-7892"
	LeadEmail    string    `json:"lead_email"`
	BudgetUSD    float64   `json:"budget_usd"`     // Monthly commercial CA budget cap
	SpentUSD     float64   `json:"spent_usd"`      // Month-to-date commercial CA spend
	CertQuota    int       `json:"cert_quota"`     // Maximum allowable active certificates
	CertCount    int       `json:"cert_count"`     // Current active certificates
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DepartmentController coordinates business unit and financial allocation.
type DepartmentController struct {
	mu          sync.RWMutex
	departments map[uuid.UUID]*Department
	logger      *zap.Logger
}

// NewDepartmentController creates a new department controller.
func NewDepartmentController(logger *zap.Logger) *DepartmentController {
	return &DepartmentController{
		departments: make(map[uuid.UUID]*Department),
		logger:      logger,
	}
}

// CreateDepartment registers a new department.
func (c *DepartmentController) CreateDepartment(ctx context.Context, tenantID uuid.UUID, dept *Department) (*Department, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check code uniqueness
	for _, d := range c.departments {
		if d.TenantID == tenantID && d.Code == dept.Code {
			return nil, ErrDuplicateDeptCode
		}
	}

	now := time.Now()
	dept.ID = uuid.New()
	dept.TenantID = tenantID
	dept.CreatedAt = now
	dept.UpdatedAt = now

	if dept.BudgetUSD <= 0 {
		dept.BudgetUSD = 5000.00
	}
	if dept.CertQuota <= 0 {
		dept.CertQuota = 250
	}

	c.departments[dept.ID] = dept

	c.logger.Info("department created",
		zap.String("dept_id", dept.ID.String()),
		zap.String("name", dept.Name),
		zap.String("code", dept.Code),
	)

	return dept, nil
}

// ListDepartments returns all business units for a tenant.
func (c *DepartmentController) ListDepartments(tenantID uuid.UUID) []*Department {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var list []*Department
	for _, d := range c.departments {
		if d.TenantID == tenantID {
			list = append(list, d)
		}
	}
	return list
}

// CheckAndRecordSpend verifies department budget and adds spend.
func (c *DepartmentController) CheckAndRecordSpend(ctx context.Context, deptID uuid.UUID, amountUSD float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	d, ok := c.departments[deptID]
	if !ok {
		return ErrDepartmentNotFound
	}

	if d.SpentUSD+amountUSD > d.BudgetUSD {
		return ErrBudgetExceeded
	}

	d.SpentUSD += amountUSD
	d.UpdatedAt = time.Now()
	return nil
}

// IncrementCertCount records new certificate activation.
func (c *DepartmentController) IncrementCertCount(ctx context.Context, deptID uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	d, ok := c.departments[deptID]
	if !ok {
		return ErrDepartmentNotFound
	}

	if d.CertCount >= d.CertQuota {
		return ErrQuotaExceeded
	}

	d.CertCount++
	d.UpdatedAt = time.Now()
	return nil
}
