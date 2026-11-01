# CLM-SPEC-01: SYSTEM ARCHITECTURE SPECIFICATION
## Zero-Trust Hybrid SaaS Control Plane + Customer In-VPC Satellite Gateway

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-ARCH-001  
**Classification:** Enterprise Engineering Architecture Standard  
**Release Classification:** STAGING CANDIDATE — LIVE INFRASTRUCTURE VERIFICATION PENDING  

---

### 1. Architectural Philosophy & Zero-Trust Tenet

The Enterprise Contract & Certificate Lifecycle Management (CLM) Platform is architected to address the enterprise paradox: organizations demand the scalability, collaboration, and centralized visibility of a cloud SaaS platform, but compliance, data privacy, and legal liability prohibit exposing proprietary cryptographic signing keys or unencrypted master documents to third-party multi-tenant environments.

To reconcile this, the platform enforces a **Zero-Trust Hybrid Topology**:
1. **Control Plane (Multi-Tenant SaaS):** Handles state machine orchestration, workflow routing, approval chains, metadata indexing, deterministic heuristic AI risk scoring, and administrative UI rendering.
2. **Satellite Gateway (Customer In-VPC Enclave):** Deployed within the customer's private cloud VPC or on-premises data center. Retains sole custody of cryptographic private signing keys (in hardware HSMs or customer KMS), executes document envelope encryption/decryption, and signs contracts locally.
3. **Outbound-Only mTLS Connectivity:** In compliance with enterprise network security perimeter standards, customer firewalls and NAT gateways prohibit inbound connections from public clouds. Therefore, the **Customer In-VPC Satellite Gateway initiates an outbound-only, persistent Mutual TLS 1.3 (mTLS) session** to the SaaS Control Plane API Gateway. This tunnel carries bidirectional, encrypted command-and-response protocol buffers.
4. **Data Boundary Integrity:** While the SaaS Control Plane handles active draft texts in Go memory during authoring and clause classification, documents at rest are protected by AES-256-GCM envelope encryption where master keys remain strictly under customer custody. Furthermore, AI clause analysis runs entirely in-process with **zero egress** to third-party LLM providers.

---

### 2. End-to-End Hybrid Architecture Diagram

