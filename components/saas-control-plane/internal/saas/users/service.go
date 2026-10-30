package users

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/config"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrInvalidRole          = errors.New("invalid user role")
	ErrRoleAssignmentDenied = errors.New("role assignment is not permitted")
	// ErrUserNotFound wraps core.ErrNotFound so handlers can match either.
	ErrUserNotFound = fmt.Errorf("user not found: %w", core.ErrNotFound)
	ErrUserExists   = errors.New("user already exists")
	ErrUserDisabled = errors.New("user account is disabled")
)

// Service handles user management operations.
type Service struct {
	repo   core.UserRepository
	jwt    config.JWTConfig
	logger *zap.Logger
}

// NewService creates a new user service.
func NewService(repo core.UserRepository, jwtCfg config.JWTConfig, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		jwt:    jwtCfg,
		logger: logger,
	}
}

// CanAssignRole applies the privilege ceiling for local user administration.
// Tenant roles can assign only tenant roles, platform admins cannot create a
// super-admin peer, and only super_admin can assign every valid role.
func CanAssignRole(actorRole string, requested models.UserRole) error {
	if !isValidRole(requested) {
		return ErrInvalidRole
	}
	switch actorRole {
	case string(models.RoleSuperAdmin):
		return nil
	case string(models.RoleAdmin):
		if requested != models.RoleSuperAdmin {
			return nil
		}
	default:
		if requested != models.RoleSuperAdmin && requested != models.RoleAdmin {
			return nil
		}
	}
	return ErrRoleAssignmentDenied
}

func isValidRole(role models.UserRole) bool {
	switch role {
	case models.RoleSuperAdmin, models.RoleAdmin, models.RoleTenantAdmin,
		models.RoleCertManager, models.RoleApprover, models.RoleOperator,
		models.RoleViewer, models.RoleAuditor, models.RoleFinance,
		models.RoleServiceAcct:
		return true
	default:
		return false
	}
}

// Login authenticates a user and returns JWT tokens.
func (s *Service) Login(ctx context.Context, req *models.LoginRequest) (*models.TokenResponse, error) {
	user, err := s.repo.GetByEmail(ctx, req.Email)
	if err != nil {
		// An unknown email is a credential failure, but a database error is not:
		// reporting both as "invalid credentials" previously hid a broken query
		// behind a login failure and made it effectively undebuggable.
		if errors.Is(err, core.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		s.logger.Error("login lookup failed",
			zap.String("email", req.Email), zap.Error(err))
		return nil, fmt.Errorf("authentication temporarily unavailable: %w", err)
	}

	if user.Status != "active" {
		return nil, ErrUserDisabled
	}

	// An SSO-provisioned user has no local password. bcrypt would reject the
	// empty hash anyway, but saying so explicitly keeps the reason clear.
	if user.PasswordHash == "" {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Update last login
	now := time.Now()
	user.LastLoginAt = &now
	_ = s.repo.Update(ctx, user)

	// Generate tokens
	accessToken, err := s.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateRefreshToken(user)
	if err != nil {
		return nil, err
	}

	return &models.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.jwt.ExpiryMinutes * 60,
		TokenType:    "Bearer",
	}, nil
}

// CreateUser registers a new user in the tenant.
func (s *Service) CreateUser(ctx context.Context, tenantID uuid.UUID, email, name, password string, role models.UserRole) (*models.User, error) {
	if tenantID == uuid.Nil {
		return nil, errors.New("tenant_id is required")
	}
	if !isValidRole(role) {
		return nil, ErrInvalidRole
	}
	// Only a definitive "not found" means the address is free. Swallowing every
	// error here would let a database failure look like an available email and
	// push the problem down to the INSERT.
	existing, err := s.repo.GetByEmail(ctx, email)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		return nil, fmt.Errorf("check existing user: %w", err)
	}
	if existing != nil {
		return nil, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:           uuid.New(),
		TenantID:     tenantID,
		Email:        email,
		Name:         name,
		Role:         role,
		Status:       "active",
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	s.logger.Info("user created",
		zap.String("user_id", user.ID.String()),
		zap.String("email", email),
		zap.String("role", string(role)),
	)

	return user, nil
}

// GetUser returns a user only when it belongs to tenantID.
func (s *Service) GetUser(ctx context.Context, tenantID, id uuid.UUID) (*models.User, error) {
	if tenantID == uuid.Nil {
		return nil, ErrUserNotFound
	}
	return s.repo.GetByIDForTenant(ctx, tenantID, id)
}

// GetUserAllTenants is an explicit platform-only primitive; authorization is
// required at the API boundary before calling it.
func (s *Service) GetUserAllTenants(ctx context.Context, id uuid.UUID) (*models.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListUsers(ctx context.Context, tenantID uuid.UUID) ([]*models.User, error) {
	if tenantID == uuid.Nil {
		return nil, ErrUserNotFound
	}
	return s.repo.ListByTenant(ctx, tenantID)
}

func (s *Service) ListUsersAllTenants(ctx context.Context) ([]*models.User, error) {
	return s.repo.ListAll(ctx)
}

func (s *Service) UpdateRole(ctx context.Context, tenantID, userID uuid.UUID, role models.UserRole) error {
	if !isValidRole(role) {
		return ErrInvalidRole
	}
	user, err := s.repo.GetByIDForTenant(ctx, tenantID, userID)
	if err != nil {
		return ErrUserNotFound
	}
	user.Role = role
	user.UpdatedAt = time.Now()
	return s.repo.UpdateForTenant(ctx, tenantID, user)
}

func (s *Service) DisableUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	user, err := s.repo.GetByIDForTenant(ctx, tenantID, userID)
	if err != nil {
		return ErrUserNotFound
	}
	user.Status = "disabled"
	user.UpdatedAt = time.Now()
	return s.repo.UpdateForTenant(ctx, tenantID, user)
}

func (s *Service) generateAccessToken(user *models.User) (string, error) {
	claims := &middleware.Claims{
		UserID:   user.ID,
		TenantID: &user.TenantID,
		Email:    user.Email,
		Role:     string(user.Role),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.jwt.ExpiryMinutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    s.jwt.Issuer,
			Audience:  jwt.ClaimStrings{s.jwt.Audience},
			Subject:   user.ID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwt.Secret))
}

func (s *Service) generateRefreshToken(user *models.User) (string, error) {
	claims := &jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.jwt.RefreshDays) * 24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    s.jwt.Issuer,
		Subject:   user.ID.String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwt.Secret))
}
