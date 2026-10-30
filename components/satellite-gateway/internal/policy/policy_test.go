package policy

import (
	"testing"

	"go.uber.org/zap"
)

func testLogger() *zap.Logger { return zap.NewNop() }

func boolPtr(b bool) *bool { return &b }

func TestPutRequiresName(t *testing.T) {
	s := NewStore(testLogger(), nil)
	_, err := s.Put(&Policy{Environment: "prod"}, "tester")
	if err == nil {
		t.Fatal("expected an error for a policy with no name")
	}
}

func TestPutDefaultsEnvironmentToWildcard(t *testing.T) {
	s := NewStore(testLogger(), nil)
	p, err := s.Put(&Policy{Name: "no-env"}, "tester")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Environment != "*" {
		t.Errorf("Environment = %q, want \"*\"", p.Environment)
	}
}

func TestPutRejectsInvalidCertType(t *testing.T) {
	s := NewStore(testLogger(), nil)
	_, err := s.Put(&Policy{Name: "bad", AllowedCertTypes: []string{"xv"}}, "tester")
	if err == nil {
		t.Fatal("expected an error for an invalid cert type")
	}
}

func TestPutRejectsNegativeLimits(t *testing.T) {
	s := NewStore(testLogger(), nil)
	_, err := s.Put(&Policy{Name: "neg", MaxValidityDays: -1}, "tester")
	if err == nil {
		t.Fatal("expected an error for a negative limit")
	}
}

func TestPutThenGetRoundTrips(t *testing.T) {
	var saved []*Policy
	s := NewStore(testLogger(), func(p []*Policy) { saved = p })

	created, err := s.Put(&Policy{Name: "prod-rules", Environment: "prod", Enabled: true}, "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.ID == "" {
		t.Error("expected an ID to be assigned")
	}
	if created.CreatedBy != "alice" {
		t.Errorf("CreatedBy = %q, want alice", created.CreatedBy)
	}
	if len(saved) != 1 {
		t.Fatalf("persist callback saw %d policies, want 1", len(saved))
	}

	list := s.List()
	if len(list) != 1 || list[0].ID != created.ID {
		t.Errorf("List() did not return the created policy")
	}
}

func TestPutUpdatePreservesCreatedAt(t *testing.T) {
	s := NewStore(testLogger(), nil)
	created, _ := s.Put(&Policy{Name: "v1"}, "alice")
	createdAt := created.CreatedAt

	updated, err := s.Put(&Policy{ID: created.ID, Name: "v2", CreatedBy: ""}, "bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt changed on update: %v vs %v", updated.CreatedAt, createdAt)
	}
	if updated.CreatedBy != "alice" {
		t.Errorf("CreatedBy changed on update to %q, want it preserved as alice", updated.CreatedBy)
	}
	if updated.Name != "v2" {
		t.Errorf("Name = %q, want v2", updated.Name)
	}
}

func TestPutUnknownIDIsRejected(t *testing.T) {
	s := NewStore(testLogger(), nil)
	_, err := s.Put(&Policy{ID: "does-not-exist", Name: "x"}, "alice")
	if err == nil {
		t.Fatal("expected an error updating a policy that does not exist")
	}
}

func TestDeleteUnknownIsRejected(t *testing.T) {
	s := NewStore(testLogger(), nil)
	if err := s.Delete("nope"); err == nil {
		t.Fatal("expected an error deleting an unknown policy")
	}
}

func TestDeleteRemovesFromList(t *testing.T) {
	s := NewStore(testLogger(), nil)
	created, _ := s.Put(&Policy{Name: "temp"}, "alice")
	if err := s.Delete(created.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Count() != 0 {
		t.Errorf("Count() = %d after delete, want 0", s.Count())
	}
}

func TestEvaluateSkipsDisabledPolicies(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "disabled", Enabled: false, WildcardAllowed: boolPtr(false)}, "a")

	v := s.Evaluate(Request{Domains: []string{"*.example.com"}, Environment: "prod"})
	if len(v) != 0 {
		t.Errorf("expected no violations from a disabled policy, got %v", v)
	}
}

func TestEvaluateEnvironmentScoping(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "prod-only", Environment: "prod", Enabled: true, WildcardAllowed: boolPtr(false)}, "a")

	// Matching environment: rule applies.
	v := s.Evaluate(Request{Domains: []string{"*.example.com"}, Environment: "prod"})
	if len(v) != 1 {
		t.Fatalf("expected 1 violation in prod, got %d: %v", len(v), v)
	}

	// Different environment: rule does not apply.
	v = s.Evaluate(Request{Domains: []string{"*.example.com"}, Environment: "staging"})
	if len(v) != 0 {
		t.Errorf("expected no violations in staging, got %v", v)
	}
}

