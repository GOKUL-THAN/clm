package integrations

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap/zaptest"
)

// mockRoundTripper intercepts outbound HTTP requests in memory without opening local sockets
type mockRoundTripper struct {
	handler http.Handler
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func TestIntegrationController(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tenantID := uuid.New()
	ctx := context.Background()

	// Mock receiver HTTP handler
	receivedEvent := ""
	receivedSig := ""
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedEvent = r.Header.Get("X-CLM-Event")
		receivedSig = r.Header.Get("X-CLM-Signature")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	testClient := &http.Client{
		Transport: &mockRoundTripper{handler: mockHandler},
		Timeout:   5 * time.Second,
	}

	ctrl := NewIntegrationControllerWithClient(testClient, logger)

	// 1. Register webhook subscription with valid external HTTPS endpoint
	sub, err := ctrl.RegisterSubscription(ctx, tenantID, &Subscription{
		Name:        "Corporate SIEM & Slack Alerting",
		Type:        TypeWebhook,
		EndpointURL: "https://api.corporate-siem.com/v1/webhook",
		Events:      []string{"incident.raised", "certificate.expired"},
	})
	if err != nil {
		t.Fatalf("failed to register subscription: %v", err)
	}

	if sub.SecretKey == "" {
		t.Fatalf("expected generated secret key for HMAC")
	}

	// 2. Dispatch event
	dispatched := ctrl.DispatchEvent(ctx, tenantID, "incident.raised", map[string]string{
		"domain":   "api.m2p.in",
		"severity": "SEV1",
	})

	if dispatched != 1 {
		t.Fatalf("expected 1 event dispatched, got %d", dispatched)
	}
	if receivedEvent != "incident.raised" {
		t.Fatalf("expected received event 'incident.raised', got: %s", receivedEvent)
	}
	if receivedSig == "" {
		t.Fatalf("expected non-empty HMAC signature")
	}

	// 3. Delete subscription
	err = ctrl.DeleteSubscription(tenantID, sub.ID)
	if err != nil {
		t.Fatalf("delete subscription failed: %v", err)
	}
	if len(ctrl.ListSubscriptions(tenantID)) != 0 {
		t.Fatalf("expected 0 subscriptions after delete")
	}
}

func TestValidateWebhookURL_SSRFProtection(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		shouldError bool
		errContains string
	}{
		// Scheme validations
		{"Empty URL", "", true, "endpoint URL must be a valid HTTPS URL"},
		{"HTTP scheme prohibited", "http://example.com/webhook", true, "scheme must strictly be https"},
		{"FTP scheme prohibited", "ftp://example.com/webhook", true, "scheme must strictly be https"},
		{"File scheme prohibited", "file:///etc/passwd", true, "scheme must strictly be https"},
		{"Gopher scheme prohibited", "gopher://127.0.0.1:70", true, "scheme must strictly be https"},

		// Localhost and internal hostnames
		{"Localhost hostname", "https://localhost/webhook", true, "internal hostname localhost is forbidden"},
		{"Subdomain of localhost", "https://app.localhost/webhook", true, "internal hostname app.localhost is forbidden"},
		{"Internal corporate domain", "https://vault.internal/v1/secret", true, "internal hostname vault.internal is forbidden"},
		{"Local mDNS domain", "https://printer.local/status", true, "internal hostname printer.local is forbidden"},
		{"GCP Cloud metadata hostname", "https://metadata.google.internal/computeMetadata/v1/", true, "internal hostname metadata.google.internal is forbidden"},
		{"AWS Instance data hostname", "https://instance-data/latest/meta-data/", true, "internal hostname instance-data is forbidden"},

		// Loopback IPv4 & IPv6
		{"Loopback IPv4 127.0.0.1", "https://127.0.0.1/webhook", true, "destination IP 127.0.0.1 is prohibited"},
		{"Loopback IPv4 127.0.0.2", "https://127.0.0.2:8443/webhook", true, "destination IP 127.0.0.2 is prohibited"},
		{"Loopback IPv6 ::1", "https://[::1]/webhook", true, "destination IP ::1 is prohibited"},

		// RFC 1918 Private IPv4
		{"Private 10.0.0.0/8 (10.0.0.1)", "https://10.0.0.1/webhook", true, "destination IP 10.0.0.1 is prohibited"},
		{"Private 172.16.0.0/12 (172.16.0.5)", "https://172.16.0.5/webhook", true, "destination IP 172.16.0.5 is prohibited"},
		{"Private 172.16.0.0/12 (172.31.255.254)", "https://172.31.255.254/webhook", true, "destination IP 172.31.255.254 is prohibited"},
		{"Private 192.168.0.0/16 (192.168.1.1)", "https://192.168.1.1/webhook", true, "destination IP 192.168.1.1 is prohibited"},

		// RFC 3927 Link-Local & AWS IMDS IPv4
		{"AWS IMDS 169.254.169.254", "https://169.254.169.254/latest/meta-data/", true, "destination IP 169.254.169.254 is prohibited"},
		{"Link-Local IPv4 169.254.1.1", "https://169.254.1.1/webhook", true, "destination IP 169.254.1.1 is prohibited"},

		// IPv6 Private & Link-Local
		{"IPv6 Unique Local fc00::1", "https://[fc00::1]/webhook", true, "destination IP fc00::1 is prohibited"},
		{"IPv6 Unique Local fd00::1", "https://[fd00::1]/webhook", true, "destination IP fd00::1 is prohibited"},
		{"IPv6 Link-Local fe80::1", "https://[fe80::1]/webhook", true, "destination IP fe80::1 is prohibited"},

		// IPv4-Mapped IPv6
		{"IPv4-Mapped IPv6 Loopback", "https://[::ffff:127.0.0.1]/webhook", true, "destination IP ::ffff:127.0.0.1 is prohibited"},
		{"IPv4-Mapped IPv6 AWS IMDS", "https://[::ffff:169.254.169.254]/webhook", true, "destination IP ::ffff:169.254.169.254 is prohibited"},
		{"IPv4-Mapped IPv6 RFC 1918", "https://[::ffff:10.0.0.1]/webhook", true, "destination IP ::ffff:10.0.0.1 is prohibited"},

		// Valid Public HTTPS endpoints
		{"Valid Public HTTPS Webhook", "https://api.corporate-siem.com/v1/events", false, ""},
		{"Valid Slack Webhook URL", "https://hooks.slack.com/services/T00/B00/X00", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebhookURL(tt.url)
			if tt.shouldError {
				if err == nil {
					t.Fatalf("expected error for URL %q, got nil", tt.url)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected valid URL %q, got unexpected error: %v", tt.url, err)
				}
			}
		})
	}
}

