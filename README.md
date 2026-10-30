# Certificate Lifecycle Management (CLM)

> Enterprise product requirements, target architecture, portal specification, and delivery baseline.
>
> **Document status:** Product requirements baseline  
> **Last reviewed:** 2026-08-07  
> **Scope:** Public and private X.509/TLS certificates across SaaS, cloud, Kubernetes, operating systems, databases, network appliances, and on-premises environments.

CLM is a multi-tenant certificate lifecycle management platform for discovering, owning, issuing, deploying, monitoring, renewing, rotating, and revoking certificates without moving customer private keys into the SaaS control plane.

This document defines what the enterprise product **must become**. It is not a claim that every requirement is implemented. Component READMEs remain the implementation and operations references:

- [`components/README.md`](components/README.md) — component boundaries and build layout
- [`components/saas-control-plane/README.md`](components/saas-control-plane/README.md) — control-plane implementation, APIs, operations, and gaps
- [`components/satellite-gateway/README.md`](components/satellite-gateway/README.md) — gateway lifecycle capabilities, APIs, deployment, policy, and audit
- [`components/vsatellite/README.md`](components/vsatellite/README.md) — endpoint agent status and intended scope

No credentials, private keys, test passwords, or reusable access tokens belong in this document.

---

## 1. Requirement Language and Status

The words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are normative.

| Status | Meaning |
|---|---|
| **Verified** | Implemented and exercised through tests or a live deployment described by a component README |
| **Implemented** | Code and UI are documented as present, but complete production evidence is not recorded |
| **Partial** | A useful subset exists; important workflow, integration, scale, or security work remains |
| **Planned** | Design or scaffolding exists, but the capability is not usable end to end |
| **Required** | Enterprise requirement introduced by this document; not yet mapped to an implementation |
| **Blocked** | Present behavior is unsuitable for production until the stated blocker is resolved |

Requirement priorities:

| Priority | Meaning |
|---|---|
| **P0** | Production or security blocker; required before enterprise use |
| **P1** | Required for broad enterprise parity and reliable operations |
| **P2** | Strategic expansion or differentiating capability after the core platform is production ready |

A capability is not **Verified** merely because a route, button, or package exists. Verification requires the relevant API/UI path, authorization denials, persistence, failure handling, audit events, and integration behavior to be tested.

---

## 2. Product Vision and Outcomes

### 2.1 Vision

Provide one policy-controlled system of record and automation plane for machine identities, beginning with TLS certificates, across hybrid and multi-cloud estates.

### 2.2 Required outcomes

1. **Complete visibility:** discover certificates, trust chains, endpoints, owners, and dependencies before they become outages or security incidents.
2. **Zero-touch lifecycle:** automate issuance, deployment, renewal, verification, rollback, and revocation at short certificate lifetimes.
3. **Private-key safety:** generate and use keys inside customer-controlled trust boundaries; never send private keys to the SaaS control plane.
4. **Policy before issuance:** prevent non-compliant certificates instead of only reporting them afterward.
5. **Operational accountability:** every privileged or automated change must be attributable, immutable, searchable, and exportable.
6. **Multi-CA independence:** avoid lock-in by supporting public CAs, private CAs, cloud CAs, and standard enrollment protocols behind one policy model.
7. **Enterprise integration:** fit into IAM, ITSM, SIEM, CI/CD, Kubernetes, cloud, secrets, and observability ecosystems.
8. **Tenant isolation and scale:** serve multiple organizations without identity, metadata, secret, event, or workload leakage.
9. **Measurable reliability:** expose service health, automation success, renewal risk, and recovery objectives as product SLOs.

### 2.3 Initial scope and expansion scope

| Scope | Included |
|---|---|
| **Primary product scope** | X.509/TLS discovery, inventory, issuance, deployment, renewal, revocation, policy, approvals, reporting, and trust-store visibility |
| **Near-term expansion** | Workload identity, service-mesh certificates, Kubernetes/cert-manager, Microsoft ADCS, private CA, EST/SCEP/CMPv2, HSM-backed keys |
| **Market-parity expansion** | SSH key lifecycle, code-signing certificates, document signing, certificate transparency monitoring, cryptographic asset discovery, and post-quantum migration planning |
| **Not a current claim** | The expansion items above are requirements, not statements that the repository already implements them |

---

## 3. Personas and Segregation of Duties

| Persona | Primary responsibilities | Required access boundary |
|---|---|---|
| Platform super administrator | Operate SaaS, global integrations, plans, tenants, and platform policy | Cross-tenant metadata only where explicitly authorized; no customer private-key access |
| Tenant administrator | Configure tenant IAM, gateways, integrations, defaults, and ownership | One tenant and its child scopes |
| PKI / certificate manager | Manage CAs, templates, policies, requests, renewals, and revocations | Tenant or delegated business-unit scope |
| Approver | Review risk, policy exceptions, OV/EV or production issuance, and high-risk deployment | Decision rights without secret read access |
| Operator / SRE | Run discovery, deployments, retries, rollback, and incident response | Operational actions without IAM or billing administration |
| Application owner | Request and view certificates for owned applications and endpoints | Ownership-scoped self-service |
| Auditor / risk analyst | Read evidence, policy posture, changes, and reports | Read-only; audit export subject to explicit permission |
| Viewer | Read non-sensitive inventory and health summaries | No mutation, secret, raw CSR, or sensitive audit access |
| Automation principal | Use APIs/protocols for a bounded workload | Short-lived, scoped credentials; no interactive privileges |

The product MUST support segregation of duties. A requester MUST NOT approve their own high-risk request unless an explicit, audited policy permits it.

---

## 4. Target Architecture and Trust Boundaries

CLM keeps the existing three-component model because it separates global orchestration from customer-controlled key custody.

```text
┌──────────────────────────────────────────────────────────────────────┐
│ SaaS Control Plane                                                  │
│ Multi-tenant metadata, SSO identity federation, Gateway  fleet management,       │
│ global policy, reporting, integrations, licensing, and orchestration
│  Subscription & Billing,                                            │
│  (Keycloak),   Remote Deployment  
│                                                                      │
│ MUST NOT receive or persist customer private keys                   │
└───────────────────────────────┬──────────────────────────────────────┘
                                │ outbound mTLS from customer boundary
┌───────────────────────────────▼──────────────────────────────────────┐
│ Satellite Gateway                                                   │
│ Per customer/location: local inventory, crypto, keys, CA execution, │
│ policy enforcement, deployment, audit, discovery, and agent control │
└───────────────────────────────┬──────────────────────────────────────┘
                                │ authenticated pull/heartbeat
             ┌──────────────────┼──────────────────┐
       ┌─────▼─────┐      ┌─────▼─────┐      ┌─────▼─────┐
       │vSatellite │      │vSatellite │      │vSatellite │
       │cloud/VPC  │      │Kubernetes │      │on-prem/DC │
       │scan/deploy│      │scan/deploy│      │scan/deploy│
       └───────────┘      └───────────┘      └───────────┘
```

### 4.1 Component ownership

| Component | MUST own | MUST NOT own |
|---|---|---|
| SaaS control plane | Organizations, tenants, identity federation, fleet, global metadata, orchestration, product analytics, billing, cross-gateway posture | Customer private keys or unencrypted deployment credentials |
| Satellite gateway | Local key custody, crypto, certificate lifecycle execution, local policy enforcement, discovery, endpoint deployment, local audit, credential references | Global tenant identity source of truth or cross-tenant data |
| vSatellite | Execution in network segments unreachable by the gateway; local discovery, deployment, verification, and health reporting | Independent policy decisions, permanent CA credentials, or unauthenticated auto-registration |

### 4.2 Connectivity requirements

- Gateway-to-SaaS communication MUST be outbound initiated and mutually authenticated with mTLS.
- Browser, API, gateway, agent, database, and identity traffic MUST use TLS 1.2 or newer; TLS 1.3 SHOULD be preferred.
- vSatellite MUST pull signed, leased commands and acknowledge completion; command reads MUST NOT destructively clear unacknowledged work.
- Every gateway and agent MUST have a unique, rotatable machine identity.
- Offline gateways MUST continue local renewal and policy enforcement for a configurable grace period.

### 4.3 Architecture principles and invariants

These constraints apply regardless of whether a logical module is implemented inside one Go binary or later deployed as an independent service.

| ID | Architectural invariant |
|---|---|
| ARC-001 | **Customer private keys never cross into the SaaS control plane.** The control plane stores metadata and opaque key references only. |
| ARC-002 | **Execution follows network ownership.** Operations run at the gateway or vSatellite that can reach the CA, key backend, endpoint, and verification path. |
| ARC-003 | **Policy is centrally governed and locally enforceable.** A disconnected gateway continues using the last signed, valid policy bundle. |
| ARC-004 | **Every command and mutation is tenant scoped, authenticated, authorized, idempotent, and auditable.** |
| ARC-005 | **Credentials and endpoints are separate entities.** Authentication material is never modeled as an account ID, ARN, hostname, listener, or resource path. |
| ARC-006 | **Automation reports observed truth.** A provider API success is not deployment success until the serving endpoint presents the expected certificate and remains healthy. |
| ARC-007 | **Asynchronous work is durable.** Queue reads do not destroy work; commands are leased, acknowledged, retried, and dead-lettered. |
| ARC-008 | **The system tolerates duplicate delivery.** Issuance, renewal, deployment, and event handling use idempotency keys and immutable attempt records. |
| ARC-009 | **No UI bypass exists.** Portals consume the same versioned APIs and authorization rules available to approved automation clients. |
| ARC-010 | **Logical services do not require premature microservices.** Module boundaries, contracts, and ownership are mandatory; process separation is driven by scale, isolation, or availability evidence. |
| ARC-011 | **Tenant data is isolated at every layer.** Storage, cache, queue, object, event, log, trace, search, export, and backup paths carry and enforce tenant identity. |
| ARC-012 | **Security-sensitive state is fail closed.** Expired policy, invalid identity, unavailable secrets, or unverifiable command signatures block mutation rather than falling back to insecure defaults. |

### 4.4 System context

```text
                               ┌─────────────────────┐
                               │ Enterprise IdP      │
                               │ OIDC/SAML + SCIM    │
                               └──────────┬──────────┘
                                          │
┌──────────────┐   HTTPS/OIDC   ┌────────▼─────────┐   APIs/events   ┌─────────────────┐
│ Human users  ├───────────────▶│ CLM SaaS         ├───────────────▶│ ITSM/SIEM/Chat  │
│ and API apps │                │ Control Plane     │                 │ CI/CD/Terraform │
└──────────────┘                └────────┬─────────┘                 └─────────────────┘
                                          ▲
                                          │ outbound mTLS, commands, metadata, events
                                          │ no private keys
                             customer trust boundary
                                          │
                               ┌──────────┴──────────┐
                               │ Satellite Gateway  │
                               │ policy + lifecycle  │
                               │ crypto + key refs   │
                               └───┬────────┬────────┘
                      CA/API calls │        │ authenticated command pull
                                  │        │
                  ┌───────────────▼─┐   ┌──▼──────────────┐
                  │ Public/private  │   │ vSatellite      │
                  │ CA / ACME / PKI │   │ execution nodes │
                  └─────────────────┘   └──┬──────────────┘
                                          │ deploy/discover/verify
                ┌─────────────────────────┼─────────────────────────┐
         ┌──────▼──────┐          ┌───────▼───────┐         ┌──────▼──────┐
         │ Cloud/K8s   │          │ Servers/DBs   │         │ LB/ADC/Edge │
         │ cert stores │          │ app keystores │         │ appliances  │
         └─────────────┘          └───────────────┘         └─────────────┘
```

External systems are integrations, not hidden sources of truth. CLM remains authoritative for lifecycle intent, policy, ownership, workflow, and automation evidence; a CA remains authoritative for its orders and revocation state; an endpoint remains authoritative for what certificate it is actually serving.

### 4.5 Logical architecture

The boxes below are logical modules. The current implementation MAY host several modules in one process while preserving these contracts.

```text
┌──────────────────────────────── SaaS CONTROL PLANE ────────────────────────────────┐
│ Experience: Admin Portal │ Tenant Portal │ Public API │ CLI/SDK/Provider           │
├────────────────────────────────────────────────────────────────────────────────────┤
│ API Edge │ AuthN/AuthZ │ Tenant & IAM │ Fleet │ Global Inventory │ Reporting       │
│ Policy Distribution │ Workflow/Approval │ SaaS Deployment Orchestration Engine       │
│ Job/Command Scheduler │ Integration/Event Hub │ Notification │ Audit Query          │
│ Subscription/Billing │ Support Administration │ Deployment Artifacts                 │
├────────────────────────────────────────────────────────────────────────────────────┤
│ PostgreSQL │ Queue/Lease Store │ Cache │ Object/Report Store │ Audit/Event Sink     │
└───────────────────────────────────┬────────────────────────────────────────────────┘
                                    │ outbound mTLS session initiated by gateway
┌───────────────────────────────────▼────────────────────────────────────────────────┐
│ SATELLITE GATEWAY                                                                  │
│ Portal/API │ Machine Auth │ Local Policy │ Lifecycle Coordinator │ Scheduler         │
│ Inventory/Ownership │ CA Registry/Adapters │ ACME/DCV │ Crypto/Key Manager          │
│ Discovery │ Gateway Certificate Deploy Engine │ Executor Router │ Approval Cache       │
│ Credential Resolver │ Target Adapter Registry │ Verification/Rollback │ Agent Coord.  │
│ Local Audit │ Sync/Outbox                                                           │
├────────────────────────────────────────────────────────────────────────────────────┤
│ Shared Gateway DB (target) │ Encrypted local spool │ Key/HSM/KMS/Vault references   │
└───────────────────────────────────┬────────────────────────────────────────────────┘
                                    │ signed leased commands + mTLS
┌───────────────────────────────────▼────────────────────────────────────────────────┐
│ vSATELLITE EXECUTION NODE                                                          │
│ Agent Identity │ Command Lease Client │ Scanner │ Deployer │ Verifier │ Local Spool │
└────────────────────────────────────────────────────────────────────────────────────┘
```

### 4.6 Architecture planes

