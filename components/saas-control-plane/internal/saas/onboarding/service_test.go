package onboarding

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// mockTenantRepo implements core.TenantRepository in-memory for testing.
type mockTenantRepo struct {
	tenants map[string]*models.Tenant
}

func (m *mockTenantRepo) Create(ctx context.Context, t *models.Tenant) error {
	m.tenants[t.Slug] = t
	return nil
}
func (m *mockTenantRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Tenant, error) {
	for _, t := range m.tenants {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, nil
}
func (m *mockTenantRepo) GetBySlug(ctx context.Context, slug string) (*models.Tenant, error) {
	return m.tenants[slug], nil
}
func (m *mockTenantRepo) List(ctx context.Context) ([]*models.Tenant, error) { return nil, nil }
func (m *mockTenantRepo) Update(ctx context.Context, t *models.Tenant) error  { return nil }
func (m *mockTenantRepo) CountDependents(ctx context.Context, id uuid.UUID) (map[string]int, error) {
	return nil, nil
}
func (m *mockTenantRepo) Delete(ctx context.Context, id uuid.UUID) error        { return nil }
func (m *mockTenantRepo) DeleteCascade(ctx context.Context, id uuid.UUID) error { return nil }

// mockUserRepo implements core.UserRepository in-memory for testing.
type mockUserRepo struct {
	users []*models.User
}

func (m *mockUserRepo) Create(ctx context.Context, u *models.User) error {
	m.users = append(m.users, u)
	return nil
}
func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) ListAll(ctx context.Context) ([]*models.User, error) { return nil, nil }
func (m *mockUserRepo) Update(ctx context.Context, u *models.User) error    { return nil }
func (m *mockUserRepo) UpdateForTenant(ctx context.Context, tenantID uuid.UUID, u *models.User) error {
	return nil
}
func (m *mockUserRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockUserRepo) DeleteForTenant(ctx context.Context, tenantID, id uuid.UUID) error {
	return nil
}

// mockSatRepo implements core.SatelliteRepository in-memory for testing.
type mockSatRepo struct {
	sats []*models.Satellite
}

func (m *mockSatRepo) Create(ctx context.Context, s *models.Satellite) error {
	m.sats = append(m.sats, s)
	return nil
}
func (m *mockSatRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Satellite, error) {
	return nil, nil
}
func (m *mockSatRepo) GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.Satellite, error) {
	return nil, nil
}
func (m *mockSatRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*models.Satellite, error) {
	return nil, nil
}
func (m *mockSatRepo) Update(ctx context.Context, s *models.Satellite) error { return nil }
func (m *mockSatRepo) UpdateForTenant(ctx context.Context, tenantID uuid.UUID, s *models.Satellite) error {
	return nil
}
func (m *mockSatRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockSatRepo) DeleteForTenant(ctx context.Context, tenantID, id uuid.UUID) error {
	return nil
}
func (m *mockSatRepo) ListAll(ctx context.Context) ([]*models.Satellite, error) { return nil, nil }
func (m *mockSatRepo) SetCredentialForTenant(ctx context.Context, tenantID, satelliteID uuid.UUID, encodedHash string) error {
	return nil
}
func (m *mockSatRepo) RevokeCredentialForTenant(ctx context.Context, tenantID, satelliteID uuid.UUID) error {
	return nil
}

func TestOnboardingPreflightAndBootstrap(t *testing.T) {
	tRepo := &mockTenantRepo{tenants: make(map[string]*models.Tenant)}
	uRepo := &mockUserRepo{users: make([]*models.User, 0)}
	sRepo := &mockSatRepo{sats: make([]*models.Satellite, 0)}
	logger := zap.NewNop()

	svc := NewService(tRepo, uRepo, sRepo, logger)

	validInput := &OnboardingInput{
		Slug:                "m2p-hyc-uat",
		Name:                "M2P HYC UAT Environment",
		ContactEmail:        "cloud-sec@m2pfintech.com",
		AWSAccountID:        "279078306945",
		Region:              "ap-south-1",
		CrossAccountRoleARN: "arn:aws:iam::279078306945:role/CLMOrchestratorRole",
		ExternalID:          "db95ac738a746230ddda613271903536",
		VPCID:               "vpc-00ec0a6fc7686398c",
		Plan:                "enterprise",
	}

	report, err := svc.Preflight(context.Background(), validInput)
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if !report.Valid {
		t.Fatalf("expected preflight to pass, but failed: %+v", report.Checks)
	}

	result, err := svc.Bootstrap(context.Background(), validInput)
	if err != nil {
		t.Fatalf("unexpected bootstrap error: %v", err)
	}

	if result.Slug != validInput.Slug {
		t.Errorf("expected slug %s, got %s", validInput.Slug, result.Slug)
	}
	if result.Quotas.MaxCerts != 10000 {
		t.Errorf("expected 10000 certs for enterprise plan, got %d", result.Quotas.MaxCerts)
	}
	if len(uRepo.users) != 1 {
		t.Errorf("expected 1 user created, got %d", len(uRepo.users))
	}
	if len(sRepo.sats) != 1 {
		t.Errorf("expected 1 gateway created, got %d", len(sRepo.sats))
	}
}
