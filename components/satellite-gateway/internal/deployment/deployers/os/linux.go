package osdeployer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"go.uber.org/zap"
)

// LinuxDeployer handles certificate deployment on Linux systems.
type LinuxDeployer struct {
	logger *zap.Logger
}

// NewLinuxDeployer creates a new Linux deployer.
func NewLinuxDeployer(logger *zap.Logger) *LinuxDeployer {
	return &LinuxDeployer{logger: logger}
}

func (d *LinuxDeployer) Name() string     { return "linux" }
func (d *LinuxDeployer) Category() string { return "os" }

// Deploy writes cert/key files and reloads the target service.
func (d *LinuxDeployer) Deploy(ctx context.Context, req *deployers.DeployRequest) (*deployers.DeployResult, error) {
	start := time.Now()
	target := req.Target

	// Default paths if not specified
	certPath := target.CertPath
	keyPath := target.KeyPath
	if certPath == "" {
		certPath = "/etc/ssl/certs/clm-cert.pem"
	}
	if keyPath == "" {
		keyPath = "/etc/ssl/private/clm-key.pem"
	}

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return &deployers.DeployResult{Success: false, Error: err.Error(), Duration: time.Since(start)}, err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return &deployers.DeployResult{Success: false, Error: err.Error(), Duration: time.Since(start)}, err
	}

	// Capture public rollback identity before replacing the file. A missing or
	// unparsable old certificate simply means rollback cannot be reverified by
	// fingerprint; it is not a deployment secret.
	previousFingerprint := ""
	if previousPEM, err := os.ReadFile(certPath); err == nil {
		previousFingerprint, _ = deployers.LeafSHA256(previousPEM)
	}

	// Backup existing certs
	_ = backupFile(certPath)
	_ = backupFile(keyPath)

	// Write fullchain (cert + intermediates) for the service
	certData := req.FullChainPEM
	if len(certData) == 0 {
		certData = req.CertPEM
	}

	if err := os.WriteFile(certPath, certData, 0644); err != nil {
		return &deployers.DeployResult{Success: false, Error: err.Error(), Duration: time.Since(start)}, err
	}

	// Write private key with restricted permissions
	if err := os.WriteFile(keyPath, req.KeyPEM, 0600); err != nil {
		return &deployers.DeployResult{Success: false, Error: err.Error(), Duration: time.Since(start)}, err
	}

	// Reload service if specified
	if target.Service != "" {
		reloadCmd := target.ReloadCmd
		if reloadCmd == "" {
			reloadCmd = fmt.Sprintf("systemctl reload %s", target.Service)
		}

		if err := exec.CommandContext(ctx, "sh", "-c", reloadCmd).Run(); err != nil {
			d.logger.Error("service reload failed", zap.String("service", target.Service), zap.Error(err))
			return &deployers.DeployResult{
				Success:  false,
				Target:   target,
				Error:    fmt.Sprintf("service reload failed: %v", err),
				Duration: time.Since(start),
			}, err
		}
	}

	d.logger.Info("certificate deployed to Linux",
		zap.String("cert_path", certPath),
		zap.String("service", target.Service),
	)

	return &deployers.DeployResult{
		Success:             true,
		Target:              target,
		PreviousFingerprint: previousFingerprint,
		Message:             fmt.Sprintf("deployed to %s, service %s reloaded", certPath, target.Service),
		Duration:            time.Since(start),
	}, nil
}

// Verify checks the local adapter state. Endpoint verification is performed
// independently by the executor and is the evidence that controls commit.
func (d *LinuxDeployer) Verify(ctx context.Context, req *deployers.DeployRequest, applied *deployers.DeployResult) (*deployers.VerificationEvidence, error) {
	target := req.Target
	certPath := target.CertPath
	if certPath == "" {
		certPath = "/etc/ssl/certs/clm-cert.pem"
	}
	evidence := &deployers.VerificationEvidence{
		Kind: "adapter", Method: "linux-file", Adapter: d.Name(), Resource: certPath,
		CheckedAt: time.Now().UTC(),
	}

	info, err := os.Stat(certPath)
	if err != nil {
		evidence.Message = err.Error()
		return evidence, err
	}
	if info.Size() == 0 {
		err := fmt.Errorf("cert file is empty")
		evidence.Message = err.Error()
		return evidence, err
	}
	evidence.Success = true
	evidence.Message = "certificate file exists and is non-empty; serving state checked separately"
	return evidence, nil
}

// Rollback restores the backed-up certificate and reports every recovery
// failure. Returning nil after a missing backup or failed reload would claim a
// safe recovery while the target could still be serving the unverified cert.
func (d *LinuxDeployer) Rollback(ctx context.Context, target *deployers.DeployTarget) error {
	certPath := target.CertPath
	keyPath := target.KeyPath
	if certPath == "" {
		certPath = "/etc/ssl/certs/clm-cert.pem"
	}
	if keyPath == "" {
		keyPath = "/etc/ssl/private/clm-key.pem"
	}

	var rollbackErr error
	if err := restoreFile(certPath); err != nil {
		rollbackErr = fmt.Errorf("restore certificate backup: %w", err)
	}
	if err := restoreFile(keyPath); err != nil {
		if rollbackErr != nil {
			rollbackErr = fmt.Errorf("%v; restore key backup: %w", rollbackErr, err)
		} else {
			rollbackErr = fmt.Errorf("restore key backup: %w", err)
		}
	}

	if target.Service != "" {
		reloadCmd := target.ReloadCmd
		if reloadCmd == "" {
			reloadCmd = fmt.Sprintf("systemctl reload %s", target.Service)
		}
		if err := exec.CommandContext(ctx, "sh", "-c", reloadCmd).Run(); err != nil {
			if rollbackErr != nil {
				rollbackErr = fmt.Errorf("%v; reload service after rollback: %w", rollbackErr, err)
			} else {
				rollbackErr = fmt.Errorf("reload service after rollback: %w", err)
			}
		}
	}

	return rollbackErr
}

func backupFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak", data, 0600)
}

func restoreFile(path string) error {
	data, err := os.ReadFile(path + ".bak")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
