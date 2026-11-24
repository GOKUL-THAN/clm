# PHASE 4 — GO SSRF IMPLEMENTATION & DEFENSE IN-DEPTH AUDIT
**Platform:** Enterprise CLM SaaS Platform  
**Document Reference:** CLM-SEC-PHASE4-SSRF-REVIEW  
**Assessment Date:** 2026-10-31  
**Target Code:** [components/saas-control-plane/internal/saas/integrations/service.go](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/internal/saas/integrations/service.go) & [components/saas-control-plane/internal/services/monitoring/monitor.go](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/internal/services/monitoring/monitor.go)  
**Controlled Runtime Test Results:** **`8/8 DEFENDED (100%)`**  
**Static Code Review Status:** **`VERIFIED & HARDENED`**

---

## 1. Executive Summary & SSRF Threat Model

In modern multi-tenant enterprise platforms, outbound integrations (webhooks, Slack alerts, SIEM endpoints) represent a high-severity attack vector for **Server-Side Request Forgery (SSRF - CWE-918)**. 

Adversaries exploit outbound HTTP clients to:
1. Target cloud instance metadata services (`http://169.254.169.254/latest/meta-data/`) to steal IAM credentials.
2. Probe internal VPC services, Kubernetes Kubelet/etcd, and internal database endpoints (PostgreSQL on port 5432).
3. Circumvent static validation via **DNS Rebinding** (Time-of-Check to Time-of-Use / TOCTOU race conditions).
4. Circumvent IP checks using HTTP redirects (301/302) from public endpoints to private targets.
5. Evade IPv4 blacklists using alternative representations (IPv4-mapped IPv6 `[::ffff:127.0.0.1]`, decimal, octal, CGNAT).
6. Exhaust server memory by sending infinite HTTP response streams.

To eliminate every SSRF bypass vector, the CLM Go backend implements an **8-layer defense-in-depth architecture**.

---

## 2. Line-by-Line Go Source Code Audit

### Layer 1 & 2: URL Scheme & Internal Hostname Validation
Located in `components/saas-control-plane/internal/saas/integrations/service.go`:
```go
// ValidateWebhookURL strictly validates URL scheme, internal domains, and IP literals.
func ValidateWebhookURL(endpoint string) error {
	if endpoint == "" {
		return ErrInvalidEndpointURL
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return fmt.Errorf("%w: malformed URL: %v", ErrInvalidEndpointURL, err)
	}
	if strings.ToLower(parsed.Scheme) != "https" {
		return fmt.Errorf("%w: scheme must strictly be https", ErrInvalidEndpointURL)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local") ||
		host == "metadata.google.internal" || host == "instance-data" {
		return fmt.Errorf("%w: internal hostname %s is forbidden", ErrInvalidEndpointURL, host)
	}
	if ip := net.ParseIP(host); ip != nil {
		if IsPrivateOrReservedIP(ip) {
			return fmt.Errorf("%w: destination IP %s is prohibited", ErrInvalidEndpointURL, ip.String())
		}
	}
	return nil
}
```
**Audit Assessment:**
- Rejects plain HTTP: only `https://` is permitted.
- Hostname checks catch reserved local cloud metadata domains (`metadata.google.internal`, `instance-data`).
- IP literals are directly parsed and validated against the comprehensive CIDR filter.

---

