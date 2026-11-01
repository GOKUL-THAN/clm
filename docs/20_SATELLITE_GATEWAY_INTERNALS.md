# CLM-SPEC-20: SATELLITE GATEWAY INTERNALS SPECIFICATION
## In-VPC Enclave Daemon, mTLS Handshake, Local KMS/HSM Custody, and Offline Queueing

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-SAT-020  
**Classification:** Enterprise Systems & Infrastructure Standard  

---

### 1. Satellite Gateway System Overview

The **Satellite Gateway** (`components/satellite-gateway/`) is a lightweight Go daemon deployed directly inside the customer's secure Virtual Private Cloud (VPC) or on-premises perimeter:
* **The Zero-Knowledge Guarantee:** The gateway acts as an isolated cryptographic bastion. At no point do private signing keys, HSM master PINs, or unencrypted confidential agreement files leave this customer-controlled boundary.

---

### 2. Enrollment Handshake & Mutual Authentication

```
Customer In-VPC Satellite                        SaaS Control Plane
           |                                              |
           | ----- 1. Initiates TLS 1.3 mTLS Handshake -->| (Presents Satellite X.509 Cert)
           |<----- 2. Verifies Pinned SaaS Server Cert ---|
           |                                              |
           | ----- 3. Sends Enclave Attestation Token --->| (KMS Key ID, Firmware, Tenant ID)
           |<----- 4. Acknowledges & Establishes Session -|
           |                                              |
           |<==== Encrypted Bidirectional gRPC Channel ===>
```

1. **Certificate Pinning:** The Satellite validates the SaaS Control Plane's TLS certificate against a local hardcoded CA bundle.
2. **Attestation:** The Satellite provides proof of local KMS connectivity (e.g. AWS KMS Key ARN, PKCS#11 token serial).
3. **Command Channel:** Listens for signing and decryption requests originating from authenticated tenant workflows.

---

### 3. Offline Resilience & Event Draining

During WAN network partitions or cloud provider outages:
1. **Local Queueing:** The Satellite continues executing local certificate renewals, audit event logging, and HSM signatures, writing them to an encrypted on-disk SQLite/WAL queue (`clm-satellite-local-pvc`).
2. **Backpressure & Recovery:** When mTLS connectivity to the SaaS Control Plane is re-established, the queue manager automatically drains backlogged events in FIFO order.
3. **Verification:** Tested and certified in **Scenario F**: Event queued during simulated outage; queue successfully drained upon reconnection with zero private key leakage.

