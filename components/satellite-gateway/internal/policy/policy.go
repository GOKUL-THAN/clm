// Package policy enforces certificate issuance rules before an order reaches a
// CA, rather than only scoring the estate against them after the fact.
//
// The gateway already had a compliance report (internal/api/reporting.go) that
// checks key strength, validity length, self-signed certificates and so on —
// but it runs on demand against certificates that already exist. By the time a
// weak-key or over-long-validity certificate shows up in that report, it has
// already been issued, possibly deployed, and the fix is a second lifecycle
// event rather than a request that was never allowed to succeed. An industry
// CLM product's policy engine sits in front of issuance: a request that
// violates a rule is refused with the specific rule and value, before any CA is
// contacted and before any money is spent on a commercial order.
//
// A policy here is deliberately narrow: a named set of constraints scoped to an
// environment (or "*" for all environments), evaluated against one issuance
// request. There is no rule scripting language — every field is a concrete,
// checkable value, so a policy that cannot be evaluated cannot exist.
package policy

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Policy is one named set of issuance constraints.
type Policy struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Environment scopes the policy: "prod", "staging", or "*" for every
	// environment. When several policies match a request, every one of them
	// must pass — policies narrow what is allowed, they never widen it.
	Environment string `json:"environment"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`

	// MinKeySizeRSA and MinKeySizeECDSA are in bits. Zero means "not checked for
	// this algorithm", not "any size passes" — a policy that only cares about
	// RSA strength should leave the ECDSA minimum unset rather than writing 0
	// twice with different meanings.
	MinKeySizeRSA   int `json:"min_key_size_rsa,omitempty"`
	MinKeySizeECDSA int `json:"min_key_size_ecdsa,omitempty"`

	// MaxValidityDays caps how long a certificate may be valid for. Public CAs
	// have not issued beyond 398 days since 2020; an internal CA might allow
	// more, which is exactly why this is a policy value and not a hardcoded
	// constant.
	MaxValidityDays int `json:"max_validity_days,omitempty"`

	// AllowedCAAccounts restricts which named CA accounts this policy's scope may
	// order from. Empty means unrestricted. Referencing an account by the name it
	// was registered under, not by vendor, so "namecheap-prod" and
	// "namecheap-sandbox" can be governed differently even though both are the
	// same vendor.
	AllowedCAAccounts []string `json:"allowed_ca_accounts,omitempty"`

	// AllowedCertTypes restricts dv/ov/ev. Empty means unrestricted.
	AllowedCertTypes []string `json:"allowed_cert_types,omitempty"`

	// WildcardAllowed defaults to true (zero value would incorrectly forbid
	// wildcards for every policy that does not mention the field), so it is a
	// pointer: nil means "not stated by this policy", false means "explicitly
	// forbidden".
	WildcardAllowed *bool `json:"wildcard_allowed,omitempty"`

	// RequireApprovalForCertTypes lists cert types that must have a decided
	// approval attached before issuance proceeds — the enforcement half of the
	// approval workflow that already exists but was never wired to actually gate
	// anything.
	RequireApprovalForCertTypes []string `json:"require_approval_for_cert_types,omitempty"`

	// MaxSANCount bounds how many names one certificate may cover. Zero means
	// unrestricted.
	MaxSANCount int `json:"max_san_count,omitempty"`

	// DeniedDomainPatterns are substrings (not full regex, to keep evaluation
	// predictable and fast) that must not appear in any requested name — for
	// blocking known-bad or legacy domains from ever getting a new certificate.
	DeniedDomainPatterns []string `json:"denied_domain_patterns,omitempty"`

	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Request is the subset of an issuance request a policy needs to evaluate. It
// is a separate type from ca.IssueRequest so this package does not depend on
// the ca package, keeping the dependency direction the same as the rest of the
// component (api depends on policy, policy depends on nothing gateway-specific).
type Request struct {
	Domains         []string
	CertType        string // dv, ov, ev — already normalised by the caller
	Environment     string
	CAAccount       string
	ValidityYears   int
	KeyType         string // ecdsa, rsa
	KeyBits         int    // 0 if not yet known (e.g. ECDSA has a fixed curve, not "bits")
	ApprovalDecided bool   // true when a decided (not merely pending) approval exists
}

