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

func TestRiskController_LogRiskIsTenantScoped(t *testing.T) {
	ctrl := NewRiskController(zaptest.NewLogger(t))
	tenantID := uuid.New()
	item, err := ctrl.LogRisk(tenantID, &CryptographicRiskItem{
		Category:    RiskWeakCipherOrKey,
		Severity:    "HIGH",
		Score:       75,
		Affected:    "legacy.example.com",
		Description: "Certificate uses a weak key.",
		Remediation: "Rotate the certificate.",
	})
	if err != nil {
		t.Fatalf("LogRisk() error = %v", err)
	}
	if item.ID == "" {
		t.Fatal("LogRisk() returned an empty ID")
	}

	got := ctrl.ListRisks(tenantID)
	if len(got) != 1 || got[0].ID != item.ID {
		t.Fatalf("ListRisks() = %v, want the recorded risk", got)
	}
	if otherTenant := ctrl.ListRisks(uuid.New()); len(otherTenant) != 0 {
		t.Fatalf("ListRisks() exposed another tenant's risks: %v", otherTenant)
	}

	item.Description = "caller mutation"
	if got[0].Description == item.Description {
		t.Fatal("ListRisks() returned an alias to stored risk data")
	}
}

func TestRiskController_LogRiskRejectsInvalidInput(t *testing.T) {
	ctrl := NewRiskController(zaptest.NewLogger(t))
	_, err := ctrl.LogRisk(uuid.New(), &CryptographicRiskItem{
		Category: RiskWeakCipherOrKey,
		Severity: "UNKNOWN",
		Score:    101,
		Affected: "legacy.example.com",
	})
	if err == nil {
		t.Fatal("LogRisk() accepted an invalid risk item")
	}
}
