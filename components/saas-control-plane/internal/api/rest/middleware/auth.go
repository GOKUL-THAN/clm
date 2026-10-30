package middleware

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/config"
)

const maxJWKSBody = 1 << 20

// Claims holds JWT token claims for internally-issued tokens. TenantID is a
// pointer so a missing claim is distinguishable from an explicitly invalid one.
type Claims struct {
	UserID   uuid.UUID  `json:"user_id"`
	TenantID *uuid.UUID `json:"tenant_id,omitempty"`
	Email    string     `json:"email"`
	Role     string     `json:"role"`
	jwt.RegisteredClaims
}

// KeycloakClaims holds externally-issued JWT token claims.
type KeycloakClaims struct {
	Email         string       `json:"email"`
	Name          string       `json:"name"`
	PreferredUser string       `json:"preferred_username"`
	RealmAccess   *RealmAccess `json:"realm_access"`
	Roles         []string     `json:"roles"`
	Role          string       `json:"role"`
	TenantID      string       `json:"tenant_id"`
	jwt.RegisteredClaims
}

type RealmAccess struct {
	Roles []string `json:"roles"`
}

type cachedKey struct {
	key       *rsa.PublicKey
	expiresAt time.Time
}

// Authenticator validates configured internal and external JWTs. Its parsed
// JWKS keys are shared safely by concurrent requests and refreshed as a set.
type Authenticator struct {
	cfg    config.JWTConfig
	client *http.Client
	now    func() time.Time

	mu         sync.RWMutex
	refreshMu  sync.Mutex
	keys       map[string]cachedKey
	cacheUntil time.Time
}

func NewAuthenticator(cfg config.JWTConfig) *Authenticator {
	timeout := cfg.JWKSHTTPTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Authenticator{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
		now:    time.Now,
		keys:   make(map[string]cachedKey),
	}
}

// AuthMiddleware validates JWT tokens using the complete configured trust
// policy. HMAC validation is not attempted when the secret is empty.
func AuthMiddleware(cfg config.JWTConfig) gin.HandlerFunc {
	return NewAuthenticator(cfg).Middleware()
}

func (a *Authenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") || strings.TrimSpace(parts[1]) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			return
		}

		identity, err := a.validate(strings.TrimSpace(parts[1]))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		c.Set("user_id", identity.userID)
		if identity.tenantID != nil {
			c.Set("tenant_id", *identity.tenantID)
		}
		c.Set("email", identity.email)
		c.Set("role", identity.role)
		c.Next()
	}
}

type authenticatedIdentity struct {
	userID   uuid.UUID
	tenantID *uuid.UUID
	email    string
	role     string
}

func (a *Authenticator) validate(tokenString string) (*authenticatedIdentity, error) {
	alg, err := tokenAlgorithm(tokenString)
	if err != nil || !contains(a.cfg.AllowedAlgorithms, alg) {
		return nil, errors.New("algorithm is not allowed")
	}
	if strings.HasPrefix(alg, "HS") {
		if a.cfg.Secret == "" {
			return nil, errors.New("HMAC authentication is disabled")
		}
		return a.validateInternal(tokenString, alg)
	}
	if a.cfg.JWKSURL == "" {
		return nil, errors.New("JWKS authentication is disabled")
	}
	return a.validateExternal(tokenString, alg)
}

func tokenAlgorithm(tokenString string) (string, error) {
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", err
	}
	alg, ok := token.Header["alg"].(string)
	if !ok || alg == "" || alg == "none" {
		return "", errors.New("missing algorithm")
	}
	return alg, nil
}

func parserOptions(alg, issuer, audience string) []jwt.ParserOption {
	return []jwt.ParserOption{
		jwt.WithValidMethods([]string{alg}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	}
}

func (a *Authenticator) validateInternal(tokenString, alg string) (*authenticatedIdentity, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != alg || !strings.HasPrefix(token.Method.Alg(), "HS") {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(a.cfg.Secret), nil
	}, parserOptions(alg, a.cfg.Issuer, a.cfg.Audience)...)
	if err != nil || !token.Valid {
		return nil, err
	}
	userID, err := validSubject(claims.Subject)
	if err != nil || claims.UserID == uuid.Nil || claims.UserID != userID {
		return nil, errors.New("invalid subject")
	}
	return validateIdentity(userID, claims.TenantID, claims.Email, claims.Role)
}

func (a *Authenticator) validateExternal(tokenString, alg string) (*authenticatedIdentity, error) {
	if strings.TrimSpace(a.cfg.ExternalIssuer) == "" || strings.TrimSpace(a.cfg.ExternalAudience) == "" {
		return nil, errors.New("external issuer and audience are required")
	}
	claims := &KeycloakClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != alg || !strings.HasPrefix(token.Method.Alg(), "RS") {
			return nil, jwt.ErrSignatureInvalid
		}
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("missing key id")
		}
		return a.publicKey(kid)
	}, parserOptions(alg, a.cfg.ExternalIssuer, a.cfg.ExternalAudience)...)
	if err != nil || !token.Valid {
		return nil, err
	}
	userID, err := validSubject(claims.Subject)
	if err != nil {
		return nil, err
	}
	role := claims.Role
	if claims.RealmAccess != nil && len(claims.RealmAccess.Roles) > 0 {
		role = extractHighestRole(claims.RealmAccess.Roles)
	} else if len(claims.Roles) > 0 {
		role = extractHighestRole(claims.Roles)
	}
	var tenantID *uuid.UUID
	if claims.TenantID != "" {
		parsed, err := uuid.Parse(claims.TenantID)
		if err != nil || parsed == uuid.Nil {
			return nil, errors.New("invalid tenant_id")
		}
		tenantID = &parsed
	}
	return validateIdentity(userID, tenantID, claims.Email, role)
}

