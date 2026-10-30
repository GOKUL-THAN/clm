package satellite

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/internal/satellite/connector"
	cryptoengine "github.com/m2p/clm/internal/satellite/crypto"
	"github.com/m2p/clm/internal/satellite/deployment"
	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"github.com/m2p/clm/internal/satellite/discovery/scanners"
	"github.com/m2p/clm/internal/satellite/secrets"
	"github.com/m2p/clm/internal/satellite/state"
	"github.com/m2p/clm/pkg/config"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// Runtime is the main satellite process that manages all local operations.
type Runtime struct {
	config     *config.SatelliteConfig
	connector  *connector.Connector
	crypto     *cryptoengine.Engine
	secrets    *secrets.Manager
	scanner    *scanners.NetworkScanner
	dnsScanner *scanners.DNSScanner
	deployer   *deployment.Executor
	journal    *state.CommandJournal
	logger     *zap.Logger
	active     sync.Map

	certCount  int
	agentCount int
}

// NewRuntime creates a new satellite runtime. dnsScanner may be nil, in which
// case domain targets fall back to an address probe without SNI.
func NewRuntime(cfg *config.SatelliteConfig, conn *connector.Connector, cryptoEng *cryptoengine.Engine, secretsMgr *secrets.Manager, scanner *scanners.NetworkScanner, dnsScanner *scanners.DNSScanner, deployer *deployment.Executor, journal *state.CommandJournal, logger *zap.Logger) *Runtime {
	if journal == nil {
		journal, _ = state.NewCommandJournal(nil)
	}
	return &Runtime{
		config:     cfg,
		connector:  conn,
		crypto:     cryptoEng,
		secrets:    secretsMgr,
		scanner:    scanner,
		dnsScanner: dnsScanner,
		deployer:   deployer,
		journal:    journal,
		logger:     logger,
	}
}

// Start begins the satellite runtime loop.
func (r *Runtime) Start(ctx context.Context) error {
	r.logger.Info("starting satellite runtime",
		zap.String("id", r.config.ID),
		zap.String("name", r.config.Name),
	)

	// Connect to SaaS
	if err := r.connector.Connect(ctx); err != nil {
		r.logger.Warn("initial connection to SaaS failed, running in offline mode", zap.Error(err))
	}

	// Start heartbeat loop
	go r.connector.StartHeartbeatLoop(ctx, 30*time.Second, r.buildHeartbeat)

	// Start command polling loop
	go r.commandLoop(ctx)

	<-ctx.Done()
	r.logger.Info("satellite runtime stopped")
	return nil
}

// commandLoop claims and executes durable leased commands.
func (r *Runtime) commandLoop(ctx context.Context) {
	pollInterval := parseRuntimeDuration(r.config.Commands.PollInterval, 5*time.Second)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !r.connector.IsConnected() {
				continue
			}
			cmds, err := r.connector.PullCommands(ctx)
			if err != nil {
				r.logger.Warn("failed to pull commands", zap.Error(err))
				continue
			}
			for _, cmd := range cmds {
				if cmd == nil || cmd.LeaseToken == nil {
					r.logger.Warn("discarding command without a lease token")
					continue
				}
				if _, loaded := r.active.LoadOrStore(cmd.ID, struct{}{}); loaded {
					continue
				}
				go func(command *models.SatelliteCommand) {
					defer r.active.Delete(command.ID)
					r.executeCommand(ctx, command)
				}(cmd)
			}
		}
	}
}

