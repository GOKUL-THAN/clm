package connector

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

type Connector struct {
	satelliteID    string
	saasURL        string
	token          string
	client         *http.Client
	connected      bool
	mu             sync.RWMutex
	logger         *zap.Logger
	networkRetries int
	maxResultBytes int
}

type Config struct {
	SatelliteID        string
	SaaSEndpoint       string
	Token              string
	TLSCert            string
	TLSKey             string
	TLSCA              string
	TLSServerName      string
	InsecureSkipVerify bool
	ProxyURL           string
	Mode               string
	Environment        string
	MaxResultBytes     int
	NetworkRetries     int
}

func New(cfg *Config, logger *zap.Logger) (*Connector, error) {
	endpoint, err := url.Parse(strings.TrimRight(strings.TrimSpace(cfg.SaaSEndpoint), "/"))
	if err != nil || endpoint.Host == "" {
		return nil, fmt.Errorf("invalid SaaS endpoint")
	}
	production := strings.EqualFold(cfg.Environment, "production") || strings.EqualFold(cfg.Environment, "prod")
	if production && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("production SaaS endpoint must use HTTPS")
	}
	if production && cfg.InsecureSkipVerify {
		return nil, fmt.Errorf("production insecure TLS verification is forbidden")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.TLSServerName, InsecureSkipVerify: cfg.InsecureSkipVerify} // #nosec G402 -- production validation rejects it; retained for explicit tests only.
	if cfg.TLSCA != "" {
		pem, readErr := os.ReadFile(cfg.TLSCA)
		if readErr != nil {
			return nil, fmt.Errorf("reading SaaS CA bundle: %w", readErr)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("SaaS CA bundle contains no certificates")
		}
		tlsConfig.RootCAs = pool
	}
	if cfg.TLSCert != "" || cfg.TLSKey != "" {
		if cfg.TLSCert == "" || cfg.TLSKey == "" {
			return nil, errors.New("both client TLS certificate and key are required")
		}
		cert, loadErr := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if loadErr != nil {
			return nil, fmt.Errorf("loading gateway client certificate: %w", loadErr)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if production && (cfg.TLSCA == "" || len(tlsConfig.Certificates) == 0) {
		return nil, errors.New("production connector requires server CA and client certificate")
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig, MaxIdleConns: 10, IdleConnTimeout: 90 * time.Second, DisableCompression: false}
	if cfg.ProxyURL != "" {
		proxy, parseErr := url.Parse(cfg.ProxyURL)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", parseErr)
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	retries := cfg.NetworkRetries
	if retries <= 0 {
		retries = 3
	}
	maxResultBytes := cfg.MaxResultBytes
	if maxResultBytes <= 0 {
		maxResultBytes = 256 << 10
	}
	return &Connector{satelliteID: cfg.SatelliteID, saasURL: endpoint.String(), token: cfg.Token, client: &http.Client{Transport: transport, Timeout: 30 * time.Second}, logger: logger, networkRetries: retries, maxResultBytes: maxResultBytes}, nil
}

func (c *Connector) Connect(ctx context.Context) error {
	id, err := uuid.Parse(c.satelliteID)
	if err != nil {
		return fmt.Errorf("satellite id %q is not a UUID: %w", c.satelliteID, err)
	}
	resp, err := c.doRequestWithRetry(ctx, http.MethodPost, "/api/v1/satellites/"+c.satelliteID+"/heartbeat", &models.SatelliteHeartbeat{SatelliteID: id, Status: "online", Timestamp: time.Now()})
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer drainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("connection rejected: %s", statusDetail(resp))
	}
	c.setConnected(true)
	c.logger.Info("connected to SaaS control plane", zap.String("endpoint", c.saasURL), zap.String("satellite_id", c.satelliteID))
	return nil
}
func (c *Connector) IsConnected() bool   { c.mu.RLock(); defer c.mu.RUnlock(); return c.connected }
func (c *Connector) setConnected(v bool) { c.mu.Lock(); c.connected = v; c.mu.Unlock() }
func (c *Connector) SendHeartbeat(ctx context.Context, hb *models.SatelliteHeartbeat) error {
	resp, err := c.doRequestWithRetry(ctx, http.MethodPost, "/api/v1/satellites/"+c.satelliteID+"/heartbeat", hb)
	if err != nil {
		c.setConnected(false)
		return err
	}
	defer drainAndClose(resp)
	ok := resp.StatusCode == http.StatusOK
	c.setConnected(ok)
	if !ok {
		return fmt.Errorf("heartbeat rejected: %s", statusDetail(resp))
	}
	return nil
}
func (c *Connector) PullCommands(ctx context.Context) ([]*models.SatelliteCommand, error) {
	path := "/api/v1/satellites/" + c.satelliteID + "/commands?lease_owner=" + url.QueryEscape(c.satelliteID)
	resp, err := c.doRequestWithRetry(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pulling commands failed: %s", statusDetail(resp))
	}
	var result struct {
		Commands []*models.SatelliteCommand `json:"commands"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, err
	}
	return result.Commands, nil
}

func (c *Connector) AcknowledgeCommand(ctx context.Context, cmdID string, leaseToken uuid.UUID) error {
	return c.leaseRequest(ctx, cmdID, "ack", leaseToken)
}
func (c *Connector) RenewCommandLease(ctx context.Context, cmdID string, leaseToken uuid.UUID) error {
	return c.leaseRequest(ctx, cmdID, "lease", leaseToken)
}
func (c *Connector) leaseRequest(ctx context.Context, cmdID, action string, leaseToken uuid.UUID) error {
	resp, err := c.doRequestWithRetry(ctx, http.MethodPost, "/api/v1/satellites/"+c.satelliteID+"/commands/"+cmdID+"/"+action, map[string]string{"lease_token": leaseToken.String()})
	if err != nil {
		return err
	}
	defer drainAndClose(resp)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s command %s failed: %s", action, cmdID, statusDetail(resp))
	}
	return nil
}

type resultRequest struct {
	LeaseToken string `json:"lease_token"`
	Status     string `json:"status"`
	Result     string `json:"result"`
	Error      string `json:"error"`
	Retryable  bool   `json:"retryable"`
}

func (c *Connector) ReportResult(ctx context.Context, cmdID string, leaseToken uuid.UUID, status, resultData, errorText string, retryable bool) error {
	body := &resultRequest{LeaseToken: leaseToken.String(), Status: status, Result: resultData, Error: errorText, Retryable: retryable}
	if err := boundResultRequest(body, c.maxResultBytes); err != nil {
		return err
	}
	resp, err := c.doRequestWithRetry(ctx, http.MethodPost, "/api/v1/satellites/"+c.satelliteID+"/commands/"+cmdID+"/result", body)
	if err != nil {
		return err
	}
	defer drainAndClose(resp)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("reporting result for command %s failed: %s", cmdID, statusDetail(resp))
	}
	return nil
}

// boundResultRequest ensures the complete serialized request, including lease
// metadata and JSON escaping, fits the control-plane body limit. Field-level
// truncation alone is insufficient because quotes and control characters expand.
func boundResultRequest(body *resultRequest, maximum int) error {
	for {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding command result: %w", err)
		}
		if len(encoded) <= maximum {
			return nil
		}
		overflow := len(encoded) - maximum
		if len(body.Result) >= len(body.Error) && len(body.Result) > 0 {
			body.Result = truncateUTF8(body.Result, len(body.Result)-overflow-1)
			continue
		}
		if len(body.Error) > 0 {
			body.Error = truncateUTF8(body.Error, len(body.Error)-overflow-1)
			continue
		}
		return fmt.Errorf("command result limit %d is too small for the result envelope", maximum)
	}
}

func truncateUTF8(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	if len(value) <= maximum {
		return value
	}
	for maximum > 0 && (value[maximum]&0xc0) == 0x80 {
		maximum--
	}
	return value[:maximum]
}

func statusDetail(resp *http.Response) string {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		return fmt.Sprintf("status=%d", resp.StatusCode)
	}
	return fmt.Sprintf("status=%d body=%s", resp.StatusCode, detail)
}
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}

func (c *Connector) doRequestWithRetry(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var last error
	for attempt := 0; attempt < c.networkRetries; attempt++ {
		resp, err := c.doRequest(ctx, method, path, body)
		if err == nil {
			return resp, nil
		}
		last = err
		delay := time.Duration(1<<attempt) * 200 * time.Millisecond
		if delay > 5*time.Second {
			delay = 5 * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, last
}
func (c *Connector) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.saasURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Satellite-ID", c.satelliteID)
	return c.client.Do(req)
}
func (c *Connector) StartHeartbeatLoop(ctx context.Context, interval time.Duration, statusFn func() *models.SatelliteHeartbeat) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.SendHeartbeat(ctx, statusFn()); err != nil {
				c.logger.Warn("heartbeat failed", zap.String("satellite_id", c.satelliteID), zap.Error(err))
			}
		}
	}
}