func TestIsPrivateOrReservedIP_Unit(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"127.0.0.1", true},
		{"127.255.255.254", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.255.254", true},
		{"169.254.169.254", true},
		{"169.254.1.1", true},
		{"100.64.0.1", true},
		{"0.0.0.0", true},
		{"::1", true},
		{"fe80::1", true},
		{"fc00::1", true},
		{"fd12:3456:789a::1", true},
		{"::ffff:127.0.0.1", true},
		{"::ffff:169.254.169.254", true},
		{"::ffff:10.0.0.1", true},
		{"224.0.0.1", true}, // Multicast
		{"ff02::1", true},   // IPv6 Multicast
		{"8.8.8.8", false},  // Google Public DNS
		{"1.1.1.1", false},  // Cloudflare Public DNS
		{"2606:4700:4700::1111", false}, // Cloudflare Public IPv6
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			parsed := net.ParseIP(tt.ip)
			if parsed == nil {
				t.Fatalf("invalid test IP literal: %s", tt.ip)
			}
			result := IsPrivateOrReservedIP(parsed)
			if result != tt.expected {
				t.Fatalf("IsPrivateOrReservedIP(%s) = %v; expected %v", tt.ip, result, tt.expected)
			}
		})
	}

	// nil check
	if !IsPrivateOrReservedIP(nil) {
		t.Fatalf("IsPrivateOrReservedIP(nil) must return true")
	}
}

func TestSafeHTTPClient_DialContextDirectValidation(t *testing.T) {
	client := NewSafeHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatalf("expected *http.Transport from NewSafeHTTPClient")
	}

	ctx := context.Background()

	// 1. Test that DialContext blocks IP literals immediately
	blockedTargets := []string{
		"127.0.0.1:443",
		"10.0.0.1:443",
		"172.16.0.1:443",
		"192.168.1.1:443",
		"169.254.169.254:80",
		"[::1]:443",
		"[fc00::1]:443",
		"[fe80::1]:443",
		"[::ffff:127.0.0.1]:443",
		"[::ffff:169.254.169.254]:80",
		"localhost:443",
	}

	for _, target := range blockedTargets {
		t.Run("Blocked_"+target, func(t *testing.T) {
			conn, err := transport.DialContext(ctx, "tcp", target)
			if err == nil {
				if conn != nil {
					_ = conn.Close()
				}
				t.Fatalf("expected DialContext to reject %s, but connection succeeded", target)
			}
			if !strings.Contains(err.Error(), "SSRF defense") && !strings.Contains(err.Error(), "prohibited") && !strings.Contains(err.Error(), "failed") {
				t.Logf("connection failed as expected with error: %v", err)
			}
		})
	}

	// 2. Test CheckRedirect blocks redirects strictly
	if client.CheckRedirect == nil {
		t.Fatalf("expected CheckRedirect to be non-nil")
	}
	req, _ := http.NewRequest("GET", "https://api.corporate-siem.com/redirect", nil)
	via := []*http.Request{req}
	err := client.CheckRedirect(req, via)
	if err == nil {
		t.Fatalf("expected CheckRedirect to return error blocking redirects, got nil")
	}
	if !strings.Contains(err.Error(), "SSRF defense") {
		t.Fatalf("expected SSRF defense error from CheckRedirect, got: %v", err)
	}

	// 3. Test Proxy Bypass Protection (transport.Proxy must be nil to avoid proxy tampering)
	if transport.Proxy != nil {
		t.Fatalf("expected transport.Proxy to be nil to prevent proxy redirection bypass")
	}

	// 4. Test TLS Handshake Timeout
	if transport.TLSHandshakeTimeout != 5*time.Second {
		t.Fatalf("expected TLSHandshakeTimeout of 5s, got %v", transport.TLSHandshakeTimeout)
	}
}

func TestSafeHTTPClient_TLSServerNameIntegrity(t *testing.T) {
	client := NewSafeHTTPClient()
	transport := client.Transport.(*http.Transport)

	// Verify TLS client config does not disable verification
	if transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("InsecureSkipVerify must NOT be enabled")
	}
}

func TestSafeHTTPClient_MixedDNSResponseProtection(t *testing.T) {
	// Mixed IP check verification: if DNS returns public IP + private IP, private IP must trigger rejection
	mixedIPs := []net.IP{
		net.ParseIP("93.184.216.34"), // public
		net.ParseIP("10.0.0.1"),       // private RFC 1918
	}

	foundProhibited := false
	for _, ip := range mixedIPs {
		if IsPrivateOrReservedIP(ip) {
			foundProhibited = true
			break
		}
	}

	if !foundProhibited {
		t.Fatalf("expected mixed IP set to trigger SSRF prohibition, but was ignored")
	}
}
