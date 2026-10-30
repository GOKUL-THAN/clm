package config

import (
	"fmt"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/viper"
)

var envReferencePattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ServerConfig holds the SaaS control plane configuration.
type ServerConfig struct {
	Port     int    `mapstructure:"port"`
	Host     string `mapstructure:"host"`
	Env      string `mapstructure:"env"`
	LogLevel string `mapstructure:"log_level"`

	Database      DatabaseConfig         `mapstructure:"database"`
	Redis         RedisConfig            `mapstructure:"redis"`
	JWT           JWTConfig              `mapstructure:"jwt"`
	CloudAccounts CloudAccountsConfig    `mapstructure:"cloud_accounts"`
	GRPC          GRPCConfig             `mapstructure:"grpc"`
	TLS           ServerTLSConfig        `mapstructure:"tls"`
	Gateway       GatewayTransportConfig `mapstructure:"gateway"`
}

// ServerTLSConfig controls HTTPS and gateway client-certificate verification.
type ServerTLSConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	CertPath     string `mapstructure:"cert_path"`
	KeyPath      string `mapstructure:"key_path"`
	ClientAuth   string `mapstructure:"client_auth"` // none, optional, required
	ClientCAPath string `mapstructure:"client_ca_path"`
}

// GatewayTransportConfig controls leased delivery and the only development-only
// compatibility escape hatch. Production validation always rejects legacy mode.
type GatewayTransportConfig struct {
	AllowInsecureLegacy bool          `mapstructure:"allow_insecure_legacy"`
	LeaseDuration       time.Duration `mapstructure:"lease_duration"`
	MaxPullCommands     int           `mapstructure:"max_pull_commands"`
	MaxResultBytes      int64         `mapstructure:"max_result_bytes"`
	RetryBase           time.Duration `mapstructure:"retry_base"`
}

// SatelliteConfig holds the satellite runtime configuration.
type SatelliteConfig struct {
	ID            string `mapstructure:"id"`
	Name          string `mapstructure:"name"`
	Env           string `mapstructure:"env"`
	SaaSEndpoint  string `mapstructure:"saas_endpoint"`
	ConnectorMode string `mapstructure:"connector_mode"`
	TLSCertPath   string `mapstructure:"tls_cert_path"`
	TLSKeyPath    string `mapstructure:"tls_key_path"`
	LogLevel      string `mapstructure:"log_level"`

	Crypto    CryptoConfig      `mapstructure:"crypto"`
	Secrets   SecretsConfig     `mapstructure:"secrets"`
	Discovery DiscoveryConfig   `mapstructure:"discovery"`
	Auth      GatewayAuthConfig `mapstructure:"auth"`
}

// GatewayAuthConfig holds authentication settings for the Gateway UI.
// The gateway delegates login to SaaS and validates SaaS-issued JWTs locally.
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

// JWTConfig defines separate trust policies for internally-issued HMAC tokens
// and externally-issued JWKS-backed tokens.
type JWTConfig struct {
	Secret            string        `mapstructure:"secret"`
	ExpiryMinutes     int           `mapstructure:"expiry_minutes"`
	RefreshDays       int           `mapstructure:"refresh_days"`
	Issuer            string        `mapstructure:"issuer"`
	Audience          string        `mapstructure:"audience"`
	ExternalIssuer    string        `mapstructure:"external_issuer"`
	ExternalAudience  string        `mapstructure:"external_audience"`
	JWKSURL           string        `mapstructure:"jwks_url"`
	AllowedAlgorithms []string      `mapstructure:"allowed_algorithms"`
	JWKSCacheTTL      time.Duration `mapstructure:"jwks_cache_ttl"`
	JWKSHTTPTimeout   time.Duration `mapstructure:"jwks_http_timeout"`
}

// CloudAccountsConfig holds the credential-encryption master key. The value can
// be an environment or file:// reference and must resolve to exactly 32 bytes.
type CloudAccountsConfig struct {
	MasterKey string `mapstructure:"master_key"`
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
	Provider string `mapstructure:"provider"`
	Region   string `mapstructure:"region"`
	KMSKeyID string `mapstructure:"kms_key_id"`
	Prefix   string `mapstructure:"prefix"`

	AWSRoleARN string `mapstructure:"aws_role_arn"`
	GCPProject string `mapstructure:"gcp_project"`

	AzureVaultURL string `mapstructure:"azure_vault_url"`
	VaultAddr     string `mapstructure:"vault_addr"`
	VaultToken    string `mapstructure:"vault_token"`
}

