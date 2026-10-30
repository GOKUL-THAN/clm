package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/m2p/clm/vsatellite/internal/deployer"
	"github.com/m2p/clm/vsatellite/internal/scanner"
	"go.uber.org/zap"
)

// Config defines the configuration for a vSatellite agent.
type Config struct {
	ID              string   `mapstructure:"id" json:"id"`
	Name            string   `mapstructure:"name" json:"name"`
	GatewayEndpoint string   `mapstructure:"gateway_endpoint" json:"gateway_endpoint"`
	Token           string   `mapstructure:"token" json:"token"`
	AgentSecret     string   `mapstructure:"agent_secret" json:"agent_secret"`
	VPC             string   `mapstructure:"vpc" json:"vpc"`
	Subnet          string   `mapstructure:"subnet" json:"subnet"`
	CIDRs           []string `mapstructure:"cidrs" json:"cidrs"`
	Capabilities    []string `mapstructure:"capabilities" json:"capabilities"`
	HeartbeatSec    int      `mapstructure:"heartbeat_interval_sec" json:"heartbeat_interval_sec"`
	ScanPorts       []int    `mapstructure:"scan_ports" json:"scan_ports"`
	Concurrency     int      `mapstructure:"concurrency" json:"concurrency"`
	LogLevel        string   `mapstructure:"log_level" json:"log_level"`
}

// GatewayCommand represents a leased command issued by the coordinator.
type GatewayCommand struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// Agent is the core runtime execution engine of vSatellite.
type Agent struct {
	cfg         *Config
	scanner     *scanner.Scanner
	deployer    *deployer.Deployer
	client      *http.Client
	logger      *zap.Logger
	certCount   atomic.Int64
	startTime   time.Time
	secretMu    sync.RWMutex
	agentSecret string
}

