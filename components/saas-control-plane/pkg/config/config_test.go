package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerConfigValidate(t *testing.T) {
	valid := ServerConfig{
		Env: "production",
		Database: DatabaseConfig{
			Host: "db.internal", Port: 5432, User: "clm", DBName: "clm",
			Password: strongTestValue(24), SSLMode: "verify-full",
		},
		JWT: JWTConfig{
			Secret: strongTestValue(32), Issuer: "clm", Audience: "clm-api",
			ExternalIssuer: "https://id.example.test/realms/clm", ExternalAudience: "clm-api",
			AllowedAlgorithms: []string{"HS256"},
		},
		CloudAccounts: CloudAccountsConfig{MasterKey: strongTestValue(32)},
		TLS:           ServerTLSConfig{Enabled: true, CertPath: "/tls/server.crt", KeyPath: "/tls/server.key", ClientAuth: "required", ClientCAPath: "/tls/gateway-ca.crt"},
		Gateway:       GatewayTransportConfig{LeaseDuration: 2 * time.Minute, MaxPullCommands: 10, MaxResultBytes: 262144, RetryBase: 5 * time.Second},
	}

	tests := []struct {
		name    string
		mutate  func(*ServerConfig)
		wantErr string
	}{
		{name: "valid production"},
		{name: "development remains permissive", mutate: func(c *ServerConfig) {
			c.Env = "development"
			c.Database.Password = ""
			c.Database.SSLMode = "disable"
			c.JWT.Secret = ""
		}},
		{name: "missing external issuer with JWKS", mutate: func(c *ServerConfig) {
			c.JWT.AllowedAlgorithms = []string{"RS256"}
			c.JWT.JWKSURL = "https://id.example.test/realms/clm/protocol/openid-connect/certs"
			c.JWT.ExternalIssuer = ""
		}, wantErr: "external JWT issuer and audience"},
		{name: "missing external audience with JWKS", mutate: func(c *ServerConfig) {
			c.JWT.AllowedAlgorithms = []string{"RS256"}
			c.JWT.JWKSURL = "https://id.example.test/realms/clm/protocol/openid-connect/certs"
			c.JWT.ExternalAudience = ""
		}, wantErr: "external JWT issuer and audience"},
		{name: "insecure external issuer with JWKS", mutate: func(c *ServerConfig) {
			c.JWT.JWKSURL = "https://id.example.test/realms/clm/protocol/openid-connect/certs"
			c.JWT.ExternalIssuer = "http://id.example.test/realms/clm"
		}, wantErr: "external JWT issuer"},
		{name: "insecure JWKS URL", mutate: func(c *ServerConfig) {
			c.JWT.JWKSURL = "http://id.example.test/realms/clm/protocol/openid-connect/certs"
		}, wantErr: "JWT JWKS URL"},
		{name: "weak database password", mutate: func(c *ServerConfig) { c.Database.Password = "short" }, wantErr: "database password"},
		{name: "unencrypted database", mutate: func(c *ServerConfig) { c.Database.SSLMode = "disable" }, wantErr: "sslmode"},
		{name: "weak JWT secret", mutate: func(c *ServerConfig) { c.JWT.Secret = "change-me-in-production-change-me" }, wantErr: "JWT secret"},
		{name: "missing cloud account master key", mutate: func(c *ServerConfig) { c.CloudAccounts.MasterKey = "" }, wantErr: "cloud account master key"},
		{name: "wrong-size cloud account master key", mutate: func(c *ServerConfig) { c.CloudAccounts.MasterKey = strongTestValue(31) }, wantErr: "cloud account master key"},
		{name: "low-diversity JWT secret", mutate: func(c *ServerConfig) { c.JWT.Secret = strings.Repeat("A", 40) }, wantErr: "diversity"},
		{name: "TLS disabled", mutate: func(c *ServerConfig) { c.TLS.Enabled = false }, wantErr: "control-plane TLS"},
		{name: "mTLS optional in production", mutate: func(c *ServerConfig) { c.TLS.ClientAuth = "optional" }, wantErr: "gateway mTLS"},
		{name: "legacy gateway transport", mutate: func(c *ServerConfig) { c.Gateway.AllowInsecureLegacy = true }, wantErr: "legacy unauthenticated"},
		{name: "excessive pull batch", mutate: func(c *ServerConfig) { c.Gateway.MaxPullCommands = 101 }, wantErr: "safe positive bounds"},
		{name: "excessive result size", mutate: func(c *ServerConfig) { c.Gateway.MaxResultBytes = 5 << 20 }, wantErr: "safe positive bounds"},
		{name: "missing retry base", mutate: func(c *ServerConfig) { c.Gateway.RetryBase = 0 }, wantErr: "safe positive bounds"},
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

func TestLoadServerConfigReferences(t *testing.T) {
	secretDir := t.TempDir()
	databaseSecretPath := filepath.Join(secretDir, "database-password")
	if err := os.WriteFile(databaseSecretPath, []byte(strongTestValue(24)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cloudKeyPath := filepath.Join(secretDir, "cloud-account-key")
	if err := os.WriteFile(cloudKeyPath, []byte(strongTestValue(32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_TEST_JWT_SECRET", strongTestValue(32))

	tests := []struct {
		name    string
		jwtRef  string
		wantErr string
	}{
		{name: "environment and secret files resolve", jwtRef: "${CONFIG_TEST_JWT_SECRET}"},
		{name: "unresolved production reference fails", jwtRef: "${CONFIG_TEST_MISSING_SECRET}", wantErr: "CONFIG_TEST_MISSING_SECRET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "server.yaml")
			contents := fmt.Sprintf(`env: production
database:
  host: db.internal
  port: 5432
  user: clm
  password: %q
  dbname: clm
  sslmode: verify-full
jwt:
  secret: %q
cloud_accounts:
  master_key: %q
tls:
  enabled: true
  cert_path: /tls/server.crt
  key_path: /tls/server.key
  client_auth: required
  client_ca_path: /tls/gateway-ca.crt
gateway:
  lease_duration: 2m
  max_pull_commands: 10
  max_result_bytes: 262144
  retry_base: 5s
`, "file://"+databaseSecretPath, tt.jwtRef, "file://"+cloudKeyPath)
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}

			cfg, err := LoadServerConfig(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadServerConfig() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadServerConfig() error = %v", err)
			}
			if cfg.Database.Password != strongTestValue(24) {
				t.Fatal("database secret file was not resolved")
			}
			if cfg.CloudAccounts.MasterKey != strongTestValue(32) {
				t.Fatal("cloud-account master-key file was not resolved")
			}
		})
	}
}

func TestLoadServerConfigRejectsUnresolvedEnvironmentMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yaml")
	if err := os.WriteFile(path, []byte("env: ${CONFIG_TEST_MISSING_ENV}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadServerConfig(path)
	if err == nil || !strings.Contains(err.Error(), "config environment reference") {
		t.Fatalf("LoadServerConfig() error = %v, want unresolved environment mode", err)
	}
}

func strongTestValue(length int) string {
	const alphabet = "aB3dE5fG7hJ9kL2mN4pQ6rS8tV0xYz"
	return strings.Repeat(alphabet, length/len(alphabet)+1)[:length]
}

func TestServerConfigDevelopmentExternalPolicyFallback(t *testing.T) {
	cfg := ServerConfig{
		Env: "test",
		JWT: JWTConfig{
			Issuer: "legacy-issuer", Audience: "legacy-audience",
			JWKSURL: "http://keycloak:8180/realms/clm/protocol/openid-connect/certs",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if cfg.JWT.ExternalIssuer != cfg.JWT.Issuer || cfg.JWT.ExternalAudience != cfg.JWT.Audience {
		t.Fatalf("external fallback = %q/%q, want %q/%q", cfg.JWT.ExternalIssuer, cfg.JWT.ExternalAudience, cfg.JWT.Issuer, cfg.JWT.Audience)
	}
}
