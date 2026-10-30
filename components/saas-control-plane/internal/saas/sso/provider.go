package sso

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
)

var (
	ErrProviderNotConfigured = errors.New("SSO provider not configured for this tenant")
	ErrInvalidToken          = errors.New("invalid or expired SSO token")
	ErrDomainNotAllowed      = errors.New("email domain not allowed for this tenant")
	ErrCallbackFailed        = errors.New("SSO callback processing failed")
	ErrUnsupportedProvider   = errors.New("unsupported SSO provider type")
)

// ProviderType identifies the SSO protocol/provider.
type ProviderType string

const (
	ProviderSAML      ProviderType = "saml"
	ProviderOIDC      ProviderType = "oidc"
	ProviderGoogle    ProviderType = "google"
	ProviderMicrosoft ProviderType = "microsoft"
)

// SSOConfig holds the per-tenant SSO configuration.
type SSOConfig struct {
	TenantID       uuid.UUID    `json:"tenant_id" db:"tenant_id"`
	Enabled        bool         `json:"enabled" db:"enabled"`
	Provider       ProviderType `json:"provider" db:"provider"`
	AllowedDomains []string     `json:"allowed_domains" db:"allowed_domains"`

	// Auto-provisioning settings
	AutoProvision  bool            `json:"auto_provision"`
	DefaultRole    models.UserRole `json:"default_role"`
	DefaultTeam    string          `json:"default_team"`

	// Provider-specific config
	SAML      *SAMLConfig      `json:"saml,omitempty"`
	OIDC      *OIDCConfig      `json:"oidc,omitempty"`
	Google    *GoogleConfig    `json:"google,omitempty"`
	Microsoft *MicrosoftConfig `json:"microsoft,omitempty"`
}

// SAMLConfig holds SAML 2.0 SP configuration.
type SAMLConfig struct {
	IDPMetadataURL string            `json:"idp_metadata_url"`
	IDPSSOURL      string            `json:"idp_sso_url"`
	IDPCert        string            `json:"idp_cert"`
	SPEntityID     string            `json:"sp_entity_id"`
	SPACSURL       string            `json:"sp_acs_url"`
	AttributeMap   map[string]string `json:"attribute_mapping"`
}

// OIDCConfig holds generic OpenID Connect configuration.
type OIDCConfig struct {
	Issuer       string            `json:"issuer"`
	ClientID     string            `json:"client_id"`
	ClientSecret string            `json:"client_secret"`
	RedirectURI  string            `json:"redirect_uri"`
	Scopes       []string          `json:"scopes"`
	ClaimMapping map[string]string `json:"claim_mapping"`
}

// GoogleConfig holds Google OAuth2/OIDC configuration.
type GoogleConfig struct {
	ClientID       string   `json:"client_id"`
	ClientSecret   string   `json:"client_secret"`
	RedirectURI    string   `json:"redirect_uri"`
	AllowedDomains []string `json:"allowed_domains"`
}

// MicrosoftConfig holds Microsoft/Azure AD OAuth2 configuration.
type MicrosoftConfig struct {
	TenantID       string   `json:"tenant_id"`
	ClientID       string   `json:"client_id"`
	ClientSecret   string   `json:"client_secret"`
	RedirectURI    string   `json:"redirect_uri"`
	AllowedDomains []string `json:"allowed_domains"`
}

// SSOUser represents user identity extracted from an SSO assertion/token.
type SSOUser struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Provider ProviderType `json:"provider"`
	// Optional IdP-provided attributes
	Groups []string `json:"groups,omitempty"`
	Role   string   `json:"role,omitempty"` // mapped from IdP groups/attributes
}

// AuthURL is the redirect URL to initiate SSO login.
type AuthURL struct {
	URL      string `json:"url"`
	State    string `json:"state"`
	Provider ProviderType `json:"provider"`
}

// Provider defines the interface for all SSO providers.
type Provider interface {
	// Type returns the provider type identifier.
	Type() ProviderType
	// GetAuthURL returns the URL to redirect the user for authentication.
	GetAuthURL(ctx context.Context, state string) (*AuthURL, error)
	// HandleCallback processes the callback from the identity provider.
	HandleCallback(ctx context.Context, params map[string]string) (*SSOUser, error)
	// ValidateConfig checks if the provider configuration is valid.
	ValidateConfig() error
}
