# Enterprise Contract & Certificate Lifecycle Management (CLM) Platform

> **Version:** 4.0.0  
> **Live Demo:** [https://clm-website-1cc7.vercel.app/](https://clm-website-1cc7.vercel.app/)  
> **GitHub:** [https://github.com/GOKUL-THAN/cmd](https://github.com/GOKUL-THAN/cmd)  
> **Status:** Production Demo — All 476 tests passing  
> **Architecture:** Zero-Trust Hybrid SaaS Control Plane + Customer In-VPC Satellite Gateway  

---

## Table of Contents

1. [Overview](#1-overview)
2. [Quick Start](#2-quick-start)
3. [Live Demo Credentials](#3-live-demo-credentials)
4. [Architecture](#4-architecture)
5. [The 26-Stage Contract Lifecycle](#5-the-26-stage-contract-lifecycle)
6. [Certificate Lifecycle Management](#6-certificate-lifecycle-management)
7. [Admin Portal — All Menu Pages](#7-admin-portal--all-menu-pages)
8. [API Reference](#8-api-reference)
9. [End-to-End Demo Walkthrough](#9-end-to-end-demo-walkthrough)
10. [Technology Stack](#10-technology-stack)
11. [Project Structure](#11-project-structure)
12. [Running Locally](#12-running-locally)
13. [Test Suites](#13-test-suites)
14. [Security Architecture](#14-security-architecture)
15. [400 Enterprise Features](#15-400-enterprise-features)
16. [Production Requirements](#16-production-requirements)
17. [Demo vs Production](#17-demo-vs-production)

---

## 1. Overview

The CLM Platform is a unified enterprise system that manages two interconnected lifecycles:

- **Contract Lifecycle** — 26 stages from intake to AI-assisted archival, covering every department in the organization
- **Certificate/TLS Lifecycle** — Discovery, issuance, deployment, renewal, revocation via in-VPC satellite gateway

### Platform Numbers

| Metric | Value |
|---|---|
| Live contracts | 15 (across all lifecycle stages) |
| Active portfolio value | $14,745,000 |
| Enterprise features | 400 (across 20 domains) |
| API endpoints | 87 |
| Test coverage | 476/476 tests passing |
| Frontend pages | 35 menu pages |
| Go source files | 365 |
| Database migrations | 14 |
| Supported CA providers | 4 (Let's Encrypt, DigiCert, Private CA, AWS ACM) |
| Certificate lifecycle stages | 6 |
| Compliance frameworks | GDPR, PCI-DSS, SOC 2, HIPAA |

---

## 2. Quick Start

### Demo (no dependencies required)

```bash
# Clone the repository
git clone https://github.com/GOKUL-THAN/cmd.git
cd cmd

# Start the demo server (Node.js 18+ required)
node dev-server.js

# Open browser
# Admin Portal:   http://localhost:3000
# Gateway Portal: http://localhost:8080
```

### Or use npm scripts

```bash
npm start                 # Start both portals
npm test                  # Run enterprise scenarios A-H (40 tests)
npm run test:lifecycle    # Run 26-stage lifecycle tests
npm run test:400          # Run 400 feature registry tests
npm run test:corporate    # Run corporate module tests
npm run demo:health       # Health check all endpoints
npm run demo:setup        # Verify setup
npm run demo:stop         # Stop all servers
```

---

## 3. Live Demo Credentials

> These are **demo-only** credentials. Never use in production.

| Role | Email | Password |
|---|---|---|
| Super Admin | `admin@enterprise.internal` | `Demo@2024!` |
| Legal Counsel | `legal-lead@enterprise.internal` | `Demo@2024!` |
| Auditor | `auditor@enterprise.internal` | `Demo@2024!` |
| Finance Approver | `finance@enterprise.internal` | `Demo@2024!` |

**Live URL:** [https://clm-website-1cc7.vercel.app/](https://clm-website-1cc7.vercel.app/)

**Gateway Portal:** [https://clm-website-1cc7.vercel.app/gateway](https://clm-website-1cc7.vercel.app/gateway)

**Health Check:** [https://clm-website-1cc7.vercel.app/api/v1/health](https://clm-website-1cc7.vercel.app/api/v1/health)

---

## 4. Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                    INTERNET (Vercel / Production Cloud)                  │
│                                                                          │
│   Browser/API Client                                                     │
│        │                                                                 │
│        │ HTTPS + JWT (Keycloak OIDC)                                     │
│        ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │            SaaS Control Plane  (port 3000)                      │    │
│  │  Admin Portal │ REST API │ Keycloak Auth │ Fleet Mgmt           │    │
│  │  Contracts │ Approvals │ Templates │ AI Intelligence            │    │
│  │  Obligations │ Risk │ Compliance │ Integrations                │    │
│  │  PostgreSQL + Redis + S3 + Audit Event Sink                    │    │
│  └────────────────────────────────┬────────────────────────────────┘    │
│                                   │ outbound mTLS — NO private keys     │
│              ─── CUSTOMER TRUST BOUNDARY ───                            │
│  ┌────────────────────────────────▼────────────────────────────────┐    │
│  │         Satellite Gateway  (port 8080)                          │    │
│  │  Gateway Portal │ CA Adapters │ Key Custody │ Discovery         │    │
│  │  Certificate Deploy │ ACME │ Policies │ Scheduler              │    │
│  │  Local PostgreSQL │ Vault/HSM refs │ Encrypted spool           │    │
│  └────────────────────────────────┬────────────────────────────────┘    │
│                                   │ authenticated pull                   │
│               ┌───────────────────┼───────────────────┐                 │
│         ┌─────▼──────┐     ┌──────▼──────┐     ┌──────▼──────┐         │
│         │ vSatellite │     │ vSatellite  │     │ vSatellite  │         │
│         │  Cloud/VPC │     │ Kubernetes  │     │  On-Prem    │         │
│         └─────┬──────┘     └──────┬──────┘     └──────┬──────┘         │
│               └───────────────────┼───────────────────┘                 │
│                         deploy/scan/verify                               │
└──────────────────────────────────────────────────────────────────────────┘
```

### Architectural Invariants

| ID | Rule |
|---|---|
| ARC-001 | Customer private keys never enter the SaaS control plane |
| ARC-002 | Operations run at the gateway that can reach the CA and endpoints |
| ARC-003 | Policy is centrally governed and locally enforceable offline |
| ARC-004 | Every command is tenant-scoped, authenticated, and auditable |
| ARC-011 | Tenant data isolated at every layer (DB, cache, queue, logs) |
| ARC-012 | Security-sensitive state is fail-closed |

---

## 5. The 26-Stage Contract Lifecycle

```
MACRO 1 — INTAKE
  Stage 1 ▶ Request Intake & Triage       Self-service portal, dept tagging, budget validation
  Stage 2 ▶ Contract Intake               Auto-triage, risk pre-screening, legal assignment
  Stage 3 ▶ Contract Creation             Wizard metadata capture, counterparty verification

MACRO 2 — AUTHORING
  Stage 4 ▶ Contract Drafting             Dynamic clause assembly, jurisdiction rules
  Stage 5 ▶ Contract Authoring            Collaborative doc generation, variable replacement
  Stage 6 ▶ Template Management           Versioned NDA/MSA/SOW/SLA/Vendor template catalog
  Stage 7 ▶ Clause Library                Pre-approved clause repo with risk ratings

MACRO 3 — NEGOTIATION
  Stage 8 ▶ Contract Negotiation          Counterparty workspace, redlines, round tracking
  Stage 9 ▶ Contract Collaboration        Concurrent comments, internal legal notes
  Stage 10 ▶ Contract Review              Policy pre-flight, deviation flags, diff engine

MACRO 4 — APPROVAL & SIGNING
  Stage 11 ▶ Approval Workflow            L1 Manager → L2 Legal → L3 CISO & Finance
  Stage 12 ▶ Electronic Signature         ESIGN Act / eIDAS compliant, email OTP
  Stage 13 ▶ Digital Signature            X.509 PKI, hardware HSM, non-repudiation

MACRO 5 — EXECUTION
  Stage 14 ▶ Contract Execution           Tamper-evident PDF, sealed document ingestion
  Stage 15 ▶ Contract Repository          Full-text search, metadata filtering

MACRO 6 — POST-EXECUTION
  Stage 16 ▶ Contract Monitoring          SLA health, deadline notifications
  Stage 17 ▶ Obligation Management        Deliverable calendar, milestone tracking, penalties
  Stage 18 ▶ Compliance Management        GDPR, PCI-DSS, SOC 2, HIPAA verification
  Stage 19 ▶ Risk Management              Scorecards, liability caps, blast radius

MACRO 7 — RENEWAL / EXPIRY
  Stage 20 ▶ Contract Amendment           Versioned addendums
  Stage 21 ▶ Contract Renewal             90/60/30-day proactive alerts
  Stage 22 ▶ Contract Expiration          Grace period, auto-state transitions
  Stage 23 ▶ Contract Termination         Runbooks, survival clause tracking

MACRO 8 — ANALYTICS & AI
  Stage 24 ▶ Contract Analytics           Cycle times, bottlenecks, counterparty spend
  Stage 25 ▶ Contract Reporting           PDF/CSV/JSON auditor-ready exports
  Stage 26 ▶ AI Contract Intelligence     Clause extraction, risk detection, Q&A
```

**Test result:** `26/26 Stages Verified — LIFECYCLE VERIFICATION SUMMARY PASSED`

---

## 6. Certificate Lifecycle Management

The **Satellite Gateway Portal** (`/gateway`) manages the full TLS/X.509 certificate lifecycle inside the customer network.

### Certificate Lifecycle Stages

```
Stage 1: Discovery      ──▶  CIDR scanning, DNS resolution, endpoint inventory
Stage 2: Issuance       ──▶  CA orders (Let's Encrypt, DigiCert, Private CA, AWS ACM)
Stage 3: Deployment     ──▶  AWS ALB, K8s Secrets, Nginx, PKCS#12 files
Stage 4: Verification   ──▶  Endpoint health check, chain validation, TLS version
Stage 5: Renewal        ──▶  Automated scheduler, 30-day warning, ACME auto-renew
Stage 6: Revocation     ──▶  OCSP stapling, CRL distribution, emergency revoke
```

### Demo Certificate Inventory

| Domain | Issuer | Key | Days Left | Status |
|---|---|---|---|---|
| `*.enterprise.internal` | DigiCert TLS RSA SHA256 | RSA 2048 | 42 | Active |
| `api-uat.enterprise.internal` | Let's Encrypt | ECDSA P-256 | 12 | **Expiring Soon** |
| `internal-mgmt.local` | Enterprise Private CA | RSA 4096 | 280 | Active |

### CA Providers Configured

| Provider | Type | Status | Orders |
|---|---|---|---|
| Let's Encrypt | ACME | Active | 12 completed |
| DigiCert | API | Active | 3 completed, 1 pending |
| Enterprise Private CA | ADCS | Active | 8 completed |
| AWS ACM Private CA | AWS SDK | Inactive | Not configured |

### Deploy Targets

| Target | Type | Cluster/Host | Status |
|---|---|---|---|
| AWS ALB Production | aws-alb | ap-south-1 | Healthy |
| Kubernetes Secret | k8s-secret | prod-cluster-01 | Healthy |
| Nginx On-Premises | file | 10.125.4.50 | Healthy |

### Certificate Policy Engine

| Policy | Type | Action | Status |
|---|---|---|---|
| Enforce RSA-2048 Minimum | key_strength | Block | Enabled |
| Auto-Renew 30 Days Before | renewal_window | Warn | Enabled |
| Production Requires EV | validation_level | Block | Disabled |
| Block Self-Signed in Prod | self_signed | Block | Enabled |
| Minimum TLS 1.2 | tls_version | Warn | Enabled |

---

## 7. Admin Portal — All Menu Pages

### SaaS Control Plane (`http://localhost:3000`)

| Menu | Page | API Endpoint | Data |
|---|---|---|---|
| Dashboard | Overview stats, satellites, approvals | Multiple | Live |
| Contracts | Repository, 26-stage view, filter | `GET /api/v1/contracts` | 15 contracts, $14.7M |
| Contract Metrics | Portfolio analytics bar | `GET /api/v1/contracts/metrics` | 10 active, 3 negotiating |
| Intake & Requests | Request management, convert to contract | `GET /api/v1/contract-requests` | Live |
| Templates & Clauses | Template catalog, clause library | `GET /api/v1/templates` | 5+ templates |
| Negotiation & Redlines | Counterparty workspace, redline tracking | `GET /api/v1/negotiations/:id` | Active sessions |
| E-Sign & Digital PKI | Signature workflow, evidence bundles | `GET /api/v1/signatures/:id` | 24 signature records |
| Obligation Calendar | Milestone tracking, fulfillment | `GET /api/v1/obligations` | 3 obligations |
| Approval Hub | L1/L2/L3 pending queue | `GET /api/v1/approvals/pending` | 2 pending |
| AI Legal Intelligence | Analysis, extraction, Q&A | `GET /api/v1/ai-contracts/:id/analysis` | Risk + clauses |
| CLM Analytics | Compliance rate, spend analysis | `GET /api/v1/clm-reports/summary` | Live |
| Corporate Depts (14) | Department budgets and contracts | `GET /api/v1/corporate-departments` | 14 departments |
| Procurement & Suppliers | Supplier scorecards | `GET /api/v1/suppliers` | 3 suppliers |
| Legal Operations | Matter management, litigation hold | `GET /api/v1/legal-matters` | 2 matters |
| Financial & Spend | Commitment tracking, 3-way match | `GET /api/v1/financials` | Live |
| Risk Register | Cryptographic risk posture | `GET /api/v1/risk/posture` | Score: 28 |
| Webhooks & Integrations | Slack, ServiceNow, webhook mgmt | `GET /api/v1/integrations` | 2 integrations |
| ITSM War Room | Incident + service request tracking | `GET /api/v1/itsm/incidents` | Live |
| Departments & Budgets | Cost centers, cert quotas | `GET /api/v1/departments` | 3 departments |
| Cloud Accounts | AWS/GCP cloud account mgmt | `GET /api/v1/cloud-accounts` | 2 accounts |
| Gateways | Fleet management, deploy history | `GET /api/v1/satellites` | 1 gateway |
| Deploy Jobs | Terraform deploy job tracking | `GET /api/v1/satellites/deploy` | Live |
| Identity Management | Keycloak users, roles, groups | `GET /api/v1/identity/status` | Live |
| Onboarding | New customer preflight + bootstrap | `POST /api/v1/onboarding/preflight` | Live |
| Directory Mappings | Group → role SSO mappings | `GET /api/v1/tenants/:id/sso/mappings` | Live |
| 400 Enterprise Features | Full feature domain explorer | `GET /api/v1/features` | 400 features |
| SDLC Workflows | Kiro workflow engine | `GET /api/v1/workflows/audit-log` | Live |
| Activity Log | Workflow + event history | Multiple | Live |
| Organization Hub | Tenants, subscription, billing | `GET /api/v1/tenants` | 2 tenants |

### Gateway Portal (`http://localhost:8080`)

| Menu | Data |
|---|---|
| Dashboard | Gateway status, cert counts, alerts |
| Certificate Inventory | 3 certs with expiry, status, key info |
| Discover & Scan | CIDR + DNS discovery engine |
| Issue Certificate | CA selection, CSR, order tracking |
| Deploy to Endpoints | Target selection, deployment jobs |
| CA Management | Provider config, accounts, order history |
| Policies | Enforcement rules engine |
| Scheduler | Automated job management |
| Credentials | Deploy + CA credential vault |
| Alerts | Critical/warning/info alert feed |
| Audit Log | Tamper-evident operation history |
| vSatellite Agents | Enrollment, heartbeat, capabilities |
| Compliance Report | GDPR/PCI-DSS/SOC 2/HIPAA posture |
| Multi-Cloud Sync | AWS ACM + GCP Certificate Manager |

---

## 8. API Reference

All endpoints respond to `https://clm-website-1cc7.vercel.app/api/v1/...`

### Contract Management

```
GET    /api/v1/contracts                         List all contracts
POST   /api/v1/contracts                         Create new contract
GET    /api/v1/contracts/metrics                 Portfolio metrics
POST   /api/v1/contracts/:id/transition          Advance lifecycle stage
GET    /api/v1/contracts/:id/versions            Version history
GET    /api/v1/contract-requests                 List intake requests
POST   /api/v1/contract-requests                 Submit new request
POST   /api/v1/contract-requests/:id/convert     Convert to contract
GET    /api/v1/templates                         List contract templates
POST   /api/v1/templates/:id/instantiate         Create from template
GET    /api/v1/clauses                           Clause library
```

### Negotiation & Signatures

```
GET    /api/v1/negotiations/:id                  Negotiation session
GET    /api/v1/negotiations/:id/redlines         Redline list
POST   /api/v1/negotiations/:id/redlines         Submit redline
POST   /api/v1/negotiations/:id/redlines/:id/resolve  Resolve redline
GET    /api/v1/signatures/:id                    Signers for contract
POST   /api/v1/signatures/:id/sign               Execute signature
GET    /api/v1/signatures/:id/evidence           Cryptographic evidence bundle
```

### AI & Analytics

```
GET    /api/v1/ai-contracts/:id/analysis         AI contract analysis
POST   /api/v1/ai-contracts/:id/analyze          Trigger AI analysis
POST   /api/v1/ai-contracts/:id/ask              AI Q&A (natural language)
GET    /api/v1/clm-reports/summary               CLM analytics dashboard
GET    /api/v1/corporate-departments             14-department breakdown
```

### Approvals, Obligations, Risk

```
GET    /api/v1/approvals/pending                 Pending approval queue
POST   /api/v1/approvals/:id/approve             Approve request
POST   /api/v1/approvals/:id/reject              Reject request
GET    /api/v1/obligations                       Active obligations
GET    /api/v1/obligations/summary               Obligation KPIs
POST   /api/v1/obligations/:id                   Update obligation status
GET    /api/v1/risk/posture                      Risk register
GET    /api/v1/suppliers                         Supplier directory
GET    /api/v1/legal-matters                     Legal matter tracker
GET    /api/v1/financials                        Financial commitments
POST   /api/v1/financials/three-way-match        Reconcile PO/GR/Invoice
```

### Certificate Lifecycle (Gateway)

```
GET    /api/v1/status                            Gateway health
GET    /api/v1/certificates                      Certificate inventory
POST   /api/v1/certificates/issue                Issue new certificate
GET    /api/v1/certificates/:id                  Certificate detail
GET    /api/v1/ca/providers                      CA provider list
GET    /api/v1/ca/orders                         CA order history
POST   /api/v1/ca/quote                          Get CA pricing quote
GET    /api/v1/inventory                         Full cert inventory
GET    /api/v1/inventory/deployments             Deployment records
POST   /api/v1/discover                          Start CIDR discovery scan
POST   /api/v1/discover/dns                      DNS discovery
POST   /api/v1/deploy                            Deploy certificate
GET    /api/v1/deploy/targets                    Available deploy targets
GET    /api/v1/deploy/endpoints                  Monitored endpoints
GET    /api/v1/policies                          Certificate policies
POST   /api/v1/policies/evaluate                 Evaluate cert against policies
GET    /api/v1/scheduler                         Automated job schedule
POST   /api/v1/scheduler/run/:id                 Trigger job manually
GET    /api/v1/lifecycle                         Certificate lifecycle stages
GET    /api/v1/alerts                            Alert feed
GET    /api/v1/audit                             Gateway audit log
GET    /api/v1/credentials                       Credential vault
GET    /api/v1/acme/status                       ACME client status
POST   /api/v1/rotation/execute/:id              Execute cert rotation
GET    /api/v1/vsatellites                       vSatellite agent list
POST   /api/v1/vsatellites/enroll                Enroll new agent
GET    /api/v1/reports/compliance                Compliance report
```

### Identity & Tenants

```
GET    /api/v1/tenants                           Tenant list
GET    /api/v1/tenants/:id                       Tenant detail
GET    /api/v1/tenants/me                        Current caller tenant
GET    /api/v1/tenants/:id/sso/mappings          SSO group mappings
GET    /api/v1/users                             User list
GET    /api/v1/users/:id                         User detail
GET    /api/v1/identity/status                   Keycloak status
GET    /api/v1/identity/users                    IAM users
GET    /api/v1/identity/roles                    IAM roles
GET    /api/v1/identity/groups                   IAM groups
GET    /api/v1/identity/providers                SSO providers
GET    /api/v1/identity/sessions                 Active sessions
```

---

## 9. End-to-End Demo Walkthrough

### Step 1 — Login

1. Open [https://clm-website-1cc7.vercel.app/](https://clm-website-1cc7.vercel.app/)
2. Enter: `admin@enterprise.internal` / `Demo@2024!`
3. Click **Sign In**

### Step 2 — Dashboard

View the overview showing:
- **2 tenants**, **5 users**, **1 satellite gateway**
- **15 contracts** worth **$14.7M** total portfolio value
- **2 pending approvals** (L2 Security Review + L3 CISO Signoff)

### Step 3 — Contract Repository

Navigate to **Contracts Repository** to see all 15 contracts:
- Filter by type: MSA, NDA, SOW, SLA, Vendor, Employment
- Filter by lifecycle stage: Intake → Authoring → Negotiation → Approval → Signature → Post-Execution
- Click any contract to view details, version history, risk score, compliance status

### Step 4 — Create a Contract

1. Click **+ New Contract** button
2. Fill in title, type, counterparty, value, department
3. Select a template (MSA, NDA, SOW, SLA)
4. Submit — contract created at **Intake** stage

### Step 5 — Negotiate a Contract

1. Open `cnt-002` (Cloud Infrastructure — in negotiation)
2. Navigate to **Negotiations** tab
3. View active redlines from counterparty
4. Accept or reject each redline with comments

### Step 6 — Approval Workflow

1. Navigate to **Approvals Hub**
2. View 2 pending approvals:
   - `appr-001`: L2 — wildcard SSL certificate creation
   - `appr-002`: L3 — EV Multi-Domain cert purchase ($899)
3. Click **Approve** or **Reject** with comment
4. Observe multi-tier progression

### Step 7 — E-Signature

1. Open `cnt-003` (Mutual NDA — ready for signature)
2. Navigate to **E-Sign & Digital PKI** tab
3. View 2 signers: 1 signed, 1 awaiting
4. Click **Sign Contract** for the pending signer
5. View the cryptographic evidence bundle (SHA-256 hash, timestamp, compliance certifications)

### Step 8 — AI Contract Intelligence

1. Navigate to **AI Legal Intelligence**
2. Select any contract
3. View auto-generated analysis: risk score, extracted clauses, compliance flags
4. Ask the AI Legal Assistant: *"What are the payment terms?"*
5. Receive answer with section citations and confidence score

### Step 9 — Certificate Management (Gateway Portal)

1. Open [https://clm-website-1cc7.vercel.app/gateway](https://clm-website-1cc7.vercel.app/gateway)
2. View 3 certificates — note `api-uat.enterprise.internal` expiring in 12 days
3. Navigate to **CA Management** → view 4 providers
4. Navigate to **Discover** → trigger CIDR scan on `10.125.0.0/16`
5. Navigate to **Policies** → see enforcement rules in action
6. Navigate to **Scheduler** → trigger a renewal job manually

### Step 10 — Compliance & Audit

1. Navigate to **CLM Analytics & Reports** — view 92% compliance rate
2. Navigate to **Risk Register** — risk score 28, PQC transition readiness flag
3. Navigate to **Activity Log** — view immutable audit trail
4. Gateway: navigate to **Compliance Report** — GDPR/PCI-DSS/SOC 2/HIPAA green

---

## 10. Technology Stack

### Demo / Frontend

| Component | Technology |
|---|---|
| Server | Node.js 24 (built-in `http` module) |
| Admin Portal | Vanilla JS, HTML5, CSS3 |
| Gateway Portal | Vanilla JS, HTML5, CSS3 |
| API Engine | dev-server.js (in-memory mock, 87 routes) |
| Deployment | Vercel (serverless, `/api/index.js`) |
| Auth (demo) | Mock JWT (`mock-jwt-token-clm-admin-access`) |

### Production Backend (Go)

| Component | Technology |
|---|---|
| Framework | `gin` v1.9.1 |
| Database | PostgreSQL (`lib/pq` v1.10.9) + Row Level Security |
| Cache | Redis |
| Auth | Keycloak + JWT (`golang-jwt` v5.2.1) |
| Cloud SDK | AWS SDK v2 (ACM, EKS, IAM, ALB, Secrets Manager) |
| Logging | `go.uber.org/zap` |
| CLI | Cobra + Viper |
| Cryptography | `golang.org/x/crypto` |
| Identity | UUID (`github.com/google/uuid`) |

### Infrastructure

| Component | Technology |
|---|---|
| Containerization | Docker |
| Orchestration | Kubernetes (k8s manifests in `deploy/k8s/`) |
| IaC | Terraform (AWS, Azure, GCP under `deploy/terraform/`) |
| Observability | Prometheus + Grafana (`deploy/observability/`) |
| CI/CD | GitHub Actions (`.github/workflows/ci.yml`) |

---

## 11. Project Structure

```
CLM/
├── dev-server.js                    # Unified demo server (87 API routes)
├── api/
│   └── index.js                     # Vercel serverless function bridge
├── public/
│   ├── index.html                   # SaaS Admin Portal (port 3000)
│   ├── app.js                       # Admin portal app (~6000 lines)
│   └── gateway/
│       ├── index.html               # Gateway Portal (port 8080)
│       └── app.js                   # Gateway portal app (~5000 lines)
├── components/
│   ├── saas-control-plane/          # Go backend: SaaS orchestration
│   │   ├── internal/
│   │   │   ├── api/                 # REST + gRPC handlers
│   │   │   ├── core/                # Shared domain interfaces
│   │   │   ├── saas/                # Domain packages (20 packages)
│   │   │   │   ├── contract/        # 26-stage contract engine
│   │   │   │   ├── approval/        # Multi-tier approval workflow
│   │   │   │   ├── signature/       # E-sign + PKI
│   │   │   │   ├── negotiation/     # Redlines + collaboration
│   │   │   │   ├── aicontract/      # AI intelligence
│   │   │   │   ├── obligation/      # Post-execution tracking
│   │   │   │   ├── risk/            # Risk scorecards
│   │   │   │   ├── template/        # Template + clause library
│   │   │   │   ├── tenant/          # Multi-tenant management
│   │   │   │   ├── identity/        # Keycloak IAM
│   │   │   │   ├── integrations/    # Webhooks, ServiceNow, Slack
│   │   │   │   └── ...              # 10 more packages
│   │   │   └── store/               # PostgreSQL + Redis store
│   │   ├── deploy/
│   │   │   └── migrations/          # 14 PostgreSQL migrations
│   │   └── web/admin-portal/        # Compiled admin portal assets
│   ├── satellite-gateway/           # Go backend: In-VPC gateway
│   │   └── internal/
│   │       ├── ca/                  # CA adapters (ACME, DigiCert, ADCS)
│   │       ├── crypto/              # Key gen, CSR, HSM
│   │       ├── deployment/          # Certificate deployment engine
│   │       ├── discovery/           # CIDR + DNS scanning
│   │       ├── policy/              # Policy enforcement
│   │       ├── scheduler/           # Renewal + verification jobs
│   │       └── audit/               # Immutable event log
│   ├── vsatellite/                  # Lightweight endpoint agent
│   └── shared/                      # Common libraries
├── deploy/
│   ├── k8s/                         # 3 Kubernetes manifests
│   ├── terraform/                   # 3 Terraform configs (AWS/Azure/GCP)
│   └── observability/               # Prometheus + Grafana
├── features/                        # 400-feature registry (20 domains)
├── scripts/                         # 24 test + utility scripts
│   ├── verify_scenarios_a_to_h.js   # 40 enterprise scenario tests
│   ├── test_lifecycle_26_stages.js  # 26 lifecycle stage tests
│   ├── test_400_features.js         # 400 feature smoke tests
│   ├── test_corporate_modules.js    # 10 corporate module tests
│   ├── health-check.js              # API health checker
│   ├── start-demo.js                # Demo startup script
│   └── stop-demo.js                 # Demo shutdown script
├── .kiro/
│   ├── workflows/                   # 4 Kiro AI workflow definitions
│   │   ├── clm-feature-dev.workflow.json
│   │   ├── clm-bugfix.workflow.json
│   │   ├── clm-plan-only.workflow.json
│   │   └── clm-specfast.workflow.json
│   └── steering/                    # AI context files
├── vercel.json                      # Vercel deployment configuration
├── package.json                     # Node.js package (v4.0.0)
├── Makefile                         # Go build targets
└── README.md                        # This file
```

---

## 12. Running Locally

### Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| Node.js | >= 18.0.0 | Demo server (required) |
| Go | 1.22 | Production backend (optional) |
| PostgreSQL | 14+ | Production database (optional) |
| Docker | latest | Container builds (optional) |

### Demo Mode (Node.js only)

```bash
# Start both portals
node dev-server.js

# Portals available at:
#   http://localhost:3000   Admin Portal
#   http://localhost:8080   Gateway Portal

# Stop server
node scripts/stop-demo.js
```

### Health Check

```bash
node scripts/health-check.js

# Or via curl
curl https://clm-website-1cc7.vercel.app/api/v1/health
```

### Production Mode (requires Go + PostgreSQL)

```bash
# Build Go services
make -C components/saas-control-plane build
make -C components/satellite-gateway build

# Apply database migrations
psql -U clm_app -d clm -f components/saas-control-plane/deploy/migrations/*.sql

# Start with Docker Compose
docker-compose -f components/saas-control-plane/deploy/docker-compose.yml up
```

### Environment Variables

Copy `.env.example` to `.env.local` for configuration:

```bash
cp .env.example .env.local
```

Key variables:

```env
# Demo mode (no DB/Redis needed)
CLM_DEMO_MODE=true

# Production
DATABASE_URL=postgresql://clm_app:changeme@localhost:5432/clm
REDIS_URL=redis://localhost:6379
KEYCLOAK_URL=https://auth.enterprise.internal
KEYCLOAK_REALM=clm
JWT_SECRET=<256-bit-random-secret>
AWS_REGION=ap-south-1
```

---

## 13. Test Suites

```bash
# Suite 1: Enterprise Scenarios A through H
npm test
# VERIFICATION SUMMARY: 40/40 TESTS PASSED

# Suite 2: 26 Contract Lifecycle Stages
npm run test:lifecycle
# LIFECYCLE VERIFICATION SUMMARY: 26/26 Stages Verified (0 Failed)

# Suite 3: 400 Feature Registry
npm run test:400
# SUMMARY: 400/400 REGISTRY SMOKE CHECKS PASSED

# Suite 4: Corporate Modules
npm run test:corporate
# ALL 10 TEST SUITES (CORPORATE MODULES & FIXES) PASSED OK!

# Run all suites
npm run demo:test
# Total: 476/476 tests passing
```

### What the Tests Verify

| Scenario | Tests | What it checks |
|---|---|---|
| A — Contract Lifecycle Flow | 4 | Intake → Authoring → Negotiation → Execution |
| B — Obligation & Financial | 4 | Milestones, 3-way invoice match, CLM analytics |
| C — Multi-Tenant Isolation | 4 | BOLA protection, cross-tenant data blocking |
| D — Delegation & Segregation | 4 | DOA matrix, approval segregation of duties |
| E — Digital Signature & PKI | 4 | SHA-256 digest, zero-knowledge verification |
| F — Satellite Gateway Resilience | 4 | Offline mode, queue drain, zero-egress AI |
| G — AI Safety & Prompt Injection | 4 | LLM guardrails, citation verification |
| H — Audit Trail Immutability | 4 | WORM triggers, RLS, tamper detection |

---

## 14. Security Architecture

### Zero-Trust Model

- **No private keys in SaaS** — All cryptographic operations stay inside the satellite gateway
- **mTLS** — All gateway-to-SaaS communication uses mutual TLS
- **Tenant RLS** — PostgreSQL Row Level Security on every table
- **JWT validation** — Every API request requires a valid JWT
- **Fail-closed** — Expired policy, invalid identity → block, never fallback to insecure default

### Compliance

| Framework | Coverage |
|---|---|
| GDPR | Data subject rights, retention, breach notification |
| PCI-DSS v4 | Cardholder data protection, certificate standards |
| SOC 2 Type II | Availability, confidentiality, security controls |
| HIPAA | PHI technical safeguards |

### Multi-Tenant Isolation

```
Tenant A ──── tenant_id = "tenant-A" ──── [RLS Policy] ──── DB Row A
Tenant B ──── tenant_id = "tenant-B" ──── [RLS Policy] ──── DB Row B
                                                               ↑
                                              Cross-tenant query = BLOCKED (BOLA)
```

### RBAC Roles

| Role | Capabilities |
|---|---|
| super_admin | Full system, cross-tenant metadata |
| legal_counsel | Contract authoring, redlines, approvals |
| procurement_manager | Supplier onboarding, purchase agreements |
| approver | Review and approve/reject requests |
| auditor | Read-only audit trail and reports |
| viewer | Read-only inventory and health summaries |

---

## 15. 400 Enterprise Features

The platform covers 400 enterprise CLM features across 20 operational domains:

| Domain | Features | Description |
|---|---|---|
| Core Contract Repository | 1–47 | Full contract lifecycle, AI intelligence |
| Specialized Agreements | 48–63 | Vendor, Customer, NDA, MSA, SOW, SLA, Employment |
| Legal Operations | 64–83 | ITSM integration, legal matters, outside counsel |
| Document Intelligence | 84–100 | OCR, extraction, comparison, deduplication |
| Advanced Enterprise CLM | 101–120 | Multi-entity, multi-currency, portfolios |
| Section A — Intake | 121–138 | Self-service portal, triage, SLA tracking |
| Section B — Authoring | 139–154 | Conditional assembly, playbooks, fallbacks |
| Section C — Negotiation | 155–168 | External workspace, redline management |
| Section D — Legal Ops | 169–182 | Legal cases, hold management, legal spend |
| Section E — Procurement | 183–201 | Supplier scorecards, diversity, RFPs |
| Section F — Financial | 202–220 | Milestone payments, 3-way match, leakage |
| Section G — Obligations | 221–239 | Evidence tracking, SLA credits, penalties |
| Section H — Risk | 240–259 | Risk register, questionnaires, sanctions |
| Section I — AI Intelligence | 260–284 | NLP Q&A, citations, confidence scoring |
| Section J — Governance | 285–300 | DOA matrix, PoA, segregation of duties |
| Section K — Digital Signing | 301–320 | Tamper-evident bundles, PKI seal |
| Section L — Repository | 321–340 | Folder hierarchy, parent/child, deduplication |
| Section M — Integration | 341–360 | SAP, Oracle, Salesforce, Coupa, ServiceNow |
| Section N — Reporting | 361–375 | Cycle times, bottlenecks, renewal forecast |
| Section O — Security | 376–400 | Multi-tenancy, RBAC/ABAC, audit logs |

---

## 16. Production Requirements

To run this platform in production, you need:

| Requirement | Purpose | Notes |
|---|---|---|
| Go 1.22+ toolchain | Compile 365 Go source files | 20 backend packages |
| PostgreSQL 14+ | Persistent data store | 14 migrations ready |
| Redis | Session cache, queue | Any Redis-compatible |
| Keycloak 22+ | Enterprise SSO | OIDC + SAML |
| AWS Account | ACM, EKS, Secrets Manager | Terraform ready |
| Docker + Kubernetes | Container orchestration | K8s manifests in `deploy/k8s/` |
| HSM / Vault | Private key custody | In-VPC only |
| SMTP Service | Email notifications | Approval alerts |
| S3-compatible store | Document storage | Contract PDFs |
| AI/LLM API | Contract intelligence | Pluggable |

---

## 17. Demo vs Production

| Feature | Demo Mode | Production |
|---|---|---|
| Data persistence | In-memory (resets on restart) | PostgreSQL + S3 |
| Authentication | Mock JWT (any creds work) | Keycloak OIDC |
| Signatures | Simulated hash | Real X.509 PKI |
| AI responses | Hardcoded mock answers | Real LLM API |
| Cloud connectivity | Mocked SDK calls | Real AWS/GCP |
| Certificate issuance | Mock order simulation | Real CA API calls |
| Email notifications | Not sent | SMTP delivery |
| Multi-tenant isolation | Simulated (BOLA test) | PostgreSQL RLS |
| Startup | `node dev-server.js` (< 2s) | Docker Compose + K8s |

---

## Contributing

This project uses Kiro AI-assisted development workflows:

```bash
# Feature development
feature: <description>

# Bug fix
bug: <description>

# Plan only (no code changes)
plan: <description>

# Rapid spec generation
spec: <description>
```

Workflow files are in `.kiro/workflows/`. Steering files are in `.kiro/steering/`.

---

## License

UNLICENSED — Enterprise proprietary. All rights reserved.

**M2P Engineering Team** · Built with [Kiro](https://kiro.dev) · Deployed on [Vercel](https://vercel.com)