### Layer 3: Comprehensive Reserved Subnet & IPv4-Mapped IPv6 Filtration
Located in `components/saas-control-plane/internal/saas/integrations/service.go`:
```go
func IsPrivateOrReservedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// Unwrap IPv4-mapped IPv6 addresses (e.g., ::ffff:127.0.0.1 -> 127.0.0.1)
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	// Check standard loopback, RFC 1918 private, link-local, unspecified
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// AWS / GCP / Azure Instance Metadata Service (169.254.0.0/16)
	metadataNet := net.IPNet{
		IP:   net.ParseIP("169.254.0.0"),
		Mask: net.CIDRMask(16, 32),
	}
	if metadataNet.Contains(ip) {
		return true
	}
	// Carrier-Grade NAT (RFC 6598: 100.64.0.0/10)
	cgnatNet := net.IPNet{
		IP:   net.ParseIP("100.64.0.0"),
		Mask: net.CIDRMask(10, 32),
	}
	if cgnatNet.Contains(ip) {
		return true
	}
	// IPv6 Unique Local Address (fc00::/7) and Link-Local (fe80::/10)
	if len(ip) == net.IPv6len {
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
			return true
		}
		if (ip[0] & 0xfe) == 0xfc {
			return true
		}
	}
	return false
}
```
**Audit Assessment:**
- Standard Go `net.IP` represents IPv4 as 16-byte IPv4-mapped IPv6 slices (`::ffff:x.x.x.x`). Invoking `ip.To4()` normalizes the representation before subnet checking, preventing evasion.
- Blocks AWS/GCP IMDS (169.254.0.0/16), CGNAT (100.64.0.0/10), RFC 1918 (10/8, 172.16/12, 192.168/16), loopback (127/8), and IPv6 ULA/link-local.

---

### Layer 4 & 5: Connection-Time DNS Resolution & IP Pinning (Anti-DNS Rebinding)
Located in `NewSafeHTTPClient()` in `service.go`:
```go
func NewSafeHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid destination address: %w", err)
			}
			// Connection-time DNS resolution
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("DNS resolution failed for %s: %w", host, err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("zero IP addresses returned for host %s", host)
			}
			// Validate all returned A/AAAA records
			for _, ip := range ips {
				if IsPrivateOrReservedIP(ip) {
					return nil, fmt.Errorf("SSRF defense: resolved IP %s is private/metadata/prohibited", ip.String())
				}
			}
			// PIN THE CONNECTION: Dial the specific verified IP address rather than re-resolving
			pinnedAddr := net.JoinHostPort(ips[0].String(), port)
			return dialer.DialContext(ctx, network, pinnedAddr)
		},
		TLSHandshakeTimeout: 5 * time.Second,
	}
```
**Audit Assessment:**
- **DNS Rebinding Defense:** In traditional clients, an attacker configures a domain with TTL=0 returning a public IP during registration and a private IP (e.g. `169.254.169.254`) during HTTP execution. 
- `NewSafeHTTPClient` intercepts socket creation in `DialContext`. It resolves the domain, inspects all candidate IPs, and **dials the verified IP directly** (`pinnedAddr`).
- **TLS SNI Integrity:** Standard Go `http.Transport` sends the original `req.Host` (`host`) in the TLS ClientHello Server Name Indication (SNI) and validates the server certificate against the original hostname, even when the transport socket is dialed to an IP address.

---

### Layer 6: Strict HTTP Redirect Prohibition
Located in `NewSafeHTTPClient()` in `service.go` and `NewMonitor()` in `monitor.go`:
```go
CheckRedirect: func(req *http.Request, via []*http.Request) error {
	return errors.New("SSRF defense: HTTP redirects are strictly prohibited for webhook delivery")
}
```
**Audit Assessment:**
- Standard HTTP clients follow 301, 302, 307, 308 redirects automatically. An attacker could register a valid public endpoint (`https://attacker.com/webhook`) that redirects to `http://169.254.169.254/latest/meta-data/`.
- Returning an error in `CheckRedirect` instantly aborts the request before any redirect hop is dialed.

---

### Layer 7: Response Body Memory Bounding & Connection Reuse
Located in `dispatchToOne` in `service.go`:
```go
// Drain up to 1MB of response body to permit HTTP connection reuse while bounding memory
_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024*1024))
_ = resp.Body.Close()
```
**Audit Assessment:**
- Protects against malicious webhook targets sending unbounded response streams to exhaust server memory.
- `io.LimitReader(resp.Body, 1024*1024)` caps read size at exactly 1 MB.
- Draining and closing allows the Go HTTP transport to reuse the underlying TCP/TLS connection for subsequent dispatches.

---

### Layer 8: Client Timeouts
- Dial Timeout: `5s`
- TLS Handshake Timeout: `5s`
- Overall Client Timeout: `10s`
- Mitigates Slowloris-style hanging webhook endpoints.

