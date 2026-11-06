package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/m2p/clm/internal/satellite/vsatellite"
	"go.uber.org/zap"
)

func TestDispatchVSatelliteCommandRejectsCertificateDeployment(t *testing.T) {
	agents := vsatellite.NewCoordinator(zap.NewNop())
	if err := agents.RegisterAgent(&vsatellite.Agent{ID: "agent-1"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	server := &Server{agents: agents}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/vsatellites/agent-1/command",
		strings.NewReader(`{"type":"deploy_cert","payload":{"cert_pem":"certificate","key_pem":"private-key"}}`))
	request.SetPathValue("id", "agent-1")
	request = request.WithContext(context.WithValue(request.Context(), claimsKey{}, &Claims{Role: "operator"}))
	response := httptest.NewRecorder()

	server.handleDispatchVSatelliteCommand(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusBadRequest, response.Body)
	}
	if queued := agents.PendingCommands("agent-1"); queued != 0 {
		t.Fatalf("queued commands = %d, want 0", queued)
	}
}

func TestDispatchVSatelliteCommandQueuesHealthCheck(t *testing.T) {
	agents := vsatellite.NewCoordinator(zap.NewNop())
	if err := agents.RegisterAgent(&vsatellite.Agent{ID: "agent-1"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	server := &Server{agents: agents, logger: zap.NewNop()}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/vsatellites/agent-1/command",
		strings.NewReader(`{"type":"health_check","payload":{}}`))
	request.SetPathValue("id", "agent-1")
	request = request.WithContext(context.WithValue(request.Context(), claimsKey{}, &Claims{Role: "operator"}))
	response := httptest.NewRecorder()

	server.handleDispatchVSatelliteCommand(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusAccepted, response.Body)
	}
	if queued := agents.PendingCommands("agent-1"); queued != 1 {
		t.Fatalf("queued commands = %d, want 1", queued)
	}
}

func TestDispatchVSatelliteCommandRejectsUnimplementedTypes(t *testing.T) {
	for _, commandType := range []string{"verify_cert", "collect_inventory"} {
		t.Run(commandType, func(t *testing.T) {
			agents := vsatellite.NewCoordinator(zap.NewNop())
			if err := agents.RegisterAgent(&vsatellite.Agent{ID: "agent-1"}); err != nil {
				t.Fatalf("register agent: %v", err)
			}
			server := &Server{agents: agents}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/vsatellites/agent-1/command",
				strings.NewReader(`{"type":"`+commandType+`","payload":{}}`))
			request.SetPathValue("id", "agent-1")
			request = request.WithContext(context.WithValue(request.Context(), claimsKey{}, &Claims{Role: "operator"}))
			response := httptest.NewRecorder()

			server.handleDispatchVSatelliteCommand(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusBadRequest, response.Body)
			}
			if queued := agents.PendingCommands("agent-1"); queued != 0 {
				t.Fatalf("queued commands = %d, want 0", queued)
			}
		})
	}
}

func TestValidatedGatewayURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "https URL", input: "https://gateway.example/", want: "https://gateway.example"},
		{name: "quoted path", input: "https://gateway.example/a'b", want: "https://gateway.example/a'b"},
		{name: "relative URL", input: "/gateway", wantErr: true},
		{name: "unsupported scheme", input: "javascript:alert(1)", wantErr: true},
		{name: "credentials", input: "https://user:pass@gateway.example", wantErr: true},
		{name: "query", input: "https://gateway.example?next=/other", wantErr: true},
		{name: "fragment", input: "https://gateway.example/#other", wantErr: true},
		{name: "control character", input: "https://gateway\n.example", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := validatedGatewayURL(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("validatedGatewayURL(%q) error = %v, wantErr %v", test.input, err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("validatedGatewayURL(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestEnrolVSatelliteRejectsInvalidGatewayURLBeforeIssuingToken(t *testing.T) {
	agents := vsatellite.NewCoordinator(zap.NewNop())
	server := &Server{agents: agents}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/vsatellites/enroll",
		strings.NewReader(`{"name":"agent-1","gateway_url":"https://gateway.example?next=/other"}`))
	request = request.WithContext(context.WithValue(request.Context(), claimsKey{}, &Claims{Role: "admin"}))
	response := httptest.NewRecorder()

	server.handleEnrolVSatellite(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusBadRequest, response.Body)
	}
	if enrolments := agents.ListEnrolments(); len(enrolments) != 0 {
		t.Fatalf("enrolments = %d, want 0", len(enrolments))
	}
}

func TestShellSingleQuote(t *testing.T) {
	input := "https://gateway.example/a'b; touch /tmp/not-run"
	want := "'https://gateway.example/a'\"'\"'b; touch /tmp/not-run'"
	if got := shellSingleQuote(input); got != want {
		t.Fatalf("shellSingleQuote(%q) = %q, want %q", input, got, want)
	}
}

func TestVSatelliteInstallScriptQuotesShellValues(t *testing.T) {
	gatewayURL := "https://gateway.example/$(touch /tmp/not-run)"
	token := "token'$(id)"
	script := vsatelliteInstallScript(gatewayURL, token, "gateway\nunexpected line")

	for _, expected := range []string{
		"GATEWAY_URL='https://gateway.example/$(touch /tmp/not-run)'",
		"ENROL_TOKEN='token'\"'\"'$(id)'",
		"# Generated by gateway: gateway unexpected line",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("installer does not contain safely rendered value %q", expected)
		}
	}
}
