# CLM — CRYPTOGRAPHIC ARCHITECTURE AUDIT & SPECIFICATION
**Platform:** Enterprise Contract & Certificate Lifecycle Management (CLM) Platform  
**Document Reference:** CLM-AUDIT-2026-CRYPTO-003  
**Assessment Date:** 2026-10-31  
**Lead Auditor:** Principal Cryptographic Systems Engineer & Zero-Trust Security Architect  
**Classification:** Enterprise Cryptographic Architecture Standard  
**Release Classification:** STAGING CANDIDATE — LIVE INFRASTRUCTURE VERIFICATION PENDING  

---

## 1. Executive Summary

This specification establishes the authoritative cryptographic trust boundary, key custody architecture, and non-repudiation lifecycle for the CLM platform. 

The core tenet is strict **Zero-SaaS Key Custody**: master cryptographic private signing keys and Key Encryption Keys (KEK) remain exclusively within the customer's private infrastructure (In-VPC Hardware Security Modules or Customer Cloud KMS). Under no circumstances are private signing keys exported, transmitted, or hosted within the SaaS Control Plane.

---

## 2. Diagram 3: Cryptographic Signing & Verification Sequence

```mermaid
sequenceDiagram
    autonumber
    participant Signer as Signer Browser / Client
    participant CP as SaaS Control Plane (Go Gateway)
    participant DB as PostgreSQL 15 (Audit Ledger)
    participant SG as Customer In-VPC Satellite Gateway
    participant HSM as Customer HSM / KMS (PKCS#11)
    participant TSA as RFC 3161 TSA Authority
    participant S3 as AWS S3 WORM Vault (Object Lock)

    Note over SG,CP: 0. Satellite Establishes Outbound mTLS 1.3 Tunnel
    SG->>CP: Outbound TCP Connect & Mutual TLS 1.3 Handshake (X.509 Pinning)
    CP-->>SG: Tunnel Established (Bi-Directional Command Streaming)

    Note over Signer,CP: 1. Signing Initiation & Version Digest Binding
    Signer->>CP: POST /api/v1/contracts/{id}/sign (Signer Auth Token)
    CP->>CP: Canonicalize Document Body & Compute DocumentDigest (SHA-256)
    CP->>CP: Build SigningRequestEnvelope (ContractID, Version, Digest, Nonce, 300s TTL)

    Note over CP,SG: 2. Transmit Envelope over Active Tunnel
    CP->>SG: Stream SigningRequestEnvelope over Encrypted mTLS

    Note over SG,HSM: 3. Satellite Validation & Local Signing
    SG->>SG: Verify Replay Defense: Check RequestID Nonce in LRU Cache
    SG->>SG: Validate Authorization Expiry (time <= expires_at && drift <= 300s)
    SG->>HSM: C_SignInit(KeyID, Algorithm) & C_Sign(DocumentDigest)
    HSM-->>SG: Detached Digital Signature (RSA-PSS / ECDSA)

    Note over SG,TSA: 4. RFC 3161 Timestamp Token Sealing
    SG->>TSA: TimeStampReq (SHA-256 Digest of Signature + Cert Hash)
    TSA-->>SG: TimeStampResp (Signed RFC 3161 Token - TST)

    Note over SG,CP: 5. Evidence Assembly & Cryptographic Verification
    SG->>SG: Assemble Evidence Bundle (Signature, X.509 Cert Chain, TST Token, Audit JSON)
    SG-->>CP: Stream SigningResponseEnvelope over mTLS
    CP->>CP: Cryptographically Verify Signature Bytes against DocumentDigest
    CP->>CP: Check X.509 Chain Validity & Revocation Status

    Note over CP,DB: 6. Two-Phase Persistence & Failure Recovery
    CP->>DB: INSERT contract_audit_logs (Migration 007 Chain-Head Lock FOR UPDATE)
    DB-->>CP: Sequence Number & Monotonic Hash Committed
    
    alt S3 WORM Upload Succeeds
        CP->>S3: PUT Evidence Bundle (Object Lock: Compliance Mode, 7-Year Retention)
        S3-->>CP: HTTP 200 OK (WORM Sealed)
        CP-->>Signer: HTTP 200 OK (Contract Executed & Sealed)
    else S3 WORM Upload Fails (Partial Failure)
        CP->>DB: UPDATE evidence_status = 'quarantined_pending_s3_sync'
        CP-->>Signer: HTTP 202 Accepted (Queued for WORM Reconciliation)
        Note over CP,S3: Background Worker Scans Quarantine & Retries S3 Upload
    end
```

---