```mermaid
flowchart TB
    subgraph CLIENT_BOUNDARY["Enterprise Client Ecosystem"]
        direction LR
        BrowserClient["Enterprise Web Browser<br/>[IMPLEMENTED: React / Admin Portal]"]
        ExternalParty["External Counterparty<br/>[IMPLEMENTED: Negotiation Portal]"]
        ThirdPartyERP["Enterprise ERP / CRM<br/>[IMPLEMENTED: SAP / NetSuite HMAC Connectors]"]
    end

    subgraph SAAS_CONTROL_PLANE["SaaS Multi-Tenant Control Plane (Cloud Infrastructure)"]
        direction TB
        
        subgraph INGRESS_SECURITY["Ingress & Perimeter Security"]
            CloudWAF["Cloud WAF / DDoS Guard<br/>[PROPOSED: AWS WAF / Rate Limiter]"]
            APIGateway["Go Chi API Gateway / Router<br/>[IMPLEMENTED: router.go / mTLS Endpoint]"]
            SecretsVault["Secrets Management<br/>[PROPOSED: AWS Secrets Manager / Vault]"]
            CertManager["Automated Cert Rotation<br/>[PROPOSED: cert-manager / ACME]"]
        end

        subgraph CORE_ENGINES["Core Business & Orchestration Engines"]
            LifecycleEngine["26-Stage Lifecycle Orchestrator<br/>[IMPLEMENTED: saas/contract/service.go]"]
            AIHeuristic["Deterministic Legal Heuristic Engine<br/>[IMPLEMENTED: saas/aicontract/service.go]"]
            ApprovalEngine["Multi-Tier DOA & SoD Engine<br/>[IMPLEMENTED: models/contract.go (ARC-004)]"]
            ProcurementMatcher["Procurement 3-Way Matcher<br/>[IMPLEMENTED: Migration 006 & models]"]
            LegalOpsEngine["Legal Ops & Litigation Holds<br/>[IMPLEMENTED: Migration 005/006]"]
            CryptoService["Cryptographic Signature Service<br/>[IMPLEMENTED: saas/signature/service.go]"]
        end

        subgraph ASYNC_PROCESSING["Asynchronous Task Pipeline & Reliability"]
            EventBroker["Async Event Broker / Queue<br/>[PROPOSED: NATS JetStream / Kafka]"]
            DeadLetterQueue["Dead Letter Queue (DLQ)<br/>[PROPOSED: Failed Task Quarantine]"]
            WorkerNodes["Background Workers<br/>[IMPLEMENTED: Monitoring & SLA Checkers]"]
        end

        subgraph PERSISTENCE_LAYER["Enterprise Persistence Tier"]
            direction LR
            PostgresDB[("PostgreSQL 15+ Cluster (HA)<br/>[IMPLEMENTED: Migrations 000-008]<br/>- Four-Tier DB Roles<br/>- Migration 007 Chain-Heads FOR UPDATE<br/>- Migration 008 Full FORCE RLS<br/>- Truncate & Mutation Triggers")]
            S3Vault[("Encrypted S3 Document Vault<br/>[PROPOSED / CONFIG: AWS S3]<br/>- AES-256-GCM Envelope Ciphertext<br/>- Object Lock Compliance Mode WORM<br/>- Cross-Region Replication (CRR)")]
        end

        subgraph MONITORING_TIER["Telemetry & Observability Tier"]
            PrometheusNode["Prometheus Metrics (/metrics)<br/>[IMPLEMENTED: monitor.go]"]
            OpenTelemetryNode["OpenTelemetry Collector<br/>[IMPLEMENTED: W3C TraceContext]"]
        end
    end

    subgraph SATELLITE_ENCLAVE["Customer In-VPC Satellite Enclave (Private Network / VPC)"]
        direction TB
        SatelliteNode["Customer Satellite Gateway Daemon<br/>[IMPLEMENTED: cmd/clm-satellite/main.go]"]
        CustomerKMS["Customer KMS / CloudHSM (PKCS#11)<br/>[PROPOSED / HYBRID: Hardware Key Custody]"]
        LocalQueue["Local Durable Outage Queue<br/>[IMPLEMENTED: Satellite Local Store]"]
        CustomerIdP["Enterprise Identity / Active Directory<br/>[IMPLEMENTED: Keycloak / SAML / OIDC]"]
        CertRotationAgent["Satellite Cert Auto-Renewal<br/>[PROPOSED: In-VPC Cert Watcher]"]
    end

    %% Client Traffic Ingress
    BrowserClient -->|"HTTPS (TLS 1.3)"| CloudWAF
    ExternalParty -->|"HTTPS (TLS 1.3)"| CloudWAF
    ThirdPartyERP -->|"Signed Webhooks (HMAC-SHA256)"| CloudWAF
    CloudWAF --> APIGateway
    SecretsVault -.->|Inject Credentials| APIGateway
    CertManager -.->|Rotate TLS Certs| APIGateway

    %% Control Plane Internal Flow
    APIGateway --> LifecycleEngine
    APIGateway --> ApprovalEngine
    APIGateway --> CryptoService

    LifecycleEngine --> AIHeuristic
    LifecycleEngine --> ProcurementMatcher
    LifecycleEngine --> LegalOpsEngine
    LifecycleEngine --> EventBroker

    EventBroker --> WorkerNodes
    EventBroker -.->|Max Retries Exceeded| DeadLetterQueue
    WorkerNodes --> PostgresDB

    LifecycleEngine --> PostgresDB
    ApprovalEngine --> PostgresDB
    ProcurementMatcher --> PostgresDB
    LegalOpsEngine --> PostgresDB
    CryptoService --> PostgresDB
    CryptoService --> S3Vault

    %% Outbound-Initiated mTLS Connection from Customer VPC to SaaS Control Plane
    SatelliteNode ==>|"Outbound-Initiated mTLS 1.3 Tunnel<br/>(Bi-Directional Command Streaming)"| APIGateway

    %% Inside Satellite Enclave
    SatelliteNode <-->|"PKCS#11 C_Sign API"| CustomerKMS
    SatelliteNode <--> LocalQueue
    SatelliteNode <--> CustomerIdP
    CertRotationAgent -.->|Rotate Client Cert| SatelliteNode

    %% Telemetry Connections
    APIGateway --> PrometheusNode
    LifecycleEngine --> OpenTelemetryNode
    SatelliteNode -.->|Heartbeat (30s)| APIGateway

    classDef boundary fill:#f8f9fa,stroke:#333,stroke-width:2px;
    classDef saas fill:#eef2ff,stroke:#4f46e5,stroke-width:2px;
    classDef satellite fill:#ecfdf5,stroke:#059669,stroke-width:2px;
    classDef storage fill:#fffbeb,stroke:#d97706,stroke-width:2px;
    classDef telemetry fill:#fdf2f8,stroke:#db2777,stroke-width:2px;
    class CLIENT_BOUNDARY boundary;
    class SAAS_CONTROL_PLANE saas;
    class SATELLITE_ENCLAVE satellite;
    class PERSISTENCE_LAYER storage;
    class MONITORING_TIER telemetry;
```