| Plane | Responsibilities | Primary components | Data allowed |
|---|---|---|---|
| Experience plane | Human and automation access, workflow presentation, reporting | Portals, public API, CLI/SDK, Terraform provider | Authorized metadata and redacted results |
| Identity and governance plane | Authentication, tenant context, RBAC/ABAC, policies, approvals, exceptions | IdP, authorization middleware, tenant/IAM, policy and workflow modules | Identity claims, scopes, policy versions; no private keys |
| Control plane | Fleet desired state, orchestration, schedules, global posture, integration routing | SaaS deployment orchestration engine, fleet manager, job/command scheduler, inventory projection, event hub | Metadata, immutable plans, commands, health, summaries, opaque key references; no private keys or endpoint secrets |
| Execution plane | CA calls, CSR/key operations, discovery, deployment, endpoint verification and rollback | Gateway Certificate Deploy Engine, executor router, target adapters and vSatellite | Private keys only inside approved local key boundary; short-lived secret use |
| Data plane | Certificate/chain transfer from CA to target and observed TLS traffic used for verification | Gateway, agent, endpoint adapters | Public certificate/chain; private key only when target requires exportable material and policy permits |
| Secret plane | Credential resolution and cryptographic key operations | Credential broker, secret manager, HSM/KMS/TPM/Vault | Write-only secrets and non-exportable key handles |
| Event and evidence plane | Immutable audit, domain events, metrics, logs, traces, reports | Local audit, outbox, event hub, SIEM/object retention | Redacted, tenant-scoped evidence with correlation IDs |

A plane describes responsibility and trust, not necessarily a separately deployed network service.

### 4.7 SaaS control-plane logical modules

| Module | Responsibility | Scale/availability boundary |
|---|---|---|
| API edge | TLS termination, request limits, WAF policy, routing, correlation IDs | Horizontally scalable and stateless |
| Identity and authorization | OIDC/SAML federation, tenant resolution, RBAC/ABAC, service principals | Fail closed; cached authorization has short TTL and tenant key |
| Tenant and organization | Tenant lifecycle, scopes, plans, residency, retention, ownership model | Transactional system of record |
| Fleet manager | Gateway/agent enrollment, desired version, heartbeat, health, capacity | High-write metadata with stale-node detection |
| Global inventory projection | Search/report projection of gateway certificate metadata | Event-derived, eventually consistent; not key custody |
| Policy service | Versioned policy/profile source, simulation, signed policy bundles | Strong consistency for activation; cached locally at gateways |
| Workflow and approval | Requests, approvals, exceptions, SLA/escalation, segregation of duties | Durable state machine; immutable decisions |
| SaaS Deployment Orchestration Engine | Own deployment intent, select infrastructure/certificate deployment path, build immutable plan revisions, enforce approval, choose gateway, create rollout waves, aggregate progress and reconcile desired versus reported state | Horizontally scalable controllers over durable job/command state; never handles private keys or resolved endpoint secrets |
| Fleet Infrastructure Deploy Controller | Provision/register/upgrade/drain/decommission gateways and vSatellites through cloud provider adapters, IaC/artifact generation and bootstrap validation | Provider-specific workers with idempotency, rate limits and explicit resource ownership |
| Certificate Rollout Controller | Orchestrate deploy/redeploy/emergency rotation from certificate version to endpoint groups, rollout strategy and execution location | Tenant-fair scheduling; certificate metadata and endpoint references only |
| Job/command scheduler | Persist desired work, create signed scoped commands, lease/retry/dead-letter and reconcile indeterminate operations | Durable queue; no read-and-clear semantics |
| Integration/event hub | Signed webhooks, SIEM/ITSM/chat/CI/CD adapters, event schemas | Isolated retries and per-tenant rate limits |
| Reporting and compliance | Risk, coverage, evidence, scheduled reports and exports | Read replicas/warehouse/object store as scale requires |
| Audit service | Cross-component audit ingestion, verification, retention and query | Append-only/tamper-evident external retention |
| Subscription and billing | Entitlements, usage metering, marketplace reconciliation | Must not interrupt safety-critical renewal during grace policy |

### 4.8 Satellite-gateway logical modules

| Module | Responsibility | Key rule |
|---|---|---|
| Gateway API and portal | Local operations and status when SaaS access is unavailable | Same authorization and audit rules as remote operations |
| Machine identity/session | Establish outbound mTLS, rotate identity, receive signed desired state | One logical gateway identity per customer location/cluster |
| Local policy engine | Evaluate effective policy for request, renewal, deployment and revocation | Last valid signed bundle may be used offline; expired bundle fails closed for new mutations |
| Lifecycle coordinator | Drive request/order/issue/deploy/verify/renew/revoke state machines | Idempotent and serialized per certificate lineage |
| CA registry and adapters | Normalize ACME, public/private CA and commercial order behavior | Vendor errors map to stable internal states without hiding original correlation data |
| Crypto and key manager | Generate keys/CSRs, resolve key handles, assemble chains, destroy keys safely | Private key never enters control-plane data or logs |
| Inventory and ownership | Local certificate versions, observations, endpoints and relationships | Fingerprint dedupe with all observations preserved |
| Discovery engine | Network, DNS, cloud, store and agent scans | Bounded, cancellable, rate limited and resumable |
| Gateway Certificate Deploy Engine | Own local certificate deployment plans and attempts; load certificate/key references locally; route execution; coordinate preflight, stage, apply/reload, verify, rollback and reconciliation | SaaS sends desired deployment metadata only; private key and resolved secrets remain local |
| Executor router | Select direct gateway, a capable vSatellite, or an approved provider-API adapter using endpoint reachability, capability, health, load and policy | Selection is deterministic, explainable and persisted in the attempt |
| Target adapter registry | Data-driven endpoint schemas and adapters for OS, database, cloud, Kubernetes, ADC/load balancer and custom targets | Every adapter declares credential types, idempotency, verification, rollback and secret-handling support |
| Deployment verifier and rollback controller | Verify the actual listener/store fingerprint and service health; retain prior version and restore binding/configuration on failure | Provider success remains provisional until independent verification passes |
| Credential resolver | Resolve scoped secret references only at execution time | Secret values are write-only, short lived in memory and redacted everywhere |
| Scheduler | Renewal, discovery, alert, CA-order and reconciliation jobs | Leader-only in HA; missed-run recovery after restart |
| Agent coordinator | Enroll, authenticate, lease commands and process results | Commands survive disconnect and duplicate delivery |
| Audit and outbox | Append local evidence and reliably synchronize events upstream | Local append succeeds before mutation reports completion |
| State repository | Persist lifecycle and command state | Shared transactional store for HA; encrypted local spool for transient outage |

#### 4.8.1 Two-tier Deploy Engine architecture

CLM has two cooperating deploy engines with different trust boundaries. **SaaS orchestrates; the gateway executes.** They are not interchangeable and MUST NOT share secret responsibilities.

| Engine | Runs in | Owns | MUST NOT own |
|---|---|---|---|
| **SaaS Deployment Orchestration Engine** | Multi-tenant SaaS control plane | Deployment intent, plan revision, policy/approval, infrastructure provider orchestration, gateway selection, endpoint-group rollout, job/command state, progress aggregation, notifications and fleet-level reconciliation | Customer private keys, resolved CA/endpoint credentials, local filesystem/keystore access, or a claim that an endpoint is serving a certificate without gateway verification |
| **Gateway Certificate Deploy Engine** | Customer-controlled satellite gateway | Local certificate/key references, credential resolution, target adapter selection, gateway/vSatellite execution routing, preflight, staging, apply/reload/bind, serving-endpoint verification, rollback, cleanup, attempt evidence and local reconciliation | Global tenant administration, billing, cross-tenant scheduling, or infrastructure provisioning outside its approved local/provider scope |
| **vSatellite deployment executor** | Restricted network/application segment | Execute one leased deployment step through an approved adapter and return verification evidence | Independent policy/approval decisions, permanent broad-scope credentials, or deployment outside assigned capability/reachability scope |

```text
User / Renewal / Emergency Campaign / API
                    │
                    ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ SaaS Deployment Orchestration Engine                                    │
│ Intent → policy/approval → immutable plan → target waves → gateway route │
│ Fleet Deploy Controller │ Certificate Rollout Controller │ Job Scheduler │
└───────────────────────────────┬──────────────────────────────────────────┘
                                │ signed DeploymentCommand
                                │ IDs, references, policy/approval digest,
                                │ rollout/verification/rollback rules; no secrets
                                ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ Gateway Certificate Deploy Engine                                       │
│ authorize/re-evaluate → load local cert/key ref → resolve endpoint       │
│ → resolve credential → select executor → preflight → stage/apply/reload  │
│ → verify actual endpoint → commit or rollback → audit/result/reconcile   │
└─────────────────────────┬────────────────────────────┬───────────────────┘
                          │ direct gateway             │ leased command
                          ▼                            ▼
              Cloud/API/local adapter          vSatellite executor
                          │                            │
                          └──────────────┬─────────────┘
                                         ▼
                              Target endpoint/store
                                         │
                                         ▼ independent observation
                               fingerprint + service health
                                         │
                                         ▼
                    gateway result/event → SaaS progress/projection
```

##### SaaS Deploy Engine operation options

| Option | Trigger | SaaS responsibility | Gateway responsibility |
|---|---|---|---|
| Gateway infrastructure deployment | Deploy Gateway wizard/API | Build/provider plan, approval, provision or generate artifact, bootstrap job, registration validation | New gateway registers and proves health/capability; it does not provision itself unless using an explicitly approved local installer path |
| vSatellite deployment | Deploy vSatellite from parent gateway | Validate parent/placement, create immutable request and enrollment artifact/job, track activation | Create scope-bound enrollment, validate agent identity/capability, execute safe test and activate |
| Certificate deployment | User/API selects certificate version and endpoints | Validate ownership/policy, group endpoints, choose rollout strategy and gateway, create waves/commands | Load local material, resolve each endpoint/credential, execute, verify and report |
| Renewal redeployment | Lifecycle coordinator issues a replacement version | Build deployment plan from prior active bindings, enforce changed policy, prioritize safety deadline | Deploy new version, verify, preserve old version for rollback, report endpoint exceptions |
| Emergency rotation campaign | Security/operator campaign | Calculate scope/blast radius, approval, canary/waves, pause thresholds, global progress and exception list | Execute priority attempts, health-gate each wave, rollback/pause on threshold and retain evidence |
| Rollback | Operator or automatic failure policy | Authorize and command selected job/wave rollback; aggregate recovery status | Restore previous binding/files/store version, reload, verify previous fingerprint and service health |
| Reconciliation | Schedule, restart, timeout or indeterminate result | Ask for observed state and compare desired/global projection | Inspect provider/store/listener and local attempt records; complete, retry safely, rollback or report manual intervention |
| Decommission cleanup | Endpoint/gateway/agent retirement | Dependency/impact plan, approval and ownership reassignment | Remove managed bindings/material only when policy permits, revoke identities, wipe eligible cached secrets and verify cleanup |

The SaaS option describes **why and where** deployment is required. The gateway operation describes **how** it is performed safely inside the customer boundary.

##### SaaS Deployment Orchestration Engine components

| Component | Responsibility |
|---|---|
| Deployment API and intent validator | Normalize tenant, actor, certificate/version, endpoint group, operation, strategy, schedule, idempotency and correlation IDs |
| Plan builder | Resolve endpoint inventory/projections, gateway ownership, dependency order, canary/waves, concurrency, maintenance window, verification and rollback policy |
| Policy/approval gate | Evaluate global/tenant/environment rules and bind approval to the exact immutable plan digest |
| Fleet Infrastructure Deploy Controller | Provision gateway infrastructure or generate IaC/install artifacts; manage bootstrap, validation, upgrade, drain and decommission jobs |
| Certificate Rollout Controller | Create deployment waves and endpoint commands for issue, renewal, manual deploy and emergency rotation workflows |
| Execution-location selector | Select a Ready gateway using endpoint ownership/location, policy, version/capability and health; gateway later selects direct versus vSatellite execution |
| Job/command scheduler | Persist jobs/waves/commands, sign commands, enforce priority/fairness, lease delivery, cancellation boundaries, retry and dead-letter behavior |
| Progress and reconciliation controller | Aggregate endpoint attempts, detect timeout/indeterminate state, request observed-state reconciliation and calculate final job outcome |
| Notification/integration publisher | Emit redacted events and notifications to portal, webhook, ITSM, SIEM and on-call systems |
| Infrastructure provider registry | Data-driven AWS/Azure/GCP/Kubernetes/on-prem provisioning adapters with permission schema, plan, apply, status, rollback/removal and cost metadata |

SaaS infrastructure provider credentials MAY be resolved in a SaaS-approved secret boundary for **gateway infrastructure provisioning only**. Certificate endpoint credentials and private keys remain in the gateway execution boundary.

##### SaaS-to-gateway deployment contract

A `DeploymentCommand` MUST contain references and signed authorization, not secret material:

| Field group | Required contents |
|---|---|
| Identity | Tenant ID, command/job/wave/attempt IDs, gateway ID, actor/service-principal ID, correlation/causation IDs |
| Authorization | Operation, approved-plan digest, policy revision, approval IDs/digests, issued-at/expiry, nonce and control-plane signature |
| Certificate reference | Certificate lineage/version ID, expected public fingerprint/chain metadata, local key-reference expectation; no private key or resolved key bytes |
| Target reference | Endpoint ID(s), target type, desired binding/resource behavior and non-secret endpoint revision; no embedded endpoint credential |
| Execution policy | Executor constraints, strategy/wave, concurrency, maintenance window, timeout, retry budget, verification tests, rollback/version-retention rules |
| Compatibility | Command schema version, minimum gateway/adapter version and required capability names |

The gateway returns a signed/redacted `DeploymentResult` containing attempt state, selected executor/adapter, stages and timings, provider resource/version IDs where safe, expected and observed fingerprints, verification/health outcome, rollback state, stable error code, retryability, evidence references and audit event IDs.

##### Gateway Certificate Deploy Engine pipeline

| Stage | Required behavior | Failure disposition |
|---|---|---|
| 1. Command intake | Authenticate/signature-check, enforce tenant/gateway/expiry/nonce/schema, persist command before acknowledgement, deduplicate | Reject invalid/expired/out-of-scope; return prior result for completed duplicate |
| 2. Authorization and policy | Re-evaluate current local policy and approved-plan digest; verify actor/service scope and maintenance window | Block with all violations; never use stale approval for materially changed plan |
| 3. Material resolution | Load certificate/chain and local key handle from gateway inventory/key backend; validate fingerprint, chain and key match | Fail closed; no fallback to material supplied by SaaS |
| 4. Endpoint resolution | Load current endpoint revision and non-secret provider/resource fields; detect drift from approved revision | Re-plan/re-approve material drift or reconcile harmless metadata change |
| 5. Credential resolution | Resolve endpoint's credential reference through local secret/workload-identity provider; issue short-lived token when available | Fail closed and identify credential health/permission issue without exposing secret |
| 6. Executor routing | Select direct gateway, approved provider API, or capable vSatellite using reachability, assignment, health, load and adapter version | Queue/wait/re-route only within policy; never route outside endpoint ownership scope |
| 7. Preflight | Validate permissions, target existence, current binding/version, write/reload capability, capacity, backup/rollback path and verification reachability | No mutation; return aggregated actionable failures |
| 8. Stage | Create encrypted temporary material or provider version, preserve existing state, set restrictive permissions and record cleanup plan | Remove incomplete stage safely or mark reconciliation required |
| 9. Apply/reload/bind | Perform idempotent adapter operation with provider idempotency token and stage checkpoint | Retry only if operation is known safe; otherwise inspect actual target first |
| 10. Verify | Independently inspect store/provider and probe serving endpoint for expected fingerprint/chain plus service/application health | Mark failed/indeterminate even when apply API returned success |
| 11. Commit or rollback | Mark new version active only after required verification; otherwise restore prior version/binding and verify recovery | Escalate if both deploy and rollback verification fail; preserve evidence/material needed for recovery |
| 12. Cleanup and report | Delete temporary secret files/tokens, retain configured previous versions, append audit/outbox, return signed result | Audit backlog is durable; high-risk success is not reported without durable local evidence |