// DiscoveryConfig holds satellite discovery engine settings.
type DiscoveryConfig struct {
	Schedule    string   `mapstructure:"schedule"`
	Networks    []string `mapstructure:"networks"`
	Ports       []int    `mapstructure:"ports"`
	Concurrency int      `mapstructure:"concurrency"`
	Timeout     string   `mapstructure:"timeout"`
}

// Validate rejects unsafe production server configuration. Development and test
// profiles retain their existing permissive behavior.
func (c *ServerConfig) Validate() error {
	switch strings.ToLower(strings.TrimSpace(c.TLS.ClientAuth)) {
	case "", "none", "optional", "required":
	default:
		return fmt.Errorf("TLS client_auth must be none, optional, or required")
	}
	if c.TLS.Enabled && (strings.TrimSpace(c.TLS.CertPath) == "" || strings.TrimSpace(c.TLS.KeyPath) == "") {
		return fmt.Errorf("control-plane TLS certificate and key are required when TLS is enabled")
	}
	if (strings.EqualFold(c.TLS.ClientAuth, "optional") || strings.EqualFold(c.TLS.ClientAuth, "required")) && strings.TrimSpace(c.TLS.ClientCAPath) == "" {
		return fmt.Errorf("gateway client CA bundle is required when client certificate verification is enabled")
	}
	if !isProduction(c.Env) {
		// Preserve legacy development/test configurations while keeping the
		// runtime authenticator strict: only non-production config validation may
		// inherit the internal policy for an enabled external trust path.
		if strings.TrimSpace(c.JWT.JWKSURL) != "" {
			if strings.TrimSpace(c.JWT.ExternalIssuer) == "" {
				c.JWT.ExternalIssuer = c.JWT.Issuer
			}
			if strings.TrimSpace(c.JWT.ExternalAudience) == "" {
				c.JWT.ExternalAudience = c.JWT.Audience
			}
		}
		return nil
	}
	if strings.TrimSpace(c.Database.Host) == "" || c.Database.Port <= 0 || strings.TrimSpace(c.Database.User) == "" || strings.TrimSpace(c.Database.DBName) == "" {
		return fmt.Errorf("production database host, port, user, and dbname are required")
	}
	if err := validateStrongSecret("database password", c.Database.Password, 24); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(c.Database.SSLMode)) {
	case "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("production database sslmode must be require, verify-ca, or verify-full")
	}
	if c.JWT.Secret != "" {
		if err := validateStrongSecret("JWT secret", c.JWT.Secret, 32); err != nil {
			return err
		}
	} else if strings.TrimSpace(c.JWT.JWKSURL) == "" {
		return fmt.Errorf("production requires a JWT secret or JWKS URL")
	}
	if strings.TrimSpace(c.JWT.Issuer) == "" || strings.TrimSpace(c.JWT.Audience) == "" {
		return fmt.Errorf("production JWT issuer and audience are required")
	}
	if len(c.JWT.AllowedAlgorithms) == 0 {
		return fmt.Errorf("production JWT allowed_algorithms is required")
	}
	for _, algorithm := range c.JWT.AllowedAlgorithms {
		if algorithm != "HS256" && algorithm != "RS256" && algorithm != "RS384" && algorithm != "RS512" {
			return fmt.Errorf("unsupported JWT algorithm %q", algorithm)
		}
	}
	if c.JWT.JWKSURL != "" {
		if strings.TrimSpace(c.JWT.ExternalIssuer) == "" || strings.TrimSpace(c.JWT.ExternalAudience) == "" {
			return fmt.Errorf("production external JWT issuer and audience are required when JWKS URL is configured")
		}
		if err := requireHTTPS("external JWT issuer", c.JWT.ExternalIssuer); err != nil {
			return err
		}
		if err := requireHTTPS("JWT JWKS URL", c.JWT.JWKSURL); err != nil {
			return err
		}
	}
	if len(c.CloudAccounts.MasterKey) != 32 {
		return fmt.Errorf("production cloud account master key must be exactly 32 bytes")
	}
	if c.Redis.Password != "" {
		if err := validateStrongSecret("Redis password", c.Redis.Password, 24); err != nil {
			return err
		}
	}
	if !c.TLS.Enabled || strings.TrimSpace(c.TLS.CertPath) == "" || strings.TrimSpace(c.TLS.KeyPath) == "" {
		return fmt.Errorf("production control-plane TLS certificate and key are required")
	}
	if !strings.EqualFold(c.TLS.ClientAuth, "required") || strings.TrimSpace(c.TLS.ClientCAPath) == "" {
		return fmt.Errorf("production gateway mTLS requires client_auth=required and a client CA bundle")
	}
	if c.Gateway.AllowInsecureLegacy {
		return fmt.Errorf("production legacy unauthenticated gateway transport is forbidden")
	}
	if c.Gateway.LeaseDuration <= 0 || c.Gateway.MaxPullCommands <= 0 || c.Gateway.MaxPullCommands > 100 || c.Gateway.MaxResultBytes <= 0 || c.Gateway.MaxResultBytes > 4<<20 || c.Gateway.RetryBase <= 0 {
		return fmt.Errorf("production gateway lease duration, pull limit, result size limit, and retry base must use safe positive bounds")
	}
	return nil
}

