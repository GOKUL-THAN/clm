package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// ServerConfig holds the SaaS control plane configuration.
type ServerConfig struct {
	Port     int    `mapstructure:"port"`
	Host     string `mapstructure:"host"`
	Env      string `mapstructure:"env"`
	LogLevel string `mapstructure:"log_level"`

	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	GRPC     GRPCConfig     `mapstructure:"grpc"`
}

// SatelliteConfig holds the satellite runtime configuration.
type SatelliteConfig struct {
	ID                 string `mapstructure:"id"`
	Name               string `mapstructure:"name"`
	Env                string `mapstructure:"env"`
	SaaSEndpoint       string `mapstructure:"saas_endpoint"`
	ConnectorMode      string `mapstructure:"connector_mode"`
	TLSCertPath        string `mapstructure:"tls_cert_path"`
	TLSKeyPath         string `mapstructure:"tls_key_path"`
	TLSCAPath          string `mapstructure:"tls_ca_path"`
	TLSServerName      string `mapstructure:"tls_server_name"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
	LogLevel           string `mapstructure:"log_level"`

	Crypto      CryptoConfig      `mapstructure:"crypto"`
	Secrets     SecretsConfig     `mapstructure:"secrets"`
	Discovery   DiscoveryConfig   `mapstructure:"discovery"`
	Auth        GatewayAuthConfig `mapstructure:"auth"`
	State       StateConfig       `mapstructure:"state"`
	Alerting    AlertingConfig    `mapstructure:"alerting"`
	ACME        ACMEConfig        `mapstructure:"acme"`
	Commands    CommandConfig     `mapstructure:"commands"`
	FileStorage FileStorageConfig `mapstructure:"file_storage"`
}

// CommandConfig bounds command execution and result transport.
type CommandConfig struct {
	PollInterval   string `mapstructure:"poll_interval"`
	Timeout        string `mapstructure:"timeout"`
	LeaseRenewal   string `mapstructure:"lease_renewal"`
	MaxResultBytes int    `mapstructure:"max_result_bytes"`
	NetworkRetries int    `mapstructure:"network_retries"`
}

// GatewayAuthConfig is retained only so an existing config file with an `auth:`
// block still parses.
//
// None of these keys is enforced, and they must not be reintroduced as if they
// were. The struct previously fed a validator that verified HS256 with
// []byte(JWTSecret) — an empty secret verified any attacker-computed HS256
// signature, and the same validator rejected the RS256 tokens Keycloak actually
// issues, so the only tokens it accepted were forged ones. That validator has
// been removed.
//
// Real gateway authentication lives in internal/api/auth.go and is configured
// from the environment: CLM_JWKS_URL for Keycloak RS256 (verified against the
// realm JWKS) and CLM_JWT_SECRET for internal HS256 tokens, which are refused
// outright when no secret is set. Per-route roles are enforced by the route
// table in internal/api/server.go, not by an allow-list here.
//
// Deprecated: setting any of these has no effect.
type GatewayAuthConfig struct {
	Mode           string   `mapstructure:"mode"`
	SaaSLoginURL   string   `mapstructure:"saas_login_url"`
	JWTSecret      string   `mapstructure:"jwt_secret"`
	JWTPublicKey   string   `mapstructure:"jwt_public_key"`
	AllowedRoles   []string `mapstructure:"allowed_roles"`
	AllowedIssuers []string `mapstructure:"allowed_issuers"`
}

// DatabaseConfig holds PostgreSQL connection details.
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// RedisConfig holds Redis connection details.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// JWTConfig holds JWT signing configuration.
type JWTConfig struct {
	Secret        string `mapstructure:"secret"`
	ExpiryMinutes int    `mapstructure:"expiry_minutes"`
	RefreshDays   int    `mapstructure:"refresh_days"`
	Issuer        string `mapstructure:"issuer"`
}

// GRPCConfig holds gRPC server settings.
type GRPCConfig struct {
	Port int `mapstructure:"port"`
}

// CryptoConfig holds satellite crypto engine settings.
type CryptoConfig struct {
	DefaultAlgorithm string `mapstructure:"default_algorithm"`
	ECDSACurve       string `mapstructure:"ecdsa_curve"`
	RSABits          int    `mapstructure:"rsa_bits"`
	HSMEnabled       bool   `mapstructure:"hsm_enabled"`
	HSMLibrary       string `mapstructure:"hsm_library"`
}

// SecretsConfig holds cloud secret store settings.
type SecretsConfig struct {
	Provider string `mapstructure:"provider"` // aws, gcp, azure, vault
	Region   string `mapstructure:"region"`
	KMSKeyID string `mapstructure:"kms_key_id"`
	Prefix   string `mapstructure:"prefix"`

	// AWS specific
	AWSRoleARN string `mapstructure:"aws_role_arn"`

	// GCP specific
	GCPProject string `mapstructure:"gcp_project"`

	// Azure specific
	AzureVaultURL string `mapstructure:"azure_vault_url"`

	// Vault specific
	VaultAddr  string `mapstructure:"vault_addr"`
	VaultToken string `mapstructure:"vault_token"`
}

// DiscoveryConfig holds satellite discovery engine settings.
type DiscoveryConfig struct {
	Schedule    string   `mapstructure:"schedule"` // cron expression, informational
	Networks    []string `mapstructure:"networks"` // CIDR ranges
	Ports       []int    `mapstructure:"ports"`
	Concurrency int      `mapstructure:"concurrency"`
	Timeout     string   `mapstructure:"timeout"`

	// Scheduled discovery. Interval is used rather than the cron Schedule
	// because a cron parser would mean another dependency; Schedule is retained
	// so existing config files keep working and it is surfaced for display.
	Enabled    bool     `mapstructure:"enabled"`
	Interval   string   `mapstructure:"interval"` // Go duration, e.g. "6h"
	Domains    []string `mapstructure:"domains"`  // scanned by name with SNI
	DNSServer  string   `mapstructure:"dns_server"`
	Enumerate  bool     `mapstructure:"enumerate"` // wordlist subdomain enumeration
	ScanACM    bool     `mapstructure:"scan_acm"`
	ACMRegions []string `mapstructure:"acm_regions"`
	// MaxNames caps candidate names per domain scan. Defaults to 2000, which is
	// also the hard maximum, because every candidate costs a DNS query.
	MaxNames int `mapstructure:"max_names"`
	// DNSConcurrency is how many lookups and handshakes run at once. Set too low,
	// a wide enumeration cannot finish inside the request timeout.
	DNSConcurrency int `mapstructure:"dns_concurrency"`
}

// StateConfig controls where the gateway keeps state that must survive restart.
type StateConfig struct {
	// Dir holds the JSON snapshots. Empty disables persistence, which leaves the
	// inventory and fleet in memory only.
	Dir string `mapstructure:"dir"`
}

// FileStorageConfig is the administrator-approved destination policy for
// certificate and cryptographic material. Tenant roots replace, rather than
// extend, the gateway-wide roots for a configured tenant.
type FileStorageConfig struct {
	AllowedRoots []string            `mapstructure:"allowed_roots"`
	TenantRoots  map[string][]string `mapstructure:"tenant_roots"`
}

// AlertingConfig controls proactive expiry and risk notifications.
type AlertingConfig struct {
	Enabled bool `mapstructure:"enabled"`
	// WebhookURL receives a generic JSON payload; SlackWebhookURL receives a
	// Slack-shaped payload. Either or both may be set.
	WebhookURL      string `mapstructure:"webhook_url"`
	SlackWebhookURL string `mapstructure:"slack_webhook_url"`
	// WarnDays and CriticalDays are the expiry thresholds that raise an alert.
	WarnDays     int `mapstructure:"warn_days"`
	CriticalDays int `mapstructure:"critical_days"`
	// Interval is how often the evaluation runs.
	Interval string `mapstructure:"interval"`
	// Cooldown suppresses repeat alerts for the same certificate and severity,
	// without which every evaluation would re-notify the same problem.
	Cooldown string `mapstructure:"cooldown"`
	// AlertOnSelfSigned and AlertOnWeakKey extend alerting beyond expiry.
	AlertOnSelfSigned bool `mapstructure:"alert_on_self_signed"`
	AlertOnWeakKey    bool `mapstructure:"alert_on_weak_key"`
}

// DNS01Config contains DNS provider metadata and credential references. Raw DNS
// credentials must not be placed in this configuration or certificate inventory.
type DNS01Config struct {
	Provider           string   `mapstructure:"provider"`
	Zone               string   `mapstructure:"zone"`
	Nameserver         string   `mapstructure:"nameserver"`
	CredentialRef      string   `mapstructure:"credential_ref"`
	Resolvers          []string `mapstructure:"resolvers"`
	TTL                int      `mapstructure:"ttl"`
	PollInterval       string   `mapstructure:"poll_interval"`
	PropagationTimeout string   `mapstructure:"propagation_timeout"`
	CleanupTimeout     string   `mapstructure:"cleanup_timeout"`
}

// ACMEConfig configures automated issuance and renewal against an ACME CA.
type ACMEConfig struct {
	Enabled bool `mapstructure:"enabled"`
	// DirectoryURL is the ACME directory. Defaults to Let's Encrypt staging,
	// deliberately: production has tight rate limits and issues trusted
	// certificates, so it should be an explicit choice.
	DirectoryURL string `mapstructure:"directory_url"`
	Email        string `mapstructure:"email"`
	// AcceptTOS must be true. The ACME account cannot be created without
	// agreeing to the CA's terms, and agreeing on someone's behalf silently
	// would be wrong.
	AcceptTOS bool `mapstructure:"accept_tos"`
	// KeyType is "ecdsa" or "rsa" for issued certificate keys.
	KeyType string `mapstructure:"key_type"`
	// ChallengeType is auto, http-01, or dns-01. Auto keeps HTTP-01 for ordinary
	// names and selects DNS-01 for wildcards when a live provider is configured.
	ChallengeType string      `mapstructure:"challenge_type"`
	DNS01         DNS01Config `mapstructure:"dns01"`
	// RenewBeforeDays triggers renewal this far ahead of expiry.
	RenewBeforeDays int `mapstructure:"renew_before_days"`
	// RenewInterval is how often the renewal sweep runs.
	RenewInterval string `mapstructure:"renew_interval"`
	// OrderPollInterval is how often pending commercial CA orders are re-read.
	OrderPollInterval string `mapstructure:"order_poll_interval"`
	// InsecureSkipVerify allows a test ACME server with an untrusted CA, such as
	// Pebble. It is rejected for every production gateway.
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
}

// LoadServerConfig reads, resolves references, and validates server config.
func LoadServerConfig(path string) (*ServerConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("port", 8080)
	v.SetDefault("host", "0.0.0.0")
	v.SetDefault("env", "development")
	v.SetDefault("log_level", "info")
	v.SetDefault("jwt.expiry_minutes", 60)
	v.SetDefault("jwt.refresh_days", 7)
	v.SetDefault("grpc.port", 9090)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}
	if err := resolveAndValidate(&cfg, &cfg.Env, cfg.Validate); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadSatelliteConfig reads, resolves references, and validates satellite config.
func LoadSatelliteConfig(path string) (*SatelliteConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("env", "development")
	v.SetDefault("connector_mode", "direct")
	v.SetDefault("log_level", "info")
	v.SetDefault("crypto.default_algorithm", "ECDSA")
	v.SetDefault("crypto.ecdsa_curve", "P-256")
	v.SetDefault("crypto.rsa_bits", 4096)
	v.SetDefault("discovery.concurrency", 100)
	v.SetDefault("discovery.timeout", "5s")
	v.SetDefault("commands.poll_interval", "5s")
	v.SetDefault("commands.timeout", "15m")
	v.SetDefault("commands.lease_renewal", "45s")
	v.SetDefault("commands.max_result_bytes", 262144)
	v.SetDefault("commands.network_retries", 5)
	v.SetDefault("acme.challenge_type", "auto")
	v.SetDefault("acme.dns01.ttl", 60)
	v.SetDefault("acme.dns01.poll_interval", "2s")
	v.SetDefault("acme.dns01.propagation_timeout", "2m")
	v.SetDefault("acme.dns01.cleanup_timeout", "15s")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg SatelliteConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}
	if err := resolveAndValidate(&cfg, &cfg.Env, cfg.Validate); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadYAML reads a YAML config file into any struct type and resolves references.
func LoadYAML[T any](path string) (*T, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg T
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}
	if _, err := resolveReferences(&cfg); err != nil {
		return nil, fmt.Errorf("resolving config references: %w", err)
	}
	return &cfg, nil
}