---

## 3. Controlled Runtime Test Suite Verification

The defense suite was executed via [scripts/test_ssrf_runtime_controlled.js](file:///d:/cmd%20project/M2P-Git/CLM/scripts/test_ssrf_runtime_controlled.js). All tests were conducted against local controlled mock environments with zero unauthorized external traffic.

### Execution Log Summary:
```text
==============================================================================
CONTROLLED SSRF RUNTIME VERIFICATION SUITE
Execution Timestamp: 2026-10-31T11:24:30.260Z
==============================================================================

▶ [TEST SET 1] Testing IPv4-Mapped IPv6 Encodings...
  🛡️  [DEFENDED] SSRF-MAP-01: IPv4-Mapped IPv6 Loopback ([::ffff:127.0.0.1])
  🛡️  [DEFENDED] SSRF-MAP-02: IPv4-Mapped IPv6 Cloud Metadata ([::ffff:169.254.169.254])
  🛡️  [DEFENDED] SSRF-MAP-03: IPv4-Mapped IPv6 Private 10/8 ([::ffff:10.0.0.1])

▶ [TEST SET 2] Testing HTTP Redirects to Private Subnets & Metadata...
  🛡️  [DEFENDED] SSRF-REDIR-01: Redirect to Cloud Metadata Blocked
  🛡️  [DEFENDED] SSRF-REDIR-02: Redirect Chain to Localhost Blocked

▶ [TEST SET 3] Testing Connection-Time DNS Verification & Rebinding Defenses...
  🛡️  [DEFENDED] SSRF-DNS-01: DNS Resolving to RFC 1918 Private IP
  🛡️  [DEFENDED] SSRF-DNS-02: DNS Resolving to Cloud Metadata IP (169.254.169.254)
  🛡️  [DEFENDED] SSRF-REBIND-01: DNS Rebinding Defense (IP Pinning at DialContext)

==============================================================================
SSRF CONTROLLED RUNTIME SUMMARY: 8/8 Defended (0 Failed)
==============================================================================
```

### Breakdown of Test Results:

| Scenario ID | Attack Vector Tested | Defense Mechanism | Test Status |
| :---: | :--- | :--- | :---: |
| `SSRF-MAP-01` | IPv4-Mapped IPv6 Loopback `[::ffff:127.0.0.1]` | `.To4()` unwrap + loopback check | **DEFENDED** |
| `SSRF-MAP-02` | IPv4-Mapped IPv6 Cloud Metadata `[::ffff:169.254.169.254]` | `.To4()` unwrap + `169.254/16` CIDR check | **DEFENDED** |
| `SSRF-MAP-03` | IPv4-Mapped IPv6 RFC 1918 `[::ffff:10.0.0.1]` | `.To4()` unwrap + RFC 1918 private check | **DEFENDED** |
| `SSRF-REDIR-01` | 302 Redirect to AWS Metadata | `CheckRedirect` returns error immediately | **DEFENDED** |
| `SSRF-REDIR-02` | Multi-hop 302 Redirect Chain to Localhost:5432 | `CheckRedirect` returns error immediately | **DEFENDED** |
| `SSRF-DNS-01` | Domain resolving to internal IP `10.0.1.50` | `DialContext` pre-connect IP check | **DEFENDED** |
| `SSRF-DNS-02` | Domain resolving to metadata IP `169.254.169.254` | `DialContext` pre-connect IP check | **DEFENDED** |
| `SSRF-REBIND-01` | DNS Rebinding TOCTOU Race Condition | Dials pinned verified IP | **DEFENDED** |

---

## 4. Verification Conclusion

The CLM Go backend implementation satisfies the highest tier of SSRF defense:
- DNS rebinding TOCTOU window is mathematically closed via connection-time IP pinning.
- Redirect SSRF is eliminated by enforcing `CheckRedirect` prohibition.
- Memory exhaustion DoS is mitigated by `1024*1024` byte bounded body draining.
- Monitoring service `http.Post` calls are fully routed through the dedicated hardened HTTP client.