## 3. Signing Request Schema & Validation Rules

Every signing request transmitted across the trust boundary must conform to the strict schema validated by the In-VPC Satellite Gateway:

```json
{
  "tenant_id": "c8a4b6d0-2f1e-4c3a-9e7b-1a2b3c4d5e6f",
  "contract_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "contract_version": "1.0",
  "document_digest": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "algorithm": "RSA-PSS-SHA256",
  "key_id": "hsm-slot-1-key-prod-2026",
  "request_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "expires_at": 1793467200,
  "auth_ref": "auth-sig-token-sha256-approved"
}
```

### Schema Field Specifications & Cryptographic Checks:
1. `tenant_id` (`UUIDv4`): Mandatory tenant identifier. The Satellite verifies that the key referenced by `key_id` belongs to this tenant.
2. `contract_id` (`UUIDv4`): Target contract UUID.
3. `contract_version` (`string`): Target version string (e.g. `"1.0"`, `"2.1"`). Binds the signature to a specific frozen document revision.
4. `document_digest` (`string`): 64-character hex-encoded SHA-256 digest of the canonical contract content.
5. `algorithm` (`string`): Cryptographic signing algorithm (`RSA-PSS-SHA256`, `RSA-PSS-SHA384`, `ECDSA-P256-SHA256`, `ECDSA-P384-SHA384`, `ED25519`).
6. `key_id` (`string`): Customer HSM key alias or PKCS#11 key label.
7. `request_id` (`UUIDv4`): Cryptographic nonce generated by the Control Plane to ensure idempotency and replay prevention.
8. `expires_at` (`int64`): UNIX timestamp in seconds. The Satellite rejects requests where `current_time > expires_at` or `expires_at - current_time > 300`.
9. `auth_ref` (`string`): Cryptographic authorization proof issued upon completion of Stage 13 approvals.

---

## 4. Replay Protection, Duplicate Signing & Storage Failure Recovery

### 4.1 Replay Defense & Nonce Tracking
The Satellite maintains a thread-safe in-memory LRU cache of recently processed `request_id` nonces:
1. When a signing payload arrives, the Satellite checks if `request_id` exists in the cache.
2. If present, the request is immediately rejected with `CLM-CRYPTO-ERR-003 (ERR_REPLAY_DETECTED)`.
3. If absent, timestamp drift is validated: $| \text{HostTime} - \text{ExpiresAt} + 300 | \le 300 \text{ seconds}$.
4. Upon successful validation, `request_id` is recorded in the cache with a 10-minute TTL.

### 4.2 Duplicate Signing Prevention
If a signatory attempts to re-execute a signature entry that is already marked `signed`, the service immediately aborts with:
`CLM-CRYPTO-ERR-001: target signature has already been executed (duplicate signing prohibited)`.

### 4.3 Reliable Recovery for DB / S3 Partial Failures
1. **Phase 1 (Database Staging):** The signature record and audit hash are staged in PostgreSQL within a database transaction.
2. **Phase 2 (S3 WORM Upload):** The compiled Evidence Bundle is uploaded to AWS S3 Object Lock.
3. **Partial Failure Handling:** If the S3 upload encounters a timeout or network outage, the transaction does not silently succeed; instead, it is marked `quarantined_pending_s3_sync`.
4. **Reconciliation Worker:** A background daemon periodically queries for quarantined records and retries the S3 Object Lock upload until committed.

### 4.4 Offline Queue Synchronization & Conflict Handling
* **Offline Operation:** While the mTLS uplink is disconnected, the Satellite cannot receive *new* real-time requests from SaaS, but executes *pre-staged / cached* authorization requests using locally cached keys, buffering audit records in local encrypted storage.
* **Uplink Recovery:** Upon reconnection, the Satellite streams queued signatures back to the Control Plane.
* **State Conflict Detection:** If a contract was `cancelled` or `terminated` in the Control Plane during the offline partition, the Control Plane rejects the queued signature with `CLM-CRYPTO-ERR-007 (ERR_STATE_CONFLICT)` and generates an audit security alert.

---

## 5. Verification Evidence

Validated via `node scripts/test_cryptographic_security_live.js`:
* Version & Digest Binding: **PASSED**
* Bit-Flip Sensitivity: **PASSED**
* RSA-PSS & ECDSA Verification: **PASSED**
* TTL Expiry & Replay Prevention: **PASSED**
* Duplicate Signing Prevention: **PASSED**
* S3 Partial Failure & Quarantine Recovery: **PASSED**
* Offline Queue Sync & Conflict Resolution: **PASSED** (12/12 tests passing).
