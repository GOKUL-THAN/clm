// Package api serves the Satellite Gateway's local REST API, which backs the
// Gateway Portal UI. It is deliberately separate from the vSatellite
// coordinator API (port 9090) that endpoint agents talk to.
package api

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// Claims covers both token flavours the portal can present: Keycloak RS256
// tokens (roles at the top level in the clm realm) and the control plane's own
// HS256 tokens (a single role claim).
type Claims struct {
	Email         string   `json:"email"`
	PreferredUser string   `json:"preferred_username"`
	Role          string   `json:"role"`
	Roles         []string `json:"roles"`
	RealmAccess   *struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
	TenantID string `json:"tenant_id"`
	jwt.RegisteredClaims
}

// EffectiveRole picks the caller's role, ignoring Keycloak's internal roles.
func (c *Claims) EffectiveRole() string {
	if c.Role != "" {
		return c.Role
	}
	roles := c.Roles
	if len(roles) == 0 && c.RealmAccess != nil {
		roles = c.RealmAccess.Roles
	}
	for _, r := range roles {
		if strings.HasPrefix(r, "default-roles-") ||
			r == "offline_access" || r == "uma_authorization" {
			continue
		}
		return r
	}
	return "viewer"
}

func isGlobalAdmin(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "super_admin", "admin":
		return true
	default:
		return false
	}
}

// Authenticator validates bearer tokens from the Gateway Portal.
type Authenticator struct {
	hmacSecret []byte
	jwksURL    string
	logger     *zap.Logger

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	http      *http.Client
}

// NewAuthenticator builds a validator that accepts Keycloak RS256 tokens
// (verified against the realm's JWKS) and internal HS256 tokens.
func NewAuthenticator(hmacSecret, jwksURL string, logger *zap.Logger) *Authenticator {
	return &Authenticator{
		hmacSecret: []byte(hmacSecret),
		jwksURL:    jwksURL,
		logger:     logger,
		keys:       make(map[string]*rsa.PublicKey),
		http:       &http.Client{Timeout: 10 * time.Second},
	}
}

// Parse validates a raw token string and returns its claims.
func (a *Authenticator) Parse(raw string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		switch t.Method.Alg() {
		case "HS256":
			if len(a.hmacSecret) == 0 {
				return nil, fmt.Errorf("HS256 tokens not accepted: no shared secret configured")
			}
			return a.hmacSecret, nil
		case "RS256":
			kid, _ := t.Header["kid"].(string)
			return a.rsaKey(kid)
		default:
			return nil, fmt.Errorf("unexpected signing algorithm: %s", t.Method.Alg())
		}
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// rsaKey returns the realm signing key for kid, refreshing the JWKS at most
// once a minute when the key is unknown (Keycloak rotates keys).
func (a *Authenticator) rsaKey(kid string) (*rsa.PublicKey, error) {
	a.mu.RLock()
	key, ok := a.keys[kid]
	fetched := a.fetchedAt
	a.mu.RUnlock()
	if ok {
		return key, nil
	}
	if time.Since(fetched) < time.Minute {
		return nil, fmt.Errorf("unknown signing key %q", kid)
	}
	if err := a.refreshJWKS(); err != nil {
		return nil, err
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	if key, ok := a.keys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("signing key %q not present in realm JWKS", kid)
}

// refreshJWKS pulls the realm's public keys.
func (a *Authenticator) refreshJWKS() error {
	if a.jwksURL == "" {
		return fmt.Errorf("no JWKS URL configured; cannot verify Keycloak tokens")
	}

	resp, err := a.http.Get(a.jwksURL)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
	}

	a.mu.Lock()
	a.keys = keys
	a.fetchedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("refreshed Keycloak JWKS", zap.Int("keys", len(keys)))
	return nil
}

// claimsKey is the request-context key holding the caller's claims.
type claimsKey struct{}

// bearerToken pulls the raw token out of the Authorization header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