func TestEvaluateWildcardPolicyAppliesEverywhere(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "global", Environment: "*", Enabled: true, WildcardAllowed: boolPtr(false)}, "a")

	for _, env := range []string{"prod", "staging", "anything"} {
		v := s.Evaluate(Request{Domains: []string{"*.example.com"}, Environment: env})
		if len(v) != 1 {
			t.Errorf("env %q: expected 1 violation, got %d", env, len(v))
		}
	}
}

func TestEvaluateWildcardAllowedNilMeansUnrestricted(t *testing.T) {
	s := NewStore(testLogger(), nil)
	// WildcardAllowed left nil entirely.
	s.Put(&Policy{Name: "no-opinion", Enabled: true}, "a")

	v := s.Evaluate(Request{Domains: []string{"*.example.com"}, Environment: "prod"})
	if len(v) != 0 {
		t.Errorf("a policy that does not mention wildcards must not forbid them, got %v", v)
	}
}

func TestEvaluateAllowedCertTypes(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "dv-only", Enabled: true, AllowedCertTypes: []string{"dv"}}, "a")

	v := s.Evaluate(Request{Domains: []string{"example.com"}, CertType: "ov"})
	if len(v) != 1 || v[0].Rule != "cert_type" {
		t.Errorf("expected a cert_type violation for ov, got %v", v)
	}

	v = s.Evaluate(Request{Domains: []string{"example.com"}, CertType: "dv"})
	if len(v) != 0 {
		t.Errorf("dv should pass a dv-only policy, got %v", v)
	}
}

func TestEvaluateAllowedCAAccounts(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "restricted-ca", Enabled: true,
		AllowedCAAccounts: []string{"internal-ca", "acme"}}, "a")

	v := s.Evaluate(Request{Domains: []string{"example.com"}, CAAccount: "godaddy-prod"})
	if len(v) != 1 || v[0].Rule != "ca_account" {
		t.Errorf("expected a ca_account violation, got %v", v)
	}

	v = s.Evaluate(Request{Domains: []string{"example.com"}, CAAccount: "acme"})
	if len(v) != 0 {
		t.Errorf("an allowed account must pass, got %v", v)
	}

	// Case-insensitive match.
	v = s.Evaluate(Request{Domains: []string{"example.com"}, CAAccount: "ACME"})
	if len(v) != 0 {
		t.Errorf("account matching must be case-insensitive, got %v", v)
	}
}

func TestEvaluateMaxValidityDays(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "398-days", Enabled: true, MaxValidityDays: 398}, "a")

	v := s.Evaluate(Request{Domains: []string{"example.com"}, ValidityYears: 2})
	if len(v) != 1 || v[0].Rule != "max_validity" {
		t.Errorf("2 years (730 days) should violate a 398-day max, got %v", v)
	}

	v = s.Evaluate(Request{Domains: []string{"example.com"}, ValidityYears: 1})
	if len(v) != 0 {
		t.Errorf("1 year (365 days) should pass a 398-day max, got %v", v)
	}
}

func TestEvaluateMaxSANCount(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "limit-sans", Enabled: true, MaxSANCount: 2}, "a")

	v := s.Evaluate(Request{Domains: []string{"a.com", "b.com", "c.com"}})
	if len(v) != 1 || v[0].Rule != "max_san_count" {
		t.Errorf("3 names over a limit of 2 should violate, got %v", v)
	}

	v = s.Evaluate(Request{Domains: []string{"a.com", "b.com"}})
	if len(v) != 0 {
		t.Errorf("exactly the limit should pass, got %v", v)
	}
}

func TestEvaluateMinKeySizeRSA(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "strong-rsa", Enabled: true, MinKeySizeRSA: 2048}, "a")

	v := s.Evaluate(Request{Domains: []string{"example.com"}, KeyType: "rsa", KeyBits: 1024})
	if len(v) != 1 || v[0].Rule != "min_key_size" {
		t.Errorf("RSA 1024 should violate a 2048 minimum, got %v", v)
	}

	v = s.Evaluate(Request{Domains: []string{"example.com"}, KeyType: "rsa", KeyBits: 2048})
	if len(v) != 0 {
		t.Errorf("RSA 2048 should pass a 2048 minimum, got %v", v)
	}

	// An RSA minimum must not be applied to an ECDSA request just because
	// bits(256) < minRSA(2048) — that would fail every ECDSA request regardless
	// of its actual strength.
	v = s.Evaluate(Request{Domains: []string{"example.com"}, KeyType: "ecdsa", KeyBits: 256})
	if len(v) != 0 {
		t.Errorf("an RSA-only minimum must not apply to ECDSA, got %v", v)
	}
}

