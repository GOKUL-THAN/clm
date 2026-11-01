# CLM — ENTERPRISE ARCHITECTURE CONSISTENCY AUDIT
**Platform:** Enterprise Contract & Certificate Lifecycle Management (CLM) SaaS Platform  
**Document Reference:** CLM-ARCH-AUDIT-2026.1  
**Assessment Date:** 2026-10-31  
**Author:** Principal Enterprise Architect, Zero-Trust Security Architect & Cloud Systems Engineer  
**Classification:** Enterprise Engineering Architecture Audit  

---

## 1. Executive Summary

This architecture consistency audit reconciles the documented architecture in `docs/` (`01_ARCHITECTURE.md`, `04_CRYPTOGRAPHIC_DESIGN.md`, `18_INTEGRATION_ARCHITECTURE.md`, `21_DATABASE_ARCHITECTURE.md`) with the physical Go codebase, container manifests, and deployment configurations across `components/` and `deploy/`.

The assessment confirms the viability of the **Zero-Knowledge Hybrid SaaS Control Plane + Customer In-VPC Satellite Gateway** architecture while surfacing and documenting **critical inconsistencies, port collisions, protocol dualities, and documentation-versus-implementation discrepancies**.

---

## 2. Component & Transport Consistency Analysis

### 2.1 Ports and Protocols Matrix

An exhaustive scan of configuration files, entrypoints, and Kubernetes manifests identified several port allocations and potential namespace collisions:

| Subsystem / Process | Source File | Configured Port | Protocol | Scope / Network Namespace | Consistency Assessment |
| :--- | :--- | :---: | :---: | :--- | :--- |
| **SaaS Control Plane (REST)** | `configs/server.yaml` | `8080` | HTTP / HTTPS (TLS 1.3) | `clm-production` (Cloud EKS) | Consistent. Standard container ingress port. |
| **SaaS Control Plane (gRPC)** | `configs/server.yaml` | `9090` | gRPC / HTTP/2 (mTLS) | `clm-production` (Cloud EKS) | Consistent for cloud deployment. |
| **Satellite Gateway (Portal API)** | `satellite/main.go` | `8080` | HTTP (`CLM_GATEWAY_API_ADDR`) | Customer In-VPC Enclave | **COLLISION RISK**: Default `8080` collides with Control Plane if run on same local host/docker bridge without port remapping. |
| **Satellite Gateway (vSatellite API)** | `satellite/main.go` | `9090` | HTTP mux | Customer In-VPC Enclave | **COLLISION RISK**: Default `9090` collides with Control Plane gRPC port if collocated. |
| **Keycloak IAM** | `docker-compose.yml` | `8180:8080` | HTTP | Local / Staging Container | Consistent. Keycloak runs on internal `8080`, mapped externally to `8180`. |
| **PostgreSQL Database** | `server.yaml` / K8s | `5432` | PostgreSQL Relational | VPC Internal Subnet | Consistent. RLS enforced. |
| **Redis Cache** | `server.yaml` | `6379` | RESP | VPC Internal Subnet | Consistent. Session caching & rate limiting. |

> [!WARNING]
> **Port Collision Finding:** If a developer or test runner attempts to run both `clm-server` and `clm-satellite` on a single developer workstation without environment overrides, both binaries will compete for `8080` (REST) and `9090` (gRPC vs vSatellite). In production Kubernetes deployments, this is safely isolated by namespace and VPC boundaries (`clm-production` vs customer In-VPC). For local environments, `CLM_GATEWAY_API_ADDR=:8082` and `CLM_VSATELLITE_API_ADDR=:9092` must be explicitly configured.

---

### 2.2 Command Transport Duality: gRPC Streaming vs. HTTP Polling

- **Architectural Documentation Claim (`docs/01_ARCHITECTURE.md` & `docs/20_SATELLITE_GATEWAY_INTERNALS.md`):**  
  Documents assert an *"Encrypted Bidirectional gRPC Channel"* where the SaaS Control Plane pushes signing requests and decryption tasks directly to the Satellite over persistent mTLS streams.
- **Physical Code Implementation (`components/satellite-gateway/configs/satellite.yaml` & `cmd/clm-satellite/main.go`):**  
  The Satellite runtime supports both a direct gRPC connector and a pull-based HTTP command polling engine:
  ```yaml
  commands:
    poll_interval: "5s"
    timeout: "15m"
    lease_renewal: "45s"
    max_result_bytes: 262144
    network_retries: 5
  ```
