package connector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/m2p/clm/internal/satellite/securefs"
	"go.uber.org/zap"
)

func TestProductionConnectorFailsClosed(t *testing.T) {
	base := Config{SatelliteID: "00000000-0000-0000-0000-000000000001", SaaSEndpoint: "https://clm.example.test", Environment: "production"}
	if _, err := New(&base, zap.NewNop()); err == nil || !strings.Contains(err.Error(), "requires server CA") {
		t.Fatalf("missing mTLS material error=%v", err)
	}
	base.SaaSEndpoint = "http://clm.example.test"
	if _, err := New(&base, zap.NewNop()); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("HTTP production endpoint error=%v", err)
	}
	base.SaaSEndpoint = "https://clm.example.test"
	base.InsecureSkipVerify = true
	if _, err := New(&base, zap.NewNop()); err == nil || !strings.Contains(err.Error(), "insecure TLS") {
		t.Fatalf("insecure TLS error=%v", err)
	}
}

func TestDevelopmentHTTPRequiresExplicitEnvironment(t *testing.T) {
	cfg := Config{SatelliteID: "00000000-0000-0000-0000-000000000001", SaaSEndpoint: "http://localhost:8080", Environment: "test"}
	conn, err := New(&cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if conn == nil {
		t.Fatal("connector is nil")
	}
}

func TestConnectorRejectsTLSFilesOutsideApprovedRoots(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("secure filesystem policy tests require Linux ownership and mode semantics")
	}
	root := t.TempDir()
	approvedRoot := filepath.Join(root, "approved")
	if err := os.Mkdir(approvedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	policy, err := securefs.NewPolicy([]string{approvedRoot}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer policy.Close()

	caPath := filepath.Join(root, "outside-ca.pem")
	if err := os.WriteFile(caPath, []byte("not a certificate"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		SatelliteID:  "00000000-0000-0000-0000-000000000001",
		SaaSEndpoint: "https://clm.example.test",
		TLSCA:        caPath,
		FilePolicy:   policy,
	}
	if _, err := New(&cfg, zap.NewNop()); err == nil || !strings.Contains(err.Error(), "outside the configured roots") {
		t.Fatalf("outside CA bundle error = %v, want policy denial", err)
	}
}

func TestConnectorRequiresPolicyForTLSFiles(t *testing.T) {
	cfg := Config{
		SatelliteID:  "00000000-0000-0000-0000-000000000001",
		SaaSEndpoint: "https://clm.example.test",
		TLSCA:        filepath.Join(t.TempDir(), "ca.pem"),
	}
	if _, err := New(&cfg, zap.NewNop()); err == nil || !strings.Contains(err.Error(), "file storage policy is required") {
		t.Fatalf("missing file policy error = %v, want explicit policy error", err)
	}
}

func TestBoundResultRequestLimitsSerializedEnvelope(t *testing.T) {
	body := &resultRequest{
		LeaseToken: "00000000-0000-0000-0000-000000000001",
		Status:     "failed",
		Result:     strings.Repeat("quoted\"\n\\value", 80) + "世界",
		Error:      strings.Repeat("control\tcharacter", 40),
		Retryable:  true,
	}
	const maximum = 512
	if err := boundResultRequest(body, maximum); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > maximum {
		t.Fatalf("serialized request is %d bytes, limit is %d", len(encoded), maximum)
	}
	if !utf8.ValidString(body.Result) || !utf8.ValidString(body.Error) {
		t.Fatal("result bounding split a UTF-8 code point")
	}
	if body.LeaseToken == "" || body.Status != "failed" {
		t.Fatalf("result bounding altered envelope metadata: %+v", body)
	}
}

func TestBoundResultRequestRejectsLimitBelowEnvelope(t *testing.T) {
	body := &resultRequest{LeaseToken: "00000000-0000-0000-0000-000000000001", Status: "succeeded"}
	if err := boundResultRequest(body, 8); err == nil {
		t.Fatal("limit smaller than the fixed envelope was accepted")
	}
}

func TestTruncateUTF8PreservesCodePoints(t *testing.T) {
	for maximum := 1; maximum < len("a世界b"); maximum++ {
		if got := truncateUTF8("a世界b", maximum); !utf8.ValidString(got) || len(got) > maximum {
			t.Fatalf("truncateUTF8 maximum=%d returned %q", maximum, got)
		}
	}
}
