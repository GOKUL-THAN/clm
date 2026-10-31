package sso

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/config"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// Service orchestrates SSO login flows across all providers.
type Service struct {
	userRepo    core.UserRepository
	tenantRepo  core.TenantRepository
	jwtCfg      config.JWTConfig
	groupMapper *GroupMapper
	configs     map[uuid.UUID]*SSOConfig // tenant_id → SSO config (cached)
	mu          sync.RWMutex
	logger      *zap.Logger
}

// NewService creates a new SSO service.
func NewService(userRepo core.UserRepository, tenantRepo core.TenantRepository, jwtCfg config.JWTConfig, logger *zap.Logger) *Service {
	return &Service{
		userRepo:    userRepo,
		tenantRepo:  tenantRepo,
		jwtCfg:      jwtCfg,
		groupMapper: NewGroupMapper(logger),
		configs:     make(map[uuid.UUID]*SSOConfig),
		logger:      logger,
	}
}

// GroupMapper returns the directory group mapping controller.
func (s *Service) GroupMapper() *GroupMapper {
	return s.groupMapper
}

// RegisterTenantSSO stores/updates SSO configuration for a tenant.
func (s *Service) RegisterTenantSSO(tenantID uuid.UUID, cfg *SSOConfig) error {
	if tenantID == uuid.Nil || cfg == nil {
		return ErrProviderNotConfigured
	}
	cfg.TenantID = tenantID
	s.mu.Lock()
	s.configs[tenantID] = cfg
	s.mu.Unlock()
	s.logger.Info("SSO config registered",
		zap.String("tenant_id", tenantID.String()),
		zap.String("provider", string(cfg.Provider)),
	)
	return nil
}

// GetTenantSSO returns the SSO config for a tenant.
func (s *Service) GetTenantSSO(tenantID uuid.UUID) (*SSOConfig, error) {
	s.mu.RLock()
	cfg, ok := s.configs[tenantID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrProviderNotConfigured
	}
	return cfg, nil
}

// RemoveTenantSSO disables SSO for a tenant.
func (s *Service) RemoveTenantSSO(tenantID uuid.UUID) {
	s.mu.Lock()
	delete(s.configs, tenantID)
	s.mu.Unlock()
}

// ListProviders returns available SSO login options for a tenant's login page.
func (s *Service) ListProviders(tenantID uuid.UUID) []ProviderInfo {
	s.mu.RLock()
	cfg, ok := s.configs[tenantID]
	s.mu.RUnlock()
	if !ok || !cfg.Enabled {
		return nil
	}

	var providers []ProviderInfo

	switch cfg.Provider {
	case ProviderSAML:
		providers = append(providers, ProviderInfo{Type: ProviderSAML, Name: "SSO (SAML)", Enabled: true})
	case ProviderOIDC:
		providers = append(providers, ProviderInfo{Type: ProviderOIDC, Name: "SSO (OIDC)", Enabled: true})
	case ProviderGoogle:
		providers = append(providers, ProviderInfo{Type: ProviderGoogle, Name: "Google", Enabled: true})
	case ProviderMicrosoft:
		providers = append(providers, ProviderInfo{Type: ProviderMicrosoft, Name: "Microsoft", Enabled: true})
	}

	// Some tenants may enable multiple providers
	if cfg.Google != nil && cfg.Provider != ProviderGoogle {
		providers = append(providers, ProviderInfo{Type: ProviderGoogle, Name: "Google", Enabled: true})
	}
	if cfg.Microsoft != nil && cfg.Provider != ProviderMicrosoft {
		providers = append(providers, ProviderInfo{Type: ProviderMicrosoft, Name: "Microsoft", Enabled: true})
	}

	return providers
}

// GetProvider returns the configured SSO provider for a tenant.
func (s *Service) GetProvider(tenantID uuid.UUID, providerType ProviderType) (Provider, error) {
	s.mu.RLock()
	cfg, ok := s.configs[tenantID]
	s.mu.RUnlock()
	if !ok || !cfg.Enabled {
		return nil, ErrProviderNotConfigured
	}

	switch providerType {
	case ProviderSAML:
		if cfg.SAML == nil {
			return nil, ErrProviderNotConfigured
		}
		return NewSAMLProvider(cfg.SAML, s.logger)
	case ProviderOIDC:
		if cfg.OIDC == nil {
			return nil, ErrProviderNotConfigured
		}
		return NewOIDCProvider(cfg.OIDC, s.logger)
	case ProviderGoogle:
		if cfg.Google == nil {
			return nil, ErrProviderNotConfigured
		}
		return NewGoogleProvider(cfg.Google, s.logger), nil
	case ProviderMicrosoft:
		if cfg.Microsoft == nil {
			return nil, ErrProviderNotConfigured
		}
		return NewMicrosoftProvider(cfg.Microsoft, s.logger), nil
	default:
		return nil, ErrUnsupportedProvider
	}
}

