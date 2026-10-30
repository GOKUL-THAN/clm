package purchasing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/internal/saas/approval"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

var (
	ErrNoCAConfigured   = errors.New("no CA provider configured for this tenant")
	ErrBudgetExceeded   = errors.New("purchase would exceed monthly budget")
	ErrOrderNotFound    = errors.New("purchase order not found")
	ErrInvalidCA        = errors.New("unsupported CA provider")
)

// Service orchestrates certificate purchasing from multiple CAs.
type Service struct {
	purchaseRepo core.PurchaseRepository
	approvalEng  *approval.Engine
	providers    map[string]core.CAProviderPlugin
	logger       *zap.Logger
}

// NewService creates a new purchasing service.
func NewService(
	purchaseRepo core.PurchaseRepository,
	approvalEng *approval.Engine,
	logger *zap.Logger,
) *Service {
	return &Service{
		purchaseRepo: purchaseRepo,
		approvalEng:  approvalEng,
		providers:    make(map[string]core.CAProviderPlugin),
		logger:       logger,
	}
}

// RegisterProvider adds a CA provider plugin.
func (s *Service) RegisterProvider(name string, provider core.CAProviderPlugin) {
	s.providers[name] = provider
}

// GetQuotes fetches pricing from all configured (or specified) CAs.
func (s *Service) GetQuotes(ctx context.Context, tenantID uuid.UUID, req *models.QuoteRequest) ([]*models.Quote, error) {
	accounts, err := s.purchaseRepo.GetCAAccounts(ctx, tenantID)
	if err != nil || len(accounts) == 0 {
		return nil, ErrNoCAConfigured
	}

	var quotes []*models.Quote
	for _, account := range accounts {
		provider, ok := s.providers[string(account.Provider)]
		if !ok {
			continue
		}

		quote, err := provider.GetQuote(ctx, req)
		if err != nil {
			s.logger.Warn("failed to get quote from CA",
				zap.String("ca", string(account.Provider)),
				zap.Error(err),
			)
			continue
		}
		quotes = append(quotes, quote)
	}

	return quotes, nil
}

