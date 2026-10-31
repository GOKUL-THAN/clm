package integrations

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap/zaptest"
)

func TestIntegrationController(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewIntegrationController(logger)
	tenantID := uuid.New()
	ctx := context.Background()

	// Mock receiver HTTP server
	receivedEvent := ""
	receivedSig := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedEvent = r.Header.Get("X-CLM-Event")
		receivedSig = r.Header.Get("X-CLM-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// 1. Register webhook subscription
	sub, err := ctrl.RegisterSubscription(ctx, tenantID, &Subscription{
		Name:        "Corporate SIEM & Slack Alerting",
		Type:        TypeWebhook,
		EndpointURL: srv.URL,
		Events:      []string{"incident.raised", "certificate.expired"},
	})
	if err != nil {
		t.Fatalf("failed to register subscription: %v", err)
	}

	if sub.SecretKey == "" {
		t.Fatalf("expected generated secret key for HMAC")
	}

	// 2. Dispatch event
	dispatched := ctrl.DispatchEvent(ctx, tenantID, "incident.raised", map[string]string{
		"domain":   "api.m2p.in",
		"severity": "SEV1",
	})

	if dispatched != 1 {
		t.Fatalf("expected 1 event dispatched, got %d", dispatched)
	}
	if receivedEvent != "incident.raised" {
		t.Fatalf("expected received event 'incident.raised', got: %s", receivedEvent)
	}
	if receivedSig == "" {
		t.Fatalf("expected non-empty HMAC signature")
	}

	// 3. Delete subscription
	err = ctrl.DeleteSubscription(tenantID, sub.ID)
	if err != nil {
		t.Fatalf("delete subscription failed: %v", err)
	}
	if len(ctrl.ListSubscriptions(tenantID)) != 0 {
		t.Fatalf("expected 0 subscriptions after delete")
	}
}
