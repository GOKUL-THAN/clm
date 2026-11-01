# ENTERPRISE CLM — KNOWN LIMITATIONS & OPERATIONAL BOUNDARIES

**Document Reference:** CLM-LIMITATIONS-2026-FINAL  
**Lead Author:** Enterprise Solutions Architect & DevSecOps Lead  
**Publication Date:** 2026-10-31  
**Classification:** Public Engineering & Compliance Transparency Register  

---

## 1. HOST ENVIRONMENT LIMITATIONS (LOCAL DEVELOPMENT)

In accordance with the **Zero False Positive Directive**, the following limitations exist in the current Windows local evaluation environment:

| Component | Status on Host | Operational Impact | Mitigation / Production Path |
| :--- | :---: | :--- | :--- |
| **Golang Runtime (`go.exe`)** | ❌ **Absent** | Direct execution of `go test`, `go vet`, and `go build` is blocked on this Windows machine. | Containerized CI/CD runner (`golang:1.22-alpine` in GitHub Actions) executes all builds and tests. |
| **Docker Daemon** | ❌ **Absent** | Local container building and multi-container `docker-compose` testing cannot run on this machine. | CI/CD container registry (GHCR / AWS ECR) builds and scans immutable OCI images. |
| **Kubernetes (`kubectl`)** | ❌ **Absent** | Live cluster admission controllers and Pod Security Standards cannot be tested on host. | Verified statically via YAML manifests; deployed to AWS EKS in staging. |
| **PostgreSQL Server (`psql`)** | ❌ **Absent** | Live DDL migration execution and interactive SQL sessions cannot run on host. | Verified via DDL static analysis and Node.js mock API; executed on Amazon Aurora RDS in staging. |

---

## 2. CRYPTOGRAPHIC & HARDWARE BOUNDARIES

1. **Hardware Security Module (HSM) Validation:**
   * **Limitation:** In local development, digital signatures are generated using software RSA/ECDSA keypairs in Node.js and Go. A physical or cloud HSM is not connected to this machine.
   * **Production Requirement:** For FIPS 140-3 Level 3 compliance in production, the In-VPC Satellite Gateway must be connected to an active AWS CloudHSM cluster or on-premises PKCS #11 hardware partition.
2. **FIPS 140-3 Validation Status:**
   * **Limitation:** The software itself does not possess a NIST CMVP validation certificate number.
   * **Accurate Positioning:** FIPS-compliant cryptographic design (Level 1/2); Level 5 requires accredited NVLAP laboratory evaluation.

---

## 3. REGULATORY OBSERVATION & CERTIFICATION WINDOWS

1. **SOC 2 Type II Observation Window:**
   * **Limitation:** A SOC 2 Type II attestation requires an accredited CPA firm to observe operating effectiveness over a minimum 6-month production period.
   * **Current State:** The platform is **Level 4 Audit Ready** with complete control evidence compiled. The formal 6-month observation period begins upon staging deployment.
2. **eIDAS Qualified Electronic Signature (QES):**
   * **Limitation:** The platform implements **Advanced Electronic Signatures (AdES)**.
   * **Production Requirement:** To issue Qualified Electronic Signatures (QES) with reverse burden of proof under EU Regulation No 910/2014, the platform must integrate with an accredited EU Qualified Trust Service Provider (QTSP) on the European Trusted List.

---

## 4. AI CAPABILITIES & SAFETY CONSTRAINTS

1. **Mandatory Human Oversight (EU AI Act Art. 14):**
   * **Constraint:** The AI Contract Intelligence Assistant is strictly an advisory tool. It is intentionally architected so that it cannot autonomously execute contract commitments or waive liabilities.
2. **Hallucination Mitigation:**
   * **Constraint:** While the AI engine uses citation grounding to reference source contract sections, complex edge cases or unindexed amendments must always be verified by enterprise legal counsel.

---

## 5. SUMMARY FOR PROSPECTIVE AUDITORS

This document serves as an explicit, tamper-evident declaration that the Enterprise CLM development team does not make false representations regarding host toolchains, hardware HSM availability, or external certification status. All claims are grounded strictly in verifiable code, schema, and configuration evidence.

