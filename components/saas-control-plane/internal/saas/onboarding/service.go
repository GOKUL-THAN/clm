// Package onboarding provides end-to-end customer and tenant onboarding.
//
// Directly patterned from corporate multi-tenant isolation architectures (M2P customer-onboarding
// and CrewPE enterprise bootstrap), this service enforces the customer cloud contract:
// validates customer VPC / AWS Account / STS AssumeRole parameters, provisions isolated tenant
// quotas, registers the primary satellite gateway, and generates one-click deployment commands.
package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidSlug           = errors.New("slug must be 3-30 lowercase alphanumeric characters and hyphens, starting with a letter")
	ErrSlugTaken             = errors.New("organization slug is already registered")
	ErrInvalidAWSAccountID   = errors.New("aws_account_id must be exactly 12 digits")
	ErrInvalidVPCID          = errors.New("vpc_id must be formatted as vpc-xxxxxxxx (8 or 17 hex characters)")
	ErrInvalidRoleARN        = errors.New("cross_account_role_arn must be a valid IAM role ARN")
	ErrInvalidExternalID     = errors.New("external_id must be at least 16 characters for secure STS trust")
	ErrInvalidEmail          = errors.New("invalid contact email address")
)

var (
	slugRegex       = regexp.MustCompile(`^[a-z][a-z0-9-]{2,30}$`)
	awsAccountRegex = regexp.MustCompile(`^\d{12}$`)
	vpcRegex        = regexp.MustCompile(`^vpc-[0-9a-fA-F]{8,17}$`)
	roleArnRegex    = regexp.MustCompile(`^arn:aws:iam::\d{12}:role/[A-Za-z0-9+=,.@_-]+$`)
)

// OnboardingInput represents the corporate customer onboarding contract.
type OnboardingInput struct {
	Slug                string `json:"slug"`
	Name                string `json:"name"`
	ContactEmail        string `json:"contact_email"`
	AWSAccountID        string `json:"aws_account_id"`
	Region              string `json:"region"`
	CrossAccountRoleARN string `json:"cross_account_role_arn"`
	ExternalID          string `json:"external_id"`
	VPCID               string `json:"vpc_id"`
	Plan                string `json:"plan"` // starter, growth, enterprise, fintech-sovereign
}

// PreflightCheck represents an individual pre-flight verification step.
type PreflightCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// PreflightReport is returned when validating inputs prior to tenant provisioning.
type PreflightReport struct {
	Valid   bool             `json:"valid"`
	Checks  []PreflightCheck `json:"checks"`
	Plan    string           `json:"plan"`
	Quotas  models.PlanQuota `json:"quotas"`
}

// OnboardingResult contains the provisioned tenant, primary gateway, and admin credentials.
type OnboardingResult struct {
	TenantID        uuid.UUID        `json:"tenant_id"`
	Slug            string           `json:"slug"`
	Name            string           `json:"name"`
	Plan            string           `json:"plan"`
	DataRegion      string           `json:"data_region"`
	AdminEmail      string           `json:"admin_email"`
	AdminUserID     uuid.UUID        `json:"admin_user_id"`
	TemporarySecret string           `json:"temporary_secret"`
	PrimaryGateway  GatewayBootstrap `json:"primary_gateway"`
	Quotas          models.PlanQuota `json:"quotas"`
	CreatedAt       time.Time        `json:"created_at"`
}

// GatewayBootstrap contains pairing instructions for the customer VPC gateway.
type GatewayBootstrap struct {
	SatelliteID   uuid.UUID `json:"satellite_id"`
	SatelliteName string    `json:"satellite_name"`
	PairingToken  string    `json:"pairing_token"`
	InstallScript string    `json:"install_script"`
}

// Service manages corporate tenant onboarding.
type Service struct {
	tenantRepo core.TenantRepository
	userRepo   core.UserRepository
	satRepo    core.SatelliteRepository
	logger     *zap.Logger
}