---

### 3. Component Taxonomy & Implementation Status

| Component | Architecture Role | Codebase Path / Artifact | Status |
| :--- | :--- | :--- | :---: |
| **API Gateway / Router** | Reverse proxy, JWT auth, rate limiter, SSRF guard | `components/saas-control-plane/internal/api/rest/router.go` | **IMPLEMENTED** |
| **26-Stage Lifecycle Engine** | Macro-stages 1 to 8 workflow orchestration | `components/saas-control-plane/internal/saas/contract/service.go` | **IMPLEMENTED** |
| **AI Legal Heuristic Engine** | In-process regex, dictionary, risk scorer (zero egress) | `components/saas-control-plane/internal/saas/aicontract/service.go` | **IMPLEMENTED** |
| **Multi-Tier DOA & SoD Engine** | ARC-004 Requester != Approver check & DOA tiers | `components/saas-control-plane/pkg/models/contract.go` | **IMPLEMENTED** |
| **Cryptographic Service** | Version binding, replay defense, signature verify | `components/saas-control-plane/internal/saas/signature/service.go` | **IMPLEMENTED** |
| **Customer Satellite Daemon** | In-VPC enclave, local signing, envelope crypto | `components/satellite-gateway/cmd/clm-satellite/main.go` | **IMPLEMENTED** |
| **Database Engine** | PostgreSQL 15, RLS, Migration 007/008 WORM triggers | `components/saas-control-plane/deploy/migrations/` | **IMPLEMENTED** |
| **Evidence Storage Vault** | AWS S3 Object Lock Compliance Mode WORM (7 years) | Cloud infrastructure configuration standard | **PROPOSED** |
| **Hardware PKCS#11 HSM** | Thales Luna, Entrust nShield, AWS CloudHSM | Hardware integration API standard | **PROPOSED / HYBRID** |
| **Async Task Broker & DLQ** | Distributed messaging queue with dead-lettering | Infrastructure messaging standard (NATS / Kafka) | **PROPOSED** |
| **Cloud WAF & DDoS Shield** | Layer 7 perimeter inspection and rate-limiting | Cloud perimeter configuration (AWS WAF / Cloudflare) | **PROPOSED** |

---

### 4. Cross-Boundary Communication Protocol

1. **Outbound-Only Connection Model:** The In-VPC Satellite Gateway initiates an outbound TLS 1.3 TCP connection to the SaaS Control Plane API Gateway. No inbound firewall ports or NAT pinholes are opened into the customer VPC.
2. **Mutual Authentication (mTLS):** Both parties present X.509 certificates. The Control Plane verifies the Satellite's certificate against the customer's registered tenant CA; the Satellite verifies the Control Plane certificate against pinned root fingerprints.
3. **Connection Recovery:** If the mTLS uplink drops, the Satellite automatically retries with exponential backoff and randomized jitter: intervals at 1s, 2s, 4s, 8s, 16s, up to a maximum of 30s.
4. **Queue Durability & DLQ:** Commands received during offline windows or failed operations are buffered in local durable storage. Operations exceeding maximum retry attempts are diverted to a local Dead-Letter Queue (DLQ) with alert notification.
5. **Payload Confidentiality:** Payloads contain document digests, metadata, and encrypted keys. Cleartext contract bodies and private signing keys are never transmitted over the uplink.
