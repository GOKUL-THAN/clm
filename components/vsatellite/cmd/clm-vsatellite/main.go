package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/m2p/clm/internal/satellite/deployment"
	datadeployer "github.com/m2p/clm/internal/satellite/deployment/deployers/data"
	linuxdeployer "github.com/m2p/clm/internal/satellite/deployment/deployers/os"
	"github.com/m2p/clm/internal/satellite/discovery/scanners"
	"github.com/m2p/clm/internal/satellite/securefs"
	"github.com/m2p/clm/pkg/config"
	"github.com/m2p/clm/pkg/utils"
	"go.uber.org/zap"
)

// VSatelliteConfig holds vSatellite agent configuration.
type VSatelliteConfig struct {
	ID              string                   `mapstructure:"id"`
	Name            string                   `mapstructure:"name"`
	GatewayEndpoint string                   `mapstructure:"gateway_endpoint"`
	Token           string                   `mapstructure:"token"`
	VPC             string                   `mapstructure:"vpc"`
	Subnet          string                   `mapstructure:"subnet"`
	CIDRs           []string                 `mapstructure:"cidrs"`
	Capabilities    []string                 `mapstructure:"capabilities"`
	HeartbeatSec    int                      `mapstructure:"heartbeat_interval_sec"`
	ScanPorts       []int                    `mapstructure:"scan_ports"`
	Concurrency     int                      `mapstructure:"concurrency"`
	FileStorage     config.FileStorageConfig `mapstructure:"file_storage"`
}

func main() {
	cfgPath := os.Getenv("CLM_VSATELLITE_CONFIG")
	if cfgPath == "" {
		cfgPath = "/etc/clm/vsatellite.yaml"
	}

	cfg, err := config.LoadYAML[VSatelliteConfig](cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger := utils.NewLogger("info")
	defer logger.Sync()

	logger.Info("starting CLM vSatellite agent",
		zap.String("id", cfg.ID),
		zap.String("name", cfg.Name),
		zap.String("gateway", cfg.GatewayEndpoint),
		zap.Strings("cidrs", cfg.CIDRs),
	)

	// Initialize scanner
	ports := cfg.ScanPorts
	if len(ports) == 0 {
		ports = []int{443, 8443, 636, 6443}
	}
	concurrency := cfg.Concurrency
	if concurrency == 0 {
		concurrency = 50
	}
	scanner := scanners.NewNetworkScanner(concurrency, 5*time.Second, ports, logger)

	filePolicy, err := securefs.NewPolicy(cfg.FileStorage.AllowedRoots, cfg.FileStorage.TenantRoots)
	if err != nil {
		logger.Fatal("vSatellite file storage policy is invalid", zap.Error(err))
	}
	defer func() {
		if err := filePolicy.Close(); err != nil {
			logger.Error("closing vSatellite file storage policy failed", zap.Error(err))
		}
	}()

	// Initialize deployment executor
	deployExec := deployment.NewExecutor(logger)
	deployExec.RegisterDeployer(linuxdeployer.NewLinuxDeployer(logger, filePolicy))
	deployExec.RegisterDeployer(datadeployer.NewPostgreSQLDeployer(logger, filePolicy))

	// Create agent
	agent := &Agent{
		config:   cfg,
		scanner:  scanner,
		deployer: deployExec,
		client:   &http.Client{Timeout: 30 * time.Second},
		logger:   logger,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; logger.Info("shutting down"); cancel() }()

	agent.Run(ctx)
	logger.Info("vSatellite agent stopped")
}

// Agent is the vSatellite runtime.
type Agent struct {
	config    *VSatelliteConfig
	scanner   *scanners.NetworkScanner
	deployer  *deployment.Executor
	client    *http.Client
	logger    *zap.Logger
	certCount int
}

// Run starts the agent loop — heartbeat + command execution.
func (a *Agent) Run(ctx context.Context) {
	interval := time.Duration(a.config.HeartbeatSec) * time.Second
	if interval == 0 {
		interval = 30 * time.Second
	}

	// Initial registration heartbeat
	a.sendHeartbeat()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cmds := a.sendHeartbeat()
			for _, cmd := range cmds {
				go a.executeCommand(ctx, cmd)
			}
		}
	}
}