func (r *Runtime) executeCommand(parent context.Context, cmd *models.SatelliteCommand) {
	leaseToken := *cmd.LeaseToken
	if err := r.connector.AcknowledgeCommand(parent, cmd.ID.String(), leaseToken); err != nil {
		r.logger.Warn("command acknowledgement failed", zap.String("command_id", cmd.ID.String()), zap.Error(err))
		return
	}
	if prior, ok := r.journal.Get(cmd.ID, cmd.ExecutionGeneration); ok {
		r.logger.Info("suppressing duplicate command delivery", zap.String("command_id", cmd.ID.String()), zap.Int("attempt", cmd.Attempts), zap.Int("execution_generation", cmd.ExecutionGeneration), zap.String("status", prior.Status))
		r.reportSavedResult(cmd.ID, leaseToken, prior)
		return
	}

	r.logger.Info("executing leased command", zap.String("command_id", cmd.ID.String()), zap.String("command_type", cmd.Type), zap.Int("attempt", cmd.Attempts), zap.Int("execution_generation", cmd.ExecutionGeneration))
	timeout := parseRuntimeDuration(r.config.Commands.Timeout, 15*time.Minute)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	renewEvery := parseRuntimeDuration(r.config.Commands.LeaseRenewal, 45*time.Second)
	renewDone := make(chan struct{})
	go r.renewLease(ctx, cancel, cmd.ID, leaseToken, renewEvery, renewDone)

	var result string
	var err error
	retryable := true
	switch cmd.Type {
	case "create_cert":
		result, err = r.handleCreateCert(ctx, cmd.Payload)
	case "deploy_cert":
		result, err = r.handleDeployCert(ctx, cmd.Payload)
	case "discover":
		result, err = r.handleDiscover(ctx, cmd.Payload)
	case "renew_cert":
		result, err = r.handleRenewCert(ctx, cmd.Payload)
	default:
		err = fmt.Errorf("unknown command type: %s", cmd.Type)
		retryable = false
	}
	if ctx.Err() != nil && err == nil {
		err = ctx.Err()
	}
	cancel()
	<-renewDone
	status := string(models.CommandSucceeded)
	errorText := ""
	if err != nil {
		status = string(models.CommandFailed)
		errorText = err.Error()
		r.logger.Error("command execution failed", zap.String("command_id", cmd.ID.String()), zap.String("command_type", cmd.Type), zap.Error(err))
	}
	result = limitResult(result, r.config.Commands.MaxResultBytes)
	errorText = limitResult(errorText, r.config.Commands.MaxResultBytes)
	saved := state.CommandResult{CommandID: cmd.ID, ExecutionGeneration: cmd.ExecutionGeneration, Status: status, Result: result, Error: errorText, Retryable: retryable, CompletedAt: time.Now().UTC()}
	if err := r.journal.Put(saved); err != nil {
		r.logger.Error("persisting command result failed; result will not be acknowledged", zap.String("command_id", cmd.ID.String()), zap.Error(err))
		return
	}
	r.reportSavedResult(cmd.ID, leaseToken, saved)
}

func (r *Runtime) renewLease(ctx context.Context, cancel context.CancelFunc, commandID uuid.UUID, token uuid.UUID, interval time.Duration, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.connector.RenewCommandLease(ctx, commandID.String(), token); err != nil {
				r.logger.Warn("command lease renewal failed; cancelling execution", zap.String("command_id", commandID.String()), zap.Error(err))
				cancel()
				return
			}
		}
	}
}
func (r *Runtime) reportSavedResult(commandID, token uuid.UUID, result state.CommandResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := r.connector.ReportResult(ctx, commandID.String(), token, result.Status, result.Result, result.Error, result.Retryable); err != nil {
		r.logger.Warn("command result remains durable for redelivery", zap.String("command_id", commandID.String()), zap.Int("execution_generation", result.ExecutionGeneration), zap.Error(err))
		return
	}
	if err := r.journal.Delete(commandID, result.ExecutionGeneration); err != nil {
		r.logger.Warn("confirmed command result journal cleanup failed", zap.String("command_id", commandID.String()), zap.Int("execution_generation", result.ExecutionGeneration), zap.Error(err))
	}
}
func parseRuntimeDuration(value string, fallback time.Duration) time.Duration {
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}
func limitResult(value string, maximum int) string {
	if maximum <= 0 {
		maximum = 256 << 10
	}
	if len(value) <= maximum {
		return value
	}
	suffix := "\n[result truncated]"
	limit := maximum - len(suffix)
	if limit < 0 {
		return suffix[:maximum]
	}
	return value[:limit] + suffix
}

