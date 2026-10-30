package deployer

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// DeployRequest represents a certificate deployment specification.
type DeployRequest struct {
	Target       DeployTarget `json:"target"`
	CertPEM      string       `json:"cert_pem"`
	KeyPEM       string       `json:"key_pem"`
	CertPath     string       `json:"cert_path"`
	KeyPath      string       `json:"key_path"`
	ReloadCmd    string       `json:"reload_cmd"`
	VerifyPort   int          `json:"verify_port"`
	VerifyHost   string       `json:"verify_host"`
	AutoRollback bool         `json:"auto_rollback"`
}

// DeployTarget describes the destination service or file location.
type DeployTarget struct {
	Type     string `json:"type"`     // e.g. "linux", "postgresql", "nginx"
	Host     string `json:"host"`     // local host or IP
	Service  string `json:"service"`  // service name for reload
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}

// DeployResult contains execution metrics and verification evidence.
type DeployResult struct {
	TargetType         string    `json:"target_type"`
	Success            bool      `json:"success"`
	Verified           bool      `json:"verified"`
	ExpectedFingerprint string   `json:"expected_fingerprint"`
	ObservedFingerprint string   `json:"observed_fingerprint,omitempty"`
	RollbackAttempted  bool      `json:"rollback_attempted,omitempty"`
	RollbackSuccess    bool      `json:"rollback_success,omitempty"`
	Stage              string    `json:"stage"` // "staged", "applied", "verified", "rolled_back", "failed"
	Message            string    `json:"message"`
	ExecutedAt         time.Time `json:"executed_at"`
}

// Deployer executes local endpoint deployments and verifies live listeners.
type Deployer struct {
	logger *zap.Logger
}

// NewDeployer creates a new local deployer instance.
func NewDeployer(logger *zap.Logger) *Deployer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Deployer{logger: logger}
}