##### Executor and adapter options

| Gateway Deploy Engine option | Selection rule | Typical targets |
|---|---|---|
| Direct local executor | Gateway has network/store access and required adapter; lowest additional hop | Local Linux/Windows service, database, filesystem/keystore, directly reachable API |
| Provider API executor | Gateway can call cloud/platform control API using local workload identity/credential reference | AWS ACM/ELB/CloudFront/API Gateway, Azure Key Vault/App Gateway/Front Door, GCP Certificate Manager/load balancer, Kubernetes API |
| vSatellite executor | Endpoint is reachable only inside an assigned segment and agent capability/version satisfies the adapter | Private VPC/VNet/subnet, on-prem VLAN, Kubernetes node/cluster, DMZ, restricted appliance |
| External orchestrator adapter | Policy explicitly delegates execution and CLM can verify outcome independently | Ansible/AWX, Terraform pipeline, approved enterprise automation platform |
| Manual controlled handoff | No safe adapter exists; use only as governed exception | Legacy appliance; operator receives public material/instructions, never unapproved private-key export |

Adapter manifests MUST declare target schema, credential schema, supported certificate/key formats, exportability needs, operation/idempotency model, preflight checks, apply/reload semantics, verification method, rollback capability, cleanup behavior, required privileges and supported versions. The Deploy Engine MUST refuse an operation the adapter declares unsupported.

##### Deployment material and secret boundary

- For non-exportable HSM/KMS/TPM/Vault keys, the adapter receives a key handle or provider operation, never private-key bytes.
- If a target requires an exportable key, the key is unwrapped only inside the gateway/agent execution boundary, written only to approved encrypted/permission-restricted staging, and securely removed after apply/rollback policy allows.
- vSatellite receives the minimum per-attempt material and short-lived credential through the authenticated leased command channel or resolves it locally; it MUST NOT persist permanent broad-scope credentials.
- SaaS receives certificate public metadata/fingerprint and redacted evidence only. It MUST NOT proxy PEM private keys to the gateway or endpoint.
- Logs, traces, events, command payloads and support bundles contain hashes/references, never private keys, passwords, tokens or unredacted provider responses.

##### Deploy Engine requirements

| ID | Requirement | Priority |
|---|---|---|
| DENG-001 | The architecture MUST expose a SaaS Deployment Orchestration Engine and a Gateway Certificate Deploy Engine as separate logical components and trust boundaries. | P0 |
| DENG-002 | SaaS MUST own infrastructure/certificate deployment intent, immutable plans, approvals, rollout orchestration, job/command state and progress; it MUST NOT receive customer private keys or resolved endpoint credentials. | P0 |
| DENG-003 | The gateway MUST own local certificate/key resolution, endpoint/credential resolution, executor routing, adapter execution, verification, rollback, cleanup and attempt evidence. | P0 |
| DENG-004 | SaaS MUST support gateway infrastructure deployment, vSatellite deployment, certificate deployment, renewal redeployment, emergency rotation, rollback, reconciliation and decommission operation types. | P0 |
| DENG-005 | SaaS-to-gateway commands MUST be signed, expiring, nonce-protected, schema-versioned, tenant/gateway scoped, idempotent and bound to policy/approval/plan digests. | P0 |
| DENG-006 | The gateway MUST select direct, provider API, vSatellite, external orchestrator or governed manual handoff only when policy, reachability and adapter capability permit it. | P0 |
| DENG-007 | Target adapters MUST be manifest driven and declare schema, credentials, formats, privileges, idempotency, preflight, apply/reload, verification, rollback and cleanup capabilities. | P0 |
| DENG-008 | Every attempt MUST persist stage checkpoints and observed external state so restart/retry can reconcile before repeating side effects. | P0 |
| DENG-009 | Deployment success MUST require configured store/listener and serving-endpoint fingerprint/health verification; provider API success alone is insufficient. | P0 |
| DENG-010 | Failed or partially verified deployment MUST preserve the previous working version and perform policy-controlled rollback with independent recovery verification. | P0 |
| DENG-011 | The Gateway Deploy Engine MUST continue approved local renewal redeployments during SaaS outage and synchronize durable results when connectivity returns. | P0 |
| DENG-012 | All deployment stages, executor/adapter selection, credential reference use, verification and rollback outcomes MUST be correlated, redacted and audited across SaaS, gateway and vSatellite. | P0 |

##### Release 1 implementation evidence — gateway correctness and durability

> **Scope:** verified on the existing **test gateway only** (`3.7.21.28`). This is not a statement of production readiness or completion of the full P0 roadmap. The SaaS host (`35.154.6.28`) was not changed in this release.

| Requirement | Measured Release 1 status | Remaining gap |
|---|---|---|
| DENG-003 | **Partially implemented and live-verified.** The gateway resolves certificate material by `cert_id`, resolves target credentials by `credential_ref`, runs the adapter, requires verification, automatically invokes rollback, and persists attempt evidence. Inline certificate/private-key material and inline target credentials are rejected by `POST /api/v1/deploy`. | Preflight, cleanup reconciliation, richer observed external state, and all target-specific recovery semantics are not complete. |
| DENG-008 | **Partially implemented and live-verified.** An attempt is synchronously persisted before apply, then updated with `accepted`, `apply`, `verify`, `rollback`, and `commit` stage outcomes. Attempts survived a gateway restart. | There is no durable queue/lease, idempotency key, retry reconciler, or database-backed concurrent writer model. |
| DENG-009 | **Safety invariant implemented.** HTTP 200/`completed` is emitted only when `success=true` and `verified=true`; an unverified apply returns HTTP 502 and is never committed or labelled deployed. | Linux verification currently proves the deployed file, not the serving listener fingerprint/application health required for full DENG-009. AWS verification is listener-aware in code but real ACM/ELB mutation remains unverified due test-role permissions. |
| DENG-010 | **Partially implemented and live-verified.** An injected post-apply verification failure returned HTTP 502, attempted rollback, reported `rollback_success=true`, restored the prior certificate hash, and left the earlier verified deployment active. | Recovery is not independently re-verified by a second verifier pass; GCP/Azure rollback is still unsupported; rollback policies are not yet plan/approval bound. |
| DENG-012 | **Gateway portion implemented and live-verified.** Correlation and attempt IDs are present in response headers/body, logs, inventory API/state, audit JSON, and audit CSV; secret-marker checks found no key or inline credential material in those surfaces. | End-to-end SaaS/gateway/vSatellite correlation, signed results, and an external tamper-evident audit sink remain outstanding. |

Live test evidence:

- Verified attempt: correlation `release1-success-20260815`, HTTP 200, stages `accepted → apply → verify → commit`, status `deployed`.
- Negative attempt: correlation `release1-failure-20260815`, HTTP 502, verification failed, rollback completed, status `failed`, stage `rolled_back`.
- Post-failure inventory: `deployed=1`, `failed=1`, `superseded=0`; the failed attempt did not supersede the prior verified deployment.
- Authorization and boundary checks: viewer deploy returned 403; operator positive control succeeded; inline PEM and inline credentials returned 400; vSatellite execution returned 501 rather than queueing key material.
- Validation: Go format clean, `go build ./...`, binary build, `go vet`, existing `go test -race` suite, JavaScript syntax check, health check, API/UI asset checks, and restart persistence all passed.
- Rollback backup: `/opt/clm-gateway/backups/release1-20260815T082500Z` on the test gateway.

The remaining P0 release order now excludes the offline-complete Task 4 foundations: finish authenticated key-local vSatellite certificate execution and provider bootstrap; live-validate renewal/DNS-01 behavior; complete TLS/mTLS and secure credential backends; then HA, restore evidence, monitoring, and external audit retention. No item above upgrades the product to production-ready.

> **Task 3 code update:** the control-plane-to-gateway transport now has per-satellite scrypt credentials, optional/required tenant-bound mTLS, PostgreSQL lease/ack/result state, idempotent queueing, bounded retries/dead-lettering, and execution-generation duplicate suppression in code and offline tests. This does not revise the historical Release 1 live evidence above: no live PostgreSQL transport, mTLS/CA, or cloud-provider bootstrap validation was performed. Because existing deployment adapters cannot inject the secure bootstrap identity, production cloud gateway creation fails closed until that integration exists.

> **Task 4 code update:** gateway renewal now checkpoints immutable versions, preserves version-specific private keys, replays sanitized active bindings, requires verified deployment results before activation, compensates partial rollout with verified prior-version redeployment, and marks indeterminate restart/commit outcomes `recovery_required`. An authenticated recovery action rolls every active binding back to the last committed version; it never promotes an uncertain candidate. DNS-01 now has live RFC2136 and AWS Route 53 adapters using external credential references, while Azure DNS, GCP Cloud DNS, and OCI DNS remain explicitly planned. Canonical staged tests, vet, and build pass; live ACME, Route 53, and multi-endpoint rollout validation remains a release gate.

### 4.9 vSatellite logical modules

A vSatellite is a constrained execution worker, not a second policy authority.

| Module | Responsibility |
|---|---|
| Agent identity | Enroll once, maintain mTLS identity, rotate before expiry, support immediate revocation |
| Command client | Pull signed commands, acquire lease, renew lease, acknowledge result, deduplicate by command ID |
| Scanner | Discover certificates from assigned network, host, store, or platform scope |
| Deployer | Execute target-specific preflight/apply/reload behavior using a scoped credential reference |
| Verifier | Probe the actual listener/store and return fingerprint, chain, health, and evidence |
| Local spool | Encrypt commands/results while disconnected; contain no permanent broad-scope CA credential |
| Updater | Verify signed releases, stage rollout, health check, and roll back failed agent versions |

Agents MUST NOT make approval or policy decisions. They receive an already authorized command plus the minimum policy assertions and credentials needed for that operation.

### 4.10 Data ownership and persistence architecture

| Data class | Authoritative owner | Target storage | Replication/exposure rule |
|---|---|---|---|
| Organization, tenant, membership, role binding | SaaS control plane | HA PostgreSQL | Tenant scoped; gateway receives only claims/config needed locally |
| Global policy/profile version | SaaS policy service | HA PostgreSQL + signed bundle/object | Gateway caches signed active version and reports applied version |
| Local effective policy decision | Gateway | Gateway transactional state + audit | Summary may synchronize to SaaS; decision evidence remains queryable |
| Certificate lineage and public metadata | Gateway for local estate; SaaS keeps global projection | Gateway DB; event-derived SaaS projection/search | Eventual synchronization; fingerprint/version IDs provide dedupe |
| Private key | Gateway/agent-controlled key backend | HSM/KMS/TPM/Vault/approved encrypted store | Never replicated to SaaS; exportability policy controlled |
| Credential secret | Approved secret backend in execution boundary | Secret manager/Vault/KMS-encrypted store | APIs expose metadata/reference only; resolve at execution time |
| Endpoint definition | Gateway/local execution domain with SaaS projection | Gateway DB; SaaS projection | Resource identifiers allowed; credentials referenced, not embedded |
| Request, approval, exception | Workflow authority that accepted the request | Transactional database + immutable audit | Payload digest binds approval to exact action |
| Job/command/attempt | Orchestrator for desired work; executor for attempts | Durable queue/database | Lease/ack, idempotency key, immutable attempts, bounded retention |
| Audit event | Component where action occurred | Local append/outbox then external tamper-evident sink | At-least-once delivery with stable event ID; no update/delete |
| Metrics/logs/traces | Producing component | Tenant-safe observability backend | Redacted; tenant and correlation dimensions enforced |
| Reports/exports | Reporting service | Encrypted object store with expiry | Signed short-lived download; audited access; tenant retention policy |

#### Data consistency model

- Identity, tenant membership, policy activation, approval decisions, and command lease ownership require **strong consistency**.
- Global inventory, health dashboards, analytics, and reports MAY be **eventually consistent** and MUST display observation/synchronization time.
- Certificate lifecycle state is authoritative at the gateway executing it; the SaaS projection MUST NOT overwrite a newer gateway state.
- Conflict resolution MUST use immutable IDs, monotonic version/revision numbers, and explicit reconciliation—not last-write-wins timestamps from unsynchronized clocks.
- Every persisted time uses UTC; ordering-critical records SHOULD also use database sequence/revision values.

### 4.11 Command, event, and synchronization architecture

```text
SaaS desired action
      │  create job + idempotency key in transaction
      ▼
Durable command record ──outbox──▶ gateway outbound mTLS session
      │                                  │
      │                                  ▼
      │                           authorize + persist locally
      │                                  │
      │                         lease to gateway or vSatellite
      │                                  │
      │                       execute → verify → append audit
      │                                  │
      ◀──────── result/event + stable command/attempt IDs ───────
      │
update global projection, notify, retain evidence
```

- Command delivery is at least once; execution MUST be effectively once through idempotency and state-transition guards.
- A lease has owner, expiry, heartbeat, attempt count, and cancellation state. Expired work returns to the queue unless its external side effect is in an indeterminate state requiring reconciliation.
- Mutating database state and publishing its event MUST use a transactional outbox or equivalent atomic pattern.
- Event consumers record processed event IDs. Duplicate events MUST NOT duplicate purchases, certificates, deployments, notifications, or audit evidence.
- Large reports, chains, and diagnostics SHOULD use encrypted object storage with signed references rather than queue payloads.
- Backpressure MUST delay/reject new non-critical work before memory or queues are exhausted; renewal safety work receives priority.

### 4.12 Core runtime flows

#### 4.12.1 Certificate request, approval, issuance, and deployment

