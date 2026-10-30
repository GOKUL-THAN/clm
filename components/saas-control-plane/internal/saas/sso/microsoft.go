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

const (
	microsoftAuthURLTemplate  = "https://login.microsoftonline.com/%s/oauth2/v2.0/authorize"
	microsoftTokenURLTemplate = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
	microsoftGraphMeURL       = "https://graph.microsoft.com/v1.0/me"
)

// MicrosoftProvider implements Microsoft/Azure AD OAuth2 login.
type MicrosoftProvider struct {
	config     *MicrosoftConfig
	logger     *zap.Logger
	httpClient *http.Client
}

// NewMicrosoftProvider creates a new Microsoft SSO provider.
func NewMicrosoftProvider(config *MicrosoftConfig, logger *zap.Logger) *MicrosoftProvider {
	return &MicrosoftProvider{
		config:     config,
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *MicrosoftProvider) Type() ProviderType {
	return ProviderMicrosoft
}

// GetAuthURL returns the Microsoft OAuth2 authorization URL.
func (p *MicrosoftProvider) GetAuthURL(ctx context.Context, state string) (*AuthURL, error) {
	tenantID := p.config.TenantID
	if tenantID == "" {
		tenantID = "common" // Multi-tenant: allow any Microsoft account
	}

	authEndpoint := fmt.Sprintf(microsoftAuthURLTemplate, tenantID)

	params := url.Values{
		"client_id":     {p.config.ClientID},
		"redirect_uri":  {p.config.RedirectURI},
		"response_type": {"code"},
		"scope":         {"openid email profile User.Read"},
		"state":         {state},
		"response_mode": {"query"},
	}

	authURL := authEndpoint + "?" + params.Encode()

	return &AuthURL{
		URL:      authURL,
		State:    state,
		Provider: ProviderMicrosoft,
	}, nil
}

// HandleCallback exchanges the Microsoft auth code and retrieves user profile.
func (p *MicrosoftProvider) HandleCallback(ctx context.Context, params map[string]string) (*SSOUser, error) {
	code := params["code"]
	if code == "" {
		return nil, errors.New("missing authorization code from Microsoft")
	}

	if errMsg := params["error"]; errMsg != "" {
		desc := params["error_description"]
		return nil, fmt.Errorf("Microsoft auth error: %s - %s", errMsg, desc)
	}

	// Exchange code for token
	tokenResp, err := p.exchangeCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// Get user profile from Microsoft Graph
	profile, err := p.getProfile(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, err
	}

	// Validate domain restriction
	if len(p.config.AllowedDomains) > 0 {
		domain := extractDomain(profile.Mail)
		if domain == "" {
			domain = extractDomain(profile.UserPrincipalName)
		}
		if !containsString(p.config.AllowedDomains, domain) {
			return nil, fmt.Errorf("%w: %s not in allowed domains", ErrDomainNotAllowed, domain)
		}
	}

	email := profile.Mail
	if email == "" {
		email = profile.UserPrincipalName
	}

	user := &SSOUser{
		Email:    email,
		Name:     profile.DisplayName,
		Provider: ProviderMicrosoft,
	}

	if user.Email == "" {
		return nil, errors.New("could not determine email from Microsoft profile")
	}

	p.logger.Info("Microsoft SSO login successful",
		zap.String("email", user.Email),
		zap.String("name", user.Name),
	)

	return user, nil
}

// ValidateConfig checks Microsoft SSO configuration.
func (p *MicrosoftProvider) ValidateConfig() error {
	if p.config.ClientID == "" {
		return errors.New("Microsoft: client_id is required")
	}
	if p.config.ClientSecret == "" {
		return errors.New("Microsoft: client_secret is required")
	}
	if p.config.RedirectURI == "" {
		return errors.New("Microsoft: redirect_uri is required")
	}
	return nil
}

// --- Internal ---

type microsoftTokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type microsoftProfile struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
	JobTitle          string `json:"jobTitle"`
}

func (p *MicrosoftProvider) exchangeCode(ctx context.Context, code string) (*microsoftTokenResponse, error) {
	tenantID := p.config.TenantID
	if tenantID == "" {
		tenantID = "common"
	}
	tokenEndpoint := fmt.Sprintf(microsoftTokenURLTemplate, tenantID)

	data := url.Values{
		"client_id":     {p.config.ClientID},
		"client_secret": {p.config.ClientSecret},
		"code":          {code},
		"redirect_uri":  {p.config.RedirectURI},
		"grant_type":    {"authorization_code"},
		"scope":         {"openid email profile User.Read"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Microsoft token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Microsoft token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp microsoftTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

func (p *MicrosoftProvider) getProfile(ctx context.Context, accessToken string) (*microsoftProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, microsoftGraphMeURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Microsoft Graph request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Microsoft Graph returned %d: %s", resp.StatusCode, string(body))
	}

	var profile microsoftProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}

	return &profile, nil
}
