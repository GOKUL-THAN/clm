// Package ca gives the gateway more than one certificate authority.
//
// Issuance used to be a single hardcoded ACME directory read at startup, and the
// two API fields that accepted a CA choice — ca_provider on a certificate request
// and ca on the issue endpoint — were parsed and then silently discarded. Asking
// for DigiCert produced a Let's Encrypt certificate with no warning, which is
// worse than refusing the request.
//
// This package defines one interface every CA is reached through, and a registry
// of named accounts so several can be configured at once. Nothing here imports
// the api package; the ACME implementation lives there and is registered as a
// provider, which keeps the dependency pointing one way.
package ca

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Order states shared by every provider.
//
// ACME issues within seconds. A commercial OV or EV order can take hours or days
// because a human at the CA validates the organisation, so "issued" cannot be the
// only successful outcome — StatusPending is a normal result, not a failure, and
// the caller has to be able to come back later.
const (
	StatusIssued  = "issued"
	StatusPending = "pending"
	StatusFailed  = "failed"
	StatusRevoked = "revoked"
)

// Validation types a CA can ask for to prove domain control.
const (
	DCVHTTP  = "http"
	DCVDNS   = "dns"
	DCVEmail = "email"
)

// Capabilities describes what a provider can actually do, so the portal can stop
// offering a combination that will be rejected and the API can explain why.
type Capabilities struct {
	// Synchronous is true when Issue returns a usable certificate directly.
	Synchronous bool     `json:"synchronous"`
	CertTypes   []string `json:"cert_types"` // dv, ov, ev
	Wildcard    bool     `json:"wildcard"`
	MultiSAN    bool     `json:"multi_san"`
	// DCVMethods are the domain control validation methods supported.
	DCVMethods []string `json:"dcv_methods"`
	// RequiresCSR is true when the caller must supply a CSR, which is how every
	// commercial CA works; ACME builds its own.
	RequiresCSR bool `json:"requires_csr"`
	Revocation  bool `json:"revocation"`
	Quoting     bool `json:"quoting"`
	// MaxValidityYears is the longest term the CA will sell. Public CAs cannot
	// issue beyond 398 days, so a "3 year" product is billing, not validity.
	MaxValidityYears int    `json:"max_validity_years"`
	Notes            string `json:"notes,omitempty"`
}