```text
Requester/Automation   SaaS Workflow/Deploy Engine   Gateway Lifecycle/Deploy Engine   Key Backend   Endpoint
        │ request + idempotency │                 │                   │                │
        ├──────────────────────▶│ validate scope  │                   │                │
        │                       ├─ approval if required ─▶ Approver    │                │
        │                       │ approved payload digest             │                │
        │                       ├──── signed/leased command ─────────▶│                │
        │                       │                 │ evaluate policy   │                │
        │                       │                 ├─ generate key/CSR▶│                │
        │                       │                 ├─ order/finalize ─▶ CA              │
        │                       │                 │ certificate/chain │                │
        │                       │                 ├─ preflight/deploy ─────────────────▶│
        │                       │                 ├─ TLS/store verify ────────────────▶│
        │                       │◀── result + audit + public metadata ─┤                │
        │◀── completed/failed ──┤                 │                   │                │
```

Required behavior:

1. The request is normalized and assigned tenant, owner, profile, policy scope, correlation ID, and idempotency key.
2. Approval binds to a digest of the exact request. Any material change invalidates the approval.
3. The gateway re-evaluates policy locally before any CA purchase or issuance call.
4. The key/CSR is generated inside the selected key backend. Only the CSR reaches the CA.
5. Issuance creates an immutable certificate version; asynchronous validation remains a distinct state.
6. Deployment runs preflight, apply, reload/bind, and serving-endpoint verification.
7. Completion is reported only after verification. Failure retains the prior version and exposes rollback/remediation.

#### 4.12.2 Automated renewal and rotation

```text
Scheduler → eligibility/policy → lock certificate lineage → key/CSR/order
          → deploy in configured waves → verify every endpoint
          → mark new version active → retain old version for rollback
          → retire/delete old key after policy window → audit/report
```

- Renewal scheduling uses percentage-of-life plus absolute safety deadlines and accounts for CA/DCV/deployment lead time.
- One lineage lock prevents operator, scheduler, and retry paths from creating conflicting versions.
- A new version is not active merely because the CA issued it; activation follows required endpoint verification.
- If any critical wave fails, the rollout pauses or rolls back according to policy while the previously working version remains available.
- Offline/unreachable endpoints create explicit exceptions and escalation; they are not silently marked successful.

#### 4.12.3 Discovery and reconciliation

```text
Discovery source/schedule → gateway/agent scan → observations
       → parse/validate → fingerprint dedupe → endpoint relationship
       → compare inventory and served truth → classify new/changed/stale/drift
       → ownership/policy/risk evaluation → SaaS projection + alert/remediation queue
```

Discovery is read-only by default. Promotion to managed, replacement, deployment, trust change, or deletion is a separate authorized workflow.

#### 4.12.4 Revocation and emergency rotation

1. Calculate endpoint/application impact and determine whether replacement-before-revoke is required.
2. Obtain approval according to reason, environment, certificate type, and blast radius.
3. Issue/deploy/verify replacement where policy requires continuity.
4. Revoke through the authoritative CA, record reason and CA evidence, then verify status where supported.
5. Remove old bindings/keys only after rollback and evidence-retention requirements are satisfied.
6. For emergency campaigns, execute controlled waves with pause, health gates, exceptions, and a final completeness report.

### 4.13 Security architecture

#### Security zones

| Zone | Contents | Inbound policy | Outbound policy |
|---|---|---|---|
| Public edge | WAF/load balancer/API edge and portal static assets | HTTPS only; rate limited and protected | Only to private application/identity services |
| SaaS application | Control-plane modules and workers | From edge or authenticated internal services | Database, queue, object, IdP and approved integrations |
| SaaS data | PostgreSQL, queue/cache, object/audit stores | Private endpoints and service identity only | Backup/replication and controlled observability |
| Customer gateway | Gateway cluster, local DB, secret/key connectors | Local admin/API as configured; no SaaS-initiated inbound dependency | mTLS to SaaS, CA, secret backend, endpoints, agents |
| Restricted execution segment | vSatellite and target systems | Agent's local management boundary only | Pull to gateway plus target/verification traffic |
| Key/secret boundary | HSM/KMS/TPM/Vault/secret manager | Authenticated workload identity and scoped operation | Audit/health only; keys non-exportable where supported |

#### Identity and authorization path

- Human identity originates in the tenant's IdP and is exchanged through OIDC/SAML; SCIM/JIT controls lifecycle.
- The SaaS creates a tenant-scoped authorization context. Roles and attributes are checked by API middleware and repository queries.
- The gateway authenticates as a unique machine workload; SaaS commands carry tenant, actor, approved payload digest, policy revision, expiry, nonce, and signature.
- The gateway authorizes again using local policy and machine/human claims. vSatellite accepts only gateway-signed commands scoped to its assignment.
- Cloud/CA/endpoint credentials use workload identity or short-lived federation where available; static credentials are the last resort and must be rotatable.

#### Secret and key flow

```text
Credential metadata/reference ──stored in CLM──▶ execution command
                                                   │
                                                   ▼
                                        scoped resolver at gateway/agent
                                                   │ short-lived secret/token
                                                   ▼
                                              provider/endpoint

Key policy ──▶ HSM/KMS/TPM/software key backend ──▶ sign CSR / TLS operation
                       │
                       └── private key never enters SaaS, event, log or report
```

### 4.14 Deployment topology

| Topology | Intended use | Required production shape |
|---|---|---|
| SaaS control plane | Shared multi-tenant service | Multi-AZ load balancer, ≥2 stateless app replicas, HA PostgreSQL, durable queue, distributed cache, encrypted object/audit storage, autoscaling workers |
| Single-node gateway | Development, pilot, small non-critical site | Durable encrypted state, backup, watchdog, offline renewal; explicitly not HA |
| HA gateway cluster | Production customer location | ≥2 replicas, one logical gateway identity, shared transactional state, leader election, leader-only schedules, load balancer/VIP, disruption budget |
| Kubernetes gateway | EKS/GKE/AKS or on-prem Kubernetes | Deployment/Stateful dependencies, anti-affinity, PDB, HPA/KEDA on queue depth, workload identity, CSI secret/key integration |
| VM gateway | EC2/GCE/Azure VM/on-prem | Multi-zone/site pair behind load balancer or VIP; system service, health check, immutable/signed releases |
| vSatellite pool | Segmented VPC/VNet/subnet/DC/Kubernetes network | One or more workers per reachability domain; commands partitioned by assignment and capacity |
| Disconnected/regulated site | Intermittent SaaS connectivity | Local policy/cache, key custody, renewal scheduler, encrypted event spool, controlled synchronization and administrative break-glass |

#### 4.14.1 Gateway deployment modes

The product MUST offer **Deploy Gateway** as a first-class control-plane workflow. Deployment mode determines who creates the infrastructure; it does not change gateway identity, security, policy, or health requirements.

| Mode | Description | Best fit | Required output |
|---|---|---|---|
| SaaS-managed cloud deployment | CLM provisions approved infrastructure through a registered cloud account and least-privilege role | Standard AWS/Azure/GCP tenant deployments | Plan, approval, provider operation IDs, generated gateway identity, validation report |
| Customer-managed infrastructure as code | CLM generates versioned Terraform/CloudFormation/Bicep or Helm values; customer applies it in their pipeline | Regulated enterprises and platform teams | Signed/versioned artifact, variables, checksums, expected resources, registration result |
| Self-install bootstrap | Administrator prepares a supported host and runs a one-time signed installer generated by CLM | On-premises VM/bare metal or customer-managed cloud VM | Single-use bootstrap command/package, prerequisites, installation and registration status |
| Bring-your-own gateway | Existing installation is registered and validated without CLM creating infrastructure | Migration, appliance image, pre-provisioned platform | Registration bundle, identity exchange, compatibility/preflight report |
| Offline installation bundle | Signed package, manifest, trust bundle and limited-time enrollment response are transferred through an approved offline process | Disconnected or highly regulated site | Signed offline bundle, checksum, expiry, import/export audit and later synchronization proof |

SaaS-managed SSH is not the target default. If retained for migration, it MUST be disabled by default, explicitly approved, time bounded, source restricted, fully audited, and replaced by customer-executed bootstrap or infrastructure-as-code.

#### 4.14.2 Gateway platform options

| Platform option | Topology choices | Production intent | Required platform integration |
|---|---|---|---|
| AWS Lightsail | Single node | Development/pilot only; no enterprise HA claim | Instance/static IP/firewall bootstrap, backup and health registration |
| AWS EC2 | Single node or multi-AZ HA behind ALB/NLB | Production VM deployment | VPC/subnets/security groups, IAM role, ASG/instances, load balancer, DNS, KMS/secrets, database choice |
| AWS EKS | HA Kubernetes | Production cloud-native deployment | Helm, workload identity, ingress/load balancer, PDB, anti-affinity, HPA/KEDA, CSI secrets, external DB |
| Azure VM / VM Scale Set | Single node or zone-redundant HA | Production VM deployment | VNet/subnets/NSG, managed identity, load balancer, Key Vault, database/storage, DNS |
| Azure AKS | HA Kubernetes | Production cloud-native deployment | Helm, workload identity, ingress, PDB, autoscaling, Key Vault CSI, external DB |
| Google Compute Engine / MIG | Single node or regional HA | Production VM deployment | VPC/subnets/firewall, service account, load balancer, Secret Manager/KMS, database/storage, DNS |
| Google GKE | HA Kubernetes | Production cloud-native deployment | Helm, Workload Identity, ingress, PDB, autoscaling, secret integration, external DB |
| Generic Kubernetes / OpenShift | Single replica pilot or HA | On-premises/other-cloud Kubernetes | Helm/operator, ingress, storage class, external DB, secret/key provider, network policy |
| On-premises Linux VM/bare metal | Single node or active/passive/active cluster behind VIP | Data center deployment | Supported OS package/container, systemd, VIP/load balancer, database, backup, HSM/Vault, proxy/DNS/NTP |
| Prebuilt appliance/image | Single node or HA pair | Controlled enterprise distribution | Signed image, secure bootstrap, update channel, attestation, support bundle and recovery procedure |

A platform option is not **production supported** until its provisioning, upgrade, backup, restore, HA/failover, and removal paths are tested on the real platform.

#### 4.14.3 Gateway deployment configuration

The deployment wizard/API MUST collect and validate the following groups before creating infrastructure or issuing bootstrap material.

| Group | Required configuration |
|---|---|
| Identity and ownership | Tenant, gateway name, stable gateway ID, location/site, environment, owner/team, business unit, tags, criticality |
| Deployment selection | Mode, platform, single-node/HA topology, region, availability zones, desired version/channel, architecture (amd64/arm64 where supported) |
| Cloud or host target | Cloud account credential reference, account/subscription/project ID, VPC/VNet/network, subnet(s), security group/firewall policy, resource group/project, host inventory where applicable |
| Sizing | Certificate/endpoint estimate, scan range, concurrent jobs, API users, CPU/memory/disk class, minimum/maximum replicas, autoscaling thresholds |
| Connectivity | SaaS URL, outbound proxy and no-proxy rules, DNS/NTP, CA endpoints, secret/key backends, agent listener scope, egress allow-list; no hidden inbound SaaS dependency |
| Public/local access | Portal/API exposure mode, load balancer/VIP, private/public DNS, TLS certificate/profile, admin CIDRs, local-only option |
| State and keys | Transactional state backend, storage class/volume, backup target and schedule, encryption/KMS key, secret manager/Vault/HSM/TPM provider, key export policy |
| Lifecycle defaults | Discovery sources/schedules, renewal safety window, CA/profile defaults, policy bundle, audit/event sink, offline grace policy |
| Operations | Monitoring destination, log/metric retention, maintenance window, automatic update policy, rollback retention, support access policy |

Passwords, access keys, private keys, SSH keys, and tokens MUST be accepted only through write-only secret inputs or external secret references. Generated plans and logs MUST show references, never secret values.

#### 4.14.4 Gateway deployment workflow

```text
Select tenant/location → choose mode/platform/topology → enter network/state/security options
        → preflight permissions/connectivity/quotas → generate plan and cost/impact preview
        → policy/approval gate → provision or generate signed install artifact
        → one-time bootstrap identity → outbound mTLS registration
        → apply signed policy/config → run health and dependency checks
        → test audit/event sync → optional non-production lifecycle smoke test
        → mark Ready, Ready-with-warnings, or Failed with remediation
```

1. **Preflight:** validate cloud permissions, quotas, subnets, DNS/NTP, outbound SaaS/CA access, secret/key backend, database/storage, target OS/Kubernetes version, capacity, and name uniqueness without mutation.
2. **Plan:** display resources to create/change/delete, topology, expected recurring cost where available, network exposure, required IAM permissions, and rollback/removal behavior.
3. **Approval:** production, public exposure, new cloud account, privileged IAM, or HA infrastructure follows configured approval policy.
4. **Provision/bootstrap:** use an idempotency key and durable deployment job. A retry MUST reconcile existing resources rather than duplicate them.
5. **Identity:** exchange a single-use, short-lived bootstrap token for a unique gateway workload certificate; token is shown once and stored only as a hash.
6. **Registration:** gateway reports version, capabilities, location, topology, policy revision, state backend, and health over outbound mTLS.
7. **Validation:** verify API/portal as configured, state persistence, leader election for HA, secret/key access, CA connectivity, audit append/sync, backup readiness, scheduler, and agent enrollment endpoint.
8. **Activation:** only a validated gateway becomes selectable for certificate ownership, endpoint deployment, or vSatellite placement.
9. **Upgrade:** support canary/rolling upgrade, pre-upgrade backup, schema compatibility, health gate, automatic rollback, and version-skew policy.
10. **Decommission:** drain work, reassign ownership, revoke gateway/agent identities, export required evidence, remove infrastructure by explicit plan, and retain audit according to policy.

#### 4.14.5 Gateway deployment requirements

| ID | Requirement | Priority |
|---|---|---|
| GDEP-001 | The control-plane portal and API MUST provide Deploy Gateway, Import/Register Gateway, Upgrade, Drain and Decommission workflows. | P0 |
| GDEP-002 | Deployment MUST support SaaS-managed cloud, customer-managed IaC, self-install and bring-your-own modes; offline bundle support is required for regulated deployments. | P1 |
| GDEP-003 | Every deployment MUST have preflight, immutable plan, approval where required, idempotent execution, progress, cancellation boundary, rollback/remediation and audit. | P0 |
| GDEP-004 | Gateway bootstrap MUST use a single-use, expiring token exchanged for a unique rotatable mTLS identity. | P0 |
| GDEP-005 | Infrastructure credentials MUST be external references or write-only secrets and MUST be separate from gateway/network/resource identifiers. | P0 |
| GDEP-006 | A gateway MUST pass connectivity, persistence, policy, audit, secret/key, scheduler and lifecycle health checks before Ready state. | P0 |
| GDEP-007 | Production HA deployment MUST verify shared state, leader election, failover, queue recovery and rolling upgrade before acceptance. | P1 |
| GDEP-008 | Deployment jobs MUST expose created resources, provider IDs, status, cost estimate where available, logs, errors and correlation IDs without secrets. | P0 |
| GDEP-009 | Decommission MUST perform dependency/ownership impact analysis and MUST NOT silently orphan certificates, endpoints, agents or audit evidence. | P0 |
| GDEP-010 | Platform support status MUST distinguish available, preview, planned and production-verified rather than presenting every generated option as supported. | P0 |

