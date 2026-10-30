package models

import (
	"time"

	"github.com/google/uuid"
)

// Tenant represents an isolated organizational unit in the SaaS platform.
type Tenant struct {
	ID             uuid.UUID `json:"id" db:"id"`
	Name           string    `json:"name" db:"name"`
	Slug           string    `json:"slug" db:"slug"`
	Plan           string    `json:"plan" db:"plan"` // free, pro, enterprise
	Status         string    `json:"status" db:"status"`
	MaxCerts       int       `json:"max_certs" db:"max_certs"`
	MaxSatellites  int       `json:"max_satellites" db:"max_satellites"`
	MaxUsers       int       `json:"max_users" db:"max_users"`
	SSOEnabled     bool      `json:"sso_enabled" db:"sso_enabled"`
	SSOProvider    string    `json:"sso_provider,omitempty" db:"sso_provider"`
	SSOConfigJSON  string    `json:"-" db:"sso_config"` // JSON blob stored in DB
	DataRegion     string    `json:"data_region" db:"data_region"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// TenantSSOConfig holds the full SSO configuration for a tenant (stored as JSON in sso_config column).
type TenantSSOConfig struct {
	Enabled        bool     `json:"enabled"`
	Provider       string   `json:"provider"` // saml, oidc, google, microsoft
	AllowedDomains []string `json:"allowed_domains"`

	// Auto-provisioning
	AutoProvision bool     `json:"auto_provision"`
	DefaultRole   UserRole `json:"default_role"`
	DefaultTeam   string   `json:"default_team"`

	// Provider-specific
	SAML      *TenantSAMLConfig      `json:"saml,omitempty"`
	OIDC      *TenantOIDCConfig      `json:"oidc,omitempty"`
	Google    *TenantGoogleConfig    `json:"google,omitempty"`
	Microsoft *TenantMicrosoftConfig `json:"microsoft,omitempty"`
}

// TenantSAMLConfig holds SAML 2.0 settings per tenant.
type TenantSAMLConfig struct {
	IDPMetadataURL string            `json:"idp_metadata_url"`
	IDPSSOURL      string            `json:"idp_sso_url"`
	IDPCert        string            `json:"idp_cert"`
	SPEntityID     string            `json:"sp_entity_id"`
	SPACSURL       string            `json:"sp_acs_url"`
	AttributeMap   map[string]string `json:"attribute_mapping"`
}

// TenantOIDCConfig holds generic OIDC settings per tenant.
type TenantOIDCConfig struct {
	Issuer       string            `json:"issuer"`
	ClientID     string            `json:"client_id"`
	ClientSecret string            `json:"client_secret"`
	RedirectURI  string            `json:"redirect_uri"`
	Scopes       []string          `json:"scopes"`
	ClaimMapping map[string]string `json:"claim_mapping"`
}

// TenantGoogleConfig holds Google OAuth2 settings per tenant.
type TenantGoogleConfig struct {
	ClientID       string   `json:"client_id"`
	ClientSecret   string   `json:"client_secret"`
	RedirectURI    string   `json:"redirect_uri"`
	AllowedDomains []string `json:"allowed_domains"`
}

// TenantMicrosoftConfig holds Microsoft/Azure AD OAuth2 settings per tenant.
type TenantMicrosoftConfig struct {
	AzureTenantID  string   `json:"tenant_id"`
	ClientID       string   `json:"client_id"`
	ClientSecret   string   `json:"client_secret"`
	RedirectURI    string   `json:"redirect_uri"`
	AllowedDomains []string `json:"allowed_domains"`
}

// TenantSettings holds configurable settings per tenant.
type TenantSettings struct {
	TenantID             uuid.UUID `json:"tenant_id" db:"tenant_id"`
	DefaultKeyAlgorithm  string    `json:"default_key_algorithm" db:"default_key_algorithm"`
	DefaultValidity      string    `json:"default_validity" db:"default_validity"`
	MinKeySize           int       `json:"min_key_size" db:"min_key_size"`
	AllowedCAs           []string  `json:"allowed_cas" db:"allowed_cas"`
	AllowSelfSigned      bool      `json:"allow_self_signed" db:"allow_self_signed"`
	AutoRenewalDays      int       `json:"auto_renewal_days" db:"auto_renewal_days"`
	MonthlyBudget        float64   `json:"monthly_budget" db:"monthly_budget"`
	AlertChannels        []string  `json:"alert_channels" db:"alert_channels"`
}
