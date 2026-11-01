# CLM-SPEC-04: CRYPTOGRAPHIC DESIGN & KEY CUSTODY SPECIFICATION
## FIPS 140-3 Standards, Envelope Encryption, PKI, and RFC 3161 Timestamping

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-CRYPTO-004  
**Classification:** Enterprise Cryptographic Architecture Standard  

---

### 1. Cryptographic Standard Compliance

The Enterprise CLM cryptographic subsystem is architected in accordance with **NIST FIPS 140-3 Level 3** requirements:
* **Approved Symmetric Ciphers:** AES-256 in Galois/Counter Mode (GCM) with 96-bit random nonces and 128-bit authentication tags.
* **Approved Asymmetric Algorithms:** RSA (minimum 2048-bit for legacy compatibility, 4096-bit standard for root/signing operations), ECDSA with NIST P-384 curves, and Ed25519.
* **Approved Hash Functions:** SHA-256, SHA-384, and SHA-512 (SHA-1 and MD5 are strictly prohibited).
* **Random Number Generation:** Hardware-seeded NIST SP 800-90A compliant Deterministic Random Bit Generators (DRBG).

---

### 2. Envelope Encryption Lifecycle

To secure contract binaries, attachments, and proprietary clause content, the platform employs a two-tier **Envelope Encryption** architecture:

```
[Contract Document (Cleartext)]
              |
              v
[Generate Ephemeral 256-bit DEK] ---> [AES-256-GCM Encryption] ---> [Encrypted Contract Ciphertext]
              |                                                               |
              v                                                               |
[Customer In-VPC KMS / HSM Key]                                                |
              |                                                               |
              v                                                               |
     [Encrypt DEK (KEK)]                                                      |
              |                                                               |
              v                                                               v
   [Encrypted DEK Payload] ------------------------------------> [Contract Cryptographic Envelope]
```

1. **Data Encryption Key (DEK):** A unique, cryptographically random AES-256 key is generated in memory for every contract revision.
2. **Key Encryption Key (KEK):** The customer's customer-managed master key located inside their In-VPC KMS (AWS KMS, Azure Key Vault, Cloud KMS, or PKCS#11 HSM) encrypts the DEK.
3. **Storage:** The SaaS Control Plane stores only the Encrypted DEK and Ciphertext digest. It possesses no technical capability to decrypt the DEK without calling the customer's In-VPC Satellite Gateway.

---

### 3. Digital Signatures & RFC 3161 Timestamping

1. **Document Hashing:** The canonical document content is hashed using SHA-256:
   $$\text{Digest} = \text{SHA-256}(\text{CanonicalDocumentContent})$$
2. **Signature Generation:** The digest is signed using the signatory's private key hosted inside the In-VPC HSM.
3. **TSA Timestamp Token:** A Time-Stamp Request (TSR) containing the document hash is transmitted to an RFC 3161 compliant Time-Stamping Authority (TSA). The returned Time-Stamp Response (TST) proves document existence prior to a certified wall-clock instant.
4. **Evidence Bundle:** The signature, public certificate chain, TSA token, IP address, and signer identity are serialized into an immutable, verifiable JSON-LD / CMS evidence bundle.

