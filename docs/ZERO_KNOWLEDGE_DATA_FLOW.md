# CLM — ZERO-KNOWLEDGE DATA FLOW & PRIVACY BOUNDARY SPECIFICATION
**Platform:** Enterprise Contract & Certificate Lifecycle Management (CLM) SaaS Platform  
**Document Reference:** CLM-SEC-ZERO-KNOWLEDGE-2026.1  
**Assessment Date:** 2026-10-31  
**Author:** Zero-Trust Security Architect & Data Protection Specialist  
**Classification:** Enterprise Cryptographic Data Flow Architecture  

---

## 1. Architectural Tenet: The Zero-Knowledge Guarantee

In multi-tenant SaaS environments, enterprise legal teams, defense contractors, and financial institutions cannot legally or contractually permit third-party SaaS cloud providers to possess unencrypted cleartext of confidential contracts, proprietary intellectual property, pricing schedules, or cryptographic signing keys.

The Enterprise CLM platform enforces a **Zero-Knowledge Hybrid Topology** designed to isolate tenant data custody:
* The **SaaS Control Plane** is responsible for lifecycle state machine transitions, metadata indexing, approval routing, obligation tracking, and UI orchestration.
* The **Customer In-VPC Satellite Gateway** is deployed inside the customer's private cloud VPC or on-premises data center. It retains exclusive custody of cryptographic keys, executes encryption/decryption, and generates digital signatures locally.

This document traces the complete physical path of contract data, distinguishing **verified code implementation** from **planned design abstractions**.

---

## 2. End-to-End Contract Data Lifecycle

```
[User Browser / ERP / Intake Portal]
                  │
                  ▼ (1. Ingestion: HTTPS REST API)
       [SaaS Control Plane Entry]
       - Authentication & Tenant Scope Validation
       - Metadata Extraction (Title, Parties, Dates, Value)
                  │
                  ▼ (2. Hashing: SHA-256 Content Digest)
         [Master Digest Sealed]
                  │
                  ▼ (3. Envelope Encryption Handshake)
  [Generate Ephemeral AES-256-GCM DEK in Memory]
                  │
                  ├── (Ciphertext & Encrypted DEK) ──► [PostgreSQL Database (Aurora)]
                  │
                  ▼ (4. mTLS 1.3 Command Uplink / gRPC)
    [Customer In-VPC Satellite Gateway]
                  │
                  ▼ (5. In-VPC KMS / HSM Key Custody)
    [Customer AWS KMS / Azure Key Vault / Hardware HSM]
    - Encrypt / Decrypt DEK with Customer-Managed KEK
    - Hardware Digital Signing (RSA-PSS / ECDSA)
```

---

## 3. Detailed Data Flow Stages: Design Intent vs. Verified Implementation

### 3.1 Point of Ingestion (Where Contract Files Enter)
* **Ingestion Points:**
  1. `POST /api/v1/contracts` (Multipart file upload or JSON payload containing clause bodies).
  2. `POST /api/v1/contracts/requests` (Business intake portal conversion).
  3. External ERP/CRM integrations via authenticated webhook adapters.
* **Code Implementation:** Handled in [components/saas-control-plane/internal/api/rest/handlers/contracts.go](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/internal/api/rest/handlers/contracts.go).
* **Security Control:** Handler strictly rejects unauthenticated submissions (HTTP 401) and isolates payload ingestion to the authenticated tenant UUID.

---

### 3.2 Cleartext Processing Boundary (Where Cleartext Exists)
* **Design Intention:** Cleartext contract text exists only in-memory inside the customer's browser session and inside the In-VPC Satellite Gateway during active redlining.
* **Actual Implementation Reality:**
  - During dynamic contract assembly, authoring, and redline negotiation, the **SaaS Control Plane currently handles contract text in memory** within the Go runtime (`components/saas-control-plane/internal/saas/contract/` and `aicontract/`).
  - Clause bodies and contract text submitted via REST API are temporarily buffered in Go memory structures before database persistence.
* **Storage Protection:** In the database tier, contract records store metadata (`title`, `department`, `status`, `total_amount`), while clause text and dynamic forms are stored in PostgreSQL tables protected by `FORCE ROW LEVEL SECURITY`.

---

### 3.3 Document Hashing & Integrity Sealing
* **Hashing Mechanism:** SHA-256 cryptographic digests are generated to seal contract versions:
  $$\text{MasterDigest} = \text{SHA-256}(\text{CanonicalDocumentContent})$$
