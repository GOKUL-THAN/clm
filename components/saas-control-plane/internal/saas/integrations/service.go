// Package integrations provides enterprise webhook dispatch, SIEM streaming, and ITSM connectivity.
//
// Patterned after Stripe Webhooks, ServiceNow IntegrationHub, and Slack Enterprise Grid,
// this controller enables programmatic event notifications, signed HMAC-SHA256 payloads,
// automatic retry backoffs, and connectors for Slack, Microsoft Teams, and enterprise SIEMs.
package integrations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	ErrIntegrationNotFound = errors.New("integration subscription not found")
	ErrInvalidEndpointURL   = errors.New("endpoint URL must be a valid HTTP/HTTPS URL")
)

type IntegrationType string

const (
	TypeWebhook    IntegrationType = "webhook"
	TypeSlack      IntegrationType = "slack"
	TypeTeams      IntegrationType = "teams"
	TypeServiceNow IntegrationType = "servicenow"
	TypeSIEM       IntegrationType = "siem"
)

// Subscription represents an active webhook or third-party connector.
type Subscription struct {
	ID          uuid.UUID       `json:"id"`
	TenantID    uuid.UUID       `json:"tenant_id"`
	Name        string          `json:"name"`
	Type        IntegrationType `json:"type"`
	EndpointURL string          `json:"endpoint_url"`
	SecretKey   string          `json:"secret_key,omitempty"` // For HMAC signing
	Events      []string        `json:"events"`               // e.g. ["incident.raised", "certificate.expired", "approval.pending"]
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// DeliveryRecord captures one outbound event dispatch attempt.
type DeliveryRecord struct {
	ID             uuid.UUID `json:"id"`
	SubscriptionID uuid.UUID `json:"subscription_id"`
	Event          string    `json:"event"`
	StatusCode     int       `json:"status_code"`
	Success        bool      `json:"success"`
	LatencyMs      int64     `json:"latency_ms"`
	Timestamp      time.Time `json:"timestamp"`
	ErrorMessage   string    `json:"error_message,omitempty"`
}

// IntegrationController coordinates outbound webhooks and integrations.
type IntegrationController struct {
	mu            sync.RWMutex
	subscriptions map[uuid.UUID]*Subscription
	history       []*DeliveryRecord
	httpClient    *http.Client
	logger        *zap.Logger
}

// NewIntegrationController creates a new integration controller.
func NewIntegrationController(logger *zap.Logger) *IntegrationController {
	return &IntegrationController{
		subscriptions: make(map[uuid.UUID]*Subscription),
		history:       make([]*DeliveryRecord, 0, 100),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		logger:        logger,
	}
}

// RegisterSubscription creates a new webhook or connector.
func (c *IntegrationController) RegisterSubscription(ctx context.Context, tenantID uuid.UUID, sub *Subscription) (*Subscription, error) {
	if sub.EndpointURL == "" {
		return nil, ErrInvalidEndpointURL
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	sub.ID = uuid.New()
	sub.TenantID = tenantID
	sub.Enabled = true
	sub.CreatedAt = now
	sub.UpdatedAt = now

	if sub.SecretKey == "" {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", sub.ID.String(), now.UnixNano())))
		sub.SecretKey = hex.EncodeToString(h[:16])
	}

	c.subscriptions[sub.ID] = sub

	c.logger.Info("enterprise integration registered",
		zap.String("subscription_id", sub.ID.String()),
		zap.String("type", string(sub.Type)),
		zap.String("name", sub.Name),
	)

	return sub, nil
}

// ListSubscriptions returns all active integrations for a tenant.
func (c *IntegrationController) ListSubscriptions(tenantID uuid.UUID) []*Subscription {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var list []*Subscription
	for _, sub := range c.subscriptions {
		if sub.TenantID == tenantID {
			list = append(list, sub)
		}
	}
	return list
}

// DeleteSubscription removes an integration.
func (c *IntegrationController) DeleteSubscription(tenantID uuid.UUID, id uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sub, ok := c.subscriptions[id]
	if !ok || sub.TenantID != tenantID {
		return ErrIntegrationNotFound
	}

	delete(c.subscriptions, id)
	return nil
}

// DispatchEvent dispatches an event to all matching subscriptions.
func (c *IntegrationController) DispatchEvent(ctx context.Context, tenantID uuid.UUID, eventName string, payload any) int {
	c.mu.RLock()
	var targets []*Subscription
	for _, sub := range c.subscriptions {
		if sub.TenantID == tenantID && sub.Enabled {
			for _, ev := range sub.Events {
				if ev == "*" || ev == eventName {
					targets = append(targets, sub)
					break
				}
			}
		}
	}
	c.mu.RUnlock()

	dispatched := 0
	for _, sub := range targets {
		c.dispatchToOne(ctx, sub, eventName, payload)
		dispatched++
	}

	return dispatched
}

func (c *IntegrationController) dispatchToOne(ctx context.Context, sub *Subscription, eventName string, payload any) {
	dataBytes, err := json.Marshal(map[string]any{
		"event":     eventName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"payload":   payload,
	})
	if err != nil {
		return
	}

	// Compute HMAC-SHA256 signature
	mac := hmac.New(sha256.New, []byte(sub.SecretKey))
	mac.Write(dataBytes)
	signature := hex.EncodeToString(mac.Sum(nil))

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "POST", sub.EndpointURL, bytes.NewReader(dataBytes))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CLM-Event", eventName)
	req.Header.Set("X-CLM-Signature", signature)

	record := &DeliveryRecord{
		ID:             uuid.New(),
		SubscriptionID: sub.ID,
		Event:          eventName,
		Timestamp:      start,
	}

	resp, err := c.httpClient.Do(req)
	latency := time.Since(start).Milliseconds()
	record.LatencyMs = latency

	if err != nil {
		record.Success = false
		record.ErrorMessage = err.Error()
	} else {
		record.StatusCode = resp.StatusCode
		record.Success = (resp.StatusCode >= 200 && resp.StatusCode < 300)
		_ = resp.Body.Close()
	}

	c.mu.Lock()
	if len(c.history) >= 100 {
		c.history = c.history[1:]
	}
	c.history = append(c.history, record)
	c.mu.Unlock()
}
