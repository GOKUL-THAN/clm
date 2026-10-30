package models

import (
	"time"

	"github.com/google/uuid"
)

// UserRole defines the access level of a user within a tenant.
type UserRole string

const (
	RoleSuperAdmin  UserRole = "super_admin"
	RoleTenantAdmin UserRole = "tenant_admin"
	RoleCertManager UserRole = "cert_manager"
	RoleApprover    UserRole = "approver"
	RoleOperator    UserRole = "operator"
	RoleViewer      UserRole = "viewer"
	RoleAuditor     UserRole = "auditor"
	RoleFinance     UserRole = "finance"
	RoleServiceAcct UserRole = "service_account"
)

// User represents a platform user.
type User struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	TenantID     uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Email        string     `json:"email" db:"email"`
	Name         string     `json:"name" db:"name"`
	Role         UserRole   `json:"role" db:"role"`
	TeamID       *uuid.UUID `json:"team_id,omitempty" db:"team_id"`
	Status       string     `json:"status" db:"status"` // active, disabled, invited
	MFAEnabled   bool       `json:"mfa_enabled" db:"mfa_enabled"`
	PasswordHash string     `json:"-" db:"password_hash"`
	AuthProvider string     `json:"auth_provider,omitempty" db:"auth_provider"` // local, saml, oidc, google, microsoft
	SSOSubject   string     `json:"-" db:"sso_subject"`                        // IdP subject/nameID for linking
	LastLoginAt  *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

// Team groups users for ownership and access control.
type Team struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// APIKey represents a programmatic access key.
type APIKey struct {
	ID         uuid.UUID `json:"id" db:"id"`
	TenantID   uuid.UUID `json:"tenant_id" db:"tenant_id"`
	UserID     uuid.UUID `json:"user_id" db:"user_id"`
	Name       string    `json:"name" db:"name"`
	KeyHash    string    `json:"-" db:"key_hash"`
	Prefix     string    `json:"prefix" db:"prefix"` // first 8 chars for identification
	Scopes     []string  `json:"scopes" db:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty" db:"last_used_at"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// LoginRequest represents a login attempt.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// TokenResponse is returned after successful authentication.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}
