package sso

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// OIDCProvider implements OpenID Connect authorization code flow.
type OIDCProvider struct {
	config     *OIDCConfig
	logger     *zap.Logger
	httpClient *http.Client
	// Cached discovery document
	discovery *oidcDiscovery
}

type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// NewOIDCProvider creates a new generic OIDC provider.
func NewOIDCProvider(config *OIDCConfig, logger *zap.Logger) (*OIDCProvider, error) {
	p := &OIDCProvider{
		config:     config,
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	// Fetch OIDC discovery document
	if config.Issuer != "" {
		disc, err := p.fetchDiscovery(context.Background())
		if err != nil {
			logger.Warn("failed to fetch OIDC discovery (will retry on use)", zap.Error(err))
		} else {
			p.discovery = disc
		}
	}

	return p, nil
}

func (p *OIDCProvider) Type() ProviderType {
	return ProviderOIDC
}

// GetAuthURL returns the OIDC authorization URL for redirect.
func (p *OIDCProvider) GetAuthURL(ctx context.Context, state string) (*AuthURL, error) {
	if p.discovery == nil {
		disc, err := p.fetchDiscovery(ctx)
		if err != nil {
			return nil, fmt.Errorf("OIDC discovery failed: %w", err)
		}
		p.discovery = disc
	}

	scopes := p.config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile"}
	}

	params := url.Values{
		"client_id":     {p.config.ClientID},
		"redirect_uri":  {p.config.RedirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(scopes, " ")},
		"state":         {state},
	}

	authURL := p.discovery.AuthorizationEndpoint + "?" + params.Encode()

	return &AuthURL{
		URL:      authURL,
		State:    state,
		Provider: ProviderOIDC,
	}, nil
}

// HandleCallback exchanges the authorization code for tokens and extracts user info.
func (p *OIDCProvider) HandleCallback(ctx context.Context, params map[string]string) (*SSOUser, error) {
	code := params["code"]
	if code == "" {
		return nil, errors.New("missing authorization code")
	}

	if params["error"] != "" {
		return nil, fmt.Errorf("OIDC error: %s - %s", params["error"], params["error_description"])
	}

	if p.discovery == nil {
		disc, err := p.fetchDiscovery(ctx)
		if err != nil {
			return nil, fmt.Errorf("OIDC discovery failed: %w", err)
		}
		p.discovery = disc
	}

	// Exchange code for token
	tokenResp, err := p.exchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	// Get user info
	userInfo, err := p.getUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}

	user := &SSOUser{
		Email:    userInfo.Email,
		Name:     userInfo.Name,
		Provider: ProviderOIDC,
	}

	if user.Email == "" {
		return nil, errors.New("email not found in OIDC userinfo")
	}

	p.logger.Info("OIDC login successful",
		zap.String("email", user.Email),
		zap.String("issuer", p.config.Issuer),
	)

	return user, nil
}

// ValidateConfig checks OIDC configuration completeness.
func (p *OIDCProvider) ValidateConfig() error {
	if p.config.Issuer == "" {
		return errors.New("OIDC: issuer is required")
	}
	if p.config.ClientID == "" {
		return errors.New("OIDC: client_id is required")
	}
	if p.config.ClientSecret == "" {
		return errors.New("OIDC: client_secret is required")
	}
	if p.config.RedirectURI == "" {
		return errors.New("OIDC: redirect_uri is required")
	}
	return nil
}

// --- Internal helpers ---

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type userInfoResponse struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

func (p *OIDCProvider) fetchDiscovery(ctx context.Context) (*oidcDiscovery, error) {
	discoveryURL := strings.TrimRight(p.config.Issuer, "/") + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery endpoint returned %d", resp.StatusCode)
	}

	var disc oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		return nil, err
	}

	return &disc, nil
}

func (p *OIDCProvider) exchangeCode(ctx context.Context, code string) (*tokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.config.RedirectURI},
		"client_id":     {p.config.ClientID},
		"client_secret": {p.config.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.discovery.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

func (p *OIDCProvider) getUserInfo(ctx context.Context, accessToken string) (*userInfoResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.discovery.UserinfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned %d", resp.StatusCode)
	}

	var info userInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}