type gatewayCommand struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

func (a *Agent) sendHeartbeat() []gatewayCommand {
	body := map[string]interface{}{
		"agent_id":   a.config.ID,
		"cert_count": a.certCount,
		"version":    "1.0.0",
	}
	data, err := json.Marshal(body)
	if err != nil {
		a.logger.Error("could not encode heartbeat request", zap.Error(err))
		return nil
	}

	url := a.config.GatewayEndpoint + "/vsatellite/heartbeat"
	resp, err := a.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		a.logger.Warn("heartbeat failed", zap.Error(err))
		return nil
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			a.logger.Warn("closing heartbeat response failed", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		a.logger.Warn("heartbeat rejected", zap.Int("status", resp.StatusCode))
		return nil
	}

	var result struct {
		Commands []gatewayCommand `json:"commands"`
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		a.logger.Warn("reading heartbeat response failed", zap.Error(err))
		return nil
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		a.logger.Warn("decoding heartbeat response failed", zap.Error(err))
		return nil
	}

	if len(result.Commands) > 0 {
		a.logger.Info("received commands", zap.Int("count", len(result.Commands)))
	}

	return result.Commands
}

func (a *Agent) executeCommand(ctx context.Context, cmd gatewayCommand) {
	a.logger.Info("executing command", zap.String("id", cmd.ID), zap.String("type", cmd.Type))

	var result string
	var status string

	switch cmd.Type {
	case "scan_cidr":
		result, status = a.handleScanCIDR(ctx, cmd.Payload)
	case "deploy_cert":
		result, status = a.handleDeployCert(ctx, cmd.Payload)
	case "health_check":
		result, status = `{"status":"healthy"}`, "completed"
	default:
		result, status = fmt.Sprintf("unknown command type: %s", cmd.Type), "failed"
	}

	// Report result back to gateway
	a.reportResult(cmd.ID, status, result)
}

func (a *Agent) handleScanCIDR(ctx context.Context, payload string) (string, string) {
	var req struct {
		CIDRs []string `json:"cidrs"`
	}
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return err.Error(), "failed"
	}

	cidrs := req.CIDRs
	if len(cidrs) == 0 {
		cidrs = a.config.CIDRs
	}

	var allCerts []*scanners.DiscoveredCert
	for _, cidr := range cidrs {
		certs, err := a.scanner.ScanNetwork(ctx, cidr)
		if err != nil {
			a.logger.Warn("scan failed", zap.String("cidr", cidr), zap.Error(err))
			continue
		}
		allCerts = append(allCerts, certs...)
	}

	result := map[string]interface{}{
		"status":      "scan_complete",
		"certs_found": len(allCerts),
		"results":     allCerts,
	}
	data, _ := json.Marshal(result)
	return string(data), "completed"
}

func (a *Agent) handleDeployCert(ctx context.Context, payload string) (string, string) {
	req, err := deployment.ParseDeployPayload(payload)
	if err != nil {
		return err.Error(), "failed"
	}

	result, err := a.deployer.Deploy(ctx, req)
	if err != nil {
		return err.Error(), "failed"
	}

	data, _ := json.Marshal(result)
	return string(data), "completed"
}

func (a *Agent) reportResult(cmdID, status, result string) {
	body := map[string]string{
		"agent_id": a.config.ID,
		"cmd_id":   cmdID,
		"status":   status,
		"result":   result,
	}
	data, err := json.Marshal(body)
	if err != nil {
		a.logger.Error("could not encode command result", zap.Error(err))
		return
	}

	url := a.config.GatewayEndpoint + "/vsatellite/result"
	resp, err := a.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		a.logger.Warn("result report failed", zap.Error(err))
		return
	}
	if err := resp.Body.Close(); err != nil {
		a.logger.Warn("closing command result response failed", zap.Error(err))
	}
}