// Validate rejects unsafe production satellite configuration.
func (c *SatelliteConfig) Validate() error {
	if !isProduction(c.Env) {
		return nil
	}
	if !strings.EqualFold(c.ConnectorMode, "offline") {
		if err := requireHTTPS("SaaS endpoint", c.SaaSEndpoint); err != nil {
			return err
		}
		if err := validateStrongSecret("CLM_SATELLITE_TOKEN", os.Getenv("CLM_SATELLITE_TOKEN"), 24); err != nil {
			return err
		}
	}
	if strings.TrimSpace(c.TLSCertPath) == "" || strings.TrimSpace(c.TLSKeyPath) == "" {
		return fmt.Errorf("production TLS certificate and key paths are required")
	}
	if c.TLSCertPath == c.TLSKeyPath {
		return fmt.Errorf("production TLS certificate and key paths must differ")
	}
	if c.Auth.SaaSLoginURL != "" {
		if err := requireHTTPS("SaaS login URL", c.Auth.SaaSLoginURL); err != nil {
			return err
		}
	}
	if c.Auth.JWTSecret != "" {
		if err := validateStrongSecret("gateway JWT secret", c.Auth.JWTSecret, 32); err != nil {
			return err
		}
	}
	if jwksURL := os.Getenv("CLM_JWKS_URL"); jwksURL != "" {
		if err := requireHTTPS("CLM_JWKS_URL", jwksURL); err != nil {
			return err
		}
	}
	if jwtSecret := os.Getenv("CLM_JWT_SECRET"); jwtSecret != "" {
		if err := validateStrongSecret("CLM_JWT_SECRET", jwtSecret, 32); err != nil {
			return err
		}
	}
	return nil
}