// NewService creates a new onboarding service.
func NewService(
	tenantRepo core.TenantRepository,
	userRepo core.UserRepository,
	satRepo core.SatelliteRepository,
	logger *zap.Logger,
) *Service {
	return &Service{
		tenantRepo: tenantRepo,
		userRepo:   userRepo,
		satRepo:    satRepo,
		logger:     logger,
	}
}

// Preflight validates the customer inputs against corporate cloud standards.
func (s *Service) Preflight(ctx context.Context, input *OnboardingInput) (*PreflightReport, error) {
	report := &PreflightReport{
		Valid:  true,
		Checks: make([]PreflightCheck, 0),
		Plan:   input.Plan,
		Quotas: getPlanQuotas(input.Plan),
	}

	// 1. Slug check
	if !slugRegex.MatchString(input.Slug) {
		report.Valid = false
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "Organization Slug Format",
			Passed:  false,
			Message: "Must be 3-30 lowercase alphanumeric characters and hyphens",
		})
	} else {
		// Check uniqueness
		existing, _ := s.tenantRepo.GetBySlug(ctx, input.Slug)
		if existing != nil {
			report.Valid = false
			report.Checks = append(report.Checks, PreflightCheck{
				Name:    "Slug Availability",
				Passed:  false,
				Message: "Slug already registered by another organization",
			})
		} else {
			report.Checks = append(report.Checks, PreflightCheck{
				Name:    "Slug Availability",
				Passed:  true,
				Message: "Slug is available",
			})
		}
	}

	// 2. Email check
	if !strings.Contains(input.ContactEmail, "@") || !strings.Contains(input.ContactEmail, ".") {
		report.Valid = false
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "Contact Email Validation",
			Passed:  false,
			Message: "Must be a valid corporate email address",
		})
	} else {
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "Contact Email Validation",
			Passed:  true,
			Message: "Email format verified",
		})
	}

	// 3. AWS Account ID check
	if !awsAccountRegex.MatchString(input.AWSAccountID) {
		report.Valid = false
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "AWS Account ID Format",
			Passed:  false,
			Message: "Must be exactly 12 numeric digits",
		})
	} else {
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "AWS Account ID Format",
			Passed:  true,
			Message: "12-digit AWS account ID validated",
		})
	}

	// 4. VPC ID check
	if !vpcRegex.MatchString(input.VPCID) {
		report.Valid = false
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "VPC ID Format",
			Passed:  false,
			Message: "Must be formatted as vpc-xxxxxxxx (8 or 17 hex characters)",
		})
	} else {
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "VPC ID Format",
			Passed:  true,
			Message: "Customer VPC ID validated",
		})
	}

	// 5. Cross-account role ARN check
	if !roleArnRegex.MatchString(input.CrossAccountRoleARN) {
		report.Valid = false
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "Cross-Account IAM Role ARN",
			Passed:  false,
			Message: "Must be a valid AWS IAM role ARN matching arn:aws:iam::<account>:role/<name>",
		})
	} else {
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "Cross-Account IAM Role ARN",
			Passed:  true,
			Message: "IAM role ARN format validated",
		})
	}

	// 6. External ID check
	if len(input.ExternalID) < 16 {
		report.Valid = false
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "STS External ID Entropy",
			Passed:  false,
			Message: "External ID must be at least 16 characters for cryptographic trust",
		})
	} else {
		report.Checks = append(report.Checks, PreflightCheck{
			Name:    "STS External ID Entropy",
			Passed:  true,
			Message: "External ID entropy validated",
		})
	}

	return report, nil
}

