package api

import (
	"context"
	"strings"
	"testing"

	"github.com/m2p/clm/internal/satellite/ca"
	"github.com/m2p/clm/internal/satellite/dns01"
	"go.uber.org/zap"
	"golang.org/x/crypto/acme"
)

type selectorDNSProvider struct{}

func (selectorDNSProvider) Name() string                                { return "fake" }
func (selectorDNSProvider) Present(context.Context, dns01.Record) error { return nil }
func (selectorDNSProvider) Cleanup(context.Context, dns01.Record) error { return nil }

type selectorResolver struct{}

func (selectorResolver) LookupTXT(context.Context, string) ([]string, error) { return nil, nil }

func TestPickChallenge(t *testing.T) {
	dnsSolver := dns01.NewSolver(selectorDNSProvider{}, selectorResolver{}, dns01.Config{})
	challenges := []*acme.Challenge{
		{Type: "http-01", Token: "http"},
		{Type: "dns-01", Token: "dns"},
	}

	tests := []struct {
		name          string
		mode          string
		wildcard      bool
		identifier    string
		dnsSolver     *dns01.Solver
		wantType      string
		wantChallenge bool
	}{
		{name: "auto ordinary keeps HTTP", mode: "auto", identifier: "api.example.com", dnsSolver: dnsSolver, wantType: "http-01", wantChallenge: true},
		{name: "auto wildcard uses DNS", mode: "auto", wildcard: true, identifier: "example.com", dnsSolver: dnsSolver, wantType: "dns-01", wantChallenge: true},
		{name: "dns mode uses DNS for ordinary SAN", mode: "dns-01", identifier: "api.example.com", dnsSolver: dnsSolver, wantType: "dns-01", wantChallenge: true},
		{name: "explicit HTTP refuses wildcard even with DNS provider", mode: "http-01", wildcard: true, identifier: "example.com", dnsSolver: dnsSolver, wantType: "http-01"},
		{name: "wildcard without provider is rejected", mode: "auto", wildcard: true, identifier: "example.com", wantType: "dns-01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &ACMEManager{cfg: ACMEConfig{ChallengeType: tt.mode}, dns01: tt.dnsSolver}
			challenge, challengeType := manager.pickChallenge(&acme.Authorization{
				Identifier: acme.AuthzID{Type: "dns", Value: tt.identifier},
				Wildcard:   tt.wildcard, Challenges: challenges,
			})
			if challengeType != tt.wantType {
				t.Fatalf("challenge type = %q, want %q", challengeType, tt.wantType)
			}
			if (challenge != nil) != tt.wantChallenge {
				t.Fatalf("challenge = %#v, want present %v", challenge, tt.wantChallenge)
			}
		})
	}
}

func TestNewACMEManagerRejectsProductionInsecureTLS(t *testing.T) {
	_, err := NewACMEManager(ACMEConfig{
		Environment: "production", InsecureSkipVerify: true, KeyDir: t.TempDir(),
	}, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "production ACME TLS verification") {
		t.Fatalf("NewACMEManager() error = %v", err)
	}
}

func TestChallengePolicyMetadata(t *testing.T) {
	dnsSolver := dns01.NewSolver(selectorDNSProvider{}, selectorResolver{}, dns01.Config{})
	tests := []struct {
		name         string
		mode         string
		dnsSolver    *dns01.Solver
		wantHTTP     bool
		wantDNS      bool
		wantWildcard bool
	}{
		{name: "auto with DNS", mode: "auto", dnsSolver: dnsSolver, wantHTTP: true, wantDNS: true, wantWildcard: true},
		{name: "explicit HTTP", mode: "http-01", dnsSolver: dnsSolver, wantHTTP: true},
		{name: "explicit DNS", mode: "dns-01", dnsSolver: dnsSolver, wantDNS: true, wantWildcard: true},
		{name: "auto without DNS", mode: "auto", wantHTTP: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &ACMEManager{cfg: ACMEConfig{ChallengeType: tt.mode}, dns01: tt.dnsSolver}
			capabilities := NewACMEProvider(manager, zap.NewNop()).Capabilities()
			if containsStr(capabilities.DCVMethods, ca.DCVHTTP) != tt.wantHTTP {
				t.Fatalf("HTTP capability = %v, want %v", capabilities.DCVMethods, tt.wantHTTP)
			}
			if containsStr(capabilities.DCVMethods, ca.DCVDNS) != tt.wantDNS {
				t.Fatalf("DNS capability = %v, want %v", capabilities.DCVMethods, tt.wantDNS)
			}
			if capabilities.Wildcard != tt.wantWildcard {
				t.Fatalf("wildcard capability = %v, want %v", capabilities.Wildcard, tt.wantWildcard)
			}
			status := manager.Status()
			if status["http01_enabled"] != tt.wantHTTP || status["dns01_enabled"] != tt.wantDNS {
				t.Fatalf("status challenge metadata = %#v", status)
			}
		})
	}
}
