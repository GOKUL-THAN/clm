package ca

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

// Runtime registration of CA vendors.
//
// Accounts could only be configured through environment variables read once at
// startup, so adding a vendor meant editing a compose file and restarting the
// gateway. That is not something an operator should have to do to onboard a CA.
//
// This file describes each vendor's credentials as data, which does two things:
// the portal can render the correct fields without knowing anything about a
// specific vendor, and a new account can be built from a credential map at
// runtime by the same code path the startup wiring uses.

// CredentialField describes one input a vendor needs.
type CredentialField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	// Secret marks a value that must never be displayed or returned once stored.
	Secret bool `json:"secret"`
	// Placeholder and Help are for the form; Help is where a vendor's quirks are
	// explained at the point they matter.
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
	// Options, when set, makes this a fixed choice rather than free text.
	Options []string `json:"options,omitempty"`
	Default string   `json:"default,omitempty"`
}

// VendorSpec describes a supported CA vendor.
type VendorSpec struct {
	Vendor       string            `json:"vendor"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary"`
	Fields       []CredentialField `json:"fields"`
	Environments []string          `json:"environments"`
	// Capabilities as they will be once registered, so the portal can warn before
	// an account is created rather than after.
	Capabilities Capabilities `json:"capabilities"`
}

// SupportedVendors lists every CA that can be registered, with the fields each
// one needs.
func SupportedVendors() []VendorSpec {
	return []VendorSpec{
		{
			Vendor:       "acme",
			Title:        "ACME (Let's Encrypt, ZeroSSL, step-ca, EJBCA)",
			Summary:      "Free, immediate, DV only. Any RFC 8555 directory works.",
			Environments: []string{"staging", "production"},
			Fields: []CredentialField{
				{Key: "directory_url", Label: "Directory URL", Required: true,
					Placeholder: "https://acme-staging-v02.api.letsencrypt.org/directory",
					Help:        "Staging is strongly preferred while testing: production has strict rate limits."},
				{Key: "email", Label: "Contact email", Required: true,
					Placeholder: "ops@example.com",
					Help:        "The CA uses this for expiry notices and account recovery."},
				{Key: "accept_tos", Label: "Accept the CA's terms of service", Required: true,
					Options: []string{"true", "false"}, Default: "false",
					Help: "An ACME account cannot be created without agreeing to the terms."},
				{Key: "key_type", Label: "Key type", Options: []string{"ecdsa", "rsa"}, Default: "ecdsa"},
				{Key: "challenge_type", Label: "Challenge type", Options: []string{"auto", "http-01", "dns-01"}, Default: "auto",
					Help: "Auto keeps HTTP-01 for ordinary names and uses DNS-01 for wildcards when a live provider is configured."},
				{Key: "dns_provider", Label: "DNS-01 provider",
					Options: []string{"", "rfc2136", "aws-route53", "azure-dns", "gcp-cloud-dns", "oci-dns"},
					Help:    "RFC2136 is live through nsupdate. Cloud adapters are planned and are rejected until implemented."},
				{Key: "dns_zone", Label: "DNS zone", Help: "Provider routing metadata, for example example.com."},
				{Key: "dns_nameserver", Label: "DNS nameserver", Help: "RFC2136 update server and default propagation resolver."},
				{Key: "dns_credential_ref", Label: "DNS credential reference",
					Placeholder: "file-ref:///etc/clm/dns/acme.key",
					Help:        "Reference only; never paste a TSIG secret or cloud credential. RFC2136 accepts a file-ref:// nsupdate key path."},
				{Key: "dns_resolvers", Label: "Propagation resolvers", Help: "Comma-separated DNS servers; every server must observe the TXT value."},
				{Key: "dns_ttl", Label: "DNS TTL", Default: "60"},
				{Key: "dns_poll_interval", Label: "Propagation poll interval", Default: "2s"},
				{Key: "dns_propagation_timeout", Label: "Propagation timeout", Default: "2m"},
				{Key: "dns_cleanup_timeout", Label: "Cleanup timeout", Default: "15s"},
				{Key: "insecure_skip_verify", Label: "Skip TLS verification",
					Options: []string{"false", "true"}, Default: "false",
					Help: "Only for a test CA with an untrusted root, such as Pebble. Never against a real CA."},
			},
			Capabilities: Capabilities{
				Synchronous: true, CertTypes: []string{"dv"}, MultiSAN: true,
				DCVMethods: []string{DCVHTTP}, Revocation: true, MaxValidityYears: 1,
			},
		},
		{
			Vendor:       "godaddy",
			Title:        "GoDaddy Certificates API",
			Summary:      "DV, OV and EV including wildcards. Asynchronous; no pricing API.",
			Environments: []string{"ote", "production"},
			Fields: []CredentialField{
				{Key: "key", Label: "API key", Required: true, Secret: true,
					Help: "Created in the GoDaddy developer portal. OTE and production keys are different."},
				{Key: "secret", Label: "API secret", Required: true, Secret: true},
				{Key: "shopper_id", Label: "Shopper ID",
					Help: "Only needed for a reseller key acting on a customer's behalf."},
				{Key: "base_url", Label: "Base URL override",
					Help: "Leave empty. Set only to point at a test double."},
			},
			Capabilities: Capabilities{
				CertTypes: []string{"dv", "ov", "ev"}, Wildcard: true, MultiSAN: true,
				DCVMethods:  []string{DCVHTTP, DCVDNS, DCVEmail},
				RequiresCSR: true, Revocation: true, MaxValidityYears: 2,
			},
		},
		{
			Vendor:       "namecheap",
			Title:        "Namecheap SSL API",
			Summary:      "DV, OV and EV including wildcards. Asynchronous, with live pricing.",
			Environments: []string{"sandbox", "production"},
			Fields: []CredentialField{
				{Key: "api_user", Label: "API user", Required: true},
				{Key: "api_key", Label: "API key", Required: true, Secret: true},
				{Key: "username", Label: "Username",
					Help: "Defaults to the API user; differs only for a sub-account."},
				{Key: "client_ip", Label: "Client IP", Required: true,
					Placeholder: "203.0.113.10",
					Help: "Must be allow-listed under API access on the Namecheap account, " +
						"otherwise every call is rejected with error 1011150."},
				{Key: "base_url", Label: "Base URL override",
					Help: "Leave empty. Set only to point at a test double."},
			},
			Capabilities: Capabilities{
				CertTypes: []string{"dv", "ov", "ev"}, Wildcard: true, MultiSAN: true,
				DCVMethods:  []string{DCVHTTP, DCVDNS, DCVEmail},
				RequiresCSR: true, Revocation: true, Quoting: true, MaxValidityYears: 1,
			},
		},
	}
}

