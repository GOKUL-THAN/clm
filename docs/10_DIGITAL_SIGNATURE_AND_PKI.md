# CLM-SPEC-10: DIGITAL SIGNATURE & PKI INTEGRITY SPECIFICATION
## eIDAS Regulation, In-VPC HSM Signing, RFC 3161 Timestamping, and Evidence Bundles

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-SIG-010  
**Classification:** Enterprise Cryptographic Standard  

---

### 1. Legal Admissibility & eIDAS Framework

The platform satisfies all three tiers of the **eIDAS Regulation (EU No 910/2014)** and the US **ESIGN / UETA Acts**:
1. **Simple Electronic Signature (SES):** Click-to-sign with email OTP verification and forensic metadata capture (IP address, user agent, timestamp).
2. **Advanced Electronic Signature (AdES):** Uniquely linked to signatory, capable of identifying signatory, created using private key under signatory's sole control, and tamper-evident (SHA-256 seal).
3. **Qualified Electronic Signature (QES):** Backed by a Qualified Signature Creation Device (QSCD) hosted inside the Customer In-VPC HSM using certified X.509 PKI certificates.

---

### 2. In-VPC Hardware Signing Flow

```
[Contract Document Executed] 
             |
             v
[Calculate SHA-256 Digest]
             |
             v
[Transmit Digest to Customer Satellite via mTLS] (ZERO Cleartext Text Sent)
             |
             v
[Customer In-VPC HSM Signs Digest with Private Key]
             |
             v
[Query RFC 3161 Time-Stamping Authority (TSA) for Counter-Signature Token]
             |
             v
[Return Detached PKCS#7 / CMS Signature + TSA Token to SaaS Control Plane]
             |
             v
[Persist in `contract_signatures` & Archive in WORM S3 Object Lock Vault]
```

---

### 3. Non-Repudiation Evidence Bundle

Upon contract finalization, the system compiles a detached evidence bundle comprising:
* Signer legal identity, verified corporate email, and role.
* Source IP address and browser user-agent string.
* Cryptographic signature hash and public certificate thumbprint.
* RFC 3161 Time-Stamp Token (proving the exact second of signing).
* Document SHA-256 digest at moment of execution.
* CRL/OCSP revocation check verification status.