// Violation is one rule a request failed.
type Violation struct {
	PolicyID   string `json:"policy_id"`
	PolicyName string `json:"policy_name"`
	Rule       string `json:"rule"`
	Message    string `json:"message"`
}

// Store holds policies in memory and persists them through the same durable
// state mechanism as credentials and endpoints.
type Store struct {
	mu       sync.RWMutex
	policies map[string]*Policy
	persist  func([]*Policy)
	logger   *zap.Logger
}

// NewStore builds an empty store. persist is called after every mutation with
// the full current set, so the caller can wire it to state.Store.Save without
// this package needing to import it directly — mirroring how credstore and the
// endpoint store already separate "what a policy is" from "how it is saved".
func NewStore(logger *zap.Logger, persist func([]*Policy)) *Store {
	if persist == nil {
		persist = func([]*Policy) {}
	}
	return &Store{policies: map[string]*Policy{}, persist: persist, logger: logger}
}

// Restore loads previously persisted policies, e.g. from a snapshot at
// startup. It does not call persist, since nothing changed.
func (s *Store) Restore(policies []*Policy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range policies {
		if p != nil && p.ID != "" {
			s.policies[p.ID] = p
		}
	}
	if len(s.policies) > 0 && s.logger != nil {
		s.logger.Info("certificate policies restored", zap.Int("count", len(s.policies)))
	}
}

// Put creates or updates a policy.
func (s *Store) Put(p *Policy, actor string) (*Policy, error) {
	if p == nil || strings.TrimSpace(p.Name) == "" {
		return nil, fmt.Errorf("a policy name is required")
	}
	if strings.TrimSpace(p.Environment) == "" {
		p.Environment = "*"
	}
	for _, ct := range p.AllowedCertTypes {
		if !validCertType(ct) {
			return nil, fmt.Errorf("allowed_cert_types contains %q; must be dv, ov or ev", ct)
		}
	}
	for _, ct := range p.RequireApprovalForCertTypes {
		if !validCertType(ct) {
			return nil, fmt.Errorf("require_approval_for_cert_types contains %q; must be dv, ov or ev", ct)
		}
	}
	if p.MinKeySizeRSA < 0 || p.MinKeySizeECDSA < 0 || p.MaxValidityDays < 0 || p.MaxSANCount < 0 {
		return nil, fmt.Errorf("numeric limits must not be negative")
	}

	s.mu.Lock()
	if p.ID == "" {
		p.ID = uuid.NewString()
		p.CreatedAt = time.Now()
		p.CreatedBy = actor
	} else if existing, ok := s.policies[p.ID]; ok {
		p.CreatedAt = existing.CreatedAt
		if p.CreatedBy == "" {
			p.CreatedBy = existing.CreatedBy
		}
	} else {
		s.mu.Unlock()
		return nil, fmt.Errorf("no policy with id %q", p.ID)
	}
	p.UpdatedAt = time.Now()
	s.policies[p.ID] = p
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	s.persist(snapshot)
	return p, nil
}

// Delete removes a policy.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	if _, ok := s.policies[id]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("no policy with id %q", id)
	}
	delete(s.policies, id)
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	s.persist(snapshot)
	return nil
}

// List returns every policy, sorted by name.
func (s *Store) List() []*Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *Store) snapshotLocked() []*Policy {
	out := make([]*Policy, 0, len(s.policies))
	for _, p := range s.policies {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Count reports how many policies exist, so a caller can skip evaluation
// entirely when there is nothing to enforce.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.policies)
}

func validCertType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "dv", "ov", "ev":
		return true
	}
	return false
}

// Evaluate checks a request against every enabled policy whose Environment is
// "*" or matches the request's environment (case-insensitively), and returns
// every violation found. An empty result means the request is allowed to
// proceed — evaluation returns violations, not a single true/false, because an
// operator fixing a rejected request needs the whole list, not one error at a
// time.
func (s *Store) Evaluate(req Request) []Violation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Violation
	env := strings.ToLower(strings.TrimSpace(req.Environment))
	certType := strings.ToLower(strings.TrimSpace(req.CertType))

	for _, p := range s.policies {
		if !p.Enabled {
			continue
		}
		pEnv := strings.ToLower(strings.TrimSpace(p.Environment))
		if pEnv != "*" && pEnv != env {
			continue
		}
		out = append(out, evaluateOne(p, req, certType)...)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].PolicyName != out[j].PolicyName {
			return out[i].PolicyName < out[j].PolicyName
		}
		return out[i].Rule < out[j].Rule
	})
	return out
}

