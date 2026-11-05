// Package dns01 provides provider-neutral DNS-01 record management for ACME.
//
// Providers receive only the TXT record required for a challenge. Authentication
// material is represented by provider-specific references (for example a
// file-ref:// path or workload identity metadata), never by raw secrets stored in
// certificate inventory records.
package dns01

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	ProviderRoute53 = "aws-route53"
	ProviderAzure   = "azure-dns"
	ProviderGCP     = "gcp-cloud-dns"
	ProviderRFC2136 = "rfc2136"
	ProviderOCI     = "oci-dns"
)

// Adapter describes an adapter's actual implementation status. Planned adapters
// are deliberately not constructible: reporting a cloud adapter as live before
// it has provider API coverage would make wildcard issuance fail mid-order.
type Adapter struct {
	Name           string
	Live           bool
	CredentialMode string
	Notes          string
}

var adapters = []Adapter{
	{Name: ProviderRoute53, Live: true, CredentialMode: "AWS workload identity or aws-profile:// reference", Notes: "live through the AWS Route 53 API"}, // #nosec G101 -- descriptive credential mode, not a credential.
	{Name: ProviderAzure, Live: false, CredentialMode: "managed identity or external credential reference", Notes: "planned; Azure DNS API adapter is not implemented"},
	{Name: ProviderGCP, Live: false, CredentialMode: "workload identity or external credential reference", Notes: "planned; Cloud DNS API adapter is not implemented"},
	{Name: ProviderRFC2136, Live: true, CredentialMode: "file-ref:// reference to an nsupdate TSIG key", Notes: "live through the local nsupdate command"}, // #nosec G101 -- describes a secret reference scheme, not a secret.
	{Name: ProviderOCI, Live: false, CredentialMode: "instance principal or external credential reference", Notes: "planned; OCI DNS API adapter is not implemented"},
}

// Adapters returns a copy of the adapter catalog.
func Adapters() []Adapter {
	out := make([]Adapter, len(adapters))
	copy(out, adapters)
	return out
}

// AdapterByName returns provider status metadata.
func AdapterByName(name string) (Adapter, bool) {
	name = normaliseProvider(name)
	for _, adapter := range adapters {
		if adapter.Name == name {
			return adapter, true
		}
	}
	return Adapter{}, false
}

func normaliseProvider(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "route53", "aws", "aws-route53":
		return ProviderRoute53
	case "azure", "azuredns", "azure-dns":
		return ProviderAzure
	case "gcp", "google", "clouddns", "cloud-dns", "gcp-cloud-dns":
		return ProviderGCP
	case "rfc2136", "nsupdate":
		return ProviderRFC2136
	case "oci", "oracle", "oci-dns":
		return ProviderOCI
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

// Record is one ACME DNS-01 TXT value. Providers must make Present and Cleanup
// idempotent. Cleanup must remove only Value because concurrent authorizations
// can legitimately use multiple TXT values at the same record name.
type Record struct {
	FQDN  string
	Value string
	TTL   int
}

// Provider creates and removes DNS-01 TXT values.
type Provider interface {
	Name() string
	Present(ctx context.Context, record Record) error
	Cleanup(ctx context.Context, record Record) error
}

// TXTResolver is the propagation observation boundary used by Solver.
type TXTResolver interface {
	LookupTXT(ctx context.Context, fqdn string) ([]string, error)
}

// Config contains provider metadata and secret references, never raw provider
// credentials. Only RFC2136 is live today; cloud fields are retained so their
// eventual adapters can share this contract without changing ACME orchestration.
type Config struct {
	Provider           string
	Zone               string
	Nameserver         string
	CredentialRef      string
	Resolvers          []string
	TTL                int
	PollInterval       time.Duration
	PropagationTimeout time.Duration
	CleanupTimeout     time.Duration
}

// New constructs a solver for a live provider.
func New(cfg Config) (*Solver, error) {
	adapter, ok := AdapterByName(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("unsupported DNS-01 provider %q", cfg.Provider)
	}
	if !adapter.Live {
		return nil, fmt.Errorf("DNS-01 provider %q is planned but not live: %s", adapter.Name, adapter.Notes)
	}

	var provider Provider
	switch adapter.Name {
	case ProviderRoute53:
		var err error
		provider, err = newRoute53Provider(cfg)
		if err != nil {
			return nil, err
		}
	case ProviderRFC2136:
		var err error
		provider, err = newRFC2136Provider(cfg)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("DNS-01 provider %q has no registered adapter", adapter.Name)
	}

	servers := append([]string(nil), cfg.Resolvers...)
	if len(servers) == 0 && strings.TrimSpace(cfg.Nameserver) != "" {
		servers = []string{cfg.Nameserver}
	}
	return NewSolver(provider, NewNetResolver(servers), cfg), nil
}

// Solver owns propagation polling and cleanup semantics independently of ACME.
type Solver struct {
	provider           Provider
	resolver           TXTResolver
	ttl                int
	pollInterval       time.Duration
	propagationTimeout time.Duration
	cleanupTimeout     time.Duration
}

// NewSolver builds a solver around an injected provider and resolver. It is the
// main unit-test seam and also permits future cloud adapters without ACME changes.
func NewSolver(provider Provider, resolver TXTResolver, cfg Config) *Solver {
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 60
	}
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	propagationTimeout := cfg.PropagationTimeout
	if propagationTimeout <= 0 {
		propagationTimeout = 2 * time.Minute
	}
	cleanupTimeout := cfg.CleanupTimeout
	if cleanupTimeout <= 0 {
		cleanupTimeout = 15 * time.Second
	}
	return &Solver{
		provider: provider, resolver: resolver, ttl: ttl,
		pollInterval: pollInterval, propagationTimeout: propagationTimeout,
		cleanupTimeout: cleanupTimeout,
	}
}

