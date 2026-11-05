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
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"  // #nosec G101 -- fixed public OAuth endpoint.
	googleTokenURL    = "https://oauth2.googleapis.com/token"           // #nosec G101 -- fixed public OAuth endpoint.
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo" // #nosec G101 -- fixed public API endpoint.
)

// GoogleProvider implements Google OAuth2/OIDC login.
type GoogleProvider struct {
	config     *GoogleConfig
	logger     *zap.Logger
	httpClient *http.Client
}

// NewGoogleProvider creates a new Google SSO provider.
func NewGoogleProvider(config *GoogleConfig, logger *zap.Logger) *GoogleProvider {
	return &GoogleProvider{
		config:     config,
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *GoogleProvider) Type() ProviderType {
	return ProviderGoogle
}

// GetAuthURL returns the Google OAuth2 consent screen URL.
func (p *GoogleProvider) GetAuthURL(ctx context.Context, state string) (*AuthURL, error) {
	params := url.Values{
		"client_id":     {p.config.ClientID},
		"redirect_uri":  {p.config.RedirectURI},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"access_type":   {"offline"},
		"prompt":        {"select_account"},
	}

	// Restrict to hosted domain if configured
	if len(p.config.AllowedDomains) == 1 {
		params.Set("hd", p.config.AllowedDomains[0])
	}

	authURL := googleAuthURL + "?" + params.Encode()

	return &AuthURL{
		URL:      authURL,
		State:    state,
		Provider: ProviderGoogle,
	}, nil
}

// HandleCallback exchanges the Google auth code for tokens and retrieves user info.
func (p *GoogleProvider) HandleCallback(ctx context.Context, params map[string]string) (*SSOUser, error) {
	code := params["code"]
	if code == "" {
		return nil, errors.New("missing authorization code from Google")
	}

	if errMsg := params["error"]; errMsg != "" {
		return nil, fmt.Errorf("Google auth error: %s", errMsg)
	}

	// Exchange code for token
	tokenResp, err := p.exchangeCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// Get user info from Google
	userInfo, err := p.getUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, err
	}

	// Validate domain restriction
	if len(p.config.AllowedDomains) > 0 {
		domain := extractDomain(userInfo.Email)
		if !containsString(p.config.AllowedDomains, domain) {
			return nil, fmt.Errorf("%w: %s not in allowed domains", ErrDomainNotAllowed, domain)
		}
	}

	user := &SSOUser{
		Email:    userInfo.Email,
		Name:     userInfo.Name,
		Provider: ProviderGoogle,
	}

	p.logger.Info("Google SSO login successful",
		zap.String("email", user.Email),
		zap.String("name", user.Name),
	)

	return user, nil
}

// ValidateConfig checks Google SSO configuration.
func (p *GoogleProvider) ValidateConfig() error {
	if p.config.ClientID == "" {
		return errors.New("Google: client_id is required")
	}
	if p.config.ClientSecret == "" {
		return errors.New("Google: client_secret is required")
	}
	if p.config.RedirectURI == "" {
		return errors.New("Google: redirect_uri is required")
	}
	return nil
}

// --- Internal ---

type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
}

type googleUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	HD            string `json:"hd"` // Hosted domain (Google Workspace)
}

func (p *GoogleProvider) exchangeCode(ctx context.Context, code string) (*googleTokenResponse, error) {
	data := url.Values{
		"code":          {code},
		"client_id":     {p.config.ClientID},
		"client_secret": {p.config.ClientSecret},
		"redirect_uri":  {p.config.RedirectURI},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Google token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp googleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

func (p *GoogleProvider) getUserInfo(ctx context.Context, accessToken string) (*googleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google userinfo returned %d", resp.StatusCode)
	}

	var info googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}

// --- Utility functions ---

func extractDomain(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