func evaluateOne(p *Policy, req Request, certType string) []Violation {
	var v []Violation
	fail := func(rule, msg string) {
		v = append(v, Violation{PolicyID: p.ID, PolicyName: p.Name, Rule: rule, Message: msg})
	}

	if len(p.AllowedCertTypes) > 0 && certType != "" && !containsFold(p.AllowedCertTypes, certType) {
		fail("cert_type", fmt.Sprintf(
			"policy %q allows only %s certificates in this environment, not %s",
			p.Name, strings.Join(p.AllowedCertTypes, "/"), strings.ToUpper(certType)))
	}

	if len(p.AllowedCAAccounts) > 0 && req.CAAccount != "" && !containsFold(p.AllowedCAAccounts, req.CAAccount) {
		fail("ca_account", fmt.Sprintf(
			"policy %q allows only these CA accounts here: %s (requested %q)",
			p.Name, strings.Join(p.AllowedCAAccounts, ", "), req.CAAccount))
	}

	if p.WildcardAllowed != nil && !*p.WildcardAllowed && hasWildcard(req.Domains) {
		fail("wildcard", fmt.Sprintf(
			"policy %q forbids wildcard certificates in this environment", p.Name))
	}

	if p.MaxValidityDays > 0 && req.ValidityYears > 0 {
		requestedDays := req.ValidityYears * 365
		if requestedDays > p.MaxValidityDays {
			fail("max_validity", fmt.Sprintf(
				"policy %q allows at most %d day(s) of validity; %d year(s) requested is %d days",
				p.Name, p.MaxValidityDays, req.ValidityYears, requestedDays))
		}
	}

	if p.MaxSANCount > 0 && len(req.Domains) > p.MaxSANCount {
		fail("max_san_count", fmt.Sprintf(
			"policy %q allows at most %d name(s) on one certificate; %d requested",
			p.Name, p.MaxSANCount, len(req.Domains)))
	}

	// Key strength is checked only when the caller has told us which algorithm
	// and size are actually being used. A policy naming an RSA minimum must not
	// silently pass an ECDSA request just because 0 < 0 is false — that would
	// look enforced and not be.
	if req.KeyBits > 0 {
		switch strings.ToLower(req.KeyType) {
		case "rsa":
			if p.MinKeySizeRSA > 0 && req.KeyBits < p.MinKeySizeRSA {
				fail("min_key_size", fmt.Sprintf(
					"policy %q requires RSA keys of at least %d bits; %d requested",
					p.Name, p.MinKeySizeRSA, req.KeyBits))
			}
		case "ecdsa", "ec":
			if p.MinKeySizeECDSA > 0 && req.KeyBits < p.MinKeySizeECDSA {
				fail("min_key_size", fmt.Sprintf(
					"policy %q requires ECDSA keys of at least %d bits; %d requested",
					p.Name, p.MinKeySizeECDSA, req.KeyBits))
			}
		}
	}

	for _, pattern := range p.DeniedDomainPatterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		for _, d := range req.Domains {
			if strings.Contains(strings.ToLower(d), pattern) {
				fail("denied_domain", fmt.Sprintf(
					"policy %q denies any name containing %q; %q matches", p.Name, pattern, d))
			}
		}
	}

	if len(p.RequireApprovalForCertTypes) > 0 && containsFold(p.RequireApprovalForCertTypes, certType) &&
		!req.ApprovalDecided {
		fail("require_approval", fmt.Sprintf(
			"policy %q requires a decided approval before issuing a %s certificate here",
			p.Name, strings.ToUpper(certType)))
	}

	return v
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func hasWildcard(domains []string) bool {
	for _, d := range domains {
		if strings.HasPrefix(strings.TrimSpace(d), "*.") {
			return true
		}
	}
	return false
}
