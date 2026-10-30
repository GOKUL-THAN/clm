package data

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq"
	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"go.uber.org/zap"
)

// PostgreSQLDeployer handles certificate deployment to PostgreSQL servers.
type PostgreSQLDeployer struct {
	logger *zap.Logger
}

// NewPostgreSQLDeployer creates a new PostgreSQL deployer.
func NewPostgreSQLDeployer(logger *zap.Logger) *PostgreSQLDeployer {
	return &PostgreSQLDeployer{logger: logger}
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

	// Ensure directory exists
	if err := os.MkdirAll(certDir, 0750); err != nil {
		return &deployers.DeployResult{Success: false, Error: err.Error(), Duration: time.Since(start)}, err
	}

	certPath := filepath.Join(certDir, "server.crt")
	keyPath := filepath.Join(certDir, "server.key")

	previousFingerprint := ""
	if previousPEM, err := os.ReadFile(certPath); err == nil {
		previousFingerprint, _ = deployers.LeafSHA256(previousPEM)
	}

	// Backup existing
	_ = os.Rename(certPath, certPath+".bak")
	_ = os.Rename(keyPath, keyPath+".bak")

	// Write fullchain
	certData := req.FullChainPEM
	if len(certData) == 0 {
		certData = req.CertPEM
	}
	if err := os.WriteFile(certPath, certData, 0640); err != nil {
		return &deployers.DeployResult{Success: false, Error: err.Error(), Duration: time.Since(start)}, err
	}

	// Write key with strict permissions
	if err := os.WriteFile(keyPath, req.KeyPEM, 0600); err != nil {
		return &deployers.DeployResult{Success: false, Error: err.Error(), Duration: time.Since(start)}, err
	}

	// Reload PostgreSQL configuration
	reloadMethod := target.ReloadMethod
	if reloadMethod == "" {
		reloadMethod = "pg_reload_conf"
	}

	if err := d.reloadPostgres(ctx, target, reloadMethod); err != nil {
		d.logger.Error("PostgreSQL reload failed", zap.Error(err))
		return &deployers.DeployResult{
			Success:  false,
			Target:   target,
			Error:    fmt.Sprintf("reload failed: %v", err),
			Duration: time.Since(start),
		}, err
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
	info, err := os.Stat(certPath)
	if err != nil {
		evidence.Message = err.Error()
		return evidence, err
	}
	if info.Size() == 0 {
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

	certPath := filepath.Join(certDir, "server.crt")
	keyPath := filepath.Join(certDir, "server.key")

	var rollbackErr error
	if err := os.Rename(certPath+".bak", certPath); err != nil {
		rollbackErr = fmt.Errorf("restore certificate backup: %w", err)
	}
	if err := os.Rename(keyPath+".bak", keyPath); err != nil {
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