// ProviderName reports the active live adapter.
func (s *Solver) ProviderName() string {
	if s == nil || s.provider == nil {
		return ""
	}
	return s.provider.Name()
}

// Solve presents a TXT value, waits for propagation, performs CA validation,
// and removes that exact value on every exit path. Cleanup uses a context
// detached from request cancellation so a timed-out order does not strand DNS.
func (s *Solver) Solve(ctx context.Context, identifier, value string, validate func(context.Context) error) (err error) {
	if s == nil || s.provider == nil {
		return fmt.Errorf("a live DNS-01 provider is required")
	}
	if s.resolver == nil {
		return fmt.Errorf("a DNS TXT resolver is required")
	}
	recordName, err := RecordName(identifier)
	if err != nil {
		return err
	}
	record := Record{FQDN: recordName, Value: value, TTL: s.ttl}

	// Register cleanup before Present: a provider can mutate DNS and still return
	// an error (for example after a response timeout), so best-effort deletion is
	// required even when presentation did not report success.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cleanupTimeout)
		defer cancel()
		if cleanupErr := s.provider.Cleanup(cleanupCtx, record); cleanupErr != nil {
			cleanupErr = fmt.Errorf("cleaning up DNS-01 record %s with %s: %w", record.FQDN, s.provider.Name(), cleanupErr)
			if err == nil {
				err = cleanupErr
			} else {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()

	if err = s.provider.Present(ctx, record); err != nil {
		return fmt.Errorf("presenting DNS-01 record %s with %s: %w", record.FQDN, s.provider.Name(), err)
	}
	if err = s.waitForPropagation(ctx, record); err != nil {
		return err
	}
	if validate == nil {
		return fmt.Errorf("DNS-01 validation callback is required")
	}
	if err = validate(ctx); err != nil {
		return fmt.Errorf("validating DNS-01 record %s: %w", record.FQDN, err)
	}
	return nil
}

func (s *Solver) waitForPropagation(ctx context.Context, record Record) error {
	pollCtx, cancel := context.WithTimeout(ctx, s.propagationTimeout)
	defer cancel()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		values, lookupErr := s.resolver.LookupTXT(pollCtx, record.FQDN)
		if lookupErr == nil {
			for _, candidate := range values {
				if candidate == record.Value {
					return nil
				}
			}
			lastErr = fmt.Errorf("TXT value is not visible yet")
		} else {
			lastErr = lookupErr
		}

		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("DNS-01 propagation for %s timed out after %s: %w",
				record.FQDN, s.propagationTimeout, lastErr)
		case <-ticker.C:
		}
	}
}

// RecordName returns the canonical DNS-01 owner name. Wildcard validation is
// performed at the zone apex: *.example.com and example.com both use
// _acme-challenge.example.com.
func RecordName(identifier string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(identifier))
	name = strings.TrimSuffix(name, ".")
	name = strings.TrimPrefix(name, "*.")
	if name == "" || strings.Contains(name, "*") || !validDNSName(name) {
		return "", fmt.Errorf("invalid DNS-01 identifier %q", identifier)
	}
	return "_acme-challenge." + name + ".", nil
}

func validDNSName(name string) bool {
	name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			character := label[i]
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

// NetResolver queries all configured recursive or authoritative servers. A TXT
// value is considered propagated only when every configured server returns it.
type NetResolver struct {
	servers []string
}

// NewNetResolver creates a resolver. An empty server list uses the host resolver.
func NewNetResolver(servers []string) *NetResolver {
	clean := make([]string, 0, len(servers))
	for _, server := range servers {
		server = strings.TrimSpace(server)
		if server != "" {
			clean = append(clean, server)
		}
	}
	return &NetResolver{servers: clean}
}

func (r *NetResolver) LookupTXT(ctx context.Context, fqdn string) ([]string, error) {
	if len(r.servers) == 0 {
		return net.DefaultResolver.LookupTXT(ctx, fqdn)
	}

	var intersection map[string]struct{}
	for _, server := range r.servers {
		address := dnsAddress(server)
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				dialer := net.Dialer{}
				return dialer.DialContext(ctx, network, address)
			},
		}
		values, err := resolver.LookupTXT(ctx, fqdn)
		if err != nil {
			return nil, fmt.Errorf("querying TXT through %s: %w", address, err)
		}
		current := make(map[string]struct{}, len(values))
		for _, value := range values {
			current[value] = struct{}{}
		}
		if intersection == nil {
			intersection = current
			continue
		}
		for value := range intersection {
			if _, ok := current[value]; !ok {
				delete(intersection, value)
			}
		}
	}

	values := make([]string, 0, len(intersection))
	for value := range intersection {
		values = append(values, value)
	}
	return values, nil
}

func dnsAddress(server string) string {
	if _, _, err := net.SplitHostPort(server); err == nil {
		return server
	}
	return net.JoinHostPort(strings.Trim(server, "[]"), "53")
}
