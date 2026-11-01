# CLM — INTEGRATION SECURITY & OUTBOUND WEBHOOK SPECIFICATION
**Platform:** Enterprise Contract & Certificate Lifecycle Management (CLM) SaaS Platform  
**Document Reference:** CLM-INT-SECURITY-2026.1  
**Assessment Date:** 2026-10-31  
**Author:** Principal Application Security Engineer & Enterprise Integration Architect  
**Classification:** Enterprise Transport Security Standard  

---

## 1. Executive Summary

Enterprise CLM systems exchange commercial events, invoice records, and employee rosters with external enterprise systems (Salesforce, SAP S/4HANA, Workday, Coupa, ServiceNow, Slack, Microsoft Teams).

Integrations present dual attack surfaces:
1. **Inbound Webhooks / REST APIs:** Vulnerable to spoofing, replay attacks, parameter tampering, and cross-tenant leakage.
2. **Outbound Webhooks / Event Dispatch:** Vulnerable to **Server-Side Request Forgery (SSRF - CWE-918)**, DNS rebinding, internal VPC port scanning, cloud metadata theft (`169.254.169.254`), and memory exhaustion DoS.

This specification provides the end-to-end security architecture covering both inbound API validation and the **8-layer outbound SSRF defense engine** implemented in the Go backend.

---

## 2. Inbound Integration Security Controls

```
[External Enterprise System (Salesforce / SAP)]
                     │
                     ▼ (HTTPS POST /api/v1/integrations/inbound)
        ┌─────────────────────────────┐
        │ 1. Header & Scheme Check    │
        │    - TLS 1.3 Strict HTTPS   │
        └──────────────┬──────────────┘
                       │
        ┌──────────────▼──────────────┐
        │ 2. Timestamp Window (300s)  │ ──► [Fail: 401 Replay Window Expired]
        │    - |Now - X-CLM-Time| < 5m│
        └──────────────┬──────────────┘
                       │
        ┌──────────────▼──────────────┐
        │ 3. HMAC-SHA256 Signature    │ ──► [Fail: 401 Invalid Signature]
        │    - Compare Constant-Time  │
        └──────────────┬──────────────┘
                       │
        ┌──────────────▼──────────────┐
        │ 4. Idempotency Nonce Cache  │ ──► [Duplicate: 200 Cached Response]
        │    - X-Idempotency-Key Check│
        └──────────────┬──────────────┘
                       │
        ┌──────────────▼──────────────┐
        │ 5. Tenant Isolation Context │
        │    - Bind to Authenticated  │
        │      Tenant UUID via JWT/API│
        └─────────────────────────────┘
```

### 2.1 Cryptographic HMAC-SHA256 Verification
Every inbound webhook payload must provide an `X-CLM-Signature` calculated across the request timestamp and raw body bytes using the tenant-shared integration secret:
$$\text{ExpectedSig} = \text{HMAC-SHA256}(\text{TenantSecret}, \text{X-CLM-Timestamp} \parallel "." \parallel \text{RawBodyBytes})$$
* **Constant-Time Comparison:** The comparison between the received signature and the calculated signature is executed using `crypto/subtle.ConstantTimeCompare` (or Node `crypto.timingSafeEqual`) to prevent timing side-channel attacks.

### 2.2 Replay Protection & Timestamp Tolerance
* All incoming requests must supply `X-CLM-Timestamp` in ISO 8601 / RFC 3339 format.
* **Tolerance Window:** The system verifies $|\text{ServerTime} - \text{RequestTimestamp}| \le 300\text{ seconds}$ (5 minutes). Requests outside this temporal window are rejected with `HTTP 401 Unauthorized (Replay window expired)`.

### 2.3 Idempotency & Nonce De-duplication
* External systems submit unique identifiers via the `Idempotency-Key` or `X-Request-ID` HTTP header.
* Keys are cached in Redis (`idempotency:{tenant_id}:{key}`) with a 24-hour TTL.
* Repeated submissions return the previously computed HTTP status and response body without re-executing state machine transitions.

