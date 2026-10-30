package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// GoDaddyProvider implements the CAProviderPlugin for GoDaddy.
type GoDaddyProvider struct {
	apiKey    string
	apiSecret string
	baseURL   string
	customerID string
	client    *http.Client
	logger    *zap.Logger
}

// NewGoDaddyProvider creates a new GoDaddy CA provider.
func NewGoDaddyProvider(apiKey, apiSecret, customerID string, production bool, logger *zap.Logger) *GoDaddyProvider {
	baseURL := "https://api.ote-godaddy.com" // OTE = test
	if production {
		baseURL = "https://api.godaddy.com"
	}

	return &GoDaddyProvider{
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		baseURL:    baseURL,
		customerID: customerID,
		client:     &http.Client{},
		logger:     logger,
	}
}

func (p *GoDaddyProvider) Name() string {
	return "godaddy"
}

func (p *GoDaddyProvider) GetQuote(ctx context.Context, req *models.QuoteRequest) (*models.Quote, error) {
	// GoDaddy pricing (approximate — real implementation queries API)
	price := 69.99 // DV SSL 1 year
	product := "DV SSL"

	switch req.CertType {
	case models.CertTypeOV:
		price = 149.99
		product = "OV SSL"
	case models.CertTypeEV:
		price = 299.99
		product = "EV SSL"
	case models.CertTypeWildcard:
		price = 295.99
		product = "Wildcard DV SSL"
	}

	if req.Validity == "2y" {
		price *= 1.8
	}

	return &models.Quote{
		CAProvider:   models.CAGoDaddy,
		ProductName:  product,
		Price:        price,
		Currency:     "USD",
		IssuanceTime: "1-5 days",
		Validity:     req.Validity,
		Features:     []string{"site_seal", "$1M warranty", "unlimited_reissue"},
	}, nil
}

func (p *GoDaddyProvider) SubmitOrder(ctx context.Context, order *models.PurchaseOrder, csr []byte) (string, error) {
	body := map[string]interface{}{
		"productType": mapCertType(order.CertType),
		"csr":         string(csr),
		"period":      parsePeriod(order.Validity),
		"contact": map[string]string{
			"email": "certs@company.com",
		},
	}

	jsonBody, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/v1/certificates", p.baseURL)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", p.apiKey, p.apiSecret))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("godaddy API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("godaddy API error: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		CertificateID string `json:"certificateId"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	p.logger.Info("GoDaddy order submitted",
		zap.String("cert_id", result.CertificateID),
		zap.String("domain", order.Domain),
	)

	return result.CertificateID, nil
}

func (p *GoDaddyProvider) GetOrderStatus(ctx context.Context, externalOrderID string) (string, error) {
	url := fmt.Sprintf("%s/v1/certificates/%s", p.baseURL, externalOrderID)

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", p.apiKey, p.apiSecret))

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	// Map GoDaddy status to our status
	switch result.Status {
	case "ISSUED":
		return "issued", nil
	case "PENDING_ISSUANCE", "PENDING_VALIDATION":
		return "pending", nil
	case "REVOKED", "CANCELED", "DENIED":
		return "cancelled", nil
	default:
		return "pending", nil
	}
}

func (p *GoDaddyProvider) DownloadCertificate(ctx context.Context, externalOrderID string) (cert []byte, chain []byte, err error) {
	url := fmt.Sprintf("%s/v1/certificates/%s/download", p.baseURL, externalOrderID)

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", p.apiKey, p.apiSecret))
	req.Header.Set("Accept", "application/pem")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Pems struct {
			Certificate string `json:"certificate"`
			Cross       string `json:"cross"`
			Intermediate string `json:"intermediate"`
			Root        string `json:"root"`
		} `json:"pems"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	certBytes := []byte(result.Pems.Certificate)
	chainBytes := []byte(result.Pems.Intermediate + "\n" + result.Pems.Root)

	return certBytes, chainBytes, nil
}

func (p *GoDaddyProvider) Revoke(ctx context.Context, serialNumber string, reason string) error {
	p.logger.Info("revoking certificate via GoDaddy",
		zap.String("serial", serialNumber),
	)
	// In production: POST to /v1/certificates/{id}/revoke
	return nil
}

func mapCertType(ct models.CertType) string {
	switch ct {
	case models.CertTypeEV:
		return "EV_SSL"
	case models.CertTypeOV:
		return "OV_SSL"
	case models.CertTypeWildcard:
		return "DV_WILDCARD_SSL"
	default:
		return "DV_SSL"
	}
}

func parsePeriod(validity string) int {
	switch validity {
	case "2y":
		return 2
	case "3y":
		return 3
	default:
		return 1
	}
}