// SubmitPurchase creates a purchase order and routes through approval.
func (s *Service) SubmitPurchase(ctx context.Context, tenantID uuid.UUID, input *PurchaseInput) (*models.PurchaseOrder, *models.ApprovalRequest, error) {
	// Validate CA provider
	if _, ok := s.providers[input.CAProvider]; !ok {
		return nil, nil, ErrInvalidCA
	}

	// Check budget
	accounts, _ := s.purchaseRepo.GetCAAccounts(ctx, tenantID)
	for _, acc := range accounts {
		if string(acc.Provider) == input.CAProvider {
			if acc.Budget > 0 && (acc.SpentMonth+input.Price) > acc.Budget {
				return nil, nil, ErrBudgetExceeded
			}
			break
		}
	}

	// Create purchase order
	order := &models.PurchaseOrder{
		ID:            uuid.New(),
		TenantID:      tenantID,
		CertRequestID: input.CertRequestID,
		CAProvider:    models.CAProvider(input.CAProvider),
		Domain:        input.Domain,
		SANs:          input.SANs,
		CertType:      input.CertType,
		Validity:      input.Validity,
		Price:         input.Price,
		Currency:      "USD",
		Status:        models.PurchasePendingApproval,
		RequestedBy:   input.RequestedBy,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.purchaseRepo.Create(ctx, order); err != nil {
		return nil, nil, err
	}

	// Submit for approval
	isWildcard := strings.HasPrefix(input.Domain, "*.")
	approvalReq, err := s.approvalEng.SubmitRequest(ctx, tenantID, &approval.SubmitInput{
		RequestType:   "cert_purchase",
		RequestedBy:   input.RequestedBy,
		Justification: input.Justification,
		Payload:       approval.PayloadToJSON(order),
		EstimatedCost: input.Price,
		CertType:      string(input.CertType),
		Environment:   input.Environment,
		IsWildcard:    isWildcard,
		CAProvider:    input.CAProvider,
	})
	if err != nil {
		return order, nil, err
	}

	// Link approval to order
	order.ApprovalID = &approvalReq.ID
	order.UpdatedAt = time.Now()

	// If auto-approved, proceed to purchase immediately
	if approvalReq.Status == models.ApprovalApproved {
		order.Status = models.PurchaseApproved
	}

	_ = s.purchaseRepo.Update(ctx, order)

	s.logger.Info("purchase order created",
		zap.String("order_id", order.ID.String()),
		zap.String("ca", input.CAProvider),
		zap.String("domain", input.Domain),
		zap.Float64("price", input.Price),
		zap.String("approval_status", string(approvalReq.Status)),
	)

	return order, approvalReq, nil
}

// ExecutePurchase submits the order to the CA after approval.
func (s *Service) ExecutePurchase(ctx context.Context, orderID uuid.UUID, csr []byte) error {
	order, err := s.purchaseRepo.GetByID(ctx, orderID)
	if err != nil {
		return ErrOrderNotFound
	}

	if order.Status != models.PurchaseApproved {
		return errors.New("order not approved")
	}

	provider, ok := s.providers[string(order.CAProvider)]
	if !ok {
		return ErrInvalidCA
	}

	// Submit to CA
	order.Status = models.PurchaseProcessing
	order.UpdatedAt = time.Now()
	_ = s.purchaseRepo.Update(ctx, order)

	externalID, err := provider.SubmitOrder(ctx, order, csr)
	if err != nil {
		order.Status = models.PurchaseFailed
		order.UpdatedAt = time.Now()
		_ = s.purchaseRepo.Update(ctx, order)
		return err
	}

	order.CAOrderID = externalID
	order.Status = models.PurchaseValidating
	order.UpdatedAt = time.Now()
	_ = s.purchaseRepo.Update(ctx, order)

	s.logger.Info("purchase order submitted to CA",
		zap.String("order_id", orderID.String()),
		zap.String("ca_order_id", externalID),
	)

	return nil
}

// CheckOrderStatus polls the CA for order status.
func (s *Service) CheckOrderStatus(ctx context.Context, orderID uuid.UUID) (*models.PurchaseOrder, error) {
	order, err := s.purchaseRepo.GetByID(ctx, orderID)
	if err != nil {
		return nil, ErrOrderNotFound
	}

	if order.CAOrderID == "" || order.Status == models.PurchaseIssued {
		return order, nil
	}

	provider, ok := s.providers[string(order.CAProvider)]
	if !ok {
		return order, nil
	}

	status, err := provider.GetOrderStatus(ctx, order.CAOrderID)
	if err != nil {
		return order, err
	}

	switch status {
	case "issued", "complete":
		order.Status = models.PurchaseIssued
		now := time.Now()
		order.IssuedAt = &now
	case "rejected", "cancelled":
		order.Status = models.PurchaseFailed
	}

	order.UpdatedAt = time.Now()
	_ = s.purchaseRepo.Update(ctx, order)

	return order, nil
}

// ListOrders returns purchase orders for a tenant.
func (s *Service) ListOrders(ctx context.Context, tenantID uuid.UUID, status *models.PurchaseStatus) ([]*models.PurchaseOrder, error) {
	return s.purchaseRepo.ListByTenant(ctx, tenantID, status)
}

// PurchaseInput holds everything needed to initiate a purchase.
type PurchaseInput struct {
	CertRequestID uuid.UUID      `json:"cert_request_id"`
	Domain        string         `json:"domain"`
	SANs          []string       `json:"sans"`
	CertType      models.CertType `json:"cert_type"`
	Validity      string         `json:"validity"`
	CAProvider    string         `json:"ca_provider"`
	Price         float64        `json:"price"`
	Environment   string         `json:"environment"`
	Justification string         `json:"justification"`
	RequestedBy   uuid.UUID      `json:"requested_by"`
}