### 2.4 Tenant Context Enforcement
* Webhook receiver endpoints inspect the authenticated API token or tenant secret.
* Context injection strictly binds the operation to the target `tenant_id`. Submitting cross-tenant payload parameters results in immediate `HTTP 403 Forbidden` rejection.

---

## 3. Outbound Webhook Security & 8-Layer SSRF Defense

Outbound webhook dispatch is implemented in [components/saas-control-plane/internal/saas/integrations/service.go](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/internal/saas/integrations/service.go) and [components/saas-control-plane/internal/services/monitoring/monitor.go](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/internal/services/monitoring/monitor.go).

### The 8 Defense Layers:

| Layer # | Security Control | Threat Mitigated | Implementation Code Reference |
| :---: | :--- | :--- | :--- |
| **1** | **Strict HTTPS Scheme Enforcement** | Cleartext traffic interception, plain HTTP probing | `ValidateWebhookURL`: `strings.ToLower(parsed.Scheme) != "https"` |
| **2** | **Forbidden Internal Hostnames** | Cloud metadata / cluster probing | Rejects `localhost`, `*.internal`, `*.local`, `metadata.google.internal`, `instance-data` |
| **3** | **Reserved Subnet & Metadata Filtering** | Internal VPC reconnaissance, AWS IMDS credential theft | `IsPrivateOrReservedIP`: Blocks loopback (`127/8`), RFC 1918 (`10/8`, `172.16/12`, `192.168/16`), AWS/GCP IMDS (`169.254/16`), CGNAT (`100.64/10`), IPv6 ULA/link-local |
| **4** | **IPv4-Mapped IPv6 Normalization** | Subnet filter bypass via IPv6 representations | `if v4 := ip.To4(); v4 != nil { ip = v4 }`: Unwraps `::ffff:x.x.x.x` before subnet check |
| **5** | **Connection-Time DNS Resolution** | Time-of-Check to Time-of-Use (TOCTOU) DNS changes | `DialContext` intercepts socket creation and executes `net.DefaultResolver.LookupIP` immediately before connecting |
| **6** | **Socket IP Pinning (Anti-DNS Rebinding)** | DNS Rebinding attacks | Intercepts returned IPs and dials `net.JoinHostPort(ips[0].String(), port)`. Closes TOCTOU window completely |
| **7** | **Strict Redirect Prohibition** | Redirect-based SSRF (e.g. 302 to metadata) | `CheckRedirect` returns error immediately: `errors.New("HTTP redirects are strictly prohibited")` |
| **8** | **Response Memory Bounding (1 MB)** | Memory exhaustion DoS via infinite responses | `io.LimitReader(resp.Body, 1024*1024)`: Limits body read to 1 MB while permitting HTTP connection reuse |

---

## 4. Controlled Runtime Test Verification

The outbound integration security controls were executed and verified via [scripts/test_ssrf_runtime_controlled.js](file:///d:/cmd%20project/M2P-Git/CLM/scripts/test_ssrf_runtime_controlled.js):

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

---

## 5. Retry Policy, Dead-Letter Queue (DLQ) & Secret Rotation

### 5.1 Outbound Retry Policy
When an external partner webhook endpoint returns an HTTP 5xx error or connection timeout:
1. **Exponential Backoff with Jitter:** Retries are scheduled at intervals:
   - Attempt 1: 1 minute
   - Attempt 2: 5 minutes
   - Attempt 3: 15 minutes
   - Attempt 4: 1 hour
   - Attempt 5: 6 hours
   - Attempt 6: 24 hours
2. **Dead-Letter Queue (DLQ):** After 6 consecutive failed delivery attempts, the delivery record is transferred to `integration_dead_letter_queue` with error diagnostics. An automated alert is generated to the tenant administrator.

### 5.2 Secret Rotation Lifecycle
* Enterprise tenants can rotate their webhook shared secrets without service disruption using a **dual-secret grace period**:
  - `active_secret`: Used for generating outbound signatures and validating inbound calls.
  - `retiring_secret`: Maintained as valid for inbound signature verification for 72 hours following rotation.
  - After 72 hours, `retiring_secret` is automatically purged.

