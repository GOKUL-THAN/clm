package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

var envReferencePattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Validate rejects unsafe production server configuration. Development and test
// profiles retain their existing permissive behavior.
func (c *ServerConfig) Validate() error {
	if !isProduction(c.Env) {
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
	if err := validateStrongSecret("JWT secret", c.JWT.Secret, 32); err != nil {
		return err
	}
	if c.Redis.Password != "" {
		if err := validateStrongSecret("Redis password", c.Redis.Password, 24); err != nil {
			return err
		}
	}
	return nil
}

// Validate rejects unsafe satellite configuration.
func (c *SatelliteConfig) Validate() error {
	if err := validateFileStorage(c.FileStorage, c.State.Dir); err != nil {
		return err
	}
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
	if strings.TrimSpace(c.TLSCertPath) == "" || strings.TrimSpace(c.TLSKeyPath) == "" || strings.TrimSpace(c.TLSCAPath) == "" {
		return fmt.Errorf("production TLS certificate, key, and server CA paths are required")
	}
	if c.TLSCertPath == c.TLSKeyPath {
		return fmt.Errorf("production TLS certificate and key paths must differ")
	}
	if c.InsecureSkipVerify {
		return fmt.Errorf("production insecure TLS verification is forbidden")
	}
	if c.ACME.InsecureSkipVerify {
		return fmt.Errorf("production ACME TLS verification cannot be disabled")
	}
	if strings.EqualFold(strings.TrimSpace(c.ACME.ChallengeType), "dns-01") && strings.TrimSpace(c.ACME.DNS01.Provider) == "" {
		return fmt.Errorf("production ACME dns-01 challenge_type requires acme.dns01.provider")
	}
	if strings.TrimSpace(c.ACME.DNS01.Provider) != "" {
		for name, value := range map[string]string{
			"acme.dns01.poll_interval":       c.ACME.DNS01.PollInterval,
			"acme.dns01.propagation_timeout": c.ACME.DNS01.PropagationTimeout,
			"acme.dns01.cleanup_timeout":     c.ACME.DNS01.CleanupTimeout,
		} {
			if duration, err := time.ParseDuration(value); err != nil || duration <= 0 {
				return fmt.Errorf("production %s must be a positive duration", name)
			}
		}
	}
	if strings.TrimSpace(c.State.Dir) == "" {
		return fmt.Errorf("production state.dir is required for command deduplication")
	}
	for name, value := range map[string]string{"commands.timeout": c.Commands.Timeout, "commands.lease_renewal": c.Commands.LeaseRenewal, "commands.poll_interval": c.Commands.PollInterval} {
		if duration, err := time.ParseDuration(value); err != nil || duration <= 0 {
			return fmt.Errorf("production %s must be a positive duration", name)
		}
	}
	if c.Commands.MaxResultBytes <= 0 || c.Commands.MaxResultBytes > 4<<20 || c.Commands.NetworkRetries <= 0 || c.Commands.NetworkRetries > 20 {
		return fmt.Errorf("production command result size and network retry bounds are invalid")
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

func validateFileStorage(cfg FileStorageConfig, stateDir string) error {
	if len(cfg.AllowedRoots) == 0 && len(cfg.TenantRoots) == 0 {
		return fmt.Errorf("file_storage is default-deny: configure allowed_roots or tenant_roots")
	}
	for _, root := range cfg.AllowedRoots {
		if err := validateApprovedRoot(root); err != nil {
			return err
		}
	}
	for tenant, roots := range cfg.TenantRoots {
		if strings.TrimSpace(tenant) == "" || tenant == "*" {
			return fmt.Errorf("file_storage.tenant_roots requires explicit tenant identifiers")
		}
		if len(roots) == 0 {
			return fmt.Errorf("file_storage.tenant_roots.%s must contain at least one root", tenant)
		}
		for _, root := range roots {
			if err := validateApprovedRoot(root); err != nil {
				return fmt.Errorf("file_storage.tenant_roots.%s: %w", tenant, err)
			}
		}
	}
	if strings.TrimSpace(stateDir) != "" && !pathWithinRoots(stateDir, cfg.AllowedRoots) {
		return fmt.Errorf("state.dir must be inside an explicitly approved file_storage.allowed_roots directory")
	}
	return nil
}

func validateApprovedRoot(root string) error {
	root = strings.TrimSpace(root)
	if root == "" || strings.ContainsRune(root, '\x00') || !filepath.IsAbs(root) {
		return fmt.Errorf("approved file roots must be non-empty absolute paths without null bytes")
	}
	if strings.Contains(root, "%2e") || strings.Contains(root, "%2E") ||
		strings.Contains(root, "%2f") || strings.Contains(root, "%2F") ||
		strings.Contains(root, "%5c") || strings.Contains(root, "%5C") ||
		strings.Contains(root, "%00") {
		return fmt.Errorf("approved file root %q contains encoded path characters", root)
	}
	for _, component := range strings.FieldsFunc(root, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if component == ".." {
			return fmt.Errorf("approved file root %q contains path traversal", root)
		}
	}
	return nil
}

func pathWithinRoots(path string, roots []string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	path = filepath.Clean(path)
	for _, root := range roots {
		relative, err := filepath.Rel(filepath.Clean(root), path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
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
			// #nosec G304 -- secret-file paths are explicitly configured locally.
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
