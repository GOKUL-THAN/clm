# CLM — CRYPTOGRAPHIC TRUST BOUNDARY & INTEGRATION EVIDENCE MATRIX
**Platform:** Enterprise Contract & Certificate Lifecycle Management (CLM) SaaS Platform  
**Document Reference:** CLM-CRYPTO-BOUNDARY-2026.1  
**Assessment Date:** 2026-10-31  
**Author:** Principal Cryptography and PKI Systems Engineer  
**Classification:** Enterprise Cryptographic Architecture Standard  

---

## 1. Executive Architectural Division

A foundational vulnerability in enterprise compliance assessments is conflating software cryptographic libraries with hardware security modules, cloud KMS integrations, and external public trust services.

This specification establishes strict cryptographic trust boundaries across all 9 subsystems, clearly separating **verified software implementations** from **configured infrastructure-as-code**, and **planned external integrations**.

```
┌────────────────────────────────────────────────────────────────────────────┐
│                    EXTERNAL PUBLIC TRUST BOUNDARY                          │
│  - RFC 3161 Qualified Time-Stamping Authorities (TSA)                      │
│  - Public CA Issuance (Let's Encrypt / DigiCert / Sectigo)                 │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      │
┌─────────────────────────────────────▼──────────────────────────────────────┐
│                    CUSTOMER IN-VPC HARDWARE BOUNDARY                       │
│  - Physical Hardware Security Modules (Thales Luna / Entrust / Utimaco)   │
│  - Cloud HSMs (AWS CloudHSM / Azure Managed HSM / Google Cloud KMS)        │
│  - OASIS PKCS#11 v2.40 Hardware Token Cryptographic Interface             │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      │
┌─────────────────────────────────────▼──────────────────────────────────────┐
│                    APPLICATION SOFTWARE CRYPTO BOUNDARY                    │
│  - Go crypto (rsa, ecdsa, aes-gcm, sha256)                                 │
│  - Node.js crypto engine (Detached signatures, bit-flip tamper rejection)  │
│  - RFC 5280 X.509 Certificate Validation Engine                            │
│  - Envelope Encryption DEK/KEK Orchestrator                                │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Detailed Subsystem Analysis & Implementation Evidence

### 2.1 AWS KMS & AWS Secrets Manager
* **Architectural Scope:** In-VPC envelope encryption, master Key Encryption Key (KEK) custody, and automated secret retrieval.
* **Code Implementation:** [components/satellite-gateway/internal/secrets/providers/aws.go](file:///d:/cmd%20project/M2P-Git/CLM/components/satellite-gateway/internal/secrets/providers/aws.go).
* **Configuration:** Defined in `configs/satellite.yaml` under `secrets.provider: aws` and provisioned via Terraform in `deploy/terraform/aws/main.tf` (`aws_kms_key.clm_key`).
* **Implementation Evidence:** Go code utilizes AWS SDK v2 client with IAM Role for Service Accounts (IRSA) authentication.
* **Status:** **`IMPLEMENTED_NOT_RUNTIME_VERIFIED`** (Code and IaC manifests verified; live execution requires Staging AWS deployment).

---

### 2.2 Azure Key Vault & Managed HSM
* **Architectural Scope:** Azure-hosted customer enclave key custody using FIPS 140-2 Level 3 single-tenant HSMs.
* **Code Implementation:** Interface abstractions in `components/satellite-gateway/internal/deployment/deployers/cloud/azure.go`.
* **Configuration:** Manifests in `deploy/terraform/azure/main.tf`.
* **Implementation Evidence:** Terraform definitions exist; Go backend utilizes generic secret store interfaces.
* **Status:** **`DOCUMENTED_ONLY`** (Infrastructure manifests defined; dedicated Azure Key Vault SDK client pending live staging environment).

---

### 2.3 Google Cloud KMS & Cloud HSM
* **Architectural Scope:** Google Cloud customer key custody using Cavium LiquidSecurity HSMs.
* **Code Implementation:** Interface abstractions in `components/satellite-gateway/internal/deployment/deployers/cloud/gcp.go`.
* **Configuration:** Manifests in `deploy/terraform/gcp/main.tf`.
* **Implementation Evidence:** Terraform manifests present.
* **Status:** **`DOCUMENTED_ONLY`** (Manifest definitions exist; active client library integration pending).

---

### 2.4 Physical HSM & PKCS#11 Token Interface
* **Architectural Scope:** On-premises and customer data center Hardware Security Modules (Thales Luna, Entrust nShield, YubiKey HSM).
* **Code Implementation:** Implemented via C-API wrapper interfaces in `components/satellite-gateway/internal/crypto/engine.go` and `components/satellite-gateway/configs/satellite.yaml` (`hsm_enabled: false`).
* **Implementation Evidence:** Software models and algorithm dispatchers present.
* **Status:** **`PARTIALLY_IMPLEMENTED`** (Software interfaces audited; physical hardware tokens are absent on developer and CI runner machines).

---

### 2.5 Two-Tier Envelope Encryption
* **Architectural Scope:** High-speed symmetrical payload protection combining local ephemeral AES-256-GCM Data Encryption Keys (DEKs) with customer KMS Key Encryption Keys (KEKs).
* **Code Implementation:**
  - AES-256-GCM encryption implemented in [components/saas-control-plane/internal/saas/cloudaccounts/service.go](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/internal/saas/cloudaccounts/service.go).
  - Design specified in [docs/04_CRYPTOGRAPHIC_DESIGN.md](file:///d:/cmd%20project/M2P-Git/CLM/docs/04_CRYPTOGRAPHIC_DESIGN.md).
* **Implementation Evidence:** 32-byte master key generation, 96-bit random nonces, and 128-bit authentication tags verified in cloud account credential storage.
* **Status:** **`PARTIALLY_IMPLEMENTED`** (Fully implemented for credential storage; contract binary payload encryption engine pending full integration with Satellite In-VPC DEK decryptor).

---

### 2.6 Detached Digital Signatures (RSA-PSS & ECDSA)
* **Architectural Scope:** Cryptographic non-repudiation for executed contracts.
* **Code Implementation:** [components/saas-control-plane/internal/saas/signature/service.go](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/internal/saas/signature/service.go) and [components/satellite-gateway/internal/crypto/engine.go](file:///d:/cmd%20project/M2P-Git/CLM/components/satellite-gateway/internal/crypto/engine.go).
* **Implementation Evidence:**
  - Evaluated in `scripts/independent_adversarial_test_suite.js` (Boundary 4).
  - Generates RSA-PSS and ECDSA P-256 signatures over SHA-256 digests.
  - **Bit-Flip Tamper Resistance Proof:** Inverting exactly 1 bit in either the contract document digest or the signature buffer results in immediate cryptographic rejection (`ATK-005: DEFENDED`).
  - Empty, null, or truncated signature submissions are rejected (`ATK-006: DEFENDED`).
* **Status:** **`IMPLEMENTED_AND_VERIFIED`** (Deterministic runtime mathematical execution passed).

---

### 2.7 RFC 5280 X.509 Certificate Validation
* **Architectural Scope:** Validation of digital certificates, intermediate CA chains, temporal bounds, and revocation status.
* **Code Implementation:** Handled in Satellite crypto engine and evaluated in `scripts/independent_adversarial_test_suite.js`.
* **Implementation Evidence:**
  - Expired certificates (`not_after < now`) are rejected (`ATK-020: DEFENDED`).
  - Revoked certificates are validated against simulated CRL / OCSP status (`ATK-021: DEFENDED`).
* **Status:** **`IMPLEMENTED_AND_VERIFIED`**.

---

### 2.8 RFC 3161 Time-Stamp Authority (TSA) Integration
* **Architectural Scope:** Independent cryptographic timestamping providing legal proof of existence prior to a verified instant under eIDAS Article 42.
* **Code Implementation:** Data structures for ASN.1 DER `TimeStampReq` and `TimeStampResp` implemented in signature service.
* **Implementation Evidence:** Synthetic timestamp response parsing verified. Live socket connections to public TSAs (e.g. DigiCert TSA) are disabled on local sandboxes to prevent unauthorized external network egress.
* **Status:** **`PARTIALLY_IMPLEMENTED`** (Token format and ASN.1 serialization implemented; live public network connectivity scheduled for Staging deployment).

---

## 3. Cryptographic Subsystem Master Ledger

| Subsystem | Primary Algorithms | Custody Entity | Verified Artifact | Current Status |
| :--- | :--- | :--- | :--- | :---: |
| **1. AWS KMS** | AES-256-GCM / RSA-4096 | Customer AWS VPC | `satellite/secrets/providers/aws.go` | `IMPLEMENTED_NOT_RUNTIME_VERIFIED` |
| **2. Azure Key Vault** | AES-256 / RSA-4096 | Customer Azure VNet | `deploy/terraform/azure/main.tf` | `DOCUMENTED_ONLY` |
| **3. Google Cloud KMS** | AES-256 / ECDSA P-256 | Customer GCP VPC | `deploy/terraform/gcp/main.tf` | `DOCUMENTED_ONLY` |
| **4. Physical PKCS#11** | C_Sign / C_Digest | Hardware Module | `satellite/crypto/engine.go` | `PARTIALLY_IMPLEMENTED` |
| **5. Envelope Encryption** | AES-256-GCM DEK + KEK | Hybrid In-VPC | `saas/cloudaccounts/service.go` | `PARTIALLY_IMPLEMENTED` |
| **6. Detached Signatures** | RSA-PSS / ECDSA P-256 | Local Software Crypto | `saas/signature/service.go` | **`IMPLEMENTED_AND_VERIFIED`** |
| **7. Certificate Engine** | X.509 / RFC 5280 | Crypto Engine | `satellite/crypto/engine.go` | **`IMPLEMENTED_AND_VERIFIED`** |
| **8. RFC 3161 TSA** | SHA-256 ASN.1 DER | External TSA | Synthetic Test Suite | `PARTIALLY_IMPLEMENTED` |

