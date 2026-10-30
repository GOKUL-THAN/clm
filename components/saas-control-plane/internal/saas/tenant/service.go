package tenant

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

var (
	// ErrTenantNotFound wraps core.ErrNotFound so handlers can match either.
	ErrTenantNotFound = fmt.Errorf("tenant not found: %w", core.ErrNotFound)
	ErrTenantExists   = errors.New("tenant with this slug already exists")
)

// Service handles tenant lifecycle operations.
type Service struct {
	repo   core.TenantRepository
	logger *zap.Logger
}

// NewService creates a new tenant service.
func NewService(repo core.TenantRepository, logger *zap.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

// CreateTenant provisions a new tenant.
func (s *Service) CreateTenant(ctx context.Context, name, plan, region string) (*models.Tenant, error) {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	existing, _ := s.repo.GetBySlug(ctx, slug)
	if existing != nil {
		return nil, ErrTenantExists
	}

	tenant := &models.Tenant{
		ID:            uuid.New(),
		Name:          name,
		Slug:          slug,
		Plan:          plan,
		Status:        "active",
		MaxCerts:      planLimits(plan).MaxCerts,
		MaxSatellites: planLimits(plan).MaxSatellites,
		MaxUsers:      planLimits(plan).MaxUsers,
		DataRegion:    region,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.repo.Create(ctx, tenant); err != nil {
		return nil, err
	}

	s.logger.Info("tenant created",
		zap.String("tenant_id", tenant.ID.String()),
		zap.String("name", name),
		zap.String("plan", plan),
	)

	return tenant, nil
}

// GetTenant returns a tenant by ID. A database failure is reported as itself
// rather than being flattened into "not found", which previously hid a broken
// query behind a 404.
func (s *Service) GetTenant(ctx context.Context, id uuid.UUID) (*models.Tenant, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, ErrTenantNotFound
		}
		s.logger.Error("failed to load tenant",
			zap.String("tenant_id", id.String()), zap.Error(err))
		return nil, err
	}
	return t, nil
}

// ListTenants returns all tenants (super admin only).
func (s *Service) ListTenants(ctx context.Context) ([]*models.Tenant, error) {
	return s.repo.List(ctx)
}

// UpdateTenant modifies tenant settings.
func (s *Service) UpdateTenant(ctx context.Context, id uuid.UUID, updates map[string]interface{}) (*models.Tenant, error) {
	t, err := s.GetTenant(ctx, id)
	if err != nil {
		return nil, err
	}

	if name, ok := updates["name"].(string); ok {
		t.Name = name
	}
	if plan, ok := updates["plan"].(string); ok {
		t.Plan = plan
		limits := planLimits(plan)
		t.MaxCerts = limits.MaxCerts
		t.MaxSatellites = limits.MaxSatellites
		t.MaxUsers = limits.MaxUsers
	}
	if status, ok := updates["status"].(string); ok {
		t.Status = status
	}

	t.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteTenant removes a tenant.
//
// Deletion is refused while dependent records exist unless force is set, because
// every foreign key onto tenants is NO ACTION and silently cascading would
// destroy users, gateways and audit history without the operator asking for it.
// ErrTenantHasDependents carries the blocking counts so the caller can explain
// exactly what is in the way.
func (s *Service) DeleteTenant(ctx context.Context, id uuid.UUID, force bool) error {
	t, err := s.GetTenant(ctx, id)
	if err != nil {
		return err
	}

	deps, err := s.repo.CountDependents(ctx, id)
	if err != nil {
		return err
	}

	if len(deps) > 0 && !force {
		return &DependentsError{TenantName: t.Name, Counts: deps}
	}

	if force {
		if err := s.repo.DeleteCascade(ctx, id); err != nil {
			return err
		}
		s.logger.Warn("tenant deleted with dependents",
			zap.String("tenant_id", id.String()),
			zap.String("name", t.Name),
			zap.Any("removed", deps),
		)
		return nil
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.logger.Info("tenant deleted",
		zap.String("tenant_id", id.String()), zap.String("name", t.Name))
	return nil
}

// DependentsError reports that a tenant still owns records.
type DependentsError struct {
	TenantName string
	Counts     map[string]int
}

func (e *DependentsError) Error() string {
	parts := make([]string, 0, len(e.Counts))
	for table, n := range e.Counts {
		parts = append(parts, fmt.Sprintf("%d %s", n, table))
	}
	sort.Strings(parts)
	return fmt.Sprintf("tenant %q still owns %s",
		e.TenantName, strings.Join(parts, ", "))
}

type limits struct {
	MaxCerts      int
	MaxSatellites int
	MaxUsers      int
}

func planLimits(plan string) limits {
	switch plan {
	case "enterprise":
		return limits{MaxCerts: 100000, MaxSatellites: 100, MaxUsers: 500}
	case "pro":
		return limits{MaxCerts: 10000, MaxSatellites: 20, MaxUsers: 50}
	default: // free
		return limits{MaxCerts: 100, MaxSatellites: 2, MaxUsers: 5}
	}
}
