package migration

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func generateTestCertificate(t *testing.T, domain string) (string, string) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   domain,
			Organization: []string{"M2P Fintech"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(180 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{domain, "www." + domain},
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}))

	return certPEM, keyPEM
}

func TestBulkMigrationController_PEMBundle(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewBulkMigrationController(logger)

	certPEM, keyPEM := generateTestCertificate(t, "api-payment.m2pfintech.com")
	bundlePEM := certPEM + "\n" + keyPEM

	batch, err := ctrl.IngestPayload(IngestionInput{
		Source:       "corporate_vault",
		Environment:  "prod",
		RawPayload:   bundlePEM,
		DryRun:       false,
		SyncToClouds: true,
	})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	if batch.TotalDiscovered != 1 {
		t.Fatalf("expected 1 discovered cert, got %d", batch.TotalDiscovered)
	}
	if batch.ValidCount != 1 {
		t.Fatalf("expected 1 valid cert, got %d", batch.ValidCount)
	}
	if !batch.CloudSyncQueued {
		t.Fatalf("expected CloudSyncQueued true")
	}

	cert := batch.Certificates[0]
	if cert.Domain != "api-payment.m2pfintech.com" {
		t.Fatalf("unexpected domain: %s", cert.Domain)
	}
	if !cert.KeyMatchPassed {
		t.Fatalf("expected key match passed")
	}
	if cert.KeySize != 2048 {
		t.Fatalf("expected key size 2048, got %d", cert.KeySize)
	}

	history := ctrl.ListHistory()
	if len(history) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(history))
	}
}

func TestBulkMigrationController_JSONSecret(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewBulkMigrationController(logger)

	certPEM, keyPEM := generateTestCertificate(t, "uat-auth.m2p.in")
	secretObj := map[string]string{
		"certificate": certPEM,
		"private_key": keyPEM,
	}
	jsonBytes, _ := json.Marshal(secretObj)

	batch, err := ctrl.IngestPayload(IngestionInput{
		Source:      "aws_secrets_manager",
		Environment: "uat",
		RawPayload:  string(jsonBytes),
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("ingest JSON secret failed: %v", err)
	}

	if batch.ValidCount != 1 {
		t.Fatalf("expected 1 valid cert, got %d", batch.ValidCount)
	}
	if !batch.DryRun {
		t.Fatalf("expected dry run true")
	}
}

func TestBulkMigrationController_KeyMismatch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewBulkMigrationController(logger)

	certPEM, _ := generateTestCertificate(t, "domain1.m2pfintech.com")
	_, otherKeyPEM := generateTestCertificate(t, "domain2.m2pfintech.com")

	mismatchedBundle := certPEM + "\n" + otherKeyPEM

	batch, err := ctrl.IngestPayload(IngestionInput{
		RawPayload: mismatchedBundle,
	})
	if err != nil {
		t.Fatalf("expected ingest not to crash on mismatch, but got err: %v", err)
	}

	cert := batch.Certificates[0]
	if cert.KeyMatchPassed {
		t.Fatalf("expected KeyMatchPassed to be false on mismatched key")
	}
	hasWarning := false
	for _, w := range cert.Warnings {
		if strings.Contains(w, "Key pair mismatch") {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Fatalf("expected key mismatch warning, got: %v", cert.Warnings)
	}
}