// handleCreateCert generates key, CSR, and stores artifacts.
func (r *Runtime) handleCreateCert(ctx context.Context, payload string) (string, error) {
	var req models.CertRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return "", fmt.Errorf("parsing payload: %w", err)
	}

	// 1. Generate private key
	keyPEM, err := r.crypto.GenerateKey(ctx, req.KeyAlgorithm)
	if err != nil {
		return "", fmt.Errorf("key generation: %w", err)
	}

	// 2. Generate CSR
	csrReq := &core.CSRRequest{
		CommonName: req.Domain,
		SANs:       append([]string{req.Domain}, req.SANs...),
	}
	csrPEM, err := r.crypto.GenerateCSR(ctx, keyPEM, csrReq)
	if err != nil {
		return "", fmt.Errorf("CSR generation: %w", err)
	}

	// 3. Store key + CSR in cloud secrets (cert will be added after CA signs)
	artifacts := &secrets.CertArtifacts{
		PrivateKey: keyPEM,
		CSR:        csrPEM,
		Metadata: models.CertMetadata{
			CertID:       req.TenantID.String(), // Will be updated with actual cert ID
			Domain:       req.Domain,
			SANs:         req.SANs,
			KeyAlgorithm: string(req.KeyAlgorithm),
			Environment:  req.Environment,
			TenantID:     req.TenantID.String(),
			Satellite:    r.config.Name,
			CreatedAt:    time.Now(),
			Version:      1,
			Status:       "pending_issuance",
		},
	}

	ref, err := r.secrets.StoreCertArtifacts(ctx, req.TenantID.String(), req.TenantID.String(), req.Domain, req.Environment, artifacts)
	if err != nil {
		return "", fmt.Errorf("storing artifacts: %w", err)
	}

	// Return CSR and secret ref to SaaS (SaaS forwards CSR to CA)
	result := map[string]string{
		"csr":        string(csrPEM),
		"secret_ref": ref.SecretID,
	}
	resultJSON, _ := json.Marshal(result)

	r.logger.Info("certificate creation completed",
		zap.String("domain", req.Domain),
		zap.String("secret_ref", ref.SecretID),
	)

	return string(resultJSON), nil
}

// handleDeployCert deploys a certificate to target systems.
func (r *Runtime) handleDeployCert(ctx context.Context, payload string) (string, error) {
	// Parse the deployment request from the command payload
	var deployPayload struct {
		SecretRef core.SecretRef            `json:"secret_ref"`
		Targets   []*deployers.DeployTarget `json:"targets"`
		Target    *deployers.DeployTarget   `json:"target"`
		Parallel  bool                      `json:"parallel"`
	}
	if err := json.Unmarshal([]byte(payload), &deployPayload); err != nil {
		return "", fmt.Errorf("parsing deploy payload: %w", err)
	}

	// Retrieve cert artifacts from secrets manager
	artifacts, err := r.secrets.RetrieveCertArtifacts(ctx, &deployPayload.SecretRef)
	if err != nil {
		return "", fmt.Errorf("retrieving cert artifacts: %w", err)
	}

	r.logger.Info("deploying certificate",
		zap.String("domain", artifacts.Metadata.Domain),
		zap.Int("targets", len(deployPayload.Targets)+1),
	)

	// Build deploy request with cert material
	req := &deployers.DeployRequest{
		CertPEM:      artifacts.Certificate,
		KeyPEM:       artifacts.PrivateKey,
		ChainPEM:     artifacts.Chain,
		FullChainPEM: artifacts.FullChain,
		PFXPEM:       artifacts.PFX,
	}

	var results []*deployers.DeployResult

	// Single target deployment
	if deployPayload.Target != nil && len(deployPayload.Targets) == 0 {
		req.Target = deployPayload.Target
		result, err := r.deployer.Deploy(ctx, req)
		if err != nil {
			return "", fmt.Errorf("deployment failed: %w", err)
		}
		results = append(results, result)
	} else if len(deployPayload.Targets) > 0 {
		// Multi-target (group) deployment
		var deployErr error
		results, deployErr = r.deployer.DeployGroup(ctx, req, deployPayload.Targets, deployPayload.Parallel)
		if deployErr != nil {
			return "", fmt.Errorf("group deployment failed: %w", deployErr)
		}
	}

	// Build summary
	succeeded := 0
	failed := 0
	for _, res := range results {
		if res.Success {
			succeeded++
		} else {
			failed++
		}
	}

	summary := map[string]interface{}{
		"status":    "completed",
		"total":     len(results),
		"succeeded": succeeded,
		"failed":    failed,
		"results":   results,
	}
	summaryJSON, _ := json.Marshal(summary)

	r.logger.Info("deployment completed",
		zap.Int("succeeded", succeeded),
		zap.Int("failed", failed),
	)

	return string(summaryJSON), nil
}

