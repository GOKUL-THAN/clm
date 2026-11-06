package api

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

func TestAuthenticatorValidatesHS256RegisteredClaims(t *testing.T) {
	const issuer = "clm-server"
	const audience = "clm-api"
	secret := []byte(strings.Repeat("x", 32))
	auth := NewAuthenticator(string(secret), "", TokenValidationPolicy{
		InternalIssuer: issuer, InternalAudience: audience,
	}, zap.NewNop())

	tests := []struct {
		name       string
		issuer     string
		audiences  jwt.ClaimStrings
		expiration *jwt.NumericDate
		wantErr    bool
	}{
		{
			name: "valid claims", issuer: issuer, audiences: jwt.ClaimStrings{audience},
			expiration: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		{
			name: "missing issuer", audiences: jwt.ClaimStrings{audience},
			expiration: jwt.NewNumericDate(time.Now().Add(time.Hour)), wantErr: true,
		},
		{
			name: "wrong issuer", issuer: "other-issuer", audiences: jwt.ClaimStrings{audience},
			expiration: jwt.NewNumericDate(time.Now().Add(time.Hour)), wantErr: true,
		},
		{
			name: "missing audience", issuer: issuer,
			expiration: jwt.NewNumericDate(time.Now().Add(time.Hour)), wantErr: true,
		},
		{
			name: "wrong audience", issuer: issuer, audiences: jwt.ClaimStrings{"other-api"},
			expiration: jwt.NewNumericDate(time.Now().Add(time.Hour)), wantErr: true,
		},
		{name: "missing expiration", issuer: issuer, audiences: jwt.ClaimStrings{audience}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := &Claims{RegisteredClaims: jwt.RegisteredClaims{
				Issuer: test.issuer, Audience: test.audiences,
				ExpiresAt: test.expiration, IssuedAt: jwt.NewNumericDate(time.Now()),
			}}
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
			if err != nil {
				t.Fatal(err)
			}
			_, err = auth.Parse(token)
			if (err != nil) != test.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestAuthenticatorValidatesRS256RegisteredClaims(t *testing.T) {
	const issuer = "https://id.example.test/auth/realms/clm"
	const audience = "clm-api"

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		document := struct {
			Keys []map[string]string `json:"keys"`
		}{Keys: []map[string]string{{
			"kid": "test-key", "kty": "RSA", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}}
		if err := json.NewEncoder(w).Encode(document); err != nil {
			t.Errorf("encode JWKS: %v", err)
		}
	}))
	defer jwks.Close()

	auth := NewAuthenticator("", jwks.URL, TokenValidationPolicy{
		ExternalIssuer: issuer, ExternalAudience: audience,
	}, zap.NewNop())
	tests := []struct {
		name       string
		issuer     string
		audiences  jwt.ClaimStrings
		expiration *jwt.NumericDate
		wantErr    bool
	}{
		{
			name: "valid claims", issuer: issuer, audiences: jwt.ClaimStrings{audience},
			expiration: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		{
			name: "wrong issuer", issuer: "https://other.example.test/realms/clm",
			audiences:  jwt.ClaimStrings{audience},
			expiration: jwt.NewNumericDate(time.Now().Add(time.Hour)), wantErr: true,
		},
		{
			name: "wrong audience", issuer: issuer, audiences: jwt.ClaimStrings{"other-api"},
			expiration: jwt.NewNumericDate(time.Now().Add(time.Hour)), wantErr: true,
		},
		{name: "missing expiration", issuer: issuer, audiences: jwt.ClaimStrings{audience}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := &Claims{RegisteredClaims: jwt.RegisteredClaims{
				Issuer: test.issuer, Audience: test.audiences,
				ExpiresAt: test.expiration, IssuedAt: jwt.NewNumericDate(time.Now()),
			}}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = "test-key"
			signed, err := token.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			_, err = auth.Parse(signed)
			if (err != nil) != test.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
