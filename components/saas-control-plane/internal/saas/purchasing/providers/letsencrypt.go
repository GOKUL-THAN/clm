package providers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"

	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// LetsEncryptProvider implements the CAProviderPlugin for Let's Encrypt (ACME).
type LetsEncryptProvider struct {
	directoryURL string
	accountEmail string
	accountKey   *ecdsa.PrivateKey
	logger       *zap.Logger
}

// NewLetsEncryptProvider creates a new Let's Encrypt ACME provider.
func NewLetsEncryptProvider(email string, logger *zap.Logger) *LetsEncryptProvider {
	// Generate account key (in production, load from vault)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	return &LetsEncryptProvider{
		directoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		accountEmail: email,
		accountKey:   key,
		logger:       logger,
	}
}

func (p *LetsEncryptProvider) Name() string {
	return "letsencrypt"
}

func (p *LetsEncryptProvider) GetQuote(ctx context.Context, req *models.QuoteRequest) (*models.Quote, error) {
	// Let's Encrypt is free
	return &models.Quote{
		CAProvider:   models.CALetsEncrypt,
		ProductName:  "Let's Encrypt DV",
		Price:        0,
		Currency:     "USD",
		IssuanceTime: "minutes",
		Validity:     "90d",
		Features:     []string{"free", "automated", "DV only", "90-day validity"},
	}, nil
}

func (p *LetsEncryptProvider) SubmitOrder(ctx context.Context, order *models.PurchaseOrder, csr []byte) (string, error) {
	// In production: implement full ACME protocol
	// 1. Create order with ACME server
	// 2. Get authorization challenges
	// 3. Respond to challenges (DNS-01 or HTTP-01)
	// 4. Finalize order with CSR
	// 5. Download certificate

	p.logger.Info("submitting ACME order",
		zap.String("domain", order.Domain),
	)

	// Placeholder — real implementation would use an ACME library
	return "acme-order-" + order.ID.String(), nil
}

func (p *LetsEncryptProvider) GetOrderStatus(ctx context.Context, externalOrderID string) (string, error) {
	// Poll ACME server for order status
	// In production: GET the order URL and check status field
	return "issued", nil
}

func (p *LetsEncryptProvider) DownloadCertificate(ctx context.Context, externalOrderID string) (cert []byte, chain []byte, err error) {
	// In production: download from the certificate URL in the finalized order
	return nil, nil, errors.New("not implemented: use ACME library for real download")
}

func (p *LetsEncryptProvider) Revoke(ctx context.Context, serialNumber string, reason string) error {
	// In production: POST revocation to ACME server
	p.logger.Info("revoking certificate via ACME",
		zap.String("serial", serialNumber),
		zap.String("reason", reason),
	)
	return nil
}