#### 4.14.6 vSatellite deployment options

The product MUST offer **Deploy vSatellite** from a selected gateway. A vSatellite is placed per reachability/security domain—such as a VPC, VNet, subnet, Kubernetes cluster, data center VLAN, DMZ, or restricted application segment—not simply per tenant.

| Option | Packaging | Best fit | Execution model |
|---|---|---|---|
| Linux service | Signed package/binary with systemd; OpenRC MAY be supported | VM/bare metal in a subnet or data center | Long-running least-privilege service |
| Container | Signed OCI image with constrained volume/network access | Existing Docker/Podman hosts | One durable agent per reachability domain |
| Kubernetes Deployment | Helm/operator-managed replicas | Cluster-level API, ingress, secret and cloud integrations | HA worker pool with lease-based command distribution |
| Kubernetes DaemonSet | Helm/operator-managed node agent | Node-local keystores/workloads that cannot be reached centrally | One constrained agent per selected node |
| Windows service | Signed MSI/service package | IIS, Windows certificate stores and Windows-only applications | Service account/gMSA with scoped rights |
| Cloud VM/image | Customer-managed VM, scale set/MIG, or signed image | Dedicated execution worker in AWS/Azure/GCP network segment | One or more labeled workers behind outbound-only connectivity |
| Offline package | Signed binary/image, manifest and enrollment bundle | Disconnected/restricted segment | Local encrypted spool and controlled result transfer/synchronization |

A vSatellite option MUST declare supported operating systems/architectures, scanner/deployer capabilities, required privileges, network ports, secret providers, resource minimums, upgrade path, and production support status.

#### 4.14.7 vSatellite deployment configuration and workflow

| Group | Required configuration |
|---|---|
| Parent and identity | Tenant, parent gateway, agent name, location, environment, owner, labels, stable assignment ID |
| Placement | Cloud/network/VPC/VNet/subnet/cluster/site/VLAN, DNS resolver, proxy/no-proxy, outbound gateway address, availability zone or node selector |
| Capabilities | Discovery scanner types, deployment target types, endpoint verifier types, local key/store access; deny undeclared capabilities |
| Assigned scope | CIDRs, domains, cloud accounts/regions, Kubernetes clusters/namespaces, endpoint groups, maximum target count |
| Capacity | CPU/memory, worker concurrency, scan rate, command queue limit, maintenance window, offline spool size |
| Security | Installation mode, service identity, bootstrap-token lifetime, gateway trust pin/bundle, secret provider/access method, allowed outbound destinations |
| Operations | Version/update channel, auto-update policy, health interval, log/metric destination, drain timeout, restart policy |

```text
Select parent gateway → choose placement/package → assign capabilities and network scope
       → validate scope overlap and least privilege → generate one-time enrollment artifact
       → install signed package/image/Helm release → verify checksum/signature
       → token exchange for unique agent mTLS identity → capability/health attestation
       → gateway sends safe test scan/verify command → confirm result/audit
       → activate for scheduling or return Failed with remediation
```

- The enrollment token MUST identify tenant, parent gateway, intended agent name/placement, allowed capabilities, expiry, and one-time use. It MUST NOT contain reusable cloud, SSH, CA, endpoint, or private-key credentials.
- Installation artifacts MUST be signed and pinned to a supported version/channel. The portal shows checksum, expiry, prerequisites, exact install command/manifest, and expected outbound destinations.
- The agent exchanges the bootstrap token for a unique workload certificate and stores it in an OS-native protected location or approved key store.
- The gateway compares reported platform/capabilities with the approved request. Extra undeclared capabilities or mismatched placement fail enrollment.
- Scope overlap with another agent MUST be reported and resolved as active/standby, partitioned ownership, or rejected; duplicate uncontrolled scans/deployments are not allowed.
- Activation requires heartbeat, lease/ack test, time synchronization, DNS/network check, scanner/deployer preflight, encrypted spool check, and a harmless verification command.
- Upgrades use signed releases, staged fleet rings, compatibility checks, drain, health gate and rollback. Unsupported versions become quarantined before they become unsafe.
- Decommission drains commands, reassigns endpoint/discovery ownership, revokes identity, deletes local cached secrets/spool according to policy, and retains audit evidence.

#### 4.14.8 vSatellite deployment requirements

| ID | Requirement | Priority |
|---|---|---|
| VSAT-001 | Deploy vSatellite MUST be available only after selecting a Ready parent gateway and must bind the agent to that gateway and tenant. | P0 |
| VSAT-002 | Linux service, container and Kubernetes deployment options are P0/P1 core targets; Windows service, cloud image and offline package are required enterprise options. | P1 |
| VSAT-003 | Enrollment MUST use a single-use, expiring, scope-bound token exchanged for a unique rotatable mTLS identity. | P0 |
| VSAT-004 | Deployment MUST collect explicit reachability scope and capabilities; the agent MUST reject commands outside both. | P0 |
| VSAT-005 | Installation artifacts MUST be signed, checksummed, versioned and accompanied by prerequisites, resource and network requirements. | P0 |
| VSAT-006 | A deployed agent MUST pass identity, heartbeat, lease/ack, capability, scope, spool and safe execution checks before Active state. | P0 |
| VSAT-007 | Agents MUST support drain, restart, staged upgrade, rollback, identity rotation, revoke and decommission from portal and API. | P0 |
| VSAT-008 | The product MUST detect overlapping assignments, offline agents, unsupported versions, capacity pressure and certificates/endpoints losing coverage. | P0 |
| VSAT-009 | Agent deployment and operation MUST NOT require inbound SaaS connectivity or permanent broad-scope credentials stored in the installer. | P0 |
| VSAT-010 | Agent status MUST distinguish requested, installing, enrolling, validating, active, degraded, draining, offline, revoked, failed and decommissioned. | P0 |

#### 4.14.9 Gateway and vSatellite portal experience

The **Gateways & Agents** workspace MUST include:

- **Gateways tab:** inventory, status, topology, version, policy revision, state/backup health, endpoint/certificate coverage, agent count, deployment job, and actions: **Deploy Gateway**, register/import, open, test, upgrade, drain, rotate identity, download diagnostics, and decommission.
- **vSatellites tab:** parent gateway, placement, package/platform, capabilities, assigned scope, version, last heartbeat, queue/capacity, covered endpoints/certificates, and actions: **Deploy vSatellite**, test, edit assignment, rotate identity, drain, upgrade, restart, revoke, and decommission.
- **Deployment Jobs tab:** gateway and agent provisioning jobs with plan, approval, progress stages, created resources, generated artifact expiry, logs, correlation ID, retry/remediation and rollback/removal status.
- **Installation Artifacts tab:** active IaC/Helm/package/bootstrap artifacts with version, checksum/signature, expiry, download count, used/unused/revoked state; secret/bootstrap values are displayed once.

Deployment wizard stages MUST be resumable before approval, but an approved plan becomes immutable. Material changes create a new revision and require re-approval. The portal MUST never label a resource Deployed when it is only provisioned; final success requires registration and all applicable validation checks.

#### 4.14.10 High availability and scaling

- **Control plane:** stateless APIs scale horizontally. Stateful services use managed HA or quorum-capable deployments. Background work scales by queue partition and tenant-aware fairness.
- **Gateway:** replicas share lifecycle/command state. A database advisory lock or TTL lease elects one scheduler/synchronizer leader while every healthy replica serves APIs and execution work.
- **Agents:** scale horizontally by reachability domain. The gateway assigns work using capability, health, load, labels, and network ownership.
- **Inventory/search:** transactional metadata remains authoritative; search/report projections may use dedicated indexes or analytical stores rebuilt from events.
- **Large tenants:** tenant-aware partitioning/cells SHOULD limit blast radius. No tenant may monopolize queues, scans, reports, or integration rate budgets.
- **Scale in:** workers drain leases and in-flight deployments before termination. Leadership transfers safely; unacknowledged commands become available after lease expiry.

### 4.15 Integration architecture

```text
                          ┌─ Enterprise IdP / SCIM
                          ├─ ITSM: ServiceNow / Jira
SaaS Integration/Event Hub├─ SIEM: Splunk / Elastic / Sentinel
                          ├─ Chat/On-call: Teams / Slack / PagerDuty
                          ├─ CI/CD / Terraform / Webhooks
                          └─ Billing / Marketplace APIs

Gateway Adapter Registries├─ CA: ACME, ADCS, public/private/cloud CAs
                          ├─ Secrets/keys: Vault, cloud secret managers, HSM/KMS
                          ├─ Cloud: AWS, Azure, GCP
                          ├─ Platforms: Kubernetes, Windows, Linux, Java, databases
                          └─ Network: F5, NetScaler, load balancers, CDN/API gateways
```

- SaaS integrations consume redacted metadata/events and MUST NOT require customer private keys.
- Execution adapters run at the gateway/agent and declare capabilities, schemas, credential types, idempotency behavior, verification method, and rollback support.
- Adapter manifests drive portal fields and API validation; vendor-specific forms SHOULD NOT be hardcoded into unrelated UI code.
- Integration failures are isolated by connector/tenant with independent retry and circuit-breaker state.
- Webhooks are signed and replay resistant. Provider callbacks are authenticated, tenant resolved, idempotent, and correlated to an existing order/job.

### 4.16 Resilience and failure behavior

| Failure | Required behavior |
|---|---|
| SaaS unavailable | Gateway continues already authorized local renewal, verification, alerting, and policy enforcement; spools redacted events and reconnects with backoff |
| Gateway unavailable | SaaS marks health stale, identifies certificates/endpoints losing automation coverage, alerts before safety deadline, and avoids pretending queued work completed |
| vSatellite offline | Leases expire, safe work is reassigned where reachability permits, affected endpoints are reported, and commands remain durable |
| CA unavailable/throttled | Circuit breaker and policy-bounded retry; renewals prioritize nearest deadline; alternate CA routing only if profile/policy permits and identity semantics remain valid |
| Secret/key backend unavailable | Mutation fails closed without caching broad static secrets; existing services continue using the last deployed certificate; incident identifies affected scope |
| Database/queue unavailable | No unaudited mutation; APIs return retryable errors; in-flight external side effects enter reconciliation before retry |
| Partial deployment | Stop/pause rollout, preserve previous version, verify actual endpoint state, automatically roll back when policy allows, and create incident evidence |
| Verification mismatch | Deployment remains failed/indeterminate even if provider API succeeded; never mark certificate active based on control-plane state alone |
| Duplicate command/event | Idempotency/state guards return prior result or continue reconciliation without repeating purchases or destructive actions |
| Clock skew | Protocols use bounded validity/nonce checks; ordering relies on revisions/sequences rather than client timestamps |
| Policy bundle expired/invalid | New issuance/deployment/revocation fails closed; safety renewal behavior follows an explicit signed emergency policy, never an implicit bypass |
| Audit sink unavailable | Local append/outbox retains evidence; backpressure or block high-risk mutation before durable audit capacity is exhausted |

### 4.17 Observability architecture

Every request, job, command, external call, event, and audit record shares a correlation chain:

```text
request_id → workflow_id → job_id → command_id → attempt_id
           ↘ certificate_id / version_id / endpoint_id / CA order ID
```

- Metrics follow RED for services and USE for infrastructure, plus renewal deadline, queue age, scheduler lag, command lease, integration health, and audit backlog measures.
- Logs are structured, redacted, tenant scoped, and include component, version, actor/service principal, correlation IDs, result, and stable error code.
- Traces cross SaaS → gateway → agent boundaries using propagated trace context that contains no secrets.
- Audit events are security evidence and are stored separately from operational logs; changing log retention MUST NOT shorten audit retention.
- Synthetic checks continuously validate login, API health, gateway session, CA sandbox connectivity, and a non-production issuance/deployment/verification path.
- SLO dashboards and alerts use user-visible outcomes—successful verified lifecycle completion—not only process uptime.

### 4.18 Current-to-target deployment mapping

| Current documented implementation | Target architecture interpretation |
|---|---|
| Go control-plane binary plus PostgreSQL, Redis, Keycloak and nginx | Modular control-plane application that can remain a modular monolith initially; add durable orchestration/event/audit patterns and HA dependencies before service extraction |
| Existing control-plane remote gateway deployment and generic command dispatch | Make the SaaS Deployment Orchestration Engine an explicit module owning immutable infrastructure/certificate rollout plans, approvals, provider/artifact controllers, durable jobs/commands, progress and reconciliation |
| Single gateway binary with local JSON state and nginx | Modular gateway process for pilot; migrate lifecycle/command state to a transactional shared store before HA |
| Existing gateway deploy API, deployment registry and target-specific deployers | Evolve into the explicit Gateway Certificate Deploy Engine with local material/credential resolution, deterministic executor routing, manifest-driven adapters, stage checkpoints, serving-endpoint verification, rollback and reconciliation |
| Gateway JSONL audit | Valid local append boundary; add reliable outbox synchronization and external tamper-evident retention |
| Pull-based SaaS connector | Retain outbound-only model; replace plain HTTP/static bearer behavior with mTLS workload identity and durable bidirectional command/event synchronization |
| vSatellite enrollment and command transport | Complete scanner/deployer/verifier execution, lease/ack, local encrypted spool, identity rotation and signed updates |
| Vanilla JavaScript portals | MAY remain buildless; must consume versioned APIs and meet the same security, accessibility and workflow requirements as any future UI stack |

The target architecture evolves the existing component boundaries rather than replacing them. The first architectural priority is securing transport and state; the second is making commands and lifecycle work durable; the third is HA and scale. Splitting the code into many network services before those foundations would add failure modes without solving the product risks.

---

## 5. Documented Current Baseline

This table consolidates the five repository READMEs. It is a documentation baseline, not a replacement for a code audit or release evidence.

