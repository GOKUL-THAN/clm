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