// NewAgent creates and initializes an agent instance.
func NewAgent(cfg *Config, s *scanner.Scanner, d *deployer.Deployer, logger *zap.Logger) *Agent {
	if cfg.HeartbeatSec <= 0 {
		cfg.HeartbeatSec = 30
	}
	if len(cfg.Capabilities) == 0 {
		cfg.Capabilities = []string{"scan_cidr", "deploy_cert", "health_check"}
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Agent{
		cfg:         cfg,
		scanner:     s,
		deployer:    d,
		client:      &http.Client{Timeout: 30 * time.Second},
		logger:      logger,
		startTime:   time.Now(),
		agentSecret: cfg.AgentSecret,
	}
}

// getSecret returns the current agent secret in a thread-safe manner.
func (a *Agent) getSecret() string {
	a.secretMu.RLock()
	defer a.secretMu.RUnlock()
	return a.agentSecret
}

// setSecret updates the agent secret.
func (a *Agent) setSecret(sec string) {
	a.secretMu.Lock()
	defer a.secretMu.Unlock()
	a.agentSecret = sec
}

// Enroll exchanges a single-use token for a per-agent secret from the Gateway.
func (a *Agent) Enroll(ctx context.Context) error {
	host, _ := os.Hostname()
	reqBody := map[string]string{
		"token":    a.cfg.Token,
		"agent_id": a.cfg.ID,
		"host":     host,
		"version":  "1.0.0",
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal enrollment request: %w", err)
	}

	url := fmt.Sprintf("%s/vsatellite/enroll", a.cfg.GatewayEndpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create enrollment request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.Token)
		req.Header.Set("X-Agent-Secret", a.cfg.Token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("enrollment call failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("enrollment rejected (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var enrollResp struct {
		Status       string   `json:"status"`
		AgentID      string   `json:"agent_id"`
		AgentSecret  string   `json:"agent_secret"`
		Name         string   `json:"name"`
		CIDRs        []string `json:"cidrs"`
		Capabilities []string `json:"capabilities"`
	}

	if err := json.Unmarshal(bodyBytes, &enrollResp); err != nil {
		return fmt.Errorf("failed to parse enrollment response: %w", err)
	}

	a.setSecret(enrollResp.AgentSecret)
	if enrollResp.AgentID != "" {
		a.cfg.ID = enrollResp.AgentID
	}
	if len(enrollResp.CIDRs) > 0 {
		a.cfg.CIDRs = enrollResp.CIDRs
	}

	a.logger.Info("enrolled successfully with satellite gateway",
		zap.String("agent_id", a.cfg.ID),
		zap.Strings("cidrs", a.cfg.CIDRs),
	)

	return nil
}

// Run executes the heartbeat loop, pulls commands, and processes work until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	// If agent_secret is empty but a token is provided, enroll first
	if a.getSecret() == "" && a.cfg.Token != "" {
		if err := a.Enroll(ctx); err != nil {
			a.logger.Warn("initial enrollment failed, will retry during heartbeat", zap.Error(err))
		}
	}

	interval := time.Duration(a.cfg.HeartbeatSec) * time.Second
	if interval < 5*time.Second {
		interval = 30 * time.Second
	}

	// Initial heartbeat
	a.heartbeatAndDispatch(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	a.logger.Info("vSatellite agent running heartbeat loop",
		zap.Duration("interval", interval),
		zap.String("gateway", a.cfg.GatewayEndpoint),
	)

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("vSatellite agent loop shutting down")
			return
		case <-ticker.C:
			a.heartbeatAndDispatch(ctx)
		}
	}
}

func (a *Agent) heartbeatAndDispatch(ctx context.Context) {
	cmds, err := a.sendHeartbeat(ctx)
	if err != nil {
		a.logger.Warn("heartbeat error", zap.Error(err))
		return
	}

	for _, cmd := range cmds {
		cmdCopy := cmd
		go a.executeCommand(ctx, cmdCopy)
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) ([]GatewayCommand, error) {
	reqBody := map[string]interface{}{
		"agent_id":   a.cfg.ID,
		"cert_count": a.certCount.Load(),
		"version":    "1.0.0",
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/vsatellite/heartbeat", a.cfg.GatewayEndpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	secret := a.getSecret()
	if secret != "" {
		req.Header.Set("X-Agent-Secret", secret)
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway dial failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		a.logger.Warn("heartbeat unauthorized - attempting re-enrollment")
		if a.cfg.Token != "" {
			_ = a.Enroll(ctx)
		}
		return nil, fmt.Errorf("unauthorized (401)")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Status   string           `json:"status"`
		Commands []GatewayCommand `json:"commands"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse commands: %w", err)
	}

	if len(result.Commands) > 0 {
		a.logger.Info("received commands from gateway coordinator", zap.Int("count", len(result.Commands)))
	}

	return result.Commands, nil
}

func (a *Agent) executeCommand(ctx context.Context, cmd GatewayCommand) {
	a.logger.Info("executing coordinator command",
		zap.String("id", cmd.ID),
		zap.String("type", cmd.Type),
	)

	var result string
	var status string

	switch cmd.Type {
	case "scan_cidr":
		result, status = a.handleScanCIDR(ctx, cmd.Payload)
	case "deploy_cert":
		result, status = a.handleDeployCert(ctx, cmd.Payload)
	case "health_check":
		result, status = a.handleHealthCheck()
	default:
		status = "failed"
		result = fmt.Sprintf("unsupported command type: %s", cmd.Type)
	}

	a.reportResult(ctx, cmd.ID, status, result)
}

func (a *Agent) handleScanCIDR(ctx context.Context, payload string) (string, string) {
	var req struct {
		CIDRs []string `json:"cidrs"`
	}
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return fmt.Sprintf(`{"error":"invalid scan payload: %s"}`, err.Error()), "failed"
	}

	cidrs := req.CIDRs
	if len(cidrs) == 0 {
		cidrs = a.cfg.CIDRs
	}
	if len(cidrs) == 0 {
		return `{"error":"no CIDRs configured for scanning"}`, "failed"
	}

	var allCerts []*scanner.DiscoveredCert
	for _, cidr := range cidrs {
		certs, err := a.scanner.ScanCIDR(ctx, cidr)
		if err != nil {
			a.logger.Warn("CIDR scan failed", zap.String("cidr", cidr), zap.Error(err))
			continue
		}
		allCerts = append(allCerts, certs...)
	}

	a.certCount.Store(int64(len(allCerts)))

	out := map[string]interface{}{
		"status":      "scan_complete",
		"certs_found": len(allCerts),
		"results":     allCerts,
	}
	data, _ := json.Marshal(out)
	return string(data), "completed"
}

func (a *Agent) handleDeployCert(ctx context.Context, payload string) (string, string) {
	req, err := deployer.ParseDeployPayload(payload)
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error()), "failed"
	}

	result, err := a.deployer.Deploy(ctx, req)
	if err != nil {
		data, _ := json.Marshal(result)
		return string(data), "failed"
	}

	data, _ := json.Marshal(result)
	return string(data), "completed"
}

func (a *Agent) handleHealthCheck() (string, string) {
	uptime := int(time.Since(a.startTime).Seconds())
	out := map[string]interface{}{
		"status":      "healthy",
		"agent_id":    a.cfg.ID,
		"uptime_sec":  uptime,
		"cert_count":  a.certCount.Load(),
		"version":     "1.0.0",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(out)
	return string(data), "completed"
}

func (a *Agent) reportResult(ctx context.Context, cmdID, status, result string) {
	body := map[string]string{
		"agent_id": a.cfg.ID,
		"cmd_id":   cmdID,
		"status":   status,
		"result":   result,
	}
	data, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/vsatellite/result", a.cfg.GatewayEndpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		a.logger.Error("failed to construct result request", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	secret := a.getSecret()
	if secret != "" {
		req.Header.Set("X-Agent-Secret", secret)
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		a.logger.Warn("result reporting failed", zap.String("cmd_id", cmdID), zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		a.logger.Warn("gateway rejected result report",
			zap.String("cmd_id", cmdID),
			zap.Int("status", resp.StatusCode),
		)
		return
	}

	a.logger.Info("command result acknowledged by coordinator",
		zap.String("cmd_id", cmdID),
		zap.String("status", status),
	)
}