// ProcessSSOLogin handles the post-callback user resolution:
// find or auto-provision user, validate domain, issue JWT.
func (s *Service) ProcessSSOLogin(ctx context.Context, tenantID uuid.UUID, ssoUser *SSOUser) (*models.TokenResponse, error) {
	cfg, err := s.GetTenantSSO(tenantID)
	if err != nil {
		return nil, err
	}

	// Validate domain restriction
	if len(cfg.AllowedDomains) > 0 {
		domain := extractDomain(ssoUser.Email)
		if !containsString(cfg.AllowedDomains, domain) {
			return nil, fmt.Errorf("%w: %s", ErrDomainNotAllowed, domain)
		}
	}

	// Find existing user
	user, err := s.userRepo.GetByEmail(ctx, ssoUser.Email)
	if err != nil {
		if !errors.Is(err, core.ErrNotFound) {
			return nil, err
		}
		// User not found — auto-provision if enabled
		if !cfg.AutoProvision {
			return nil, fmt.Errorf("user %s not found and auto-provisioning is disabled", ssoUser.Email)
		}

		user, err = s.autoProvisionUser(ctx, tenantID, ssoUser, cfg)
		if err != nil {
			return nil, fmt.Errorf("auto-provisioning user: %w", err)
		}
	}

	// A matching email in another tenant is not the same identity and must never
	// be linked across tenant boundaries.
	if user.TenantID != tenantID {
		return nil, fmt.Errorf("user does not belong to tenant")
	}
	// Check user is active
	if user.Status != "active" {
		return nil, fmt.Errorf("user account is disabled")
	}

	// JIT (Just-In-Time) role synchronization based on IdP directory groups
	if len(ssoUser.Groups) > 0 {
		resolvedRole, matchedRule := s.groupMapper.ResolveRole(tenantID, ssoUser.Groups, user.Role)
		if matchedRule != nil && user.Role != resolvedRole {
			s.logger.Info("SSO JIT role updated from directory groups",
				zap.String("user", user.Email),
				zap.String("old_role", string(user.Role)),
				zap.String("new_role", string(resolvedRole)),
				zap.String("matched_group", matchedRule.GroupName),
			)
			user.Role = resolvedRole
		}
	}

	// Update last login
	now := time.Now()
	user.LastLoginAt = &now
	_ = s.userRepo.UpdateForTenant(ctx, tenantID, user)

	// Generate CLM JWT token
	token, err := s.generateToken(user)
	if err != nil {
		return nil, err
	}

	s.logger.Info("SSO login completed",
		zap.String("email", user.Email),
		zap.String("provider", string(ssoUser.Provider)),
		zap.String("tenant_id", tenantID.String()),
	)

	return token, nil
}

// autoProvisionUser creates a new user from SSO identity.
func (s *Service) autoProvisionUser(ctx context.Context, tenantID uuid.UUID, ssoUser *SSOUser, cfg *SSOConfig) (*models.User, error) {
	role := cfg.DefaultRole
	if role == "" {
		role = models.RoleViewer
	}

	if len(ssoUser.Groups) > 0 {
		resolvedRole, matchedRule := s.groupMapper.ResolveRole(tenantID, ssoUser.Groups, role)
		if matchedRule != nil {
			s.logger.Info("auto-provisioned user assigned role via directory group mapping",
				zap.String("user", ssoUser.Email),
				zap.String("role", string(resolvedRole)),
				zap.String("matched_group", matchedRule.GroupName),
			)
			role = resolvedRole
		}
	}

	user := &models.User{
		ID:           uuid.New(),
		TenantID:     tenantID,
		Email:        ssoUser.Email,
		Name:         ssoUser.Name,
		Role:         role,
		Status:       "active",
		PasswordHash: "", // SSO users don't have a local password
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	s.logger.Info("user auto-provisioned via SSO",
		zap.String("email", user.Email),
		zap.String("provider", string(ssoUser.Provider)),
		zap.String("role", string(role)),
	)

	return user, nil
}

func (s *Service) generateToken(user *models.User) (*models.TokenResponse, error) {
	claims := jwt.MapClaims{
		"sub":       user.ID.String(),
		"email":     user.Email,
		"name":      user.Name,
		"role":      string(user.Role),
		"tenant_id": user.TenantID.String(),
		"iss":       s.jwtCfg.Issuer,
		"aud":       s.jwtCfg.Audience,
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Duration(s.jwtCfg.ExpiryMinutes) * time.Minute).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString([]byte(s.jwtCfg.Secret))
	if err != nil {
		return nil, err
	}

	// Refresh token
	refreshClaims := jwt.RegisteredClaims{
		Subject:   user.ID.String(),
		Issuer:    s.jwtCfg.Issuer,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.jwtCfg.RefreshDays) * 24 * time.Hour)),
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshStr, err := refreshToken.SignedString([]byte(s.jwtCfg.Secret))
	if err != nil {
		return nil, err
	}

	return &models.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshStr,
		ExpiresIn:    s.jwtCfg.ExpiryMinutes * 60,
		TokenType:    "Bearer",
	}, nil
}

// ProviderInfo is returned to the frontend to render login buttons.
type ProviderInfo struct {
	Type    ProviderType `json:"type"`
	Name    string       `json:"name"`
	Enabled bool         `json:"enabled"`
}

// ResolveTenantFromEmail finds the tenant by the user's email domain.
func (s *Service) ResolveTenantFromEmail(ctx context.Context, email string) (uuid.UUID, error) {
	domain := extractDomain(email)
	if domain == "" {
		return uuid.Nil, fmt.Errorf("invalid email: %s", email)
	}

	// Search configs for matching domain
	s.mu.RLock()
	defer s.mu.RUnlock()
	for tenantID, cfg := range s.configs {
		if cfg.Enabled {
			for _, d := range cfg.AllowedDomains {
				if strings.EqualFold(d, domain) {
					return tenantID, nil
				}
			}
		}
	}

	return uuid.Nil, fmt.Errorf("no tenant configured for domain: %s", domain)
}
