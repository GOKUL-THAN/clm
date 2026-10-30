package connector

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

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
