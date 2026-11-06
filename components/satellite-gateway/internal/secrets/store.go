package secrets

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// Manager handles storing/retrieving cert artifacts as a single secret (base64 JSON).
type Manager struct {
	store  core.SecretsStore
	prefix string
	logger *zap.Logger
}

// NewManager creates a new secrets manager.
func NewManager(store core.SecretsStore, prefix string, logger *zap.Logger) *Manager {
	return &Manager{store: store, prefix: prefix, logger: logger}
}

// StoreCertArtifacts stores all cert artifacts as a single secret with base64-encoded values.
func (m *Manager) StoreCertArtifacts(ctx context.Context, certID, tenantID, domain, env string, artifacts *CertArtifacts) (*core.SecretRef, error) {
	// Base64 encode all artifacts
	secret := &models.CertSecret{
		PrivateKey:  base64.StdEncoding.EncodeToString(artifacts.PrivateKey),
		CSR:         base64.StdEncoding.EncodeToString(artifacts.CSR),
		Certificate: base64.StdEncoding.EncodeToString(artifacts.Certificate),
		Chain:       base64.StdEncoding.EncodeToString(artifacts.Chain),
		FullChain:   base64.StdEncoding.EncodeToString(artifacts.FullChain),
		PFX:         base64.StdEncoding.EncodeToString(artifacts.PFX),
		Metadata:    artifacts.Metadata,
	}

	req := &core.StoreSecretRequest{
		CertID:      certID,
		TenantID:    tenantID,
		Domain:      domain,
		Environment: env,
		Secret:      secret,
		Tags: map[string]string{
			"clm-cert-id":     certID,
			"clm-domain":      domain,
			"clm-environment": env,
			"clm-tenant":      tenantID,
			"clm-managed-by":  "clm-satellite",
		},
	}

	ref, err := m.store.Store(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("storing secret: %w", err)
	}

	m.logger.Info("cert artifacts stored",
		zap.String("cert_id", certID),
		zap.String("domain", domain),
		zap.String("secret_id", ref.SecretID),
	)

	return ref, nil
}

// RetrieveCertArtifacts fetches and decodes all cert artifacts from the secret store.
func (m *Manager) RetrieveCertArtifacts(ctx context.Context, ref *core.SecretRef) (*CertArtifacts, error) {
	secret, err := m.store.Retrieve(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("retrieving secret: %w", err)
	}

	// Base64 decode
	privateKey, _ := base64.StdEncoding.DecodeString(secret.PrivateKey)
	csr, _ := base64.StdEncoding.DecodeString(secret.CSR)
	cert, _ := base64.StdEncoding.DecodeString(secret.Certificate)
	chain, _ := base64.StdEncoding.DecodeString(secret.Chain)
	fullChain, _ := base64.StdEncoding.DecodeString(secret.FullChain)
	pfx, _ := base64.StdEncoding.DecodeString(secret.PFX)

	return &CertArtifacts{
		PrivateKey:  privateKey,
		CSR:         csr,
		Certificate: cert,
		Chain:       chain,
		FullChain:   fullChain,
		PFX:         pfx,
		Metadata:    secret.Metadata,
	}, nil
}

// RetrieveField fetches a single field (decoded) from the stored secret.
func (m *Manager) RetrieveField(ctx context.Context, ref *core.SecretRef, field string) ([]byte, error) {
	encoded, err := m.store.RetrieveField(ctx, ref, field)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(string(encoded))
}

// CertArtifacts holds all raw (non-encoded) cert artifacts.
type CertArtifacts struct {
	PrivateKey  []byte             // PEM-encoded private key
	CSR         []byte             // PEM-encoded CSR
	Certificate []byte             // PEM-encoded leaf cert
	Chain       []byte             // PEM-encoded intermediates
	FullChain   []byte             // PEM-encoded cert + chain
	PFX         []byte             // PKCS12 binary
	Metadata    models.CertMetadata
}
