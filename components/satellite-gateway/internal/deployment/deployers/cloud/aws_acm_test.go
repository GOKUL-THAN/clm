package cloud

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/m2p/clm/internal/satellite/deployment/deployers"
)

const (
	listenerARN = "arn:aws:elasticloadbalancing:ap-south-1:111122223333:listener/app/alb/aaa/bbb"
	certARN     = "arn:aws:acm:ap-south-1:111122223333:certificate/1111-2222"
	otherARN    = "arn:aws:acm:ap-south-1:111122223333:certificate/3333-4444"
)

// --- plan resolution ---

func TestResolvePlanDefaults(t *testing.T) {
	// No listener: importing is all that can be done, so acm rather than a mode
	// that would immediately fail its own cross-field check.
	p, err := resolvePlan(&deployers.DeployTarget{Region: "ap-south-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.applyTo != ApplyACM {
		t.Errorf("apply_to = %q, want %q", p.applyTo, ApplyACM)
	}
	if p.versionPolicy != VersionNew {
		t.Errorf("version_policy = %q, want %q", p.versionPolicy, VersionNew)
	}
	if p.listenerPolicy != ListenerKeepPrevious {
		t.Errorf("listener_policy = %q, want %q", p.listenerPolicy, ListenerKeepPrevious)
	}

	// A listener implies the caller wants it served.
	p, err = resolvePlan(&deployers.DeployTarget{Region: "ap-south-1", ListenerARN: listenerARN})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.applyTo != ApplyBoth {
		t.Errorf("apply_to with a listener = %q, want %q", p.applyTo, ApplyBoth)
	}
}

func TestResolvePlanLegacyListenerFields(t *testing.T) {
	// The old schema documented resource_id as the listener ARN and the old code
	// read it from host. Requests exist in both shapes.
	for name, target := range map[string]*deployers.DeployTarget{
		"resource_id": {Region: "r", ResourceID: listenerARN},
		"host":        {Region: "r", Host: listenerARN},
		"listener_arn": {Region: "r", ListenerARN: listenerARN},
	} {
		p, err := resolvePlan(target)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if p.listenerARN != listenerARN {
			t.Errorf("%s: listenerARN = %q, want %q", name, p.listenerARN, listenerARN)
		}
	}
}

func TestResolvePlanCertificateARNInListenerField(t *testing.T) {
	// An ACM ARN in resource_id is the single most likely mistake, because the old
	// code passed resource_id to ImportCertificate. Treat it as the cert, not as a
	// listener that would fail deep inside ELBv2.
	p, err := resolvePlan(&deployers.DeployTarget{Region: "r", ResourceID: certARN})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.listenerARN != "" {
		t.Errorf("listenerARN = %q, want empty", p.listenerARN)
	}
	if p.certificateARN != certARN {
		t.Errorf("certificateARN = %q, want %q", p.certificateARN, certARN)
	}
	if p.applyTo != ApplyACM {
		t.Errorf("apply_to = %q, want %q", p.applyTo, ApplyACM)
	}
}

func TestResolvePlanRejectsImpossibleCombinations(t *testing.T) {
	cases := []struct {
		name   string
		target *deployers.DeployTarget
		want   string
	}{
		{"alb without listener",
			&deployers.DeployTarget{Region: "r", ApplyTo: ApplyALB},
			"needs listener_arn"},
		{"alb without certificate",
			&deployers.DeployTarget{Region: "r", ApplyTo: ApplyALB, ListenerARN: listenerARN},
			"certificate_arn is required"},
		{"both without listener",
			&deployers.DeployTarget{Region: "r", ApplyTo: ApplyBoth},
			"needs listener_arn"},
		{"replace without certificate",
			&deployers.DeployTarget{Region: "r", ApplyTo: ApplyACM, VersionPolicy: VersionReplace},
			"certificate_arn"},
		{"unknown apply_to",
			&deployers.DeployTarget{Region: "r", ApplyTo: "everywhere"},
			"apply_to must be"},
		{"unknown version_policy",
			&deployers.DeployTarget{Region: "r", VersionPolicy: "sometimes"},
			"version_policy must be"},
		{"unknown listener_policy",
			&deployers.DeployTarget{Region: "r", ListenerPolicy: "perhaps"},
			"listener_policy must be"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolvePlan(tc.target)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestResolvePlanAcceptsValidCombinations(t *testing.T) {
	cases := []*deployers.DeployTarget{
		{Region: "r", ApplyTo: ApplyACM},
		{Region: "r", ApplyTo: ApplyACM, VersionPolicy: VersionReplace, CertificateARN: certARN},
		{Region: "r", ApplyTo: ApplyALB, ListenerARN: listenerARN, CertificateARN: certARN},
		{Region: "r", ApplyTo: ApplyBoth, ListenerARN: listenerARN},
		{Region: "r", ApplyTo: ApplyBoth, ListenerARN: listenerARN, ListenerPolicy: ListenerReplace},
		{Region: "r", ApplyTo: ApplyBoth, ListenerARN: listenerARN, ListenerPolicy: ListenerAddOnly},
		{Region: "r", ApplyTo: ApplyBoth, ListenerARN: listenerARN,
			VersionPolicy: VersionReplace, CertificateARN: certARN},
	}
	for i, target := range cases {
		if _, err := resolvePlan(target); err != nil {
			t.Errorf("case %d: unexpected error: %v", i, err)
		}
	}
}

func TestResolvePlanNegativeRetainIsClamped(t *testing.T) {
	p, err := resolvePlan(&deployers.DeployTarget{Region: "r", RetainVersions: -5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.retain != 0 {
		t.Errorf("retain = %d, want 0", p.retain)
	}
}

// --- retention selection ---
//
// The consequence of a mistake here is a deleted certificate that something was
// relying on, so every branch is pinned.

func TestSelectForDeletion(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(arn string, ageDays int, inUse int) managedCert {
		return managedCert{arn: arn, importedAt: base.AddDate(0, 0, -ageDays), inUseBy: inUse}
	}

	t.Run("keeps the newest retain and deletes the rest", func(t *testing.T) {
		cands := []managedCert{
			mk("new", 0, 0), mk("v1", 1, 0), mk("v2", 2, 0), mk("v3", 3, 0), mk("v4", 4, 0),
		}
		doomed, inUse := selectForDeletion(cands, "new", 2)
		if got := strings.Join(doomed, ","); got != "v3,v4" {
			t.Errorf("doomed = %q, want \"v3,v4\"", got)
		}
		if len(inUse) != 0 {
			t.Errorf("inUse = %v, want none", inUse)
		}
	})

	t.Run("the just-deployed certificate is never deleted and does not use a slot", func(t *testing.T) {
		cands := []managedCert{mk("new", 0, 0), mk("v1", 1, 0), mk("v2", 2, 0)}
		doomed, _ := selectForDeletion(cands, "new", 2)
		if len(doomed) != 0 {
			t.Errorf("doomed = %v, want none: retain=2 should keep v1 and v2", doomed)
		}
	})

	t.Run("in-use certificates are skipped, not deleted", func(t *testing.T) {
		cands := []managedCert{mk("new", 0, 0), mk("v1", 1, 0), mk("v2", 2, 3), mk("v3", 3, 0)}
		doomed, inUse := selectForDeletion(cands, "new", 1)
		if got := strings.Join(doomed, ","); got != "v3" {
			t.Errorf("doomed = %q, want \"v3\"", got)
		}
		if got := strings.Join(inUse, ","); got != "v2" {
			t.Errorf("inUse = %q, want \"v2\"", got)
		}
	})

	t.Run("retain 0 deletes every superseded version", func(t *testing.T) {
		cands := []managedCert{mk("new", 0, 0), mk("v1", 1, 0), mk("v2", 2, 0)}
		doomed, _ := selectForDeletion(cands, "new", 0)
		if got := strings.Join(doomed, ","); got != "v1,v2" {
			t.Errorf("doomed = %q, want \"v1,v2\"", got)
		}
	})

	t.Run("a single certificate is never deleted", func(t *testing.T) {
		doomed, _ := selectForDeletion([]managedCert{mk("new", 0, 0)}, "new", 0)
		if len(doomed) != 0 {
			t.Errorf("doomed = %v, want none", doomed)
		}
	})

	t.Run("order does not depend on the listing order", func(t *testing.T) {
		a := []managedCert{mk("v2", 2, 0), mk("new", 0, 0), mk("v1", 1, 0), mk("v3", 3, 0)}
		b := []managedCert{mk("v3", 3, 0), mk("v1", 1, 0), mk("v2", 2, 0), mk("new", 0, 0)}
		da, _ := selectForDeletion(a, "new", 1)
		db, _ := selectForDeletion(b, "new", 1)
		if strings.Join(da, ",") != strings.Join(db, ",") {
			t.Errorf("order-dependent result: %v vs %v", da, db)
		}
	})

	t.Run("equal timestamps break deterministically", func(t *testing.T) {
		same := []managedCert{
			{arn: "bbb", importedAt: base}, {arn: "aaa", importedAt: base},
			{arn: "ccc", importedAt: base},
		}
		first, _ := selectForDeletion(same, "", 1)
		second, _ := selectForDeletion([]managedCert{same[2], same[0], same[1]}, "", 1)
		if strings.Join(first, ",") != strings.Join(second, ",") {
			t.Errorf("non-deterministic on equal timestamps: %v vs %v", first, second)
		}
	})
}

// --- PEM handling ---

func testCert(t *testing.T, cn string) (certPEM []byte, der []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     []string{cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), der
}

func TestLeafPEMTakesOnlyTheFirstCertificate(t *testing.T) {
	leaf, leafDER := testCert(t, "leaf.example.com")
	inter, _ := testCert(t, "intermediate.example.com")
	fullchain := append(append([]byte{}, leaf...), inter...)

	got := leafPEM(fullchain)
	if len(certificateBlocks(got)) != 1 {
		t.Fatalf("expected exactly one certificate, got %d", len(certificateBlocks(got)))
	}
	if !bytesEqual(certificateBlocks(got)[0], leafDER) {
		t.Error("leafPEM did not return the first certificate")
	}
}

func TestLeafPEMIgnoresNonCertificateBlocks(t *testing.T) {
	leaf, leafDER := testCert(t, "leaf.example.com")
	// A key ahead of the certificate must not be parsed as one.
	keyBlock := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("not a key")})
	got := leafPEM(append(keyBlock, leaf...))
	if len(got) == 0 {
		t.Fatal("expected the certificate to be found past the key block")
	}
	if !bytesEqual(certificateBlocks(got)[0], leafDER) {
		t.Error("wrong block returned")
	}
}

func TestLeafPEMOnGarbage(t *testing.T) {
	if got := leafPEM([]byte("not pem at all")); got != nil {
		t.Errorf("expected nil for non-PEM input, got %q", got)
	}
}

func TestChainWithoutLeafDropsTheLeaf(t *testing.T) {
	leaf, _ := testCert(t, "leaf.example.com")
	inter, interDER := testCert(t, "intermediate.example.com")
	fullchain := append(append([]byte{}, leaf...), inter...)

	// ACM refuses a chain that starts with the certificate being imported, and a
	// fullchain is the usual thing callers have.
	chain := chainWithoutLeaf(fullchain, leaf)
	blocks := certificateBlocks(chain)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 intermediate, got %d", len(blocks))
	}
	if !bytesEqual(blocks[0], interDER) {
		t.Error("the remaining certificate is not the intermediate")
	}
}

func TestChainWithoutLeafEmptyWhenChainIsOnlyTheLeaf(t *testing.T) {
	leaf, _ := testCert(t, "leaf.example.com")
	// The deploy handler sets cert_pem and chain_pem to the same fullchain for a
	// gateway-issued certificate, so this is the common case, and an empty result
	// must mean "send no chain" rather than "send an empty chain", which ACM
	// rejects as a validation error.
	if got := chainWithoutLeaf(leaf, leaf); len(got) != 0 {
		t.Errorf("expected an empty chain, got %d bytes", len(got))
	}
}

func TestLeafDomainAndFingerprint(t *testing.T) {
	leaf, leafDER := testCert(t, "Mixed.Case.Example.COM")
	if got := leafDomain(leaf); got != "mixed.case.example.com" {
		t.Errorf("leafDomain = %q, want lowercased CN", got)
	}
	fp := leafFingerprint(leaf)
	if len(fp) != 64 {
		t.Errorf("fingerprint = %q, want 64 hex chars", fp)
	}
	// Same bytes must give the same fingerprint, since retention and dedup rely on it.
	again := leafFingerprint(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}))
	if fp != again {
		t.Errorf("fingerprint not stable: %q vs %q", fp, again)
	}
	if leafDomain([]byte("garbage")) != "" || leafFingerprint([]byte("garbage")) != "" {
		t.Error("expected empty results for non-PEM input")
	}
}

func TestShortARN(t *testing.T) {
	if got := shortARN(certARN); got != "1111-2222" {
		t.Errorf("shortARN = %q, want \"1111-2222\"", got)
	}
	if got := shortARN(""); got != "(none)" {
		t.Errorf("shortARN(\"\") = %q, want \"(none)\"", got)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
