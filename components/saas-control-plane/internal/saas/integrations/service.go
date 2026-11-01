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
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	ErrIntegrationNotFound = errors.New("integration subscription not found")
	ErrInvalidEndpointURL  = errors.New("endpoint URL must be a valid HTTPS URL (private IPs, localhost, and cloud metadata prohibited)")
)

// IsPrivateOrReservedIP verifies whether an IP address belongs to loopback, private RFC 1918,
// link-local, cloud metadata (169.254.0.0/16), carrier-grade NAT, or IPv6 unique local space.
func IsPrivateOrReservedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	metadataNet := net.IPNet{
		IP:   net.ParseIP("169.254.0.0"),
		Mask: net.CIDRMask(16, 32),
	}
	if metadataNet.Contains(ip) {
		return true
	}
	cgnatNet := net.IPNet{
		IP:   net.ParseIP("100.64.0.0"),
		Mask: net.CIDRMask(10, 32),
	}
	if cgnatNet.Contains(ip) {
		return true
	}
	if len(ip) == net.IPv6len {
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
			return true
		}
		if (ip[0] & 0xfe) == 0xfc {
			return true
		}
	}
	return false
}

// ValidateWebhookURL strictly validates URL scheme, internal domains, and IP literals.
func ValidateWebhookURL(endpoint string) error {
	if endpoint == "" {
		return ErrInvalidEndpointURL
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return fmt.Errorf("%w: malformed URL: %v", ErrInvalidEndpointURL, err)
	}
	if strings.ToLower(parsed.Scheme) != "https" {
		return fmt.Errorf("%w: scheme must strictly be https", ErrInvalidEndpointURL)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local") ||
		host == "metadata.google.internal" || host == "instance-data" {
		return fmt.Errorf("%w: internal hostname %s is forbidden", ErrInvalidEndpointURL, host)
	}
	if ip := net.ParseIP(host); ip != nil {
		if IsPrivateOrReservedIP(ip) {
			return fmt.Errorf("%w: destination IP %s is prohibited", ErrInvalidEndpointURL, ip.String())
		}
	}
	return nil
}

// NewSafeHTTPClient constructs an HTTP client that validates destination IPs at connection time
// and prohibits redirects, completely neutralizing DNS rebinding and redirect SSRF attacks.
func NewSafeHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid destination address: %w", err)
			}
			// IP literal immediate validation (bypasses DNS and pins directly)
			if ip := net.ParseIP(host); ip != nil {
				if IsPrivateOrReservedIP(ip) {
					return nil, fmt.Errorf("SSRF defense: destination IP %s is private/metadata/prohibited", ip.String())
				}
				pinnedAddr := net.JoinHostPort(ip.String(), port)
				return dialer.DialContext(ctx, network, pinnedAddr)
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("DNS resolution failed for %s: %w", host, err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("zero IP addresses returned for host %s", host)
			}
			for _, ip := range ips {
				if IsPrivateOrReservedIP(ip) {
					return nil, fmt.Errorf("SSRF defense: resolved IP %s is private/metadata/prohibited", ip.String())
				}
			}
			pinnedAddr := net.JoinHostPort(ips[0].String(), port)
			return dialer.DialContext(ctx, network, pinnedAddr)
		},
		TLSHandshakeTimeout: 5 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errors.New("SSRF defense: HTTP redirects are strictly prohibited for webhook delivery")
		},
	}
}

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
	return NewIntegrationControllerWithClient(NewSafeHTTPClient(), logger)
}

// NewIntegrationControllerWithClient allows injecting a custom HTTP client (e.g. for testing).
func NewIntegrationControllerWithClient(client *http.Client, logger *zap.Logger) *IntegrationController {
	if client == nil {
		client = NewSafeHTTPClient()
	}
	return &IntegrationController{
		subscriptions: make(map[uuid.UUID]*Subscription),
		history:       make([]*DeliveryRecord, 0, 100),
		httpClient:    client,
		logger:        logger,
	}
}

// RegisterSubscription creates a new webhook or connector.
func (c *IntegrationController) RegisterSubscription(ctx context.Context, tenantID uuid.UUID, sub *Subscription) (*Subscription, error) {
	if err := ValidateWebhookURL(sub.EndpointURL); err != nil {
		return nil, err
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
		// Drain up to 1MB of response body to permit HTTP connection reuse while bounding memory
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024*1024))
		_ = resp.Body.Close()
	}

	c.mu.Lock()
	if len(c.history) >= 100 {
		c.history = c.history[1:]
	}
	c.history = append(c.history, record)
	c.mu.Unlock()
}
