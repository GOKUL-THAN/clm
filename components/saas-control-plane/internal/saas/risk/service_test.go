package risk

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap/zaptest"
)

func TestRiskController_EvaluateRisk(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewRiskController(logger)
	tenantID := uuid.New()
	ctx := context.Background()

	certs := []CertInput{
		{
			ID:           "c1",
			Domain:       "*.m2pfintech.com",
			KeyAlgorithm: "RSA",
			KeySize:      2048,
			DaysLeft:     4, // Imminent expiry!
			Issuer:       "DigiCert",
			IsWildcard:   true, // Wildcard blast radius!
		},
		{
			ID:           "c2",
			Domain:       "legacy.m2p.in",
			KeyAlgorithm: "RSA",
			KeySize:      1024, // Weak key!
			DaysLeft:     180,
			Issuer:       "DigiCert",
			IsWildcard:   false,
		},
	}

	summary := ctrl.EvaluateRisk(ctx, tenantID, certs)

	if summary.TotalIdentifiedRisks == 0 {
		t.Fatalf("expected identified risks, got 0")
	}
	if summary.CriticalRisks == 0 {
		t.Fatalf("expected critical risks from imminent expiry and weak key")
	}
	if summary.RiskRating == "LOW" {
		t.Fatalf("risk rating should not be LOW for vulnerable certificates")
	}
}
