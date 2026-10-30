package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/config"
)

func TestAuthMiddlewareFailClosedClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "0123456789abcdef0123456789abcdef"
	tenantID := uuid.New()
	userID := uuid.New()
	base := config.JWTConfig{
		Secret:            secret,
		Issuer:            "test-issuer",
		Audience:          "test-api",
		AllowedAlgorithms: []string{"HS256"},
	}

	tests := []struct {
		name   string
		claims jwt.Claims
		method jwt.SigningMethod
		cfg    config.JWTConfig
		want   int
	}{
		{name: "valid tenant token", claims: accessClaims(userID, &tenantID, "viewer", "test-issuer", "test-api"), method: jwt.SigningMethodHS256, cfg: base, want: http.StatusNoContent},
		{name: "missing tenant fails for non platform role", claims: accessClaims(userID, nil, "viewer", "test-issuer", "test-api"), method: jwt.SigningMethodHS256, cfg: base, want: http.StatusUnauthorized},
		{name: "platform role may omit tenant", claims: accessClaims(userID, nil, "super_admin", "test-issuer", "test-api"), method: jwt.SigningMethodHS256, cfg: base, want: http.StatusNoContent},
		{name: "invalid subject", claims: accessClaimsWithSubject(userID, &tenantID, "viewer", "not-a-uuid", "test-issuer", "test-api"), method: jwt.SigningMethodHS256, cfg: base, want: http.StatusUnauthorized},
		{name: "wrong issuer", claims: accessClaims(userID, &tenantID, "viewer", "other", "test-api"), method: jwt.SigningMethodHS256, cfg: base, want: http.StatusUnauthorized},
		{name: "wrong audience", claims: accessClaims(userID, &tenantID, "viewer", "test-issuer", "other"), method: jwt.SigningMethodHS256, cfg: base, want: http.StatusUnauthorized},
		{name: "wrong algorithm", claims: accessClaims(userID, &tenantID, "viewer", "test-issuer", "test-api"), method: jwt.SigningMethodHS384, cfg: base, want: http.StatusUnauthorized},
		{name: "empty secret skips HMAC", claims: accessClaims(userID, &tenantID, "viewer", "test-issuer", "test-api"), method: jwt.SigningMethodHS256, cfg: config.JWTConfig{Issuer: "test-issuer", Audience: "test-api", AllowedAlgorithms: []string{"HS256"}}, want: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := jwt.NewWithClaims(tt.method, tt.claims).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			if got := authenticateStatus(tt.cfg, token); got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAuthMiddlewareRejectsInvalidProvidedTenantForPlatformRole(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	userID := uuid.New()
	claims := jwt.MapClaims{
		"user_id":   userID.String(),
		"sub":       userID.String(),
		"role":      "super_admin",
		"tenant_id": "not-a-uuid",
		"iss":       "issuer",
		"aud":       "audience",
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Hour).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.JWTConfig{Secret: secret, Issuer: "issuer", Audience: "audience", AllowedAlgorithms: []string{"HS256"}}
	if got := authenticateStatus(cfg, token); got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestAuthMiddlewareValidatesAndCachesJWKS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-key"
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": kid,
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	defer server.Close()

	tenantID := uuid.New()
	userID := uuid.New()
	claims := &KeycloakClaims{
		TenantID: tenantID.String(),
		Role:     "viewer",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    "external-issuer",
			Audience:  jwt.ClaimStrings{"clm-api"},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed.Header["kid"] = kid
	token, err := signed.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.JWTConfig{
		ExternalIssuer:    "external-issuer",
		ExternalAudience:  "clm-api",
		JWKSURL:           server.URL,
		AllowedAlgorithms: []string{"RS256"},
		JWKSCacheTTL:      time.Minute,
		JWKSHTTPTimeout:   time.Second,
	}
	authenticator := NewAuthenticator(cfg)
	for i := 0; i < 2; i++ {
		router := gin.New()
		router.Use(authenticator.Middleware())
		router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("request %d status = %d, body = %s", i, response.Code, response.Body.String())
		}
	}
	unknown := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	unknown.Header["kid"] = "attacker-selected-key"
	unknownToken, err := unknown.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(authenticator.Middleware())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+unknownToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unknown kid status = %d, want 401", response.Code)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("JWKS requests = %d, want one cached fetch", got)
	}
}

func TestAuthMiddlewareRejectsJWKSHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not available", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	claims := &KeycloakClaims{
		TenantID: uuid.NewString(),
		Role:     "viewer",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.NewString(),
			Issuer:    "issuer",
			Audience:  jwt.ClaimStrings{"audience"},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed.Header["kid"] = "missing"
	token, err := signed.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.JWTConfig{ExternalIssuer: "issuer", ExternalAudience: "audience", JWKSURL: server.URL, AllowedAlgorithms: []string{"RS256"}, JWKSHTTPTimeout: time.Second}
	if got := authenticateStatus(cfg, token); got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func authenticateStatus(cfg config.JWTConfig, token string) int {
	router := gin.New()
	router.Use(AuthMiddleware(cfg))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response.Code
}

func accessClaims(userID uuid.UUID, tenantID *uuid.UUID, role, issuer, audience string) *Claims {
	return accessClaimsWithSubject(userID, tenantID, role, userID.String(), issuer, audience)
}

func accessClaimsWithSubject(userID uuid.UUID, tenantID *uuid.UUID, role, subject, issuer, audience string) *Claims {
	return &Claims{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
}

func TestRequireTenantAccessRejectsCrossTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	caller, target := uuid.New(), uuid.New()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("role", "tenant_admin")
		c.Set("tenant_id", caller)
		c.Next()
	})
	router.GET("/", func(c *gin.Context) {
		if RequireTenantAccess(c, target) {
			c.Status(http.StatusNoContent)
		}
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestAuthMiddlewareJWKSClaimsFailClosed(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "claims-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": kid, "kty": "RSA", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	defer server.Close()
	cfg := config.JWTConfig{ExternalIssuer: "issuer", ExternalAudience: "audience", JWKSURL: server.URL, AllowedAlgorithms: []string{"RS256"}, JWKSHTTPTimeout: time.Second}
	validSubject := uuid.NewString()
	validTenant := uuid.NewString()
	tests := []struct {
		name     string
		subject  string
		tenantID string
		role     string
		issuer   string
		audience string
		want     int
	}{
		{name: "valid", subject: validSubject, tenantID: validTenant, role: "viewer", issuer: "issuer", audience: "audience", want: http.StatusNoContent},
		{name: "missing tenant", subject: validSubject, role: "viewer", issuer: "issuer", audience: "audience", want: http.StatusUnauthorized},
		{name: "invalid subject", subject: "not-a-uuid", tenantID: validTenant, role: "viewer", issuer: "issuer", audience: "audience", want: http.StatusUnauthorized},
		{name: "invalid supplied platform tenant", subject: validSubject, tenantID: "not-a-uuid", role: "admin", issuer: "issuer", audience: "audience", want: http.StatusUnauthorized},
		{name: "wrong issuer", subject: validSubject, tenantID: validTenant, role: "viewer", issuer: "other", audience: "audience", want: http.StatusUnauthorized},
		{name: "wrong audience", subject: validSubject, tenantID: validTenant, role: "viewer", issuer: "issuer", audience: "other", want: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &KeycloakClaims{TenantID: tt.tenantID, Role: tt.role, RegisteredClaims: jwt.RegisteredClaims{
				Subject: tt.subject, Issuer: tt.issuer, Audience: jwt.ClaimStrings{tt.audience},
				IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			}}
			signed := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			signed.Header["kid"] = kid
			token, err := signed.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			if got := authenticateStatus(cfg, token); got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAuthMiddlewareSeparatesInternalAndExternalTrustPolicies(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "separate-policies-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": kid, "kty": "RSA", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	defer server.Close()

	const (
		internalIssuer   = "clm-server-test"
		internalAudience = "clm-api-test"
		externalIssuer   = "https://id.example.test/realms/clm"
		externalAudience = "clm-api"
		secret           = "0123456789abcdef0123456789abcdef"
	)
	cfg := config.JWTConfig{
		Secret: secret, Issuer: internalIssuer, Audience: internalAudience,
		ExternalIssuer: externalIssuer, ExternalAudience: externalAudience,
		JWKSURL: server.URL, AllowedAlgorithms: []string{"HS256", "RS256"},
		JWKSHTTPTimeout: time.Second,
	}
	userID, tenantID := uuid.New(), uuid.New()

	signInternal := func(issuer, audience string) string {
		t.Helper()
		token, signErr := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims(userID, &tenantID, "viewer", issuer, audience)).SignedString([]byte(secret))
		if signErr != nil {
			t.Fatal(signErr)
		}
		return token
	}
	signExternal := func(issuer, audience string) string {
		t.Helper()
		claims := &KeycloakClaims{TenantID: tenantID.String(), Role: "viewer", RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID.String(), Issuer: issuer, Audience: jwt.ClaimStrings{audience},
			IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		}}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = kid
		signed, signErr := token.SignedString(key)
		if signErr != nil {
			t.Fatal(signErr)
		}
		return signed
	}

	tests := []struct {
		name  string
		token string
		want  int
	}{
		{name: "valid internal policy token", token: signInternal(internalIssuer, internalAudience), want: http.StatusNoContent},
		{name: "valid external policy token", token: signExternal(externalIssuer, externalAudience), want: http.StatusNoContent},
		{name: "internal token with external claims rejected", token: signInternal(externalIssuer, externalAudience), want: http.StatusUnauthorized},
		{name: "external token with internal claims rejected", token: signExternal(internalIssuer, internalAudience), want: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authenticateStatus(cfg, tt.token); got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
		})
	}
}