// handleDiscover runs network discovery scan.
func (r *Runtime) handleDiscover(ctx context.Context, payload string) (string, error) {
	var discoverReq struct {
		CIDRs      []string `json:"cidrs"`
		Domains    []string `json:"domains"`
		Hosts      []string `json:"hosts"`
		Ports      []int    `json:"ports"`
		ScanACM    bool     `json:"scan_acm"`
		ACMRegions []string `json:"acm_regions"`
		// DNS options, applied to Domains.
		DNSServer  string `json:"dns_server"`
		Enumerate  bool   `json:"enumerate"`
		FollowSANs bool   `json:"follow_sans"`
	}
	if err := json.Unmarshal([]byte(payload), &discoverReq); err != nil {
		return "", fmt.Errorf("parsing discover payload: %w", err)
	}

	r.logger.Info("starting discovery scan",
		zap.Int("cidrs", len(discoverReq.CIDRs)),
		zap.Int("domains", len(discoverReq.Domains)),
		zap.Int("hosts", len(discoverReq.Hosts)),
		zap.Bool("scan_acm", discoverReq.ScanACM),
	)

	var allCerts []*scanners.DiscoveredCert

	// Scan CIDR ranges
	for _, cidr := range discoverReq.CIDRs {
		certs, err := r.scanner.ScanNetwork(ctx, cidr)
		if err != nil {
			r.logger.Warn("CIDR scan failed", zap.String("cidr", cidr), zap.Error(err))
			continue
		}
		allCerts = append(allCerts, certs...)
	}

	// Scan individual hosts (domains resolve to IPs)
	for _, host := range discoverReq.Hosts {
		certs, err := r.scanner.ScanHost(ctx, host)
		if err != nil {
			r.logger.Warn("host scan failed", zap.String("host", host), zap.Error(err))
			continue
		}
		allCerts = append(allCerts, certs...)
	}

	// Scan domains by name. This goes through the DNS scanner rather than
	// ScanHost because the handshake has to carry SNI: on a shared address a
	// bare probe returns only the default certificate, so every other
	// certificate on that host would be missed. Enumeration and record walking
	// are opt-in via the command payload.
	if len(discoverReq.Domains) > 0 && r.dnsScanner != nil {
		opts := scanners.DNSScanOptions{
			Enumerate:  discoverReq.Enumerate,
			FollowSANs: discoverReq.FollowSANs,
			Ports:      discoverReq.Ports,
		}
		dnsScanner := r.dnsScanner.WithDNSServer(discoverReq.DNSServer)
		for _, domain := range discoverReq.Domains {
			result, err := dnsScanner.ScanDomain(ctx, domain, opts)
			if err != nil {
				r.logger.Warn("domain scan failed", zap.String("domain", domain), zap.Error(err))
				continue
			}
			allCerts = append(allCerts, result.Certs...)
		}
	} else {
		for _, domain := range discoverReq.Domains {
			certs, err := r.scanner.ScanHost(ctx, domain)
			if err != nil {
				r.logger.Warn("domain scan failed", zap.String("domain", domain), zap.Error(err))
				continue
			}
			allCerts = append(allCerts, certs...)
		}
	}

	// Deduplicate by fingerprint
	seen := make(map[string]bool)
	var unique []*scanners.DiscoveredCert
	for _, cert := range allCerts {
		if !seen[cert.Fingerprint] {
			seen[cert.Fingerprint] = true
			unique = append(unique, cert)
		}
	}

	// Categorize results
	var expiring, expired, selfSigned, weakKey int
	for _, cert := range unique {
		if cert.DaysLeft <= 30 && cert.DaysLeft > 0 {
			expiring++
		}
		if cert.DaysLeft <= 0 {
			expired++
		}
		if cert.SelfSigned {
			selfSigned++
		}
		if cert.KeySize > 0 && cert.KeySize < 2048 {
			weakKey++
		}
	}

	result := map[string]interface{}{
		"status":       "scan_complete",
		"certs_found":  len(unique),
		"expiring":     expiring,
		"expired":      expired,
		"self_signed":  selfSigned,
		"weak_key":     weakKey,
		"certificates": unique,
		"scanned_at":   time.Now(),
	}
	resultJSON, _ := json.Marshal(result)

	r.logger.Info("discovery scan complete",
		zap.Int("certs_found", len(unique)),
		zap.Int("expiring", expiring),
		zap.Int("expired", expired),
	)

	return string(resultJSON), nil
}

