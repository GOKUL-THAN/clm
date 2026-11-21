package sso

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestSAMLProviderRequiresIdPCertificate(t *testing.T) {
	cfg := &SAMLConfig{
		IDPSSOURL:  "https://idp.example/sso",
		SPEntityID: "https://clm.example",
		SPACSURL:   "https://clm.example/api/v1/auth/saml/callback",
	}
	if _, err := NewSAMLProvider(cfg, zap.NewNop()); err == nil {
		t.Fatal("SAML provider accepted configuration without an IdP certificate")
	}
}

func TestSAMLUnsignedResponseRejected(t *testing.T) {
	p := &SAMLProvider{
		config: &SAMLConfig{
			SPEntityID: "https://clm.example",
			SPACSURL:   "https://clm.example/api/v1/auth/saml/callback",
		},
		logger: zap.NewNop(),
	}
	params := map[string]string{
		"SAMLResponse": "PHNhbWxwOlJlc3BvbnNlIHhtbG5zOnNhbWxwPSJ1cm46b2FzaXM6bmFtZXM6dGM6U0FNTDoyLjA6cHJvdG9jb2wiLz4=",
	}
	if _, err := p.HandleCallback(context.Background(), params); err == nil {
		t.Fatal("unsigned SAML response was accepted")
	}
}