| Domain | Documented state | Important qualification |
|---|---|---|
| Tenant, identity, RBAC, SSO | Partial | Task 2 fails closed on invalid subject/tenant claims and adds scoped backend operations with offline negative tests; PostgreSQL membership and Keycloak identity ownership can still drift |
| Control-plane audit | Implemented | Protected JWT mutations append tenant-scoped immutable events with filtered reads and visible write failures; response fail-closed is not transaction atomic, public/gateway events and external retention remain gaps |
| Cloud-account credentials | Implemented | Tenant-scoped AES-256-GCM backend and redacted CRUD are offline tested; validation reports `unverified`, deploy jobs remain process-local, and no live-provider support is claimed |
| Gateway fleet and remote deployment | Partial | Heartbeat, health, command dispatch, and AWS/on-prem paths exist; multi-cloud provisioning and production hardening remain incomplete |
| Certificate inventory and discovery | Verified | Network, DNS, scheduled, and AWS ACM discovery are documented; broader platform and trust-store coverage is required |
| ACME issuance | Verified/Code Ready | HTTP-01 and Pebble verification are documented; DNS-01 orchestration plus RFC2136 and Route 53 adapters pass offline validation, while live Route 53/production CA behavior and Azure/GCP/OCI DNS remain gaps |
| Commercial CA integration | Partial | GoDaddy and Namecheap are implemented against protocol mocks; live vendor behavior is unproven |
| Renewal and revocation | Code Ready | Gateway-managed certificates can be renewed/revoked; renewal redeployment, endpoint verification gates, compensation, durable lineage, restart markers, and explicit prior-version recovery are implemented offline, but a live multi-endpoint rollout is unproven |
| Deployment | Partial | Linux and PostgreSQL are documented; AWS/GCP/Azure paths have limited or no real-account proof and endpoint coverage is narrow |
| Certificate policy | Verified | Issuance-time rules and approval-required behavior are documented, unit tested, and live/browser verified |
| Audit trail | Verified at gateway | Append-only JSONL, query, CSV/JSONL export, UI, role denial, and live events are documented; control-plane audit coverage and external retention are incomplete |
| Approval workflow | Partial | Flat pending/approved/rejected flow exists; sequential multi-level approval and separation-of-duties policy are required |
| Monitoring and compliance | Implemented | Expiry alerts, lifecycle assessment, compliance scoring, and exports exist; enterprise SLOs and SIEM integration are required |
| vSatellite execution | Planned/Partial | Enrollment and command transport exist; the shipped agent does not execute certificate scan/deploy work end to end |
| High availability | Planned | Gateway remains stateful and single-node; leader election, shared state, command leasing, and multi-replica operation are required |
| Production security | Blocked | Current documentation records HTTP transport, static/shared secrets, insecure test credentials, and browser-stored cloud credentials in parts of the system |

---

## 6. Enterprise Functional Requirements

### 6.1 Governance, tenancy, identity, and access (`GOV`)

| ID | Requirement | Priority |
|---|---|---|
| GOV-001 | Every API query and mutation MUST be tenant scoped at the repository layer, not only filtered in the UI. | P0 |
| GOV-002 | Tenant isolation MUST have automated negative tests proving one tenant cannot enumerate, infer, export, or mutate another tenant's data. | P0 |
| GOV-003 | Interactive login MUST use OIDC Authorization Code with PKCE; password/direct-grant login MUST NOT be the production default. | P0 |
| GOV-004 | The product MUST support enterprise OIDC and SAML federation, MFA enforcement, session revocation, and configurable session lifetime. | P0 |
| GOV-005 | SCIM 2.0 SHOULD provision/deprovision users and groups; JIT federation MAY be supported as a fallback. | P1 |
| GOV-006 | RBAC MUST cover the documented roles and support custom roles composed from granular permissions. | P1 |
| GOV-007 | ABAC SHOULD restrict actions by tenant, business unit, environment, certificate owner, CA, endpoint class, and data sensitivity. | P1 |
| GOV-008 | Service accounts MUST use scoped, expiring credentials with rotation and last-used visibility. | P0 |
| GOV-009 | High-risk actions MUST support separation of duties and forbid self-approval by default. | P0 |
| GOV-010 | Tenant offboarding MUST support suspend, export, retention hold, scheduled purge, legal hold, and recoverable soft deletion before final destruction. | P1 |

### 6.2 Discovery and inventory (`INV`)

| ID | Requirement | Priority |
|---|---|---|
| INV-001 | The platform MUST maintain a normalized inventory of certificates, CSRs, keys as non-secret references, chains, trust anchors, endpoints, owners, sources, and deployments. | P0 |
| INV-002 | Discovery MUST support scheduled and on-demand network/SNI scanning, DNS names, IPv4/IPv6 ranges, cloud certificate stores, Kubernetes, load balancers, web servers, databases, and imported CA inventories. | P0 |
| INV-003 | Discovery SHOULD support Microsoft certificate stores/ADCS, F5, NetScaler, Java keystores, hardware appliances, service meshes, and agentless cloud APIs. | P1 |
| INV-004 | Results MUST be deduplicated by cryptographic fingerprint while preserving every observed endpoint and source. | P0 |
| INV-005 | Inventory MUST expose owner, application, business unit, environment, criticality, cost center, tags, source, managed/unmanaged state, and last-seen timestamp. | P0 |
| INV-006 | The system MUST calculate expiry risk, chain validity, hostname mismatch, weak algorithm/key, self-signed status, revocation state, duplicate reuse, excessive validity, and stale/unreachable endpoint risk. | P0 |
| INV-007 | A relationship graph SHOULD show certificate → version → endpoint → application → owner → CA/template → trust chain. | P1 |
| INV-008 | Unknown certificates MUST enter a triage queue for assign, ignore with expiry, remediate, or accept-risk actions. | P1 |
| INV-009 | Discovery jobs MUST have bounded concurrency, rate limits, maintenance windows, progress, cancellation, retries, and retained run history. | P0 |
| INV-010 | Certificate Transparency monitoring SHOULD detect unexpected public issuance for tenant-owned domains. | P2 |

### 6.3 Certificate authorities, profiles, and enrollment (`CA`)

| ID | Requirement | Priority |
|---|---|---|
| CA-001 | CA integrations MUST use a common account/profile/order interface so policy is independent of vendor. | P0 |
| CA-002 | The product MUST support multiple accounts per vendor, sandbox/production separation, account health tests, quotas, costs, and default routing by scope. | P0 |
| CA-003 | Enrollment MUST support ACME and REST API first; EST, SCEP, CMPv2, Microsoft ADCS, cloud private CAs, and Kubernetes cert-manager are required for enterprise parity. | P1 |
| CA-004 | ACME MUST support HTTP-01 and DNS-01, including wildcard issuance and pluggable DNS providers. | P0 |
| CA-005 | Public and private CA issuance MUST support DV, OV, EV, internal TLS, client authentication, mTLS, device, workload, and custom EKU profiles where the CA permits them. | P1 |
| CA-006 | Certificate profiles MUST define subject/SAN rules, key algorithm/size, signature algorithm, EKU/KU, validity, renewal window, CA routing, approval, deployment, and revocation behavior. | P0 |
| CA-007 | The request path MUST validate every policy and CA capability before purchase or order creation and return all actionable violations together. | P0 |
| CA-008 | Commercial/asynchronous orders MUST expose payment/quote, order state, DCV instructions, organization validation, polling history, and safe retry without duplicate purchase. | P1 |
| CA-009 | Every issuance request MUST support an idempotency key and correlation ID. | P0 |
| CA-010 | CA account credentials MUST be referenced from a secure credential store and MUST never be returned by read APIs. | P0 |

### 6.4 Lifecycle automation (`LCM`)

| ID | Requirement | Priority |
|---|---|---|
| LCM-001 | The canonical lifecycle MUST cover discover/import → own → request → approve → issue → deploy → verify → monitor → renew/rotate → revoke/retire. | P0 |
| LCM-002 | Renewal MUST be policy driven by percentage-of-life and absolute-day thresholds, not one global fixed date. | P0 |
| LCM-003 | The system MUST be ready for short-lived public certificates, including a 47-day operating model, without manual renewal steps. | P0 |
| LCM-004 | Renewal MUST be serialized per certificate lineage and idempotent across retries, schedulers, and operator actions. | P0 |
| LCM-005 | Successful renewal MUST trigger configured redeployment and post-deployment verification automatically. | P0 |
| LCM-006 | A failed deployment MUST retain the prior working version, support rollback, and create an actionable incident without revoking the working certificate. | P0 |
| LCM-007 | Revocation MUST require a valid reason, authorization, impact preview, and optional replacement-before-revoke workflow. | P0 |
| LCM-008 | The system MUST detect and recover stuck orders, failed validations, partial deployments, missed renewals, and offline execution nodes. | P0 |
| LCM-009 | Emergency rotation MUST support bulk selection, controlled waves, pause/resume, exception handling, and proof of completion. | P1 |
| LCM-010 | Every automation MUST report success, failure, skipped, awaiting approval, and awaiting external validation as distinct states. | P0 |

### 6.5 Deployment, credentials, and endpoints (`DEP`)

Credentials, endpoints, and deployments are related but MUST remain different domain objects:

- A **Credential Set** contains authentication material or a reference to it, such as an AWS access key/secret, assumed role, service-account identity, SSH key, or Vault token.
- A **Deployment Endpoint** identifies where a certificate is used, such as an AWS account/region and ACM, ALB listener, API Gateway, or CloudFront ARN; a Kubernetes secret; a Linux host/path; or a database listener.
- A **Deployment Job** binds one certificate version to one endpoint, executor, credential reference, strategy, and verification result.

An AWS account ID, ACM ARN, load-balancer listener ARN, API Gateway ARN, and CloudFront ARN are endpoint metadata—not credentials.

| ID | Requirement | Priority |
|---|---|---|
| DEP-001 | The portal MUST present one **Deployment** workspace with separate Deployments, Endpoints, and Credentials views/tabs. | P0 |
| DEP-002 | Stored credentials MUST support add, test, edit metadata, rotate secret, disable, and remove; secrets MUST remain write-only. | P0 |
| DEP-003 | Endpoint CRUD MUST be independent from credential CRUD and MUST validate endpoint fields by target type. | P0 |
| DEP-004 | Deploy Certificate MUST select an inventory certificate/version, one or more endpoints, a credential reference, executor, rollout strategy, and maintenance window. | P0 |
| DEP-005 | Supported target families MUST include Linux web servers, Windows/IIS, Java keystores, PostgreSQL and other common databases, AWS ACM/ALB/NLB/CloudFront/API Gateway, Azure Key Vault/Application Gateway/Front Door, GCP Certificate Manager/load balancers, Kubernetes secrets/ingress/cert-manager, F5, and NetScaler. | P1 |
| DEP-006 | Deployers MUST implement preflight validation, dry run where possible, backup/version retention, apply, endpoint verification, rollback, and secret redaction. | P0 |
| DEP-007 | Rollouts SHOULD support all-at-once, canary, percentage waves, maintenance-window scheduling, concurrency limits, pause/resume, and automatic rollback thresholds. | P1 |
| DEP-008 | Verification MUST test the serving endpoint and observed certificate fingerprint, not only whether a certificate exists in a provider store. | P0 |
| DEP-009 | Endpoint health MUST record last successful deployment, certificate served, drift, credential health, ownership, and executor reachability. | P0 |
| DEP-010 | Credential use MUST be service/type constrained; using an Azure credential for an AWS endpoint MUST fail before network activity. | P0 |

### 6.6 Policy, approvals, and exceptions (`POL`)

| ID | Requirement | Priority |
|---|---|---|
| POL-001 | Policy MUST be evaluated before issuance, renewal, import acceptance, deployment, and revocation where applicable. | P0 |
| POL-002 | Rules MUST cover algorithms, key sizes, validity, SAN count, wildcard use, subject patterns, denied domains, allowed CAs/profiles, environments, endpoint classes, ownership, and approval requirements. | P0 |
| POL-003 | Policies MUST support global → tenant → business unit → application/environment inheritance with deterministic precedence and conflict explanation. | P1 |
| POL-004 | A dry-run evaluator MUST explain every matching rule, outcome, and remediation before execution. | P0 |
| POL-005 | Approval workflows MUST support sequential and parallel stages, quorum, delegation, expiry, reminders, escalation, and comments. | P1 |
| POL-006 | Policy exceptions MUST have owner, reason, scope, approver, start, expiry, compensating controls, and automatic re-evaluation at expiry. | P0 |
| POL-007 | Production, wildcard, OV/EV, high-cost, weak-crypto exception, and bulk/emergency actions MUST be independently configurable for approval. | P0 |
| POL-008 | Policy changes MUST be versioned, diffable, auditable, and optionally require approval before activation. | P1 |

### 6.7 Key and cryptographic material management (`KEY`)

| ID | Requirement | Priority |
|---|---|---|
| KEY-001 | Private keys MUST be generated within the selected execution boundary and MUST NOT be sent to or persisted by the SaaS control plane. | P0 |
| KEY-002 | Software keys MUST be encrypted at rest with envelope encryption and least-privilege access; plaintext files MUST be avoided or mode-restricted and short lived. | P0 |
| KEY-003 | The platform MUST support non-exportable HSM/KMS/TPM-backed keys for regulated workloads. | P1 |
| KEY-004 | FIPS-validated cryptographic modules MUST be available for deployments that require them. | P1 |
| KEY-005 | Algorithms MUST include RSA and ECDSA today, with inventory and policy support for crypto-agility and post-quantum/hybrid migration. | P1 |
| KEY-006 | Key reuse MUST be policy controlled; renewal SHOULD generate a new key by default unless an integration requires otherwise. | P0 |
| KEY-007 | Sensitive material MUST be redacted from logs, events, traces, API errors, support bundles, and UI state. | P0 |
| KEY-008 | Key deletion MUST be lifecycle-aware, delayed until rollback and retention windows close, and independently audited. | P0 |

### 6.8 Trust-store and chain management (`TRUST`)

| ID | Requirement | Priority |
|---|---|---|
| TRUST-001 | The inventory MUST model roots, intermediates, chains, trust stores, distrust dates, and endpoints that depend on each trust anchor. | P1 |
| TRUST-002 | Chain validation MUST account for order, missing intermediates, expiry, signature algorithms, name constraints, EKU, and client/server trust differences. | P0 |
| TRUST-003 | Trust-store changes MUST support impact analysis, staged rollout, verification, rollback, and separation of duties. | P1 |
| TRUST-004 | The system SHOULD identify unexpected, obsolete, or unauthorized roots and intermediates. | P1 |
| TRUST-005 | CA migration MUST report all certificates/endpoints chained to the old hierarchy and track replacement progress. | P1 |

### 6.9 Monitoring, alerting, audit, and reporting (`OBS`)

