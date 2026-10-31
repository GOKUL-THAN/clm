package sync

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestMultiCloudSyncController(t *testing.T) {
	logger := zap.NewNop()
	var cfg SyncTargetConfig
	cfg.AWS.Enabled = true
	cfg.AWS.Regions = []string{"ap-south-1", "us-east-1"}
	cfg.GCP.Enabled = true
	cfg.GCP.ProjectID = "m2p-prod-cert-mgmt"
	cfg.GCP.Locations = []string{"global"}

	controller := NewMultiCloudSyncController(cfg, logger)

	certs := []CertificateDescriptor{
		{
			ID:       "cert-test-01",
			Domain:   "api.m2pfintech.com",
			SANs:     []string{"api.m2pfintech.com", "gateway.m2pfintech.com"},
			CertPEM:  "test-pem",
			NotAfter: time.Now().Add(60 * 24 * time.Hour),
		},
		{
			ID:       "cert-test-02",
			Domain:   "internal.m2p.in",
			SANs:     []string{"internal.m2p.in"},
			CertPEM:  "test-pem-2",
			NotAfter: time.Now().Add(10 * 24 * time.Hour), // Expiring soon -> drift
		},
	}

	result, err := controller.SyncNow(context.Background(), certs, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalCerts != 2 {
		t.Errorf("expected 2 certs, got %d", result.TotalCerts)
	}
	if result.SyncedCerts != 1 {
		t.Errorf("expected 1 synced cert, got %d", result.SyncedCerts)
	}
	if result.DriftedCerts != 1 {
		t.Errorf("expected 1 drifted cert, got %d", result.DriftedCerts)
	}

	history := controller.GetHistory()
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}
}
