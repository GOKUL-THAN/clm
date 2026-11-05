package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSatelliteConfigValidate(t *testing.T) {
	t.Setenv("CLM_SATELLITE_TOKEN", strongTestValue(32))
	approvedRoot := filepath.Join(t.TempDir(), "files")
	valid := SatelliteConfig{
		Env:           "production",
		SaaSEndpoint:  "https://clm.example.test",
		ConnectorMode: "direct",
		TLSCertPath:   filepath.Join(approvedRoot, "satellite.crt"),
		TLSKeyPath:    filepath.Join(approvedRoot, "satellite.key"),
		TLSCAPath:     filepath.Join(approvedRoot, "control-plane-ca.crt"),
		State:         StateConfig{Dir: filepath.Join(approvedRoot, "state")},
		FileStorage:   FileStorageConfig{AllowedRoots: []string{approvedRoot}},
		Commands:      CommandConfig{PollInterval: "5s", Timeout: "15m", LeaseRenewal: "45s", MaxResultBytes: 262144, NetworkRetries: 5},
		Auth: GatewayAuthConfig{
			SaaSLoginURL: "https://clm.example.test/login",
		},
	}

	tests := []struct {
		name    string
		mutate  func(*SatelliteConfig)
		wantErr string
	}{
		{name: "valid production"},
		{name: "test remains permissive", mutate: func(c *SatelliteConfig) {
			c.Env = "test"
			c.SaaSEndpoint = "http://localhost"
			c.TLSCertPath = ""
			c.TLSKeyPath = ""
		}},
		{name: "HTTP SaaS endpoint", mutate: func(c *SatelliteConfig) { c.SaaSEndpoint = "http://clm.example.test" }, wantErr: "HTTPS"},
		{name: "missing TLS key", mutate: func(c *SatelliteConfig) { c.TLSKeyPath = "" }, wantErr: "certificate, key, and server CA"},
		{name: "insecure ACME TLS", mutate: func(c *SatelliteConfig) { c.ACME.InsecureSkipVerify = true }, wantErr: "ACME TLS verification"},
		{name: "weak gateway JWT", mutate: func(c *SatelliteConfig) { c.Auth.JWTSecret = "short" }, wantErr: "gateway JWT secret"},
		{name: "low-diversity gateway JWT", mutate: func(c *SatelliteConfig) { c.Auth.JWTSecret = strings.Repeat("A", 40) }, wantErr: "diversity"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}
			err := cfg.Validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadSatelliteConfigReferences(t *testing.T) {
	t.Setenv("CLM_SATELLITE_TOKEN", strongTestValue(32))
	secretPath := filepath.Join(t.TempDir(), "vault-token")
	if err := os.WriteFile(secretPath, []byte(strongTestValue(24)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		endpoint string
		wantErr  string
	}{
		{name: "environment and secret file resolve", endpoint: "${CONFIG_TEST_SAAS_ENDPOINT}"},
		{name: "unresolved production reference fails", endpoint: "${CONFIG_TEST_MISSING_ENDPOINT}", wantErr: "CONFIG_TEST_MISSING_ENDPOINT"},
	}
	t.Setenv("CONFIG_TEST_SAAS_ENDPOINT", "https://clm.example.test")
	approvedRoot := filepath.Join(t.TempDir(), "files")
	tlsCertPath := filepath.Join(approvedRoot, "satellite.crt")
	tlsKeyPath := filepath.Join(approvedRoot, "satellite.key")
	tlsCAPath := filepath.Join(approvedRoot, "control-plane-ca.crt")
	stateDir := filepath.Join(approvedRoot, "state")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "satellite.yaml")
			contents := fmt.Sprintf(`env: production
saas_endpoint: %q
connector_mode: direct
tls_cert_path: %q
tls_key_path: %q
tls_ca_path: %q
state:
  dir: %q
file_storage:
  allowed_roots:
    - %q
commands:
  poll_interval: 5s
  timeout: 15m
  lease_renewal: 45s
  max_result_bytes: 262144
  network_retries: 5
secrets:
  provider: vault
  vault_token: %q
`, tt.endpoint, tlsCertPath, tlsKeyPath, tlsCAPath, stateDir, approvedRoot, "file://"+secretPath)
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}

			cfg, err := LoadSatelliteConfig(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadSatelliteConfig() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadSatelliteConfig() error = %v", err)
			}
			if cfg.Secrets.VaultToken != strongTestValue(24) {
				t.Fatalf("vault secret file was not resolved")
			}
		})
	}
}

func TestSatelliteFileStorageDefaultsToDeny(t *testing.T) {
	cfg := SatelliteConfig{Env: "test"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "default-deny") {
		t.Fatalf("Validate() error = %v, want default-deny rejection", err)
	}
}

func TestSatelliteFileStorageRequiresStateInsideApprovedRoot(t *testing.T) {
	cfg := SatelliteConfig{
		Env:         "test",
		State:       StateConfig{Dir: filepath.Join(t.TempDir(), "state")},
		FileStorage: FileStorageConfig{AllowedRoots: []string{filepath.Join(t.TempDir(), "approved")}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "state.dir") {
		t.Fatalf("Validate() error = %v, want state root rejection", err)
	}
}

func TestServerConfigValidateProductionDatabase(t *testing.T) {
	base := ServerConfig{
		Env: "production",
		Database: DatabaseConfig{
			Host: "db.internal", Port: 5432, User: "clm", DBName: "clm",
			Password: strongTestValue(24), SSLMode: "require",
		},
		JWT: JWTConfig{Secret: strongTestValue(32)},
	}

	tests := []struct {
		name    string
		sslMode string
		wantErr bool
	}{
		{name: "encrypted", sslMode: "require"},
		{name: "verified", sslMode: "verify-full"},
		{name: "disabled", sslMode: "disable", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			cfg.Database.SSLMode = tt.sslMode
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadSatelliteConfigRejectsUnresolvedEnvironmentMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "satellite.yaml")
	if err := os.WriteFile(path, []byte("env: ${CONFIG_TEST_MISSING_ENV}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSatelliteConfig(path)
	if err == nil || !strings.Contains(err.Error(), "config environment reference") {
		t.Fatalf("LoadSatelliteConfig() error = %v, want unresolved environment mode", err)
	}
}

func strongTestValue(length int) string {
	const alphabet = "aB3dE5fG7hJ9kL2mN4pQ6rS8tV0xYz"
	return strings.Repeat(alphabet, length/len(alphabet)+1)[:length]
}