| ID | Requirement | Priority |
|---|---|---|
| OBS-001 | Alerts MUST cover expiry, renewal failure, deployment failure, endpoint drift, invalid chain, policy violation, CA outage, credential expiry, gateway/agent outage, and unusual issuance. | P0 |
| OBS-002 | Alert routing MUST support severity, ownership, maintenance windows, deduplication, suppression, escalation, acknowledgement, and recovery notification. | P0 |
| OBS-003 | Every mutation and privileged read/export MUST emit an audit event containing actor, tenant, role, action, target, before/after summary, result, source, correlation ID, and UTC timestamp. | P0 |
| OBS-004 | Audit storage MUST be append-only/tamper-evident, externally exportable, and retained for a tenant-configurable period with a seven-year supported maximum or longer where required. | P0 |
| OBS-005 | Reports MUST include expiry risk, unmanaged certificates, automation coverage, policy compliance, algorithm/key posture, ownership gaps, CA usage/cost, deployment success, exceptions, and audit evidence. | P0 |
| OBS-006 | Dashboards MUST distinguish inventory truth, endpoint-observed truth, and last automation result to avoid false success. | P0 |
| OBS-007 | Metrics, logs, traces, and events MUST carry tenant-safe correlation IDs and support SIEM/observability export. | P1 |

### 6.10 APIs, protocols, and integrations (`INT`)

| ID | Requirement | Priority |
|---|---|---|
| INT-001 | All product functions available in the UI MUST have a documented, versioned API unless a security rationale explicitly prohibits it. | P0 |
| INT-002 | APIs MUST provide OpenAPI documentation, pagination, filtering, stable error codes, correlation IDs, idempotency for mutations, and deprecation policy. | P0 |
| INT-003 | Webhooks MUST be signed, replay-resistant, retryable, observable, and configurable by event type. | P0 |
| INT-004 | Enterprise integrations SHOULD include ServiceNow/Jira, Splunk/Elastic/Sentinel, Slack/Teams/PagerDuty, Vault, cloud secret managers, CI/CD systems, and Terraform. | P1 |
| INT-005 | Automation clients SHOULD have CLI and SDK support for at least Go, Python, and TypeScript. | P2 |
| INT-006 | Import/export MUST support CSV/JSON plus safe migration APIs without exporting private keys. | P0 |
| INT-007 | Provider and deployer integrations MUST expose capability metadata so UI forms and validation are data driven, not hardcoded per vendor. | P0 |

### 6.11 Gateway and vSatellite execution (`EXEC`)

| ID | Requirement | Priority |
|---|---|---|
| EXEC-001 | Gateways MUST persist inventory, policies, approvals, credentials, commands, and audit state durably. | P0 |
| EXEC-002 | Multi-replica gateways MUST use shared state, leader election, command leasing/acknowledgement, and leader-only scheduled jobs. | P1 |
| EXEC-003 | vSatellite MUST execute discovery, deployment, verification, and health commands rather than only acknowledge transport. | P0 |
| EXEC-004 | Enrollment tokens MUST be single-use and short-lived; long-lived agent identity MUST be unique, rotatable, and revocable. | P0 |
| EXEC-005 | Commands MUST be signed, scoped, leased, idempotent, cancellable, and retained through transient disconnects. | P0 |
| EXEC-006 | Agents MUST auto-update through a signed, staged, rollback-capable channel with version policy and fleet visibility. | P1 |
| EXEC-007 | Offline execution nodes MUST expose affected certificates/endpoints and the latest safe renewal deadline. | P0 |

### 6.12 Administration, billing, and product operations (`ADM`)

| ID | Requirement | Priority |
|---|---|---|
| ADM-001 | Subscription enforcement MUST be server side and MUST NOT disable certificate safety operations during a billing dispute without a documented grace policy. | P1 |
| ADM-002 | Usage metering MUST be deterministic, auditable, replay-safe, and reconciled with marketplace billing where used. | P1 |
| ADM-003 | Product configuration MUST support tenant defaults, feature flags, data residency, retention, notification channels, maintenance windows, and branding without code changes. | P1 |
| ADM-004 | Support access MUST be time bounded, tenant approved, least privilege, visible, and fully audited. | P0 |
| ADM-005 | License/plan limits MUST be visible before an operation and report actionable remediation rather than silently dropping work. | P1 |

---

## 7. Required Portal Information Architecture and Views

The portals MUST present one coherent product model even though execution is distributed. Users should not need to know which component owns an API to complete a workflow.

### 7.1 Primary navigation

```text
Overview
Certificates
  Inventory
  Requests & Orders
  Renewal & Rotation
  Revocation
Discovery
Deployment
  Deployments
  Endpoints
  Credentials
Certificate Authorities
  CA Accounts
  Certificate Profiles
  CA Orders & Validation
Policy & Governance
  Policies
  Approvals
  Exceptions
Trust
  Trust Stores
  CA Hierarchy
Operations
  Alerts & Incidents
  Jobs & Automation
  Gateways & Agents
    Gateways / Deploy Gateway
    vSatellites / Deploy vSatellite
    Deployment Jobs
    Installation Artifacts
Reports
  Compliance & Risk
  Audit Log
Integrations
Administration
  Organizations & Tenants
  Identity & Access
  Subscription & Billing
  Platform Settings
```

Navigation MUST be permission aware. Hiding a menu item is not authorization; every backing API MUST independently enforce the same permission.

### 7.2 View requirements

| View | Required information | Required actions |
|---|---|---|
| **Enterprise Overview** | Total/managed/unmanaged certificates, expiry bands, failed renewals/deployments, automation coverage, policy compliance, unknown ownership, gateway health, trends | Drill into every metric; save tenant/business-unit filters; export summary |
| **Certificate Inventory** | Searchable, sortable, faceted list with domain, SANs, issuer, CA/profile, owner, app, environment, algorithm, validity, source, managed state, endpoint count, status, last seen | Bulk assign owner/tags, request renewal/rotation, deploy, revoke, export, accept risk, open detail |
| **Certificate 360** | Certificate metadata, chain, fingerprints, key reference/type, policy result, versions, requests/orders, deployments, endpoints, owner, alerts, audit timeline | Renew, rotate key, deploy, revoke, change owner, add endpoint, download allowed public material |
| **Requests & Orders** | Requester, profile, CA, names, key policy, cost, approval state, external validation, order timeline, failure reason | Create, clone, approve/reject, cancel, retry safely, complete DCV, open issued certificate |
| **Renewal & Rotation** | Due date, percentage of life, automation policy, readiness, endpoint/credential health, last attempt, next attempt, blockers | Run now, reschedule, pause, approve exception, retry, bulk wave, inspect logs |
| **Discovery** | Sources, scopes, schedules, run progress/history, new/changed/stale findings, errors, rate limits | Add/edit source, run/cancel, triage findings, assign owner, ignore with expiry, promote to managed |
| **Deployments** | Jobs and rollout waves, certificate version, endpoint, executor, strategy, result, verification, rollback availability | Preflight, deploy, pause/resume, retry, cancel, verify, rollback, compare versions |
| **Endpoints** | Endpoint type, provider/account, resource identifier/ARN/path, environment, owner, credential reference, executor, served fingerprint, drift, health | Add/test/edit/disable/remove endpoint; discover metadata; deploy selected certificate |
| **Credentials** | Name, provider/type, scope, backing store, created/rotated/expires/last used, health, endpoints using it; never secret values | Add, test, edit metadata, rotate secret, disable, remove with dependency check |
| **CA Accounts** | Vendor/type, environment, supported profiles/protocols, health, quotas, default scopes, credential reference | Add/test/edit/disable/remove, set default, view orders and failures |
| **Certificate Profiles** | Subject/SAN/EKU, algorithm, validity, renewal, CA routing, policy, approval, deployment defaults | Version, dry run, clone, activate/deprecate, impact preview |
| **Policies** | Scope, priority, inherited/effective rules, version, violations, affected objects | Create/edit/version, simulate, compare, approve/activate, disable, view impact |
| **Approvals** | Risk, requester, policy, cost, target environment, conflicts, history, SLA | Approve/reject/delegate/comment; bulk action only when policy permits |
| **Exceptions** | Policy, scope, reason, owner, compensating controls, approver, expiry, status | Request, approve/reject, renew, revoke, export evidence |
| **Trust Stores / CA Hierarchy** | Roots/intermediates, chain paths, store locations, dependent endpoints, distrust/expiry, algorithm risk | Plan staged update, approve, deploy, verify, rollback, export dependency report |
| **Alerts & Incidents** | Severity, object, owner, first/last occurrence, suppression, automation attempts, correlated job/audit data | Acknowledge, assign, suppress with expiry, retry automation, create ITSM incident, resolve |
| **Jobs & Automation** | Scheduled/running/completed jobs, progress, executor, retries, logs, correlation ID | Run, cancel, pause, retry, download diagnostics |
| **Gateways / Deploy Gateway** | Gateway version, location, platform, topology, deployment mode/job, state/backup/policy health, last heartbeat, capacity, certificate/endpoint/agent coverage | Deploy Gateway, register/import, preflight/plan, test, upgrade, drain, rotate identity, diagnostics, decommission |
| **vSatellites / Deploy vSatellite** | Parent gateway, placement, package/platform, capabilities, assigned scope, version, heartbeat, queue/capacity, covered endpoints/certificates | Deploy vSatellite, test, edit assignment, rotate identity, drain, upgrade, restart, revoke, decommission |
| **Fleet Deployment Jobs** | Gateway/agent provisioning plan, approval, stages, provider resources, generated artifact, registration/validation results, errors and correlation ID | Resume draft, approve/reject, cancel before irreversible stage, retry/reconcile, remediate, rollback/remove |
| **Installation Artifacts** | IaC/Helm/package/bootstrap type, platform, version, signature/checksum, expiry and used/unused/revoked state | Generate revision, download, copy install command, revoke unused artifact; bootstrap secret shown once only |
| **Compliance & Risk** | Controls, grades, exceptions, evidence, trend, owner/business-unit breakdown | Filter, schedule/export report, create remediation campaign, attest |
| **Audit Log** | Actor, tenant, role, action, target, result, before/after summary, source, correlation ID, timestamp | Filter, inspect, export, stream to SIEM; no edit/delete action |
| **Integrations** | Type, scope, health, credential reference, subscriptions, failures, last delivery | Add/test/edit/disable/remove, replay failed webhook, rotate credential |
| **Identity & Access** | Users, groups, federation source, roles, scopes, sessions, service accounts, last access | Provision/deprovision, assign scope/role, revoke session/token, enforce MFA |
| **Tenant Administration** | Tenant status, limits, residency, retention, features, defaults, billing, support access | Create/suspend/restore/offboard, configure, export, approve support access |

### 7.3 Common UX requirements

- Every list MUST support search, server-side filtering, sort, pagination, saved views, shareable URLs, export, and empty/error/loading states.
- Destructive actions MUST show dependency and impact previews, require explicit confirmation, and produce audit events.
- Secrets MUST use write-only inputs and MUST NOT be copied back to the browser after submission.
- Long-running work MUST return a job ID and show progress; the UI MUST NOT imply completion while work is only queued.
- Errors MUST identify the failed object, stage, safe retry behavior, and correlation ID without exposing secrets.
- Times MUST be stored in UTC and displayed in the user's configured timezone.
- Risk and status color MUST always include text/icon meaning; color alone MUST NOT communicate state.

---

## 8. Canonical Domain Model and State Machines

### 8.1 Core entities

| Entity | Purpose |
|---|---|
| Organization / Tenant / Scope | Administrative and data-isolation boundary |
| User / Group / Role / ServiceAccount | Human and machine authorization |
| Gateway / Agent | Distributed execution identity, placement, capability and health |
| GatewayDeployment / AgentDeployment | Immutable deployment plan revision, approval, provisioning/bootstrap, registration, validation and decommission record |
| InstallationArtifact | Signed IaC, Helm, package, image or bootstrap artifact with checksum, version, expiry and one-time-use state |
| Certificate | Stable lineage across multiple versions |
| CertificateVersion | One issued certificate, public chain, key reference, validity, and fingerprints |
| CertificateRequest / CAOrder | Request intent, approval, external order, validation, and issuance result |
| CAAccount / CertificateProfile | Vendor connection and reusable issuance policy |
| Policy / PolicyVersion / Exception | Preventive governance and temporary, approved deviation |
| Approval | Decision record linked to an exact action and payload digest |
| CredentialSet | Write-only authentication material or external-secret reference |
| DeploymentEndpoint | Destination resource and non-secret target metadata |
| DeploymentIntent / DeploymentPlan / DeploymentWave | SaaS-owned desired deployment, immutable approved plan revision and rollout grouping |
| DeploymentCommand / DeploymentAttempt | Gateway-scoped signed work plus executor/adapter stage checkpoints, observed state, verification and rollback evidence |
| DiscoverySource / DiscoveryRun / Observation | Scan scope, execution, and observed certificate/endpoints |
| TrustAnchor / TrustStore | Root/intermediate and relying-store relationship |
| Alert / Incident | Risk or operational failure and its response lifecycle |
| AuditEvent | Immutable evidence for user and automation activity |

### 8.2 Required states

```text
CertificateRequest:
  draft → submitted → awaiting_approval → approved → ordering
        → awaiting_validation → issued → deployment_pending → completed
        ↘ rejected | cancelled | failed

CertificateVersion:
  pending → issued → active → renewal_due → superseded → retired
                   ↘ revoked | expired | deployment_failed

DeploymentJob:
  draft → planned → awaiting_approval → scheduled → running → verifying → completed
        ↘ rejected | cancelled | blocked | failed → rolling_back → rolled_back

DeploymentCommand:
  pending → delivered → leased → running → reconciling → verified → completed
          ↘ expired | cancelled | blocked | failed → rollback_pending → rolled_back

DeploymentAttempt:
  created → authorized → material_resolved → preflight → staged → applied
          → verifying → committed → cleaned_up
          ↘ failed | indeterminate → reconciling → retryable | rollback_required | manual_intervention

GatewayDeployment:
  draft → planned → awaiting_approval → approved → provisioning/bootstrap
        → registering → validating → ready
        ↘ rejected | cancelled | failed → remediating | rolling_back → rolled_back
        ready → draining → decommissioning → decommissioned

AgentDeployment:
  requested → artifact_ready → installing → enrolling → validating → active
            ↘ expired | revoked | failed
  active → degraded | draining → upgrading → active
         ↘ offline | quarantined | decommissioning → decommissioned

InstallationArtifact:
  generating → available → downloaded → used
             ↘ expired | revoked | failed

PolicyException:
  requested → awaiting_approval → active → expiring → expired
                              ↘ rejected | revoked
```

State transitions MUST be validated server side, idempotent where retried, and recorded in audit history. A state MUST NOT be overwritten to hide an earlier failure; attempts are separate records.

---

## 9. API and Event Contract