// Organization is required for OV and EV, and ignored for DV.
type Organization struct {
	Name       string `json:"name"`
	Department string `json:"department,omitempty"`
	Address    string `json:"address,omitempty"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
	Country    string `json:"country,omitempty"` // ISO 3166-1 alpha-2
	Phone      string `json:"phone,omitempty"`
	// JobTitle and the contact fields are demanded by some CAs for EV.
	ContactFirstName string `json:"contact_first_name,omitempty"`
	ContactLastName  string `json:"contact_last_name,omitempty"`
	ContactEmail     string `json:"contact_email,omitempty"`
	ContactPhone     string `json:"contact_phone,omitempty"`
	JobTitle         string `json:"job_title,omitempty"`
}

// IssueRequest is a certificate order.
type IssueRequest struct {
	Domains  []string
	CertType string // dv, ov, ev
	// CSRPEM is required when Capabilities.RequiresCSR. When empty and a CSR is
	// needed, the caller should use GenerateKeyAndCSR.
	CSRPEM        string
	ValidityYears int
	ContactEmail  string
	Organization  *Organization
	// DCVMethod picks how domain control is proved, when the CA offers a choice.
	DCVMethod string
	// CertID is the gateway's own identifier, passed through so a provider can
	// name keys or correlate its own records.
	CertID   string
	TenantID string
}

// ValidationInstruction is what the domain owner has to do for the CA to
// validate. Returning these is the whole point of an asynchronous order: without
// them a pending order can never progress and the operator has nothing to act on.
type ValidationInstruction struct {
	Type   string `json:"type"` // http, dns, email
	Domain string `json:"domain"`
	// For http: the path to serve and the content to serve there.
	// For dns: the record name and value.
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
	// Email lists the addresses the CA will accept for email validation.
	Emails []string `json:"emails,omitempty"`
	Detail string   `json:"detail,omitempty"`
}

// IssueResult is the outcome of an order, whether it completed or not.
type IssueResult struct {
	Status  string `json:"status"`
	OrderID string `json:"order_id,omitempty"`
	// Provider and Account record which CA produced this, so the inventory shows
	// the truth rather than whatever was requested.
	Provider string `json:"provider"`
	Account  string `json:"account"`

	ChainPEM  string    `json:"chain_pem,omitempty"`
	NotBefore time.Time `json:"not_before,omitempty"`
	NotAfter  time.Time `json:"not_after,omitempty"`
	Serial    string    `json:"serial,omitempty"`
	Issuer    string    `json:"issuer,omitempty"`

	Validation []ValidationInstruction `json:"validation,omitempty"`
	Message    string                  `json:"message,omitempty"`
	// PrivateKeyPEM is set only when the provider generated the key, which the
	// caller must then store; it is never persisted by this package.
	PrivateKeyPEM string `json:"-"`
	// KeyPath is set instead when the provider has already written the key itself,
	// as the ACME implementation does. Exactly one of the two is populated, so
	// there is never an ambiguity about who owns the key.
	KeyPath string `json:"-"`
}

// QuoteRequest asks for pricing.
type QuoteRequest struct {
	Domains       []string
	CertType      string
	ValidityYears int
}

// Quote is one purchasable product.
type Quote struct {
	Provider     string   `json:"provider"`
	Account      string   `json:"account"`
	ProductName  string   `json:"product_name"`
	ProductID    string   `json:"product_id,omitempty"`
	Price        float64  `json:"price"`
	Currency     string   `json:"currency"`
	CertType     string   `json:"cert_type"`
	Validity     string   `json:"validity"`
	Wildcard     bool     `json:"wildcard"`
	IssuanceTime string   `json:"issuance_time"`
	Features     []string `json:"features,omitempty"`
}

// RevokeRequest revokes an issued certificate.
type RevokeRequest struct {
	OrderID  string
	ChainPEM string
	// KeyPEM is needed by ACME, which requires the revocation to be signed with
	// the certificate's own key. Commercial CAs revoke by order ID on an
	// authenticated account instead.
	KeyPEM   string
	Reason   string
	TenantID string
}

// Provider is one certificate authority reached through one account.
type Provider interface {
	// Name is the vendor identifier: acme, godaddy, namecheap.
	Name() string
	Capabilities() Capabilities
	// Quote returns purchasable products. Providers without pricing return an
	// empty slice and no error rather than inventing numbers.
	Quote(ctx context.Context, req QuoteRequest) ([]Quote, error)
	Issue(ctx context.Context, req IssueRequest) (*IssueResult, error)
	// OrderStatus re-reads an order, which is how a pending commercial order is
	// eventually collected.
	OrderStatus(ctx context.Context, orderID string) (*IssueResult, error)
	Revoke(ctx context.Context, req RevokeRequest) error
}

// Account is a configured provider instance. Several accounts can share a vendor
// — a sandbox and a production GoDaddy account, for instance — which is why
// routing is by account name and not by vendor.
type Account struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Environment string `json:"environment"` // sandbox, production
	Default     bool   `json:"default"`
	provider    Provider
}

// Registry holds the configured accounts.
type Registry struct {
	mu       sync.RWMutex
	accounts map[string]*Account
	order    []string // registration order, for stable listing
	def      string
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{accounts: map[string]*Account{}}
}

// Register adds an account. The first registered account becomes the default
// unless one is explicitly marked.
func (r *Registry) Register(name, environment string, isDefault bool, p Provider) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return fmt.Errorf("account name is required")
	}
	if p == nil {
		return fmt.Errorf("provider is required for account %q", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.accounts[name]; exists {
		return fmt.Errorf("a CA account named %q is already registered", name)
	}
	r.accounts[name] = &Account{
		Name: name, Provider: p.Name(),
		Environment: environment, Default: isDefault, provider: p,
	}
	r.order = append(r.order, name)

	if isDefault || r.def == "" {
		r.def = name
	}
	return nil
}

// Get resolves an account by name, or by vendor name when only one account of
// that vendor is configured. An empty name returns the default.
//
// The error lists what is available: a caller who asked for a CA that is not set
// up needs to know what to ask for instead, and the previous behaviour of
// silently substituting a different CA is the bug this replaces.
func (r *Registry) Get(name string) (*Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.accounts) == 0 {
		return nil, fmt.Errorf("no CA accounts are configured on this gateway")
	}

	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		if acct, ok := r.accounts[r.def]; ok {
			return acct, nil
		}
		return nil, fmt.Errorf("no default CA account is configured")
	}

	if acct, ok := r.accounts[name]; ok {
		return acct, nil
	}

	// Fall back to matching the vendor, but only when it is unambiguous.
	var matches []*Account
	for _, n := range r.order {
		if r.accounts[n].Provider == name {
			matches = append(matches, r.accounts[n])
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("unknown CA %q; configured accounts are: %s",
			name, strings.Join(r.namesLocked(), ", "))
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, m.Name)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("%q matches several accounts (%s); name one of them",
			name, strings.Join(names, ", "))
	}
}

func (r *Registry) namesLocked() []string {
	out := make([]string, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, n)
	}
	return out
}

// Provider returns the implementation behind an account.
func (a *Account) Provider_() Provider { return a.provider }

// List returns the configured accounts with their capabilities.
func (r *Registry) List() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]map[string]any, 0, len(r.order))
	for _, n := range r.order {
		a := r.accounts[n]
		out = append(out, map[string]any{
			"name":         a.Name,
			"provider":     a.Provider,
			"environment":  a.Environment,
			"default":      a.Name == r.def,
			"capabilities": a.provider.Capabilities(),
		})
	}
	return out
}

// Names returns the configured account names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.namesLocked()
}

// Default returns the default account name, empty when none are configured.
func (r *Registry) Default() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.def
}

// Count returns how many accounts are registered.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.accounts)
}

// QuoteAll gathers quotes from every account that supports pricing.
//
// One vendor being down must not hide the others, so per-account errors are
// collected and returned alongside whatever succeeded.
func (r *Registry) QuoteAll(ctx context.Context, req QuoteRequest) ([]Quote, []string) {
	r.mu.RLock()
	accounts := make([]*Account, 0, len(r.order))
	for _, n := range r.order {
		accounts = append(accounts, r.accounts[n])
	}
	r.mu.RUnlock()

	var (
		mu     sync.Mutex
		quotes []Quote
		errs   []string
		wg     sync.WaitGroup
	)

	for _, a := range accounts {
		if !a.provider.Capabilities().Quoting {
			continue
		}
		wg.Add(1)
		go func(acct *Account) {
			defer wg.Done()
			qs, err := acct.provider.Quote(ctx, req)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, acct.Name+": "+err.Error())
				return
			}
			for i := range qs {
				qs[i].Provider = acct.provider.Name()
				qs[i].Account = acct.Name
			}
			quotes = append(quotes, qs...)
		}(a)
	}
	wg.Wait()

	sort.Slice(quotes, func(i, j int) bool {
		if quotes[i].Price != quotes[j].Price {
			return quotes[i].Price < quotes[j].Price
		}
		return quotes[i].ProductName < quotes[j].ProductName
	})
	sort.Strings(errs)
	return quotes, errs
}

// GenerateKeyAndCSR produces a private key and a CSR for the given names.
//
// Commercial CAs sign a CSR the customer supplies, so the gateway has to make one
// itself. The key is returned to the caller to store; it is deliberately not
// written here, so there is exactly one place in the codebase that decides where
// private keys live.
func GenerateKeyAndCSR(domains []string, keyType string) (keyPEM string, csrPEM string, err error) {
	if len(domains) == 0 {
		return "", "", fmt.Errorf("at least one domain is required")
	}

	var signer crypto.Signer
	var keyBlock *pem.Block

	if strings.EqualFold(keyType, "rsa") {
		// RSA 2048 is the safe default for commercial CAs: some still reject
		// ECDSA, and a rejected CSR after a paid order is an expensive surprise.
		key, gerr := rsa.GenerateKey(rand.Reader, 2048)
		if gerr != nil {
			return "", "", fmt.Errorf("generating RSA key: %w", gerr)
		}
		signer = key
		keyBlock = &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	} else {
		key, gerr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if gerr != nil {
			return "", "", fmt.Errorf("generating ECDSA key: %w", gerr)
		}
		der, merr := x509.MarshalECPrivateKey(key)
		if merr != nil {
			return "", "", fmt.Errorf("encoding ECDSA key: %w", merr)
		}
		signer = key
		keyBlock = &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}
	}

	tmpl := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domains[0]},
		DNSNames: domains,
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, tmpl, signer)
	if err != nil {
		return "", "", fmt.Errorf("creating CSR: %w", err)
	}

	return string(pem.EncodeToMemory(keyBlock)),
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})),
		nil
}

// ParseChain reads the leaf out of a PEM chain so issuance results can be filled
// in consistently no matter which vendor returned the certificate.
func ParseChain(chainPEM string) (*x509.Certificate, error) {
	rest := []byte(chainPEM)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, fmt.Errorf("no certificate found in the chain returned by the CA")
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		return x509.ParseCertificate(block.Bytes)
	}
}

// NormaliseCertType maps the many spellings vendors use onto dv/ov/ev.
func NormaliseCertType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "", "dv", "domain", "domainvalidation", "domain_validation", "positivessl", "essentialssl":
		return "dv"
	case "ov", "organization", "organisation", "organizationvalidation", "instantssl":
		return "ov"
	case "ev", "extended", "extendedvalidation", "evssl":
		return "ev"
	default:
		return strings.ToLower(strings.TrimSpace(t))
	}
}

// HasWildcard reports whether any name is a wildcard, which changes both the
// product needed and the validation method available.
func HasWildcard(domains []string) bool {
	for _, d := range domains {
		if strings.HasPrefix(strings.TrimSpace(d), "*.") {
			return true
		}
	}
	return false
}
