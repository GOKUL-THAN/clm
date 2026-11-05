package osdeployer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"github.com/m2p/clm/internal/satellite/securefs"
	"go.uber.org/zap"
)

// LinuxDeployer handles certificate deployment on Linux systems.
type LinuxDeployer struct {
	logger *zap.Logger
	files  *securefs.Policy
}

// NewLinuxDeployer creates a new Linux deployer.
func NewLinuxDeployer(logger *zap.Logger, files *securefs.Policy) *LinuxDeployer {
	return &LinuxDeployer{logger: logger, files: files}
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
	if d.files == nil {
		return nil, fmt.Errorf("secure file storage policy is not configured")
	}
	if _, _, err := d.files.Resolve(certPath, target.TenantID); err != nil {
		return nil, err
	}
	if _, _, err := d.files.Resolve(keyPath, target.TenantID); err != nil {
		return nil, err
	}

	// Ensure directories exist
	if err := d.files.MkdirAll(filepath.Dir(certPath), target.TenantID, 0750); err != nil {
		return &deployers.DeployResult{Success: false, Error: err.Error(), Duration: time.Since(start)}, err
	}
	if err := d.files.MkdirAll(filepath.Dir(keyPath), target.TenantID, 0750); err != nil {
		return &deployers.DeployResult{Success: false, Error: err.Error(), Duration: time.Since(start)}, err
	}

	// Capture public rollback identity before replacing the file. A missing or
	// unparsable old certificate simply means rollback cannot be reverified by
	// fingerprint; it is not a deployment secret.
	previousFingerprint := ""
	if previousPEM, err := d.files.ReadFile(certPath, target.TenantID, securefs.Certificate); err == nil {
		if !target.AllowReplace {
			return nil, fmt.Errorf("certificate destination exists; replacement requires authorized approval")
		}
		previousFingerprint, _ = deployers.LeafSHA256(previousPEM)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading previous certificate: %w", err)
	}
	if _, err := d.files.ReadFile(keyPath, target.TenantID, securefs.PrivateKey); err == nil && !target.AllowReplace {
		return nil, fmt.Errorf("private-key destination exists; replacement requires authorized approval")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading previous private key: %w", err)
	}

	if err := backupFile(d.files, certPath, target.TenantID, securefs.Certificate, target.AllowReplace); err != nil {
		return nil, fmt.Errorf("backing up certificate: %w", err)
	}
	if err := backupFile(d.files, keyPath, target.TenantID, securefs.PrivateKey, target.AllowReplace); err != nil {
		return nil, fmt.Errorf("backing up private key: %w", err)
	}

	// Write fullchain (cert + intermediates) for the service
	certData := req.FullChainPEM
	if len(certData) == 0 {
		certData = req.CertPEM
	}

	if err := d.files.WriteFile(certPath, target.TenantID, securefs.Certificate, certData, target.AllowReplace); err != nil {
		return failedFileDeployment(ctx, d, target, start, err)
	}

	if err := d.files.WriteFile(keyPath, target.TenantID, securefs.PrivateKey, req.KeyPEM, target.AllowReplace); err != nil {
		return failedFileDeployment(ctx, d, target, start, fmt.Errorf("writing private key: %w", err))
	}

	// Reload service if specified.
	if target.Service != "" {
		if err := reloadSystemdService(ctx, target.Service, target.ReloadCmd); err != nil {
			d.logger.Error("service reload failed", zap.String("service", target.Service), zap.Error(err))
			return failedFileDeployment(ctx, d, target, start, fmt.Errorf("service reload failed: %w", err))
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

	data, err := d.files.ReadFile(certPath, target.TenantID, securefs.Certificate)
	if err != nil {
		evidence.Message = err.Error()
		return evidence, err
	}
	if len(data) == 0 {
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
	if d.files == nil {
		return fmt.Errorf("secure file storage policy is not configured")
	}

	var rollbackErr error
	if err := restoreFile(d.files, certPath, target.TenantID, securefs.Certificate); err != nil {
		rollbackErr = fmt.Errorf("restore certificate backup: %w", err)
	}
	if err := restoreFile(d.files, keyPath, target.TenantID, securefs.PrivateKey); err != nil {
		if rollbackErr != nil {
			rollbackErr = fmt.Errorf("%v; restore key backup: %w", rollbackErr, err)
		} else {
			rollbackErr = fmt.Errorf("restore key backup: %w", err)
		}
	}

	if target.Service != "" {
		if err := reloadSystemdService(ctx, target.Service, target.ReloadCmd); err != nil {
			if rollbackErr != nil {
				rollbackErr = fmt.Errorf("%v; reload service after rollback: %w", rollbackErr, err)
			} else {
				rollbackErr = fmt.Errorf("reload service after rollback: %w", err)
			}
		}
	}

	return rollbackErr
}

func reloadSystemdService(ctx context.Context, service, reloadCmd string) error {
	action, service, err := systemctlCommand(service, reloadCmd)
	if err != nil {
		return err
	}

	// #nosec G204 -- executable and actions are fixed; service names and overrides are validated above, without a shell.
	output, err := exec.CommandContext(ctx, "systemctl", action, service).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s %s failed: %w: %s", action, service, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func systemctlCommand(service, reloadCmd string) (string, string, error) {
	service = strings.TrimSpace(service)
	if !validSystemdUnitName(service) {
		return "", "", fmt.Errorf("invalid systemd service name %q", service)
	}

	action := "reload"
	if strings.TrimSpace(reloadCmd) != "" {
		parts := strings.Fields(reloadCmd)
		if len(parts) != 3 || parts[0] != "systemctl" || parts[2] != service {
			return "", "", fmt.Errorf("reload_cmd must be 'systemctl <action> %s'", service)
		}
		action = parts[1]
	}
	switch action {
	case "reload", "restart", "reload-or-restart", "try-reload-or-restart":
	default:
		return "", "", fmt.Errorf("unsupported systemctl action %q", action)
	}
	return action, service, nil
}

func validSystemdUnitName(service string) bool {
	if service == "" || service[0] == '-' {
		return false
	}
	for _, r := range service {
		if (r < 'a' || r > 'z') &&
			(r < 'A' || r > 'Z') &&
			(r < '0' || r > '9') &&
			r != '_' && r != '.' && r != '@' && r != ':' && r != '-' {
			return false
		}
	}
	return true
}

func backupFile(files *securefs.Policy, path, tenant string, material securefs.Material, replace bool) error {
	data, err := files.ReadFile(path, tenant, material)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = files.Remove(path+".bak", tenant, material)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
		}
		return err
	}
	return files.WriteFile(path+".bak", tenant, material, data, replace)
}

func restoreFile(files *securefs.Policy, path, tenant string, material securefs.Material) error {
	data, err := files.ReadFile(path+".bak", tenant, material)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = files.Remove(path, tenant, material)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		return err
	}
	if err := files.WriteFile(path, tenant, material, data, true); err != nil {
		return err
	}
	return files.Remove(path+".bak", tenant, material)
}

func failedFileDeployment(ctx context.Context, d *LinuxDeployer, target *deployers.DeployTarget, started time.Time, cause error) (*deployers.DeployResult, error) {
	result := &deployers.DeployResult{
		Success: false, Target: target, Error: cause.Error(),
		RollbackAttempted: true, Duration: time.Since(started),
	}
	if err := d.Rollback(ctx, target); err != nil {
		result.RollbackError = err.Error()
		return result, fmt.Errorf("%w; rollback failed: %v", cause, err)
	}
	result.RollbackSuccess = true
	return result, cause
}