- **Architectural Reconciliation:**  
  The pull-based polling model (`commands.poll_interval = 5s`) is essential for enterprise customers whose corporate firewalls prohibit inbound connections from cloud SaaS networks. The architecture documentation must acknowledge this **dual-mode transport**:
  1. *Inbound-Restricted Customer Firewalls:* Satellite initiates outbound HTTPS long-polling (`/api/v1/satellites/{id}/commands/poll`).
  2. *Direct Hybrid In-VPC Peering:* Satellite maintains persistent bi-directional gRPC streaming over port `9090`.

---

## 3. Component Responsibility Matrix

| Component | Documented Responsibility | Actual Code Implementation | Implementation Gap / Finding |
| :--- | :--- | :--- | :--- |
| **SaaS Control Plane** | Multi-tenant orchestration, lifecycle state machine, metadata, approvals | Implemented in `components/saas-control-plane/internal/saas/` | Verified. Handles 26 stages, approvals, and metadata. |
| **Satellite Gateway** | In-VPC cryptographic enclave, local HSM custody, local document decryption | Implemented in `components/satellite-gateway/` | Certificate issuance, scanner, and AWS Secrets Manager present. Dedicated contract document decryption engine is partially implemented via mock buffers. |
| **AI Contract Engine** | Gemini-Pro Powered Legal LLM | Implemented in `internal/saas/aicontract/service.go` | **CRITICAL DISCREPANCY:** Implemented via in-process Go rule-based heuristics and prompt injection filters. No live outbound Google Gemini API call exists. |
| **Keycloak IAM** | OIDC, SAML 2.0, WebAuthn MFA, Brute-force protection | Manifests in `deploy/keycloak/clm-realm.json`, Go client in `internal/saas/identity/keycloak.go` | Verified. Full realm configuration and admin service implemented. |
| **PostgreSQL RLS** | Multi-tenant row-level security, FORCE RLS, WORM audit | Schema in `deploy/migrations/006_...rls.sql`, Go store in `internal/store/postgres/db.go` | FORCE RLS declared on 7 tables; transaction-local `set_config` implemented. WORM trigger lacks `BEFORE TRUNCATE` protection. |
| **Detached Signatures** | RSA-PSS and ECDSA digital signatures, bit-flip tamper rejection | Implemented in `internal/saas/signature/service.go` | Detached signature math verified. Hardware token integration is mocked locally. |
| **Outbound Webhooks** | ERP/CRM event streaming, HMAC-SHA256, SSRF protection | Implemented in `internal/saas/integrations/service.go` | Hardened safe dialer, IP pinning, redirect blocking, 1MB limit verified. |

---

## 4. Master Architectural Reconciliation Table

Every critical architecture claim is traced to its source code, configuration, test evidence, and verifiable status:

