package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/m2p/clm/internal/satellite/ca"
	"go.uber.org/zap"
)

func TestRuntimeACMEUsesGatewayEnvironmentForTLSPolicy(t *testing.T) {
	_, err := newRuntimeACMEProvider(ca.ACMEBuildConfig{
		Environment:        "staging",
		InsecureSkipVerify: true,
		KeyDir:             t.TempDir(),
	}, "production", nil, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "production ACME TLS verification") {
		t.Fatalf("newRuntimeACMEProvider() error = %v", err)
	}
}

func TestNewHTTPServerHasTimeouts(t *testing.T) {
	server := newHTTPServer(":8080", http.NewServeMux())

	if server.ReadHeaderTimeout <= 0 ||
		server.ReadTimeout <= 0 ||
		server.WriteTimeout <= 0 ||
		server.IdleTimeout <= 0 {
		t.Fatalf("HTTP server timeouts must all be positive: %+v", server)
	}
	if server.ReadHeaderTimeout != 5*time.Second ||
		server.ReadTimeout != 30*time.Second ||
		server.WriteTimeout != 2*time.Minute ||
		server.IdleTimeout != time.Minute {
		t.Fatalf("HTTP server timeouts changed unexpectedly: %+v", server)
	}
}