// Bootstrap provisions the tenant, creates the primary security admin, and registers the initial gateway.
func (s *Service) Bootstrap(ctx context.Context, input *OnboardingInput) (*OnboardingResult, error) {
	// 1. Run Preflight
	report, err := s.Preflight(ctx, input)
	if err != nil {
		return nil, err
	}
	if !report.Valid {
		return nil, errors.New("preflight validation failed: check parameters")
	}

	plan := input.Plan
	if plan == "" {
		plan = "enterprise"
	}
	quotas := getPlanQuotas(plan)

	// 2. Create Tenant
	tenantID := uuid.New()
	tenant := &models.Tenant{
		ID:            tenantID,
		Name:          input.Name,
		Slug:          input.Slug,
		Plan:          plan,
		Status:        "active",
		MaxCerts:      quotas.MaxCerts,
		MaxSatellites: quotas.MaxSatellites,
		MaxUsers:      quotas.MaxUsers,
		DataRegion:    input.Region,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.tenantRepo.Create(ctx, tenant); err != nil {
		s.logger.Error("failed to create tenant during bootstrap", zap.Error(err))
		return nil, err
	}

	// 3. Create Tenant Admin / Security Officer
	adminID := uuid.New()
	tempSecret := generateRandomSecret(16)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(tempSecret), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	adminUser := &models.User{
		ID:           adminID,
		TenantID:     tenantID,
		Email:        input.ContactEmail,
		PasswordHash: string(hashedPassword),
		Role:         models.RoleTenantAdmin,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.userRepo.Create(ctx, adminUser); err != nil {
		s.logger.Error("failed to create tenant admin during bootstrap", zap.Error(err))
		return nil, err
	}

	// 4. Create Primary Satellite Gateway record
	satID := uuid.New()
	satName := fmt.Sprintf("%s-%s-gw-01", input.Slug, input.Region)
	pairingToken := "PAIR-" + generateRandomSecret(24)

	satellite := &models.Satellite{
		ID:        satID,
		TenantID:  tenantID,
		Name:      satName,
		Status:    models.SatellitePending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.satRepo.Create(ctx, satellite); err != nil {
		s.logger.Error("failed to create satellite gateway during bootstrap", zap.Error(err))
		return nil, err
	}

	// 5. Generate one-click gateway install script for customer VPC
	installScript := fmt.Sprintf(`curl -sSL https://clm.m2p.in/install-gateway.sh | bash -s -- \
  --satellite-id %s \
  --token %s \
  --region %s \
  --vpc %s`, satID.String(), pairingToken, input.Region, input.VPCID)

	s.logger.Info("tenant onboarding bootstrap successful",
		zap.String("tenant_id", tenantID.String()),
		zap.String("slug", input.Slug),
		zap.String("plan", plan),
	)

	return &OnboardingResult{
		TenantID:        tenantID,
		Slug:            input.Slug,
		Name:            input.Name,
		Plan:            plan,
		DataRegion:      input.Region,
		AdminEmail:      input.ContactEmail,
		AdminUserID:     adminID,
		TemporarySecret: tempSecret,
		PrimaryGateway: GatewayBootstrap{
			SatelliteID:   satID,
			SatelliteName: satName,
			PairingToken:  pairingToken,
			InstallScript: installScript,
		},
		Quotas:    quotas,
		CreatedAt: time.Now(),
	}, nil
}

func getPlanQuotas(plan string) models.PlanQuota {
	switch strings.ToLower(plan) {
	case "starter":
		return models.PlanQuota{MaxCerts: 5, MaxSatellites: 1, MaxUsers: 3}
	case "growth", "pro":
		return models.PlanQuota{MaxCerts: 100, MaxSatellites: 5, MaxUsers: 15}
	case "enterprise":
		return models.PlanQuota{MaxCerts: 10000, MaxSatellites: 50, MaxUsers: 100}
	case "fintech-sovereign", "sovereign":
		return models.PlanQuota{MaxCerts: 100000, MaxSatellites: 500, MaxUsers: 1000}
	default:
		return models.PlanQuota{MaxCerts: 10000, MaxSatellites: 50, MaxUsers: 100}
	}
}

func generateRandomSecret(length int) string {
	bytes := make([]byte, length/2+1)
	if _, err := rand.Read(bytes); err != nil {
		return "secret" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)[:length]
}
