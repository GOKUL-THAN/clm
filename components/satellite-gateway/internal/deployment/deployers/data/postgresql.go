package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq"
	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"github.com/m2p/clm/internal/satellite/securefs"
	"go.uber.org/zap"
)

// PostgreSQLDeployer handles certificate deployment to PostgreSQL servers.
type PostgreSQLDeployer struct {
	logger *zap.Logger
	files  *securefs.Policy
}

// NewPostgreSQLDeployer creates a new PostgreSQL deployer.
func NewPostgreSQLDeployer(logger *zap.Logger, files *securefs.Policy) *PostgreSQLDeployer {
	return &PostgreSQLDeployer{logger: logger, files: files}
}

func (d *PostgreSQLDeployer) Name() string     { return "postgresql" }
func (d *PostgreSQLDeployer) Category() string { return "data" }

// Deploy writes cert/key to PostgreSQL's data directory and reloads config.
func (d *PostgreSQLDeployer) Deploy(ctx context.Context, req *deployers.DeployRequest) (*deployers.DeployResult, error) {
	start := time.Now()
	target := req.Target

	certDir := target.CertPath
	if certDir == "" {
		certDir = "/var/lib/postgresql/certs"
	}
	if d.files == nil {
		return nil, fmt.Errorf("secure file storage policy is not configured")
	}
	if _, _, err := d.files.Resolve(certDir, target.TenantID); err != nil {
		return nil, err
	}

	// Ensure directory exists
	if err := d.files.MkdirAll(certDir, target.TenantID, 0750); err != nil {
		return &deployers.DeployResult{Success: false, Error: err.Error(), Duration: time.Since(start)}, err
	}

	certPath := filepath.Join(certDir, "server.crt")
	keyPath := filepath.Join(certDir, "server.key")

	previousFingerprint := ""
	if previousPEM, err := d.files.ReadFile(certPath, target.TenantID, securefs.Certificate); err == nil {
		if !target.AllowReplace {
			return nil, fmt.Errorf("certificate destination exists; replacement requires authorized approval")
		}
		previousFingerprint, _ = deployers.LeafSHA256(previousPEM)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading previous certificate: %w", err)
	}
	if _, err := d.files.ReadFile(keyPath, target.TenantID, securefs.PrivateKey); err == nil {
		if !target.AllowReplace {
			return nil, fmt.Errorf("private-key destination exists; replacement requires authorized approval")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading previous private key: %w", err)
	}
	if err := backupFile(d.files, certPath, target.TenantID, securefs.Certificate, target.AllowReplace); err != nil {
		return nil, fmt.Errorf("backing up certificate: %w", err)
	}
	if err := backupFile(d.files, keyPath, target.TenantID, securefs.PrivateKey, target.AllowReplace); err != nil {
		return nil, fmt.Errorf("backing up private key: %w", err)
	}

	// Write fullchain
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

	// Reload PostgreSQL configuration
	reloadMethod := target.ReloadMethod
	if reloadMethod == "" {
		reloadMethod = "pg_reload_conf"
	}

	if err := d.reloadPostgres(ctx, target, reloadMethod); err != nil {
		d.logger.Error("PostgreSQL reload failed", zap.Error(err))
		return failedFileDeployment(ctx, d, target, start, fmt.Errorf("reload failed: %w", err))
	}

	d.logger.Info("certificate deployed to PostgreSQL",
		zap.String("host", target.Host),
		zap.String("cert_dir", certDir),
	)

	return &deployers.DeployResult{
		Success:             true,
		Target:              target,
		PreviousFingerprint: previousFingerprint,
		Message:             fmt.Sprintf("deployed to PostgreSQL at %s:%d", target.Host, target.Port),
		Duration:            time.Since(start),
	}, nil
}

// Verify checks the local PostgreSQL certificate file. The executor performs
// the PostgreSQL SSLRequest and certificate validation independently.
func (d *PostgreSQLDeployer) Verify(ctx context.Context, req *deployers.DeployRequest, applied *deployers.DeployResult) (*deployers.VerificationEvidence, error) {
	target := req.Target
	certDir := target.CertPath
	if certDir == "" {
		certDir = "/var/lib/postgresql/certs"
	}
	certPath := filepath.Join(certDir, "server.crt")
	evidence := &deployers.VerificationEvidence{
		Kind: "adapter", Method: "postgresql-file", Adapter: d.Name(), Resource: certPath,
		CheckedAt: time.Now().UTC(),
	}
	if d.files == nil {
		evidence.Message = "secure file storage policy is not configured"
		return evidence, fmt.Errorf("%s", evidence.Message)
	}
	data, err := d.files.ReadFile(certPath, target.TenantID, securefs.Certificate)
	if err != nil {
		evidence.Message = err.Error()
		return evidence, err
	}
	if len(data) == 0 {
		err := fmt.Errorf("PostgreSQL certificate file is empty")
		evidence.Message = err.Error()
		return evidence, err
	}
	evidence.Success = true
	evidence.Message = "PostgreSQL certificate file exists; SSLRequest serving state checked separately"
	return evidence, nil
}

// Rollback restores the previous certificate files and reloads PostgreSQL. A
// missing backup or failed reload is returned to the executor as rollback
// evidence instead of being silently reported as success.
func (d *PostgreSQLDeployer) Rollback(ctx context.Context, target *deployers.DeployTarget) error {
	certDir := target.CertPath
	if certDir == "" {
		certDir = "/var/lib/postgresql/certs"
	}
	if d.files == nil {
		return fmt.Errorf("secure file storage policy is not configured")
	}

	certPath := filepath.Join(certDir, "server.crt")
	keyPath := filepath.Join(certDir, "server.key")

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
	if err := d.reloadPostgres(ctx, target, "pg_reload_conf"); err != nil {
		if rollbackErr != nil {
			rollbackErr = fmt.Errorf("%v; reload PostgreSQL after rollback: %w", rollbackErr, err)
		} else {
			rollbackErr = fmt.Errorf("reload PostgreSQL after rollback: %w", err)
		}
	}
	return rollbackErr
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

func failedFileDeployment(ctx context.Context, d *PostgreSQLDeployer, target *deployers.DeployTarget, started time.Time, cause error) (*deployers.DeployResult, error) {
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

func (d *PostgreSQLDeployer) reloadPostgres(ctx context.Context, target *deployers.DeployTarget, method string) error {
	port := target.Port
	if port == 0 {
		port = 5432
	}

	dsn := fmt.Sprintf("host=%s port=%d sslmode=disable user=postgres dbname=postgres", target.Host, port)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, "SELECT pg_reload_conf()")
	return err
}
