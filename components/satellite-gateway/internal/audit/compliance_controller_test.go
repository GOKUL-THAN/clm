package audit

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestComplianceController(t *testing.T) {
	logger := zap.NewNop()
	controller := NewComplianceController(logger)

	records := []CertAuditRecord{
		{
			ID:           "cert-good-01",
			Domain:       "api.m2pfintech.com",
			KeyAlgorithm: "RSA",
			KeySize:      2048,
			NotBefore:    time.Now().Add(-30 * 24 * time.Hour),
			NotAfter:     time.Now().Add(60 * 24 * time.Hour),
			Issuer:       "DigiCert",
			IsSelfSigned: false,
			Environment:  "prod",
		},
		{
			ID:           "cert-weak-02",
			Domain:       "legacy.m2p.in",
			KeyAlgorithm: "RSA",
			KeySize:      1024, // Weak RSA
			NotBefore:    time.Now().Add(-100 * 24 * time.Hour),
			NotAfter:     time.Now().Add(10 * 24 * time.Hour), // Expiring soon
			Issuer:       "Internal Test",
			IsSelfSigned: true,
			Environment:  "prod", // Self-signed in prod
		},
	}

	summary := controller.Evaluate(records)
	if summary == nil {
		t.Fatal("expected summary to not be nil")
	}

	if summary.TotalCertificates != 2 {
		t.Errorf("expected 2 total certs, got %d", summary.TotalCertificates)
	}

	if summary.CompliantCerts != 1 {
		t.Errorf("expected 1 compliant cert, got %d", summary.CompliantCerts)
	}

	if summary.CriticalFindings == 0 {
		t.Errorf("expected critical findings for weak RSA and self-signed in prod")
	}

	csvData, err := controller.GenerateCSV(summary)
	if err != nil {
		t.Fatalf("failed to generate CSV: %v", err)
	}
	if len(csvData) == 0 {
		t.Errorf("expected non-empty CSV data")
	}
}
