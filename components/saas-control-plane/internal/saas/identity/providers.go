package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ListIdentityProviders returns the realm's configured external IdPs. Client
// secrets are masked before leaving this layer.
func (c *Client) ListIdentityProviders(ctx context.Context) ([]IdentityProvider, error) {
	raw, err := c.do(ctx, http.MethodGet, "/identity-provider/instances", nil)
	if err != nil {
		return nil, err
	}
	var idps []IdentityProvider
	if err := json.Unmarshal(raw, &idps); err != nil {
		return nil, fmt.Errorf("decode identity providers: %w", err)
	}
	for i := range idps {
		maskSecrets(idps[i].Config)
	}
	return idps, nil
}

// maskSecrets replaces sensitive config values so they are never returned to
// the browser.
func maskSecrets(cfg map[string]string) {
	if cfg == nil {
		return
	}
	for _, k := range []string{"clientSecret", "client_secret", "bindCredential"} {
		if v, ok := cfg[k]; ok && v != "" {
			cfg[k] = "********"
		}
	}
}

// CreateIdentityProvider registers a new external IdP (google, microsoft,
// github, oidc, saml, ...).
func (c *Client) CreateIdentityProvider(ctx context.Context, idp IdentityProvider) error {
	if idp.FirstBrokerLoginFlowAlias == "" {
		idp.FirstBrokerLoginFlowAlias = "first broker login"
	}
	_, err := c.do(ctx, http.MethodPost, "/identity-provider/instances", idp)
	return err
}

// UpdateIdentityProvider replaces an existing IdP definition. A masked secret
// is treated as "unchanged" so the caller does not have to resend it.
func (c *Client) UpdateIdentityProvider(ctx context.Context, alias string, idp IdentityProvider) error {
	if idp.Config != nil && idp.Config["clientSecret"] == "********" {
		existing, err := c.getIdentityProviderRaw(ctx, alias)
		if err != nil {
			return err
		}
		if existing.Config != nil {
			idp.Config["clientSecret"] = existing.Config["clientSecret"]
		}
	}
	idp.Alias = alias
	_, err := c.do(ctx, http.MethodPut,
		"/identity-provider/instances/"+url.PathEscape(alias), idp)
	return err
}

// getIdentityProviderRaw fetches an IdP with secrets intact (internal use).
func (c *Client) getIdentityProviderRaw(ctx context.Context, alias string) (*IdentityProvider, error) {
	raw, err := c.do(ctx, http.MethodGet,
		"/identity-provider/instances/"+url.PathEscape(alias), nil)
	if err != nil {
		return nil, err
	}
	var idp IdentityProvider
	if err := json.Unmarshal(raw, &idp); err != nil {
		return nil, fmt.Errorf("decode identity provider: %w", err)
	}
	return &idp, nil
}

// DeleteIdentityProvider removes an external IdP.
func (c *Client) DeleteIdentityProvider(ctx context.Context, alias string) error {
	_, err := c.do(ctx, http.MethodDelete,
		"/identity-provider/instances/"+url.PathEscape(alias), nil)
	return err
}

// SocialProviders are the IdP types the portal can configure with just a
// client id and secret; Keycloak knows their endpoints already.
var SocialProviders = map[string]string{
	"google":    "Google",
	"microsoft": "Microsoft",
	"github":    "GitHub",
	"gitlab":    "GitLab",
	"linkedin":  "LinkedIn",
	"bitbucket": "Bitbucket",
}

// NewSocialProvider builds an IdP representation for a built-in social login.
func NewSocialProvider(providerID, clientID, clientSecret string, defaultScope string) IdentityProvider {
	display, ok := SocialProviders[providerID]
	if !ok {
		display = providerID
	}
	cfg := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"syncMode":     "IMPORT",
		"useJwksUrl":   "true",
	}
	if defaultScope != "" {
		cfg["defaultScope"] = defaultScope
	}
	return IdentityProvider{
		Alias:       providerID,
		DisplayName: display,
		ProviderID:  providerID,
		Enabled:     true,
		TrustEmail:  true,
		Config:      cfg,
	}
}

// NewOIDCProvider builds a generic OIDC IdP representation.
func NewOIDCProvider(alias, displayName, clientID, clientSecret, authURL, tokenURL, userInfoURL, issuer string) IdentityProvider {
	return IdentityProvider{
		Alias:       alias,
		DisplayName: displayName,
		ProviderID:  "oidc",
		Enabled:     true,
		TrustEmail:  true,
		Config: map[string]string{
			"clientId":         clientID,
			"clientSecret":     clientSecret,
			"authorizationUrl": authURL,
			"tokenUrl":         tokenURL,
			"userInfoUrl":      userInfoURL,
			"issuer":           issuer,
			"clientAuthMethod": "client_secret_post",
			"defaultScope":     "openid profile email",
			"syncMode":         "IMPORT",
		},
	}
}