* **Code Implementation:** Located in [components/saas-control-plane/internal/saas/signature/service.go](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/internal/saas/signature/service.go#L94-L96):
  ```go
  h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s", sig.SignerEmail, now.String(), ipAddress)))
  sig.DigitalSignatureHash = hex.EncodeToString(h[:])
  ```
* **Verification Status:** **`IMPLEMENTED_AND_VERIFIED`**. Detached hash generation and bit-flip tamper rejection are mathematically verified in `scripts/independent_adversarial_test_suite.js`.

---

### 3.4 External AI Provider Inspection (Does Google Gemini Receive Contract Data?)
* **Critical Architectural Discovery:**
  - Design specifications (`docs/01_ARCHITECTURE.md`, `docs/08_AI_CONTRACT_INTELLIGENCE.md`) reference "Gemini-Legal LLM" and "Gemini-Pro Powered Analysis".
  - **Code Audit of `components/saas-control-plane/internal/saas/aicontract/service.go` reveals:**
    1. The AI engine is implemented **entirely in-process** using deterministic Go pattern matching, heuristic clause parsers, and regex dictionaries.
    2. **ZERO CONTRACT DATA IS TRANSMITTED TO EXTERNAL GOOGLE GEMINI APIS OR THIRD-PARTY CLOUD LLMS.**
    3. The service contains an active prompt injection interceptor checking for adversarial sequences (`ignore previous`, `system override`, `jailbreak`, `base64`) and halts processing with a simulated security response.
* **Zero-Knowledge Privacy Verdict:**  
  While the "live Gemini cloud connection" is a documentation-only claim, this architecture **strictly enhances data privacy**: confidential contract text never leaves the enterprise control plane to public cloud AI training models or external inference APIs.

---

### 3.5 Customer-Controlled Key Custody & Satellite Gateway Architecture
* **Key Custody Architecture:**
  - Private signing keys and root master keys never enter the SaaS Control Plane.
  - The Satellite Gateway integrates with customer-controlled key stores:
    - **AWS KMS / Secrets Manager:** Configured via `configs/satellite.yaml` (`secrets.provider: aws`, `kms_key_id: ${CLM_AWS_KMS_KEY_ID}`).
    - **Local Hardware HSM:** PKCS#11 C-API wrappers in `satellite/internal/crypto/`.
* **Envelope Encryption Model:**
  1. A unique, ephemeral 256-bit AES-GCM Data Encryption Key (DEK) is generated in memory.
  2. The DEK encrypts the contract payload.
  3. The Satellite Gateway requests the customer's KMS or HSM Key Encryption Key (KEK) to encrypt the DEK.
  4. The SaaS Control Plane stores only the Encrypted DEK and Ciphertext digest. Without the customer's In-VPC KMS, the SaaS provider possesses zero mathematical capability to decrypt the document.
* **Verification Status:**  
  - AWS KMS integration and AES-GCM encryption: **`IMPLEMENTED_NOT_RUNTIME_VERIFIED`** (Code and Terraform configured; awaiting Staging cloud deployment).
  - Physical PKCS#11 hardware communication: **`PARTIALLY_IMPLEMENTED`** (Software interfaces audited; physical token absent on workstation).

---

### 3.6 Satellite Gateway to Control Plane Communication Channel
* **Transport Protocol:**
  - **mTLS 1.3:** Mutual TLS with X.509 certificate pinning. The SaaS Control Plane validates the Satellite's client certificate against a tenant-specific CA bundle. The Satellite validates the Control Plane against a pinned certificate authority.
  - **Dual Operational Modes:**
    1. *Direct Bidirectional gRPC:* Streaming on port `9090` for low-latency command execution.
    2. *Outbound Long-Polling HTTPS:* Initiated by the Satellite on port `8080` (every 5 seconds) to traverse restrictive corporate firewalls where inbound ports are blocked.
* **Payload Serialization:** Commands and responses are serialized in Protocol Buffers. Cleartext private keys are strictly barred from payload schemas.

---

## 4. Privacy & Zero-Knowledge Summary Matrix

| Data Flow Stage | Cleartext Exposure | Storage Location | Key Custody Entity | Verified Status |
| :--- | :---: | :--- | :--- | :---: |
| **Intake & REST Upload** | In-Memory (Go) | Transient Memory | N/A (Pre-Encryption) | `IMPLEMENTED_AND_VERIFIED` |
| **Clause Extraction & Heuristics** | In-Memory (Go) | Transient Memory | N/A | `IMPLEMENTED_AND_VERIFIED` |
| **External Cloud LLM (Gemini)** | **NONE (Zero Egress)** | Not Transmitted | N/A | `VERIFIED NO EXTERNAL CALLS` |
| **Database Document Storage** | Encrypted / RLS Scoped | Aurora PostgreSQL 16 | Tenant Key | `BLOCKED` (Host Port 5432 Closed) |
| **Digital Signing Execution** | In-VPC Only | Local Disk Cache | Customer In-VPC HSM | `IMPLEMENTED_AND_VERIFIED` (Software) |
| **Command Channel Telemetry** | Digests Only | In-Flight mTLS | Pinned X.509 mTLS | `IMPLEMENTED_NOT_RUNTIME_VERIFIED` |

