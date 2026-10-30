// Package identity provides a thin client over the Keycloak Admin REST API so
// that the CLM Admin Portal can manage realm users, roles, identity providers
// and sessions natively, without exposing the Keycloak Admin Console.
package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Config holds the connection details for the Keycloak Admin API.
type Config struct {
	// BaseURL is the Keycloak root including any relative path,
	// e.g. http://keycloak:8180/auth
	BaseURL string
	// Realm is the realm being managed, e.g. clm
	Realm string
	// AdminRealm is the realm holding the admin account, normally master.
	AdminRealm string
	// AdminUser / AdminPass authenticate against admin-cli.
	AdminUser string
	AdminPass string
}

// Client talks to the Keycloak Admin REST API.
type Client struct {
	cfg    Config
	http   *http.Client
	logger *zap.Logger

	mu        sync.Mutex
	token     string
	tokenExp  time.Time
}

// NewClient builds a Keycloak Admin API client, applying sane defaults.
func NewClient(cfg Config, logger *zap.Logger) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://keycloak:8180/auth"
	}
	if cfg.Realm == "" {
		cfg.Realm = "clm"
	}
	if cfg.AdminRealm == "" {
		cfg.AdminRealm = "master"
	}
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	return &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: 20 * time.Second},
		logger: logger,
	}
}

// Realm returns the realm this client manages.
func (c *Client) Realm() string { return c.cfg.Realm }

// token fetches (and caches) an admin access token via the password grant on
// admin-cli. Client-credentials on a confidential client was avoided because
// granting realm-management roles through realm-import is unreliable.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", "admin-cli")
	form.Set("username", c.cfg.AdminUser)
	form.Set("password", c.cfg.AdminPass)

	endpoint := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		c.cfg.BaseURL, c.cfg.AdminRealm)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keycloak admin login failed (%d): %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("keycloak returned an empty access token")
	}

	c.token = tr.AccessToken
	// Refresh 30s early to avoid using a token that expires mid-request.
	lifetime := time.Duration(tr.ExpiresIn) * time.Second
	if lifetime > 30*time.Second {
		lifetime -= 30 * time.Second
	}
	c.tokenExp = time.Now().Add(lifetime)
	return c.token, nil
}

// invalidateToken drops the cached token so the next call re-authenticates.
func (c *Client) invalidateToken() {
	c.mu.Lock()
	c.token = ""
	c.tokenExp = time.Time{}
	c.mu.Unlock()
}

// do performs an authenticated Admin API call. path is relative to the realm
// admin root, e.g. "/users?first=0". Pass an empty path segment starting with
// "!" to target the admin root instead of the realm root.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	out, status, err := c.doRaw(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	// One transparent retry when the cached token was rejected.
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		c.invalidateToken()
		out, status, err = c.doRaw(ctx, method, path, body)
		if err != nil {
			return nil, err
		}
	}
	if status < 200 || status > 299 {
		return nil, &APIError{Status: status, Body: strings.TrimSpace(string(out))}
	}
	return out, nil
}

// APIError carries a non-2xx response from Keycloak.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	msg := e.Body
	// Keycloak returns {"errorMessage":"..."} or {"error":"..."}.
	var payload struct {
		ErrorMessage string `json:"errorMessage"`
		Error        string `json:"error"`
	}
	if json.Unmarshal([]byte(e.Body), &payload) == nil {
		if payload.ErrorMessage != "" {
			msg = payload.ErrorMessage
		} else if payload.Error != "" {
			msg = payload.Error
		}
	}
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return msg
}

// StatusCode exposes the upstream HTTP status for handler mapping.
func (e *APIError) StatusCode() int { return e.Status }

func (c *Client) doRaw(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, 0, err
	}

	var endpoint string
	if strings.HasPrefix(path, "!") {
		// Admin root scope (e.g. !/serverinfo)
		endpoint = c.cfg.BaseURL + "/admin" + strings.TrimPrefix(path, "!")
	} else {
		endpoint = fmt.Sprintf("%s/admin/realms/%s%s", c.cfg.BaseURL, c.cfg.Realm, path)
	}

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("keycloak request failed: %w", err)
	}
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return out, resp.StatusCode, nil
}
