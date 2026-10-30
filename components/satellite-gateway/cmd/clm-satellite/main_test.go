package main

import (
	"strings"
	"testing"

	"github.com/m2p/clm/internal/satellite/ca"
	"go.uber.org/zap"
)

func TestRuntimeACMEUsesGatewayEnvironmentForTLSPolicy(t *testing.T) {
	_, err := newRuntimeACMEProvider(ca.ACMEBuildConfig{
		Environment:        "staging",
		InsecureSkipVerify: true,
		KeyDir:             t.TempDir(),
	}, "production", zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "production ACME TLS verification") {
		t.Fatalf("newRuntimeACMEProvider() error = %v", err)
	}
}