// handleRenewCert generates new key and CSR for certificate renewal.
func (r *Runtime) handleRenewCert(ctx context.Context, payload string) (string, error) {
	var renewReq struct {
		CertID       string              `json:"cert_id"`
		Domain       string              `json:"domain"`
		SANs         []string            `json:"sans"`
		KeyAlgorithm models.KeyAlgorithm `json:"key_algorithm"`
		Environment  string              `json:"environment"`
		TenantID     string              `json:"tenant_id"`
		SecretRef    *core.SecretRef     `json:"secret_ref"`
	}
	if err := json.Unmarshal([]byte(payload), &renewReq); err != nil {
		return "", fmt.Errorf("parsing renew payload: %w", err)
	}

	r.logger.Info("starting certificate renewal",
		zap.String("cert_id", renewReq.CertID),
		zap.String("domain", renewReq.Domain),
	)

	// 1. Generate new private key
	keyPEM, err := r.crypto.GenerateKey(ctx, renewReq.KeyAlgorithm)
	if err != nil {
		return "", fmt.Errorf("key generation for renewal: %w", err)
	}

	// 2. Generate new CSR
	csrReq := &core.CSRRequest{
		CommonName: renewReq.Domain,
		SANs:       append([]string{renewReq.Domain}, renewReq.SANs...),
	}
	csrPEM, err := r.crypto.GenerateCSR(ctx, keyPEM, csrReq)
	if err != nil {
		return "", fmt.Errorf("CSR generation for renewal: %w", err)
	}

	// 3. Update secret with new key + CSR (cert will be updated after CA signs)
	artifacts := &secrets.CertArtifacts{
		PrivateKey: keyPEM,
		CSR:        csrPEM,
		Metadata: models.CertMetadata{
			CertID:       renewReq.CertID,
			Domain:       renewReq.Domain,
			SANs:         renewReq.SANs,
			KeyAlgorithm: string(renewReq.KeyAlgorithm),
			Environment:  renewReq.Environment,
			TenantID:     renewReq.TenantID,
			Satellite:    r.config.Name,
			CreatedAt:    time.Now(),
			Status:       "renewing",
		},
	}

	// If we have an existing secret ref, update it; otherwise create new
	var ref *core.SecretRef
	if renewReq.SecretRef != nil {
		ref, err = r.secrets.StoreCertArtifacts(ctx, renewReq.CertID, renewReq.TenantID, renewReq.Domain, renewReq.Environment, artifacts)
	} else {
		ref, err = r.secrets.StoreCertArtifacts(ctx, renewReq.CertID, renewReq.TenantID, renewReq.Domain, renewReq.Environment, artifacts)
	}
	if err != nil {
		return "", fmt.Errorf("storing renewal artifacts: %w", err)
	}

	result := map[string]string{
		"status":     "renewal_initiated",
		"cert_id":    renewReq.CertID,
		"csr":        string(csrPEM),
		"secret_ref": ref.SecretID,
	}
	resultJSON, _ := json.Marshal(result)

	r.logger.Info("renewal CSR generated",
		zap.String("cert_id", renewReq.CertID),
		zap.String("domain", renewReq.Domain),
		zap.String("secret_ref", ref.SecretID),
	)

	return string(resultJSON), nil
}

func (r *Runtime) buildHeartbeat() *models.SatelliteHeartbeat {
	return &models.SatelliteHeartbeat{
		Status:     "online",
		Version:    "1.0.0",
		AgentCount: r.agentCount,
		CertCount:  r.certCount,
		Timestamp:  time.Now(),
	}
}