- Public APIs MUST remain under versioned prefixes such as `/api/v1`.
- Additive changes MAY remain in the same version; removing/renaming fields, changing meaning, or narrowing accepted values requires a new version or compatibility adapter.
- Every collection endpoint MUST provide pagination metadata and deterministic sort.
- Every mutation MUST accept or generate a correlation ID; create/order/deploy operations MUST support idempotency keys.
- Error responses MUST include stable `code`, human-readable `message`, field/object details, retryability, and `correlation_id`.
- Event types SHOULD follow `<domain>.<entity>.<action>.v<version>`, for example `certificate.version.issued.v1`.
- Events MUST include tenant, actor/service principal, entity IDs, occurred-at time, correlation/causation IDs, schema version, and redacted payload.
- Consumers MUST be able to retry safely; delivery MUST be at least once with deduplication identifiers.
- OpenAPI and event schemas MUST be generated or checked in CI for incompatible changes.

---

## 10. Security and Compliance Requirements

| ID | Requirement | Release gate |
|---|---|---|
| SEC-001 | TLS for every network path; mTLS for gateway/agent machine communication | Production blocker |
| SEC-002 | No default/shared credentials; bootstrap secrets must be one-time, rotated, and absent from source/docs/images | Production blocker |
| SEC-003 | Secrets stored in KMS/HSM/Vault/secret-manager-backed storage; no browser `localStorage` credentials | Production blocker |
| SEC-004 | OIDC Authorization Code + PKCE, MFA, session revocation, and secure cookie/token handling | Production blocker |
| SEC-005 | Tenant isolation enforced and penetration-tested at API, storage, cache, job, event, and export layers | Production blocker |
| SEC-006 | Immutable audit coverage for identity, policy, credential, CA, certificate, deployment, tenant, and support actions | Production blocker |
| SEC-007 | Rate limiting, abuse detection, lockout/backoff, and alerting for authentication and expensive fan-out APIs | Production blocker |
| SEC-008 | Encryption at rest, documented key hierarchy, rotation, backup encryption, and restore procedures | Production blocker |
| SEC-009 | SAST, dependency scanning, secret scanning, container scanning, SBOM, signed artifacts, and provenance in CI | Production blocker |
| SEC-010 | Formal threat model covering SaaS, gateway, agent, browser, CA, cloud, supply-chain, and insider threats | Production blocker |
| SEC-011 | Least-privilege cloud/IAM policy templates and credential-health checks | Required |
| SEC-012 | Configurable retention, legal hold, privacy export/deletion, and data-residency controls | Required |
| SEC-013 | Support for common enterprise control evidence (SOC 2/ISO 27001 mappings) without claiming certification until independently attested | Required |

Security-sensitive values MUST be redacted by design, not by a best-effort logging convention. Support bundles and exports require the same review as APIs.

---

## 11. Non-Functional Requirements and SLOs

These are target acceptance criteria and MUST be validated by automated tests or operational evidence before an enterprise production release.

### 11.1 Availability and recovery

| Measure | Target |
|---|---|
| SaaS control-plane availability | ≥ 99.95% monthly, excluding announced maintenance |
| Gateway API availability | ≥ 99.9% monthly per HA deployment |
| Local renewal during SaaS outage | Continue for at least 72 hours using cached policy/config, configurable by tenant |
| Metadata RPO | ≤ 15 minutes |
| Metadata RTO | ≤ 4 hours |
| Gateway state RPO | ≤ 5 minutes for HA/shared-state deployments |
| Backup verification | Automated restore test at least monthly; documented disaster-recovery exercise at least twice yearly |

### 11.2 Performance and scale

| Measure | Target baseline |
|---|---|
| Metadata API latency | p95 ≤ 500 ms, p99 ≤ 1.5 s under rated load |
| Portal dashboard/search | p95 ≤ 2 s for standard tenant views; long exports run asynchronously |
| Authorization enforcement | Applied to every request with no cross-tenant cache leakage |
| Inventory capacity | 1,000,000 certificate versions platform-wide and 100,000 per tenant in the reference load profile |
| Endpoint capacity | 250,000 deployment endpoints platform-wide in the reference load profile |
| Gateway/agent fleet | 10,000 connected execution nodes in the reference load profile |
| Audit ingestion | Sustain peak mutation/event load without dropping events; backpressure rather than silent loss |
| Renewal completion | ≥ 99.9% of eligible automated renewals completed and verified before the policy deadline, excluding approved external blockers |

Scale values are release test profiles, not current capacity claims. They MAY be revised from measured production demand, but every release must publish its tested limits.

### 11.3 Reliability and operability

- Jobs MUST use leases, retries with bounded exponential backoff, dead-letter handling, idempotency, and visible retry state.
- Scheduled automation MUST detect missed runs after restart.
- Deployments MUST be safe under process crash, duplicate command delivery, partial network failure, and provider throttling.
- The platform MUST expose RED/USE metrics, structured logs, distributed traces, queue depth, scheduler lag, and integration health.
- Runbooks MUST exist for CA outage, mass expiry, bad deployment, credential compromise, tenant isolation incident, gateway loss, and restore.
- Release rollback MUST be documented and tested for database, API, UI, gateway, and agent changes.

### 11.4 Usability and accessibility

- Web portals MUST meet WCAG 2.2 AA.
- Keyboard navigation, visible focus, semantic labels, screen-reader announcements, reduced motion, and non-color status cues are required.
- Responsive layouts SHOULD support common desktop and tablet operations; emergency approval/incident views SHOULD remain usable on mobile.
- User-visible text, timezones, and date/number formatting SHOULD be localization ready.
- Contextual help MUST explain certificate terminology and the impact of high-risk actions.

---

## 12. Definition of Done

A capability is complete only when all applicable criteria pass:

1. Requirements and acceptance criteria are linked to implementation tasks.
2. API and UI implement the same authorization and workflow.
3. Tenant-isolation and negative-role tests exist.
4. Secrets are redacted and stored through approved secret infrastructure.
5. Success, failure, retry, timeout, partial completion, and rollback paths are tested.
6. Durable state survives restart and concurrent execution.
7. Every mutation emits a verified audit event.
8. Metrics, logs, traces, alerts, and correlation IDs are available.
9. Accessibility and browser behavior are validated for UI work.
10. OpenAPI/event schemas and operator/user documentation are updated.
11. Upgrade, rollback, backup, and restore impacts are tested.
12. Live integration is proven against the real provider or explicitly labeled protocol-mock-only.
13. No critical/high unresolved security finding remains unless formally accepted with owner and expiry.

A successful build or HTTP 200 alone is not proof of completion.

---

## 13. Enterprise Release Gates

### Gate A — Secure pilot

All P0 security blockers closed: TLS/mTLS, secret removal/rotation, encrypted credential backend, consistent identity ownership, tenant isolation tests, control-plane audit, rate limiting, backups, CI security checks, and no reusable credentials in documentation.

### Gate B — Lifecycle-complete production

Discovery → ownership → request/approval → issuance → deployment → serving-endpoint verification → automatic renewal/redeployment → revocation works end to end for at least:

- ACME with HTTP-01 and DNS-01
- One private enterprise CA
- One public commercial CA using real sandbox/production credentials
- Linux/web server, AWS, Azure or GCP, and Kubernetes target families
- Failure rollback and alert/incident workflow

### Gate C — Enterprise operations

HA control plane and gateway, command leasing, executable vSatellite, DR evidence, SLO dashboards, SIEM/ITSM integrations, multi-level approvals, policy inheritance, external audit retention, and published scale test results.

### Gate D — Market-parity expansion

Trust-store lifecycle, ADCS and enrollment protocol breadth, F5/NetScaler/Windows/Java integrations, workload identity, certificate transparency monitoring, crypto-agility/PQC reporting, and optional SSH/code-signing lifecycle modules.

---

## 14. Prioritized Delivery Roadmap

### P0 — Make the existing product safe and lifecycle complete

1. TLS on browser/API paths and mTLS for gateway/agent communication.
2. Remove committed/default credentials; rotate every exposed test or shared secret.
3. Replace browser-stored cloud credentials with an encrypted backend and write-only APIs.
4. Reconcile Keycloak identity with tenant membership through one transactional ownership model.
5. Complete tenant-isolation, RBAC-negative, rate-limit, and control-plane audit coverage.
6. Live-validate the implemented DNS-01 and renewal redeployment path against Route 53/ACME and real serving endpoints; add Azure DNS, GCP Cloud DNS, and OCI DNS adapters.
7. Make vSatellite execute scan/deploy/verify commands with lease/ack semantics.
8. Add CI, security scanning, signed artifacts, backups, restore tests, and platform monitoring.

### P1 — Reach enterprise operational parity

1. Shared gateway state, leader election, HA, graceful drain, and command leasing.
2. EST/SCEP/CMPv2, ADCS/private CA, Kubernetes/cert-manager, and real commercial CA validation.
3. Endpoint integrations for Windows/IIS, AWS edge/API targets, Azure, GCP, F5, NetScaler, and Java stores.
4. Multi-level approvals, policy inheritance/versioning, expiring exceptions, and separation of duties.
5. External immutable audit retention, SIEM/ITSM, webhooks, OpenAPI, CLI, and Terraform provider.
6. Trust-store inventory, dependency graph, staged trust changes, and CA migration campaigns.
7. SLOs, load tests, capacity publication, DR exercises, and WCAG 2.2 AA verification.

### P2 — Differentiate and prepare for future trust models

1. Certificate Transparency and domain-abuse monitoring.
2. Risk-prioritized remediation campaigns and ownership inference.
3. Workload identity and service-mesh integrations.
4. HSM/KMS-backed non-exportable key workflows and crypto-agility/PQC migration planning.
5. Optional SSH key, code-signing, and document-signing lifecycle modules.
6. Advanced analytics for automation coverage, outage likelihood, CA cost, and policy effectiveness.

---

## 15. Market-Parity Benchmark

The requirement baseline reflects recurring capabilities in established enterprise CLM platforms: broad discovery, automated renewal/provisioning, preventive policy, multi-CA support, enrollment protocols, endpoint integrations, risk visibility, and hybrid execution.

Reference material used for the benchmark:

- [DigiCert Trust Lifecycle Manager — protocols, patterns, and integrations](https://docs.digicert.com/zf/trust-lifecycle-manager/get-started/trust-architecture-playbook/automation-pillar/select-protocols--patterns--and-integrations.html)
- [Venafi TLS Protect certificate management](https://docs.venafi.com/Docs/25.1/TopNav/Content/Certificates/c_certificate_management_tpp.php)
- [Keyfactor — certificate lifecycle automation at enterprise scale](https://www.keyfactor.com/blog/certificate-lifecycle-automation-at-enterprise-scale/)
- [AppViewX — automating short certificate lifecycles](https://www.appviewx.com/blogs/automating-47-day-certificate-lifecycles/)

The goal is feature and operational parity, not copying another vendor's interface or proprietary implementation. Requirements were synthesized with the repository's customer-controlled gateway architecture and private-key boundary as non-negotiable constraints.

*Content was rephrased for compliance with licensing restrictions.*

---

## 16. Cross-Component Architecture Review

> **Document status:** Independent review, added 2026-08-21. The component READMEs each carry their own itemized gap list with acceptance criteria — this section prioritizes across all three components together and calls out structural risks that are easy to miss when reading one component README at a time.

### 16.1 Top structural risks

1. **The tree is not under version control.** There is no `.git` at the repository root. This blocks safe deletion of known dead code (`cert_repo.go`, `approval_repo.go`, `purchase_repo.go` in the control plane) and makes every other fix on this list harder to review or roll back. Do this before anything else here.
2. **"Shared" code is copied via a build-time `sed` hack, not a real shared module.** `internal/core` and `pkg/` physically live under `saas-control-plane/`; the satellite gateway's `Dockerfile.satellite` rewrites its own `go.mod` module path with `sed` so imports resolve, and `build-src-bundle.sh` separately re-roots `internal/` into `internal/satellite/` at bundle time. Both component READMEs note this has broken the build before. The durable fix is a real shared Go module (e.g. `github.com/m2p/clm/pkg`) that each component depends on normally, not text-rewritten import paths — worth doing before more shared code accumulates.
3. **The mTLS and durable SaaS-command foundation is code-ready, not live-verified.** The control plane can serve TLS with required/optional client-certificate verification, binds certificate SAN identity plus a unique hashed bearer credential to gateway routes, and leases durable commands. The gateway verifies server CA/name, presents a client certificate, persists duplicate outcomes and renews leases. No live PKI handshake or provider deployment validation was performed, commands are not yet signed, and the separate gateway-to-vSatellite coordinator transport remains Release 2 work.
4. **Neither component can run more than one replica today.** Control-plane command delivery now uses atomic leases and idempotency, but the gateway still needs shared transactional execution state and leader election, while identity/session reconciliation remains incomplete. Per ARC-010, HA work should stay inside the existing binaries rather than prematurely splitting services.
5. **vSatellite is architecture without an implementation, and its own README questions whether it should exist.** Its "Recommended Next Steps" opens with "decide whether it is still needed," since the gateway can already scan any CIDR it can route to. Resolve that question before investing in Release-2 lease/ack execution semantics for a component that may get cut.

### 16.2 Cross-component sequencing

Each component README has its own suggested sequence; merged across all three with dependencies made explicit:

| Order | Work | Why it gates the next step |
|---|---|---|
| 1 | Put the tree in git | Everything else needs safe rollback |
| 2 | Reconcile control-plane identity stores + tenant scoping | Gateway auth and multi-tenant isolation both assume correct tenant identity |
| 3 | Replace the bearer token with mTLS for gateway↔control-plane | Gates ARC-004/ARC-005 compliance and any encrypted-credential work, since the channel itself is the weakest link |
| 4 | Cloud Accounts backend + secret rotation + TLS everywhere | Gates any non-demo deployment |
| 5 | Extract a real shared Go module (retire the `sed` hack) | Cheaper now than after more shared code accumulates |
| 6 | DB-backed gateway inventory + leader election (HA prerequisite for both components) | Needed before either component can scale beyond one node |
| 7 | vSatellite go/no-go decision | Avoid building Release-2 execution semantics for a component that might not be needed |

---

## 17. Repository Documentation Map

| Document | Purpose |
|---|---|
| [`components/README.md`](components/README.md) | Component ownership, repository layout, and build boundaries |
| [`components/saas-control-plane/README.md`](components/saas-control-plane/README.md) | SaaS implementation status, API, deployment, HA design, and operational runbook |
| [`components/satellite-gateway/README.md`](components/satellite-gateway/README.md) | Gateway lifecycle behavior, policy, audit, deployment integrations, APIs, and known limitations |
| [`components/vsatellite/README.md`](components/vsatellite/README.md) | Agent scaffolding, intended execution model, and current limitations |

When documents conflict, use this priority:

1. Tested running behavior and release evidence
2. Component README for implementation status
3. This root README for product requirements and target behavior
4. Historical notes or diagrams

Status changes MUST update the relevant component README and the current-baseline table in this document. Requirements MUST NOT be marked Verified without evidence matching the Definition of Done.