func validSubject(subject string) (uuid.UUID, error) {
	id, err := uuid.Parse(subject)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, errors.New("invalid subject")
	}
	return id, nil
}

func validateIdentity(userID uuid.UUID, tenantID *uuid.UUID, email, role string) (*authenticatedIdentity, error) {
	if userID == uuid.Nil || strings.TrimSpace(role) == "" {
		return nil, errors.New("incomplete identity")
	}
	if tenantID != nil && *tenantID == uuid.Nil {
		return nil, errors.New("invalid tenant_id")
	}
	if !IsPlatformRole(role) && tenantID == nil {
		return nil, errors.New("tenant_id is required")
	}
	return &authenticatedIdentity{userID: userID, tenantID: tenantID, email: email, role: role}, nil
}

func (a *Authenticator) publicKey(kid string) (*rsa.PublicKey, error) {
	now := a.now()
	a.mu.RLock()
	entry, ok := a.keys[kid]
	cacheFresh := now.Before(a.cacheUntil)
	a.mu.RUnlock()
	if cacheFresh {
		if ok && now.Before(entry.expiresAt) {
			return entry.key, nil
		}
		// Negative-cache unknown key IDs for the current document lifetime so
		// attacker-selected kids cannot force an outbound JWKS fetch per request.
		return nil, jwt.ErrSignatureInvalid
	}

	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()

	now = a.now()
	a.mu.RLock()
	entry, ok = a.keys[kid]
	cacheFresh = now.Before(a.cacheUntil)
	a.mu.RUnlock()
	if cacheFresh {
		if ok && now.Before(entry.expiresAt) {
			return entry.key, nil
		}
		return nil, jwt.ErrSignatureInvalid
	}
	if err := a.refreshKeys(now); err != nil {
		// A still-valid cached key may be used during a transient refresh failure.
		a.mu.RLock()
		entry, ok = a.keys[kid]
		a.mu.RUnlock()
		if ok && now.Before(entry.expiresAt) {
			return entry.key, nil
		}
		return nil, err
	}
	a.mu.RLock()
	entry, ok = a.keys[kid]
	a.mu.RUnlock()
	if !ok {
		return nil, jwt.ErrSignatureInvalid
	}
	return entry.key, nil
}

func (a *Authenticator) refreshKeys(now time.Time) error {
	req, err := http.NewRequest(http.MethodGet, a.cfg.JWKSURL, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch JWKS: unexpected HTTP status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBody+1))
	if err != nil {
		return fmt.Errorf("read JWKS: %w", err)
	}
	if len(body) > maxJWKSBody {
		return errors.New("JWKS response exceeds size limit")
	}
	var document struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Use string `json:"use"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}
	ttl := a.cfg.JWKSCacheTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	parsed := make(map[string]cachedKey)
	for _, raw := range document.Keys {
		if raw.Kid == "" || raw.Kty != "RSA" || (raw.Use != "" && raw.Use != "sig") || (raw.Alg != "" && !contains(a.cfg.AllowedAlgorithms, raw.Alg)) {
			continue
		}
		nBytes, nErr := base64.RawURLEncoding.DecodeString(raw.N)
		eBytes, eErr := base64.RawURLEncoding.DecodeString(raw.E)
		if nErr != nil || eErr != nil || len(nBytes) == 0 || len(eBytes) == 0 {
			continue
		}
		exponent := new(big.Int).SetBytes(eBytes)
		if !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > int64(^uint(0)>>1) {
			continue
		}
		parsed[raw.Kid] = cachedKey{
			key:       &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(exponent.Int64())},
			expiresAt: now.Add(ttl),
		}
	}
	if len(parsed) == 0 {
		return errors.New("JWKS contains no usable signing keys")
	}
	a.mu.Lock()
	a.keys = parsed
	a.cacheUntil = now.Add(ttl)
	a.mu.Unlock()
	return nil
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func extractHighestRole(roles []string) string {
	priority := []string{"super_admin", "admin", "tenant_admin", "cert_manager", "approver", "operator", "auditor", "viewer"}
	for _, preferred := range priority {
		for _, role := range roles {
			if role == preferred {
				return role
			}
		}
	}
	return "viewer"
}

func IsPlatformRole(role string) bool {
	return role == "super_admin" || role == "admin"
}

func IsPlatformAdmin(c *gin.Context) bool {
	role, _ := c.Get("role")
	value, _ := role.(string)
	return IsPlatformRole(value)
}

// RequireTenantAccess allows platform admins to operate across tenants while
// requiring every other role to match a concrete token tenant.
func RequireTenantAccess(c *gin.Context, target uuid.UUID) bool {
	if target == uuid.Nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return false
	}
	if IsPlatformAdmin(c) {
		c.Set("audit_tenant_id", target)
		return true
	}
	caller := GetTenantID(c)
	if caller == uuid.Nil || caller != target {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "tenant access denied"})
		return false
	}
	c.Set("audit_tenant_id", target)
	return true
}

func RoleRequired(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		role, ok := userRole.(string)
		if !exists || !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid role context"})
			return
		}
		for _, allowed := range roles {
			if allowed == role {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
	}
}

func GetUserID(c *gin.Context) uuid.UUID {
	value, _ := c.Get("user_id")
	id, _ := value.(uuid.UUID)
	return id
}

func GetTenantID(c *gin.Context) uuid.UUID {
	value, _ := c.Get("tenant_id")
	id, _ := value.(uuid.UUID)
	return id
}