| Architecture Claim | Architecture Spec | Code Reference | Config / IaC Reference | Test Evidence | Classification |
| :--- | :--- | :--- | :--- | :--- | :---: |
| **Zero-Knowledge In-VPC Key Custody** | `docs/01_ARCHITECTURE.md` | `satellite/internal/crypto/engine.go` | `satellite.yaml` (`secrets.provider`) | `scripts/independent_adversarial_test_suite.js` | `IMPLEMENTED_NOT_RUNTIME_VERIFIED` |
| **PostgreSQL Multi-Tenant FORCE RLS** | `docs/21_DATABASE_ARCHITECTURE.md` | `internal/store/postgres/db.go` | Migration `006_...rls.sql` | `scripts/test_postgres_rls_live.sql` | `BLOCKED` (Host Port 5432 Closed) |
| **Transaction-Local Tenant Context** | `docs/06_MULTI_TENANCY.md` | `db.go` (`WithTenantContext`) | Migration `006_...rls.sql` | `scripts/independent_adversarial_test_suite.js` (ATK-011) | `IMPLEMENTED_AND_VERIFIED` (AST / Unit) |
| **BOLA / IDOR REST Rejection (401/403)** | `docs/02_SECURITY.md` | `rest/handlers/contracts.go` | `router.go` | `test_corporate_modules.js` | `IMPLEMENTED_AND_VERIFIED` |
| **AI Prompt Injection Guardrails** | `docs/08_AI_CONTRACT_INTELLIGENCE.md` | `saas/aicontract/service.go` | N/A (In-Memory Engine) | `independent_adversarial_test_suite.js` (ATK-007..009) | `IMPLEMENTED_AND_VERIFIED` |
| **External Google Gemini LLM API** | `docs/08_AI_CONTRACT_INTELLIGENCE.md` | `saas/aicontract/service.go` | N/A | Heuristic Regex Matcher | `DOCUMENTED_ONLY` |
| **8-Layer SSRF Webhook Protection** | `docs/18_INTEGRATION_ARCHITECTURE.md` | `saas/integrations/service.go` | `NewSafeHTTPClient()` | `test_ssrf_runtime_controlled.js` (8/8 Defended) | `IMPLEMENTED_AND_VERIFIED` |
| **Detached RSA-PSS / ECDSA Signatures** | `docs/10_DIGITAL_SIGNATURE_AND_PKI.md` | `saas/signature/service.go` | `crypto/engine.go` | `independent_adversarial_test_suite.js` (ATK-005..006) | `IMPLEMENTED_AND_VERIFIED` |
| **RFC 3161 Public TSA Integration** | `docs/04_CRYPTOGRAPHIC_DESIGN.md` | `saas/signature/service.go` | ASN.1 Token Parser | Synthetic Token Test | `PARTIALLY_IMPLEMENTED` |
| **Physical PKCS#11 Hardware HSM** | `docs/04_CRYPTOGRAPHIC_DESIGN.md` | `satellite/internal/crypto/` | `satellite.yaml` (`hsm_enabled`) | C-API Static Audit | `PARTIALLY_IMPLEMENTED` |
| **AWS CloudHSM / KMS Terraform** | `docs/04_CRYPTOGRAPHIC_DESIGN.md` | `satellite/secrets/providers/aws.go`| `deploy/terraform/aws/main.tf` | Manifest Inspection | `IMPLEMENTED_NOT_RUNTIME_VERIFIED` |
| **Azure / GCP Cloud HSM Integration** | `docs/04_CRYPTOGRAPHIC_DESIGN.md` | Stub Interfaces | `deploy/terraform/azure/`, `gcp/` | Manifest Inspection | `DOCUMENTED_ONLY` |
| **WORM Immutable Audit Triggers** | `docs/17_IMMUTABLE_AUDIT_TRAIL.md` | Migration `006_...rls.sql` | `db.go` | `independent_adversarial_test_suite.js` (ATK-010) | `PARTIALLY_IMPLEMENTED` (Lacks TRUNCATE trigger) |
| **26-Stage Lifecycle State Machine** | `docs/07_CONTRACT_LIFECYCLE_ENGINE.md` | `saas/contract/service.go` | Migration `005_...clm.sql` | `verify_scenarios_a_to_h.js` | `IMPLEMENTED_AND_VERIFIED` |
| **14 Corporate Departments Coverage** | `docs/15_CORPORATE_DEPARTMENTS_GUIDE.md`| `web/admin-portal/app.js` | Seed SQL / Handlers | `test_corporate_modules.js` (10/10 Passed) | `IMPLEMENTED_AND_VERIFIED` |
| **Kubernetes Restricted Pod Security** | `docs/22_CLOUD_AND_KUBERNETES.md` | N/A | `deploy/k8s/saas-control-plane-deployment.yaml` | YAML Schema Validation | `IMPLEMENTED_NOT_RUNTIME_VERIFIED` |
| **Default-Deny K8s NetworkPolicies** | `docs/22_CLOUD_AND_KUBERNETES.md` | N/A | `deploy/k8s/network-policies.yaml` | YAML Schema Validation | `IMPLEMENTED_NOT_RUNTIME_VERIFIED` |

---

## 5. Architectural Findings & Recommended Remediations

1. **AI Engine Transparency:** Update `docs/08_AI_CONTRACT_INTELLIGENCE.md` to state accurately that the current version employs in-process deterministic heuristic parsing, semantic risk classification, and prompt injection defense rather than an active cloud connection to Google Gemini API. This preserves privacy and eliminates third-party cloud data egress.
2. **Audit Trigger Hardening:** Add a `BEFORE TRUNCATE` trigger to `contract_audit_logs` in a new migration to block table-level truncation.
3. **Local Port Offset Configuration:** Standardize local development configuration so Satellite Gateway defaults to `:8082` for REST and `:9092` for vSatellite coordinator, preventing collisions with Control Plane ports `:8080` and `:9090`.
4. **Transport Mode Documentation:** Update `docs/01_ARCHITECTURE.md` to document both the outbound HTTPS long-polling mode (firewall-friendly) and the bi-directional gRPC mTLS streaming mode.