// LoadServerConfig reads, resolves references, and validates server config.
func LoadServerConfig(path string) (*ServerConfig, error) {
	v := newViper(path)
	v.SetDefault("port", 8080)
	v.SetDefault("host", "0.0.0.0")
	v.SetDefault("env", "development")
	v.SetDefault("log_level", "info")
	v.SetDefault("jwt.expiry_minutes", 60)
	v.SetDefault("jwt.refresh_days", 7)
	v.SetDefault("jwt.issuer", "clm-server")
	v.SetDefault("jwt.audience", "clm-api")
	v.SetDefault("jwt.allowed_algorithms", []string{"HS256", "RS256"})
	v.SetDefault("jwt.jwks_cache_ttl", "5m")
	v.SetDefault("jwt.jwks_http_timeout", "5s")
	v.SetDefault("grpc.port", 9090)
	v.SetDefault("tls.client_auth", "none")
	v.SetDefault("gateway.allow_insecure_legacy", false)
	v.SetDefault("gateway.lease_duration", "2m")
	v.SetDefault("gateway.max_pull_commands", 10)
	v.SetDefault("gateway.max_result_bytes", 262144)
	v.SetDefault("gateway.retry_base", "5s")

	var cfg ServerConfig
	if err := readConfig(v, &cfg); err != nil {
		return nil, err
	}
	if err := resolveAndValidate(&cfg, &cfg.Env, cfg.Validate); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadSatelliteConfig reads, resolves references, and validates satellite config.
func LoadSatelliteConfig(path string) (*SatelliteConfig, error) {
	v := newViper(path)
	v.SetDefault("env", "development")
	v.SetDefault("connector_mode", "direct")
	v.SetDefault("log_level", "info")
	v.SetDefault("crypto.default_algorithm", "ECDSA")
	v.SetDefault("crypto.ecdsa_curve", "P-256")
	v.SetDefault("crypto.rsa_bits", 4096)
	v.SetDefault("discovery.concurrency", 100)
	v.SetDefault("discovery.timeout", "5s")

	var cfg SatelliteConfig
	if err := readConfig(v, &cfg); err != nil {
		return nil, err
	}
	if err := resolveAndValidate(&cfg, &cfg.Env, cfg.Validate); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadYAML reads a YAML config file into any struct type and resolves references.
func LoadYAML[T any](path string) (*T, error) {
	v := newViper(path)
	var cfg T
	if err := readConfig(v, &cfg); err != nil {
		return nil, err
	}
	if _, err := resolveReferences(&cfg); err != nil {
		return nil, fmt.Errorf("resolving config references: %w", err)
	}
	return &cfg, nil
}

func newViper(path string) *viper.Viper {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	return v
}

func readConfig(v *viper.Viper, target any) error {
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("reading config: %w", err)
	}
	if err := v.Unmarshal(target); err != nil {
		return fmt.Errorf("unmarshalling config: %w", err)
	}
	return nil
}

func resolveAndValidate(target any, env *string, validate func() error) error {
	originalEnv := *env
	missing, err := resolveReferences(target)
	if err != nil {
		return fmt.Errorf("resolving config references: %w", err)
	}
	if envReferencePattern.MatchString(originalEnv) && strings.TrimSpace(*env) == "" {
		return fmt.Errorf("unresolved or empty config environment reference")
	}
	if isProduction(*env) && len(missing) > 0 {
		return fmt.Errorf("unresolved production environment references: %s", strings.Join(missing, ", "))
	}
	if err := validate(); err != nil {
		return fmt.Errorf("validating config: %w", err)
	}
	return nil
}

func resolveReferences(target any) ([]string, error) {
	missing := make(map[string]struct{})
	if err := resolveValue(reflect.ValueOf(target), missing); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(missing))
	for key := range missing {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func resolveValue(value reflect.Value, missing map[string]struct{}) error {
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		return resolveValue(value.Elem(), missing)
	}
	switch value.Kind() {
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if err := resolveValue(value.Field(i), missing); err != nil {
				return err
			}
		}
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			if err := resolveValue(value.Index(i), missing); err != nil {
				return err
			}
		}
	case reflect.String:
		if !value.CanSet() {
			return nil
		}
		resolved := envReferencePattern.ReplaceAllStringFunc(value.String(), func(reference string) string {
			name := envReferencePattern.FindStringSubmatch(reference)[1]
			if replacement, ok := os.LookupEnv(name); ok {
				return replacement
			}
			missing[name] = struct{}{}
			return ""
		})
		if strings.HasPrefix(resolved, "file://") {
			path := strings.TrimPrefix(resolved, "file://")
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("empty secret-file path")
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading secret file %q: %w", path, err)
			}
			resolved = strings.TrimRight(string(contents), "\r\n")
		}
		value.SetString(resolved)
	}
	return nil
}

func isProduction(env string) bool {
	return strings.EqualFold(strings.TrimSpace(env), "production") || strings.EqualFold(strings.TrimSpace(env), "prod")
}

func validateStrongSecret(name, secret string, minimum int) error {
	secret = strings.TrimSpace(secret)
	if len(secret) < minimum {
		return fmt.Errorf("production %s must be at least %d characters", name, minimum)
	}
	lower := strings.ToLower(secret)
	for _, marker := range []string{"change-me", "changeme", "password", "default", "example", "do-not-use", "dev-token"} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("production %s uses a known placeholder", name)
		}
	}
	unique := make(map[rune]struct{})
	for _, character := range secret {
		unique[character] = struct{}{}
	}
	if len(unique) < 10 {
		return fmt.Errorf("production %s lacks sufficient character diversity", name)
	}
	return nil
}

func requireHTTPS(name, rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
		return fmt.Errorf("production %s must be an absolute HTTPS URL", name)
	}
	return nil
}
