package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// DigiCertProvider implements the CAProviderPlugin for DigiCert CertCentral.
type DigiCertProvider struct {
	apiKey  string
	orgID   string
	baseURL string
	client  *http.Client
	logger  *zap.Logger
}

// NewDigiCertProvider creates a new DigiCert CA provider.
func NewDigiCertProvider(apiKey, orgID string, logger *zap.Logger) *DigiCertProvider {
	return &DigiCertProvider{
		apiKey:  apiKey,
		orgID:   orgID,
		baseURL: "https://www.digicert.com/services/v2",
		client:  &http.Client{Timeout: 30 * time.Second},
		logger:  logger,
	}
}

func (p *DigiCertProvider) Name() string {
	return "digicert"
}

func (p *DigiCertProvider) GetQuote(ctx context.Context, req *models.QuoteRequest) (*models.Quote, error) {
	price := 268.0
	product := "SSL Plus"

	switch req.CertType {
	case models.CertTypeOV:
		price = 268.0
		product = "SSL Plus (OV)"
	case models.CertTypeEV:
		price = 495.0
		product = "EV SSL"
	case models.CertTypeWildcard:
		price = 688.0
		product = "Wildcard Plus"
	case models.CertTypeSAN:
		price = 399.0
		product = "Multi-Domain SSL"
	}

	if req.Validity == "2y" {
		price *= 1.85
	}

	return &models.Quote{
		CAProvider:   models.CADigiCert,
		ProductName:  product,
		Price:        price,
		Currency:     "USD",
		IssuanceTime: "1-3 days",
		Validity:     req.Validity,
		Features:     []string{"$1.5M warranty", "site_seal", "priority_support", "unlimited_reissue"},
	}, nil
}

func (p *DigiCertProvider) SubmitOrder(ctx context.Context, order *models.PurchaseOrder, csr []byte) (string, error) {
	body := map[string]interface{}{
		"certificate": map[string]interface{}{
			"common_name": order.Domain,
			"dns_names":   order.SANs,
			"csr":         string(csr),
		},
		"organization": map[string]string{
			"id": p.orgID,
		},
		"validity_years": parsePeriod(order.Validity),
		"product": map[string]string{
			"name_id": digiCertProduct(order.CertType),
		},
	}

	jsonBody, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/order/certificate/%s", p.baseURL, digiCertProduct(order.CertType))

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	req.Header.Set("X-DC-DEVKEY", p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("digicert API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("digicert API error: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		ID      int `json:"id"`
		OrderID int `json:"order_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	orderID := fmt.Sprintf("%d", result.ID)
	p.logger.Info("DigiCert order submitted",
		zap.String("order_id", orderID),
		zap.String("domain", order.Domain),
	)

	return orderID, nil
}

func (p *DigiCertProvider) GetOrderStatus(ctx context.Context, externalOrderID string) (string, error) {
	url := fmt.Sprintf("%s/order/certificate/%s", p.baseURL, externalOrderID)

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("X-DC-DEVKEY", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	switch result.Status {
	case "issued":
		return "issued", nil
	case "pending", "processing":
		return "pending", nil
	case "rejected", "revoked":
		return "cancelled", nil
	default:
		return "pending", nil
	}
}

func (p *DigiCertProvider) DownloadCertificate(ctx context.Context, externalOrderID string) (cert []byte, chain []byte, err error) {
	url := fmt.Sprintf("%s/certificate/%s/download/format/pem_all", p.baseURL, externalOrderID)

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("X-DC-DEVKEY", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// DigiCert returns PEM bundle — split into cert and chain
	// In production: properly parse the PEM bundle
	return body, nil, nil
}

func (p *DigiCertProvider) Revoke(ctx context.Context, serialNumber string, reason string) error {
	p.logger.Info("revoking certificate via DigiCert",
		zap.String("serial", serialNumber),
	)
	return nil
}

func digiCertProduct(ct models.CertType) string {
	switch ct {
	case models.CertTypeEV:
		return "ssl_ev_plus"
	case models.CertTypeOV:
		return "ssl_plus"
	case models.CertTypeWildcard:
		return "ssl_wildcard"
	case models.CertTypeSAN:
		return "ssl_multi_domain"
	default:
		return "ssl_basic"
	}
}