// Deploy deploys certificate and key files, executes service reloads, and verifies listeners.
func (d *Deployer) Deploy(ctx context.Context, req *DeployRequest) (*DeployResult, error) {
	if req == nil {
		return nil, fmt.Errorf("deploy request is nil")
	}

	certPath := req.CertPath
	if certPath == "" {
		certPath = req.Target.CertFile
	}
	keyPath := req.KeyPath
	if keyPath == "" {
		keyPath = req.Target.KeyFile
	}

	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("both cert_path and key_path must be specified")
	}
	if req.CertPEM == "" || req.KeyPEM == "" {
		return nil, fmt.Errorf("both cert_pem and key_pem must be provided")
	}

	// Calculate expected fingerprint from CertPEM
	expectedFP := leafSHA256(req.CertPEM)

	result := &DeployResult{
		TargetType:          req.Target.Type,
		ExpectedFingerprint: expectedFP,
		ExecutedAt:          time.Now(),
		Stage:               "staged",
	}

	d.logger.Info("starting certificate deployment",
		zap.String("cert_path", certPath),
		zap.String("key_path", keyPath),
		zap.String("fingerprint", expectedFP),
	)

	// Step 1: Backup existing certificate and key for rollback
	backupCert := certPath + ".clm_bak"
	backupKey := keyPath + ".clm_bak"
	hasBackup := false

	if fileExists(certPath) && fileExists(keyPath) {
		if err := copyFile(certPath, backupCert); err == nil {
			if err := copyFile(keyPath, backupKey); err == nil {
				hasBackup = true
			}
		}
	}

	// Step 2: Write certificate (0644) and private key (0600) atomically
	if err := writeAtomic(certPath, []byte(req.CertPEM), 0644); err != nil {
		result.Stage = "failed"
		result.Message = fmt.Sprintf("failed to write certificate: %v", err)
		return result, err
	}

	if err := writeAtomic(keyPath, []byte(req.KeyPEM), 0600); err != nil {
		result.Stage = "failed"
		result.Message = fmt.Sprintf("failed to write private key: %v", err)
		d.rollbackFiles(hasBackup, backupCert, certPath, backupKey, keyPath)
		return result, err
	}

	result.Stage = "applied"

	// Step 3: Execute service reload if specified
	reloadCmd := req.ReloadCmd
	if reloadCmd == "" && req.Target.Service != "" {
		reloadCmd = fmt.Sprintf("systemctl reload %s", req.Target.Service)
	}

	if reloadCmd != "" {
		d.logger.Info("executing service reload", zap.String("command", reloadCmd))
		if err := d.runCommand(ctx, reloadCmd); err != nil {
			d.logger.Error("service reload failed", zap.Error(err))
			if req.AutoRollback && hasBackup {
				d.rollbackFiles(hasBackup, backupCert, certPath, backupKey, keyPath)
				result.RollbackAttempted = true
				result.RollbackSuccess = true
				result.Stage = "rolled_back"
			} else {
				result.Stage = "failed"
			}
			result.Message = fmt.Sprintf("service reload failed: %v", err)
			return result, err
		}
	}

	// Step 4: Verification of serving listener
	verifyPort := req.VerifyPort
	if verifyPort == 0 && req.Target.Type == "postgresql" {
		verifyPort = 5432
	} else if verifyPort == 0 {
		verifyPort = 443
	}

	verifyHost := req.VerifyHost
	if verifyHost == "" {
		verifyHost = req.Target.Host
	}
	if verifyHost == "" {
		verifyHost = "127.0.0.1"
	}

	d.logger.Info("verifying endpoint TLS presentation",
		zap.String("host", verifyHost),
		zap.Int("port", verifyPort),
	)

	observedFP, err := d.verifyEndpoint(ctx, verifyHost, verifyPort)
	if err != nil {
		d.logger.Warn("verification failed", zap.Error(err))
		if req.AutoRollback && hasBackup {
			d.rollbackFiles(hasBackup, backupCert, certPath, backupKey, keyPath)
			if reloadCmd != "" {
				_ = d.runCommand(ctx, reloadCmd)
			}
			result.RollbackAttempted = true
			result.RollbackSuccess = true
			result.Stage = "rolled_back"
		} else {
			result.Stage = "failed"
		}
		result.Message = fmt.Sprintf("verification failed: %v", err)
		return result, fmt.Errorf("verification failed: %w", err)
	}

	result.ObservedFingerprint = observedFP
	if !strings.EqualFold(observedFP, expectedFP) {
		d.logger.Warn("fingerprint mismatch",
			zap.String("expected", expectedFP),
			zap.String("observed", observedFP),
		)
		if req.AutoRollback && hasBackup {
			d.rollbackFiles(hasBackup, backupCert, certPath, backupKey, keyPath)
			if reloadCmd != "" {
				_ = d.runCommand(ctx, reloadCmd)
			}
			result.RollbackAttempted = true
			result.RollbackSuccess = true
			result.Stage = "rolled_back"
		}
		result.Stage = "failed"
		result.Message = fmt.Sprintf("fingerprint mismatch: expected %s, got %s", expectedFP, observedFP)
		return result, fmt.Errorf("fingerprint mismatch")
	}

	// Clean up backups on successful verification
	if hasBackup {
		_ = os.Remove(backupCert)
		_ = os.Remove(backupKey)
	}

	result.Success = true
	result.Verified = true
	result.Stage = "verified"
	result.Message = "certificate deployed and verified successfully"

	d.logger.Info("deployment succeeded and verified",
		zap.String("fingerprint", observedFP),
	)

	return result, nil
}

func (d *Deployer) rollbackFiles(hasBackup bool, bakCert, certPath, bakKey, keyPath string) {
	if !hasBackup {
		return
	}
	d.logger.Warn("initiating rollback to previous certificate/key version")
	_ = copyFile(bakCert, certPath)
	_ = copyFile(bakKey, keyPath)
	_ = os.Remove(bakCert)
	_ = os.Remove(bakKey)
}

func (d *Deployer) runCommand(ctx context.Context, cmdStr string) error {
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

func (d *Deployer) verifyEndpoint(ctx context.Context, host string, port int) (string, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 5 * time.Second}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return "", fmt.Errorf("no peer certificates presented")
	}

	leaf := state.PeerCertificates[0]
	h := sha256.Sum256(leaf.Raw)
	return hex.EncodeToString(h[:]), nil
}

func leafSHA256(certPEM string) string {
	h := sha256.Sum256([]byte(certPEM))
	return hex.EncodeToString(h[:])
}

func writeAtomic(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile := filename + ".tmp"
	if err := os.WriteFile(tmpFile, data, perm); err != nil {
		return err
	}

	return os.Rename(tmpFile, filename)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ParseDeployPayload unmarshals raw JSON payload into DeployRequest.
func ParseDeployPayload(payload string) (*DeployRequest, error) {
	var req DeployRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return nil, fmt.Errorf("invalid deploy payload: %w", err)
	}
	return &req, nil
}