func TestEvaluateMinKeySizeECDSA(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "strong-ecdsa", Enabled: true, MinKeySizeECDSA: 384}, "a")

	v := s.Evaluate(Request{Domains: []string{"example.com"}, KeyType: "ecdsa", KeyBits: 256})
	if len(v) != 1 {
		t.Errorf("ECDSA 256 should violate a 384 minimum, got %v", v)
	}
}

func TestEvaluateKeySizeSkippedWhenUnknown(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "strong", Enabled: true, MinKeySizeRSA: 2048, MinKeySizeECDSA: 384}, "a")

	// KeyBits left at zero — the caller does not yet know it.
	v := s.Evaluate(Request{Domains: []string{"example.com"}, KeyType: "rsa"})
	if len(v) != 0 {
		t.Errorf("an unknown key size must not be scored as a failure, got %v", v)
	}
}

func TestEvaluateDeniedDomainPatterns(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "no-legacy", Enabled: true,
		DeniedDomainPatterns: []string{"legacy.", "internal-only"}}, "a")

	v := s.Evaluate(Request{Domains: []string{"legacy.example.com"}})
	if len(v) != 1 || v[0].Rule != "denied_domain" {
		t.Errorf("expected a denied_domain violation, got %v", v)
	}

	v = s.Evaluate(Request{Domains: []string{"api.example.com"}})
	if len(v) != 0 {
		t.Errorf("a non-matching domain should pass, got %v", v)
	}

	// Case-insensitive.
	v = s.Evaluate(Request{Domains: []string{"LEGACY.example.com"}})
	if len(v) != 1 {
		t.Errorf("pattern matching must be case-insensitive, got %v", v)
	}
}

func TestEvaluateRequireApprovalForCertTypes(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "ov-needs-approval", Enabled: true,
		RequireApprovalForCertTypes: []string{"ov", "ev"}}, "a")

	// No approval yet: violation.
	v := s.Evaluate(Request{Domains: []string{"example.com"}, CertType: "ov", ApprovalDecided: false})
	if len(v) != 1 || v[0].Rule != "require_approval" {
		t.Errorf("expected a require_approval violation, got %v", v)
	}

	// Approval decided: passes.
	v = s.Evaluate(Request{Domains: []string{"example.com"}, CertType: "ov", ApprovalDecided: true})
	if len(v) != 0 {
		t.Errorf("a decided approval should satisfy the rule, got %v", v)
	}

	// DV is not in the list: no approval needed regardless.
	v = s.Evaluate(Request{Domains: []string{"example.com"}, CertType: "dv", ApprovalDecided: false})
	if len(v) != 0 {
		t.Errorf("dv is not gated by this policy, got %v", v)
	}
}

func TestEvaluateMultiplePoliciesAllMustPass(t *testing.T) {
	s := NewStore(testLogger(), nil)
	s.Put(&Policy{Name: "no-wildcard", Enabled: true, Environment: "prod", WildcardAllowed: boolPtr(false)}, "a")
	s.Put(&Policy{Name: "398-days", Enabled: true, Environment: "prod", MaxValidityDays: 398}, "a")

	// Violates both.
	v := s.Evaluate(Request{Domains: []string{"*.example.com"}, Environment: "prod", ValidityYears: 2})
	if len(v) != 2 {
		t.Errorf("expected 2 violations from 2 failing policies, got %d: %v", len(v), v)
	}
}

func TestEvaluateNoPoliciesMeansNoViolations(t *testing.T) {
	s := NewStore(testLogger(), nil)
	v := s.Evaluate(Request{Domains: []string{"example.com"}, Environment: "prod"})
	if len(v) != 0 {
		t.Errorf("an empty policy store should never produce a violation, got %v", v)
	}
}

func TestContainsFoldCaseInsensitive(t *testing.T) {
	if !containsFold([]string{"DV", "OV"}, "dv") {
		t.Error("expected a case-insensitive match")
	}
	if containsFold([]string{"dv"}, "ev") {
		t.Error("expected no match for an absent value")
	}
}

func TestHasWildcard(t *testing.T) {
	if !hasWildcard([]string{"example.com", "*.example.com"}) {
		t.Error("expected true when any name is a wildcard")
	}
	if hasWildcard([]string{"example.com", "www.example.com"}) {
		t.Error("expected false when no name is a wildcard")
	}
}