// VendorByName finds a vendor spec.
func VendorByName(vendor string) (VendorSpec, bool) {
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	for _, v := range SupportedVendors() {
		if v.Vendor == vendor {
			return v, true
		}
	}
	return VendorSpec{}, false
}

// ValidateCredentialKeys rejects fields the selected CA adapter does not
// understand. This prevents arbitrary values from being stored with unknown
// secret classification. Environment is gateway metadata shared by all vendors.
func ValidateCredentialKeys(vendor string, creds map[string]string) error {
	spec, ok := VendorByName(vendor)
	if !ok {
		return fmt.Errorf("unsupported CA vendor %q", vendor)
	}
	allowed := map[string]bool{"environment": true}
	for _, field := range spec.Fields {
		allowed[field.Key] = true
	}
	unknown := []string{}
	for key := range creds {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("unsupported %s credential fields: %s", vendor, strings.Join(unknown, ", "))
	}
	return nil
}

// ValidateCredentials checks a credential map against a vendor's required fields
// and reports every problem at once, rather than one per attempt.
func ValidateCredentials(vendor string, creds map[string]string) error {
	spec, ok := VendorByName(vendor)
	if !ok {
		known := []string{}
		for _, v := range SupportedVendors() {
			known = append(known, v.Vendor)
		}
		return fmt.Errorf("unsupported CA vendor %q; supported vendors are: %s",
			vendor, strings.Join(known, ", "))
	}

	missing := []string{}
	for _, f := range spec.Fields {
		if !f.Required {
			continue
		}
		if strings.TrimSpace(creds[f.Key]) == "" {
			missing = append(missing, f.Key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s requires: %s", vendor, strings.Join(missing, ", "))
	}

	if vendor == "acme" && !strings.EqualFold(strings.TrimSpace(creds["accept_tos"]), "true") {
		return fmt.Errorf("accept_tos must be true: an ACME account cannot be created " +
			"without agreeing to the CA's terms of service")
	}
	return nil
}

// ACMEBuildConfig carries ACME account metadata. DNS credentials remain an
// external reference and are never accepted as raw values in this structure.
type ACMEBuildConfig struct {
	Environment           string
	DirectoryURL          string
	Email                 string
	KeyType               string
	AcceptTOS             bool
	InsecureSkipVerify    bool
	ChallengeType         string
	DNSProvider           string
	DNSZone               string
	DNSNameserver         string
	DNSCredentialRef      string
	DNSResolvers          []string
	DNSTTL                int
	DNSPollInterval       string
	DNSPropagationTimeout string
	DNSCleanupTimeout     string
	KeyDir                string
}

// ACMEBuilder builds an ACME provider from account metadata.
type ACMEBuilder func(config ACMEBuildConfig, logger *zap.Logger) (Provider, error)

var acmeBuilder ACMEBuilder

// SetACMEBuilder registers the ACME constructor.
func SetACMEBuilder(b ACMEBuilder) { acmeBuilder = b }

// Build constructs a provider for a vendor from a credential map.
func Build(vendor string, creds map[string]string, keyDir string, logger *zap.Logger) (Provider, error) {
	if err := ValidateCredentials(vendor, creds); err != nil {
		return nil, err
	}

	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "acme":
		if acmeBuilder == nil {
			return nil, fmt.Errorf("the ACME builder is not registered on this gateway")
		}
		dnsTTL, err := strconv.Atoi(strings.TrimSpace(creds["dns_ttl"]))
		if err != nil && strings.TrimSpace(creds["dns_ttl"]) != "" {
			return nil, fmt.Errorf("dns_ttl must be an integer")
		}
		return acmeBuilder(ACMEBuildConfig{
			Environment:           creds["environment"],
			DirectoryURL:          creds["directory_url"],
			Email:                 creds["email"],
			KeyType:               creds["key_type"],
			AcceptTOS:             strings.EqualFold(creds["accept_tos"], "true"),
			InsecureSkipVerify:    strings.EqualFold(creds["insecure_skip_verify"], "true"),
			ChallengeType:         creds["challenge_type"],
			DNSProvider:           creds["dns_provider"],
			DNSZone:               creds["dns_zone"],
			DNSNameserver:         creds["dns_nameserver"],
			DNSCredentialRef:      creds["dns_credential_ref"],
			DNSResolvers:          splitCommaSeparated(creds["dns_resolvers"]),
			DNSTTL:                dnsTTL,
			DNSPollInterval:       creds["dns_poll_interval"],
			DNSPropagationTimeout: creds["dns_propagation_timeout"],
			DNSCleanupTimeout:     creds["dns_cleanup_timeout"],
			KeyDir:                keyDir,
		}, logger)

	case "godaddy":
		return NewGoDaddyProvider(GoDaddyConfig{
			Key: creds["key"], Secret: creds["secret"],
			Environment: creds["environment"], ShopperID: creds["shopper_id"],
			BaseURL: creds["base_url"],
		}, logger)

	case "namecheap":
		return NewNamecheapProvider(NamecheapConfig{
			APIUser: creds["api_user"], APIKey: creds["api_key"],
			UserName: creds["username"], ClientIP: creds["client_ip"],
			Sandbox: !strings.EqualFold(creds["environment"], "production"),
			BaseURL: creds["base_url"],
		}, logger)
	}
	return nil, fmt.Errorf("unsupported CA vendor %q", vendor)
}

// SecretKeys returns the credential keys a vendor treats as secret, so a caller
// knows which values must never be echoed back.
func SecretKeys(vendor string) map[string]bool {
	out := map[string]bool{}
	spec, ok := VendorByName(vendor)
	if !ok {
		return out
	}
	for _, f := range spec.Fields {
		if f.Secret {
			out[f.Key] = true
		}
	}
	return out
}

// Replace swaps the provider behind an existing account, used when credentials
// are updated.
func (r *Registry) Replace(name, environment string, isDefault bool, p Provider) {
	name = strings.ToLower(strings.TrimSpace(name))

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.accounts[name]; !exists {
		r.order = append(r.order, name)
	}
	r.accounts[name] = &Account{
		Name: name, Provider: p.Name(),
		Environment: environment, Default: isDefault, provider: p,
	}
	if isDefault || r.def == "" {
		r.def = name
	}
}

// Remove deletes an account. The default moves to whatever is left, because a
// registry with accounts but no default would refuse every unnamed request.
func (r *Registry) Remove(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.accounts[name]; !ok {
		return fmt.Errorf("no CA account named %q", name)
	}
	delete(r.accounts, name)

	kept := make([]string, 0, len(r.order))
	for _, n := range r.order {
		if n != name {
			kept = append(kept, n)
		}
	}
	r.order = kept

	if r.def == name {
		r.def = ""
		if len(r.order) > 0 {
			r.def = r.order[0]
		}
	}
	return nil
}

// SetDefault moves the default account.
func (r *Registry) SetDefault(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.accounts[name]; !ok {
		return fmt.Errorf("no CA account named %q", name)
	}
	r.def = name
	return nil
}

func splitCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
