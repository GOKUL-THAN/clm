# Component 1: SaaS Control Plane + Admin Portal

## Role

Central management plane hosted in AI Forgers cloud. Single multi-tenant instance serving all customer organizations. Provides the control layer for identity management, gateway fleet operations, user/tenant administration, and remote infrastructure provisioning.

**Scope:** Identity (Keycloak), Organization & Tenants, Subscription & Billing (incl. cloud marketplace), Users, SSO, Gateway Deployment & Fleet.
**Not in scope:** Certificates, Approvals, CA Purchasing, Discovery, Crypto — these run on the Satellite Gateway (Component 2).

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                    SaaS Control Plane                                │
│                                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │
│  │  Admin   │  │  REST    │  │ Keycloak │  │  Deploy Engine   │   │
│  │  Portal  │  │  API     │  │  24.0    │  │  (Lightsail/EC2/ │   │
│  │  (SPA)   │  │  (Gin)   │  │ (IdP)   │  │   EKS)           │   │
│  └────┬─────┘  └────┬─────┘  └────┬──────┘  └────────┬─────────┘  │
│       │              │             │                   │            │
│       └──────────────┼─────────────┼───────────────────┘            │
│                      │             │                                │
│              ┌───────▼──────┐  ┌───▼───────────┐                    │
│              │  PostgreSQL  │  │     Redis     │                    │
│              │  (metadata)  │  │  (cache/rate) │                    │
│              └──────────────┘  └───────────────┘                    │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Features

### Status Legend

Statuses are deliberately precise — "Done" alone hides important differences between a working feature and a prototype.

| Status | Meaning |
|--------|---------|
| **Done** | Implemented end-to-end with server-side persistence; verified against the live test deployment |
| **Ephemeral** | Works, but backing store is not durable (lost on container/VM recreation) |
| **UI-only** | Portal screen exists; data is held in browser `localStorage` with no backend API |
| **Code Ready** | Implemented but not exercised against a live target |
| **Planned** | Not implemented |

### Authentication & Identity

| Feature | Status | Description |
|---------|--------|-------------|
| Keycloak integration | Done | Realm `clm`, Direct Grant auth (password flow), RS256 JWT |
| Dual-JWT validation | Code Ready | Internal HMAC and external JWKS tokens enforce configured issuer, audience, and algorithm; missing/invalid subjects and tenant claims fail closed. JWKS fetches use bounded timeout/status/body handling and a concurrency-safe parsed-key TTL cache. Offline tests cover valid and rejected claims; no live IdP verification is claimed for Task 2 |
| Identity Management page | Done | Native Keycloak admin from the portal (users, roles, IdPs, sessions, clients, groups) |
| 8 RBAC roles | Done | super_admin, admin, tenant_admin, cert_manager, approver, operator, auditor, viewer |
| Direct Grant login | Done | Password flow via same-origin `/auth/` proxy (no blocked JS adapter) |
| Token auto-refresh | Done | Refresh token rotation every 60s |
| Realm data durability | **Done** | Keycloak runs in production mode (`start`) against PostgreSQL (`keycloak` database, 92 tables managed by Liquibase). Realm users, groups, role mappings, and identity providers survive container recreation — verified by creating a user, destroying the container, and confirming both the user and its password persisted |
| Local groups | Done | Keycloak realm groups: create, delete, assign/remove users, group-based role inheritance |
| Social IdP support | Done | Google, Microsoft, GitHub, GitLab, LinkedIn, Bitbucket via Keycloak identity broker |
| Third-party IdP groups | Code Ready | External IdP group claims mapped into Keycloak groups via identity provider mappers; not yet exercised against a live external directory |
| Generic OIDC federation | Code Ready | Per-tenant external OIDC provider with group/role claim mapping |
| SAML 2.0 federation | Code Ready | Per-tenant SAML IdP with attribute-to-group mapping |
| MFA | Planned | TOTP-based multi-factor authentication |
| Login rate limiting | Planned | Redis is deployed but no limiter is wired to the auth/token endpoints |

#### Identity Management — Groups

The Identity Management page supports two group models that work together:

**Local Groups (Keycloak realm groups)**
- Created and managed directly in the `clm` realm via the Admin Portal
- Users are assigned to groups manually or via the API (`PUT /users/:id/groups/:gid`)
- Each group can carry realm role mappings — users inherit roles from their group membership
- Nested sub-groups are supported; role mappings cascade down the group tree
- API: `GET /api/v1/identity/groups` lists the realm group tree

**Third-Party Identity Provider Groups**
- When an external IdP (Google Workspace, Azure AD, Okta, etc.) includes group claims in the token, Keycloak maps them to local groups automatically via identity provider mappers
- Supported claim types: `groups` (OIDC), `memberOf` (SAML attribute)
- First-broker-login flow auto-creates the group in Keycloak if it doesn't exist (configurable)
- Group-to-role mapping: an imported IdP group (e.g., `engineering`) can be mapped to a realm role (e.g., `operator`) so federated users inherit the correct CLM permissions on first login
- This enables organizations to manage CLM access from their corporate directory rather than manually assigning roles in the portal

### Organization, Tenants, Subscription & Billing

| Feature | Status | Description |
|---------|--------|-------------|
| Multi-tenant isolation | Code Ready | Tenant-scoped repositories/services and ownership checks cover tenants, users, satellites/commands, deployment jobs, SSO, cloud accounts, and audit reads; all-tenant user/satellite operations require platform-admin roles. Offline negative tests pass; live multi-tenant verification remains required |
| Organization profile | Done | Org name, domain, data region, contacts |
| Tenant CRUD | Done | Create, read, update (plan/status/name), delete |
| Guarded cascade delete | Done | Refuses if dependents exist; reports counts; `?force=true` to override |
| Self-lockout protection | Done | Cannot delete the tenant your own account belongs to |
| Plan-based limits | Done | free (100/2/5), pro (10K/20/50), enterprise (100K/100/500) |
| Subscription management | Planned | Plan tier, entitlements, usage vs. limit, upgrade/downgrade, renewal |
| Direct billing | Planned | Billing account, payment method, invoices, per-tenant cost breakdown |
| AWS Marketplace billing | Planned | SaaS subscription/contract listing, `ResolveCustomer`, `BatchMeterUsage` metering |
| GCP Marketplace billing | Planned | Marketplace SaaS listing, Procurement API entitlements, usage reporting |
| Azure Marketplace billing | Planned | Marketplace SaaS offer, Fulfillment API v2, Metering Service `usageEvent` |
| SSO configuration | Done | Per-tenant IdP config (SAML, OIDC, Google, Microsoft) |

### Identity Management (Users, Groups, Roles, IdPs)

| Feature | Status | Description |
|---------|--------|-------------|
| User CRUD | Done | Create, list, update profile, enable/disable, reset password, revoke sessions |
| Role assignment | Done | SetUserRole clears existing roles, assigns new one (single-role model) |
| Local groups | Done | Create/delete groups, add/remove members, member counts — exposed at `/api/v1/identity/groups` and driven from the portal's Groups tab |
| Group-to-role mapping | Done | A group carries one realm role; members inherit it. `POST /identity/groups/:gid/role` clears any previous mapping first, mirroring the single-effective-role model used for users |
| Third-party IdP groups | Code Ready | External IdP group claims auto-mapped to local groups via identity provider mappers |
| Identity providers | Done | Add/configure/remove social IdPs, generic OIDC, and SAML providers |
| Session management | Done | View all active sessions across clients; revoke individual sessions or per-user |
| Realm clients (read-only) | Done | List OIDC clients registered in the `clm` realm |

> **Dual user store caveat.** Two independent user stores exist: the PostgreSQL `users` table (served by `/api/v1/users`) and the Keycloak realm (served by `/api/v1/identity/users`). They are **not synchronised** — a user created in one is invisible to the other. RBAC is read from the Keycloak token while tenant scoping comes from PostgreSQL, so the two must be kept aligned by hand. See Recommended Improvements #2.

### Gateway Fleet Management

| Feature | Status | Description |
|---------|--------|-------------|
| Gateway registration | Code Ready | Protected registration creates a unique random bearer credential, stores only its scrypt hash, and returns plaintext once; rotate/revoke APIs are tenant authorized |
| Gateway machine authentication | Code Ready | Heartbeat, claim, ack, lease-renewal and result routes bind the bearer credential to route satellite/tenant; optional/required verified client certificates bind URI/DNS SAN identity. No live PKI handshake is claimed |
| Heartbeat monitoring | Code Ready | Authenticated heartbeat retains the 30s check interval and 2m offline timeout |
| Command dispatch | Code Ready | PostgreSQL queue uses tenant/idempotency keys with payload/type conflict detection, atomic `FOR UPDATE SKIP LOCKED` claims, lease expiry, ack/renew/result token guards, bounded retry/backoff, cancellation, retry and dead-letter state |
| Fleet listing | Done | Name, status, region, cloud, IP, last heartbeat |
| Gateway CRUD | Done | PATCH (rename/config), DELETE (unregister) |

Task 3 gateway transport APIs:

- Machine routes: `POST /api/v1/satellites/:id/heartbeat`, `GET .../commands`, and command `ack`, `lease`, and `result` routes require the satellite bearer credential; production also requires a verified client certificate.
- Operations: `POST /api/v1/satellites/:id/credentials/{rotate|revoke}` and command `:cmd_id/{cancel|retry}` use JWT role and tenant ownership checks.
- Migration `004_secure_command_transport.sql` exists in both migration trees and adds credential metadata plus leased/idempotent command state. The server installer copies and applies migrations `001`–`004` in order with stop-on-error behavior. Plaintext credentials are never persisted and appear only in registration/rotation responses.
- Production validation requires HTTPS, `client_auth: required`, a gateway CA bundle, and disables the explicit development-only legacy switch. This is offline code/test evidence only; no live certificate authority or handshake was exercised.

### Gateway Deployment Engine

| Feature | Status | Description |
|---------|--------|-------------|
| AWS Lightsail | Code Ready | Instance + static IP + firewall + bootstrap code exists, but Task 2 did not run live provisioning or teardown verification; single-node only |
| AWS EC2 + existing target group | Code Ready | Launches one instance and can register it to a caller-supplied target group; it does not create an ALB/target group, and the placeholder AMI must be replaced per region before live use |
| AWS EKS | Planned | Incomplete scaffold only: not registered in composition, no valid EKS token generation, CA verification, Helm flow, or teardown |
| GCP / Azure | Planned | GCE, GKE, Azure VM, AKS |
| Linux (multi-flavour) | Done | `internal/saas/deploy/bootstrap.go` detects the distribution from `/etc/os-release` and selects apt, dnf, or yum, exposing `pkg_update`/`pkg_install` wrappers used by both the Lightsail and EC2 bootstraps. Verified by running the generated detection inside Ubuntu 22.04, Debian 12, Rocky 9, AlmaLinux 9, and Amazon Linux 2 — each resolved to the correct manager. An unrecognised distribution fails with the detected `ID` rather than an obscure `apt-get: command not found` |
| Async job tracking | Ephemeral | Process-local background goroutines; state is lost on restart and concurrent access still needs hardening |
| Job cancellation | Planned | Current endpoint only changes displayed state; it does not cancel provider work and must not be treated as effective cancellation |
| Gateway teardown | Code Ready | Provider teardown code exists but has not been live-verified; Lightsail static-IP cleanup requires correction before relying on it |
| Gateway upgrade | Planned | Endpoint acknowledges the request but does not yet queue an upgrade command |
| Real-time UI | Done | Auto-refresh every 5s |
| **High availability** | **Planned** | Multi-replica gateway deployment — see below |
| **Load-based autoscaling** | **Planned** | Scale replicas on queue depth and CPU — see below |

---

## High Availability & Autoscaling

> **Status: design, not implemented.** Every gateway deployed today is a single node with an
> in-memory certificate and approval store. The prerequisites below are hard blockers, not
> polish — deploying replicas before they are addressed produces duplicated work and lost
> approvals rather than availability.

### Prerequisites (must land before any replica count above 1)

| # | Blocker | Why it blocks | Resolution |
|---|---------|--------------|------------|
| P1 | **Gateway state is in-memory** | Certificate inventory and approval records live in a Go map. Two replicas hold divergent inventories, and an approval created on replica A is invisible to replica B | Externalise to PostgreSQL/SQLite shared by the replica set (gateway README item 1) |
| P2 | **One satellite identity per gateway** | `satellite.yaml` carries a single `id`. N replicas heartbeating with the same ID overwrite each other's status; registering separately makes one logical gateway appear as N gateways in the fleet | Adopt the cluster identity model below |
| P3 | **Resolved for control-plane delivery** | Commands are atomically claimed with `FOR UPDATE SKIP LOCKED`, carry lease owner/token/expiry, and require ack/result token validation. Gateway-local shared state and leader election are still required before multi-replica operation | Keep the leased queue; add shared gateway execution state and leader election |
| P4 | **Singleton work has no leader** | Renewal scheduling and expiry monitoring (both planned) must run once per gateway, not once per replica, or every certificate gets N renewal attempts | Leader election (see below) |
| P5 | **Discovery scans are unpartitioned** | Two replicas told to scan `10.0.0.0/16` both scan all of it — double the network load for the same result | Partition CIDRs across replicas, or delegate scanning to vSatellite agents |

Private key custody is **not** a blocker: secrets already go to AWS Secrets Manager rather than local disk, so replicas can share key material without change. This is the one piece of the design that is already HA-ready.

### Cluster identity model

A gateway cluster registers with the control plane as **one logical satellite**, not N:

```
                    ┌──────────────────────────────┐
                    │   SaaS Control Plane         │
                    │   satellite: dc-mumbai-01    │  ← one fleet entry
                    └──────────────┬───────────────┘
                                   │ heartbeat + command pull
                                   │ (leader only)
                    ┌──────────────▼───────────────┐
                    │      Load Balancer / VIP     │  ← portal + API traffic
                    └──┬────────────┬───────────┬──┘
                       │            │           │
                  ┌────▼────┐  ┌────▼────┐  ┌───▼─────┐
                  │ replica │  │ replica │  │ replica │
                  │ LEADER  │  │ follower│  │ follower│
                  └────┬────┘  └────┬────┘  └───┬─────┘
                       └────────────┼───────────┘
                          ┌─────────▼──────────┐
                          │  shared state:     │
                          │  PostgreSQL (certs,│
                          │  approvals, leases) │
                          │  AWS SM (keys)     │
                          └────────────────────┘
```

- **Shared `satellite_id`** across replicas; each replica additionally reports a unique `replica_id`.
- **Leader election** via a database advisory lock (`pg_try_advisory_lock`) or a lease row with a TTL. Simplest correct option given PostgreSQL is already a dependency; avoids adding etcd or Consul.
- **Leader owns** the SaaS heartbeat, command pull, renewal scheduling, and expiry monitoring.
- **All replicas serve** the Portal API and execute deployment and scan work claimed from the shared queue.
- Heartbeat payload gains `replica_count` and `healthy_replicas` so the fleet view can show cluster health rather than a single node's status.

### HA topology per deploy target

| Target | HA model | Failover | Notes |
|--------|----------|----------|-------|
| **AWS Lightsail** | **Not supported** | — | No load balancer integration, no ASG. HA requires migrating to EC2+ALB or EKS. Documented explicitly because the deploy form currently defaults here |
| **AWS EC2 + ALB** | Auto Scaling Group across ≥2 AZs behind the existing ALB | ALB health checks on `/health`; ASG replaces unhealthy instances | Natural fit — the ALB and launch template are already provisioned |
| **AWS EKS** | Deployment with `replicas: N`, PodDisruptionBudget, anti-affinity across nodes | Kubernetes restarts and reschedules | Cheapest path to correct HA once P1–P4 are done |
| **GCP GCE / GKE** | Managed Instance Group + regional LB / Deployment + PDB | MIG autohealing / kubelet | Follows the AWS pattern |
| **Azure VM / AKS** | VM Scale Set + Load Balancer / Deployment + PDB | VMSS health probes / kubelet | Follows the AWS pattern |
| **On-premise Linux** | N nodes with keepalived VIP, or an existing external LB | VRRP failover on VIP | No autoscaling — capacity is fixed by the hosts registered |

### Autoscaling signals

Certificate count is **not** a load signal — a gateway managing 10,000 idle certificates is doing no work. Scale on queued work and the resources it consumes:

| Signal | Source | Scale-out trigger | Rationale |
|--------|--------|------------------|-----------|
| **Discovery scan queue depth** | Pending `scan_cidr` work items | > 5 queued per replica | Scans are the dominant burst workload; `NetworkScanner` runs 50 concurrent probes by default |
| **Deployment queue depth** | Pending deploy commands | > 10 queued per replica | Deployments are I/O-bound and parallelisable |
| **CPU utilisation** | ASG / HPA metric | > 70% for 3 minutes | Catches scan bursts the queue metric misses |
| **vSatellite agent count** | Coordinator registry | > 200 agents per replica | Each agent heartbeats every 30s; coordinator work grows linearly |

Scale-in must be **conservative**: drain in-flight scans and deployments before terminating, and never scale below 2 replicas once HA is enabled. A 10-minute cooldown and a scale-in threshold well below the scale-out one prevent flapping during intermittent scan schedules.

| Platform | Mechanism | Metric plumbing |
|----------|-----------|-----------------|
| EC2 + ALB | ASG target-tracking + step scaling | Custom CloudWatch metric published by the leader for queue depth |
| EKS | HPA on CPU, plus KEDA or a custom metrics adapter for queue depth | `external.metrics.k8s.io` |
| GKE / AKS | HPA, same shape as EKS | Managed Prometheus / Azure Monitor |
| On-premise | None — fixed capacity | Alert on sustained queue depth instead |

### Deploy form and API additions

The deploy request gains an optional HA block; omitting it preserves today's single-node behaviour:

```json
{
  "name": "dc-mumbai-01",
  "cloud": "aws",
  "mode": "ec2",
  "ha": {
    "enabled": true,
    "min_replicas": 2,
    "max_replicas": 6,
    "availability_zones": ["ap-south-1a", "ap-south-1b"],
    "autoscale": {
      "enabled": true,
      "target_cpu_percent": 70,
      "scan_queue_per_replica": 5,
      "scale_in_cooldown_seconds": 600
    }
  }
}
```

Validation should reject `ha.enabled` for `mode: "lightsail"` and for on-premise targets with a single registered host, with a message naming the supported modes rather than a generic error.

Fleet listing gains `replica_count`, `healthy_replicas`, and `ha_enabled` so a degraded cluster is visible before it fails outright.

### Suggested implementation order

| Order | Work |
|-------|------|
| 1 | Externalise gateway state to a database (P1) — nothing else is safe without it |
| 2 | Leader election via advisory lock (P4) and move heartbeat/command pull to the leader (P2, P3) |
| 3 | Add command claiming with leases so work is executed exactly once |
| 4 | EKS multi-replica first — Kubernetes provides the HA primitives, so it validates the model with the least new provisioning code |
| 5 | EC2 ASG across two AZs, publishing queue-depth to CloudWatch |
| 6 | Autoscaling policies, scale-in draining, and CIDR partitioning (P5) |
| 7 | GCP and Azure equivalents |

---

## Admin Portal Pages

Implemented as documented: the nav has exactly these entries, with Organization/Tenants/Subscription/Billing and Users/Groups/Roles/Providers/Sessions/Clients as sub-tabs rather than separate pages. Old deep links (`organization`, `tenants`, `users`, `groups`) still resolve, routing into the corresponding hub tab.

| Page | Roles | Description |
|------|-------|-------------|
| Dashboard | all | Stats: tenants, users, gateways, deploy jobs |
| **Organization & Tenants** | all (Tenants/Subscription/Billing tabs require admin) | Unified org/tenant/billing hub (see sub-tabs below) |
| SSO Config | super_admin, admin, tenant_admin | Per-tenant identity federation |
| Cloud Accounts | all | Register/validate cloud and on-premise infrastructure credentials (see below) |
| Gateways | all | Fleet list + deploy new + manage (portal, restart, status, commands) |
| Deploy Jobs | super_admin, admin, tenant_admin | Real-time deployment progress |
| Activity Log | all | Audit trail |
| **Identity Management** | super_admin, admin | Unified identity hub (see sub-tabs below) |

### Organization & Tenants Sub-Tabs

Merges the former standalone Organization and Tenants pages into a single hub covering organization profile, tenant lifecycle, subscriptions, and billing.

| Tab | Roles | Description |
|-----|-------|-------------|
| Organization | all | Org profile: name, primary domain, data region, contact details, technical/billing owners |
| Tenants | super_admin, admin | Tenant lifecycle: create, rename, change plan, suspend/activate, delete (guarded cascade with `?force=true`) |
| Subscription | super_admin, admin | Plan per tenant with usage against entitlements (gateways, users, certificates), inline plan change, and the tier definition table. Usage bars turn amber at 70% and red at 90% |
| Billing | super_admin, admin | Billing mode (direct vs. marketplace) and the three marketplace integrations with their binding/metering mechanism. **States plainly that billing is not active** rather than rendering placeholder invoices |

> **Subscription tab caveat.** Certificate usage reads 0 because certificates live on each gateway and gateway-side counts are not yet reported to the control plane. The tier table mirrors `planLimits()` in `internal/saas/tenant/service.go` — the server rewrites a tenant's limits from its plan on every update, so showing anything else would mislead.

### Subscription & Billing — Cloud Marketplace Integration

Billing can either be handled directly or delegated to a cloud provider's marketplace, so customers pay through their existing cloud commitment (and can draw down committed spend).

| Marketplace | Integration | Metering | Status |
|-------------|------------|----------|--------|
| **AWS Marketplace** | SaaS Subscription / Contract listing; `ResolveCustomer` on redirect to bind `CustomerIdentifier` + `ProductCode` to the tenant | AWS Marketplace Metering Service `BatchMeterUsage` for usage dimensions | Planned |
| **GCP Marketplace** | Google Cloud Marketplace SaaS listing; entitlement binding via Procurement API `Entitlement` + account approval | Service Control API / Cloud Billing usage reporting | Planned |
| **Azure Marketplace** | Azure Marketplace SaaS offer; landing page token resolved via SaaS Fulfillment API v2 to bind the subscription | Marketplace Metering Service `usageEvent` for custom dimensions | Planned |

**How marketplace billing binds to a tenant:**
1. Customer subscribes to the CLM listing in AWS / GCP / Azure Marketplace.
2. The marketplace redirects to the CLM registration landing page with a resolution token.
3. CLM resolves the token against the provider's fulfillment API to obtain the subscription/entitlement identifiers.
4. Those identifiers are stored against the tenant, and the tenant's plan is set from the purchased offer tier.
5. Usage (certificates managed, gateways deployed, users) is reported back to the marketplace on the metering dimensions defined for that offer.
6. Plan changes made in the marketplace propagate to CLM via the provider's webhook/notification channel; plan changes made in CLM are rejected for marketplace-billed tenants so the marketplace stays the source of truth.

**Billing model:**
- **Direct billing** — plan-based flat rate with entitlement caps enforced by `planLimits()`
- **Marketplace billing** — plan tier comes from the purchased marketplace offer; overage reported as metered usage
- Per-tenant cost attribution so a reseller/MSP can see spend broken down by customer organization

### Identity Management Sub-Tabs

| Tab | Description |
|-----|-------------|
| Users | Create, list, update profile, assign role, enable/disable, reset password, revoke sessions — replaces the standalone Users & Access page |
| Groups | Local realm groups backed by Keycloak: create with an inherited role, change the mapped role inline, add/remove members, member counts — replaces the standalone `localStorage` Groups page |
| Roles | List, create, delete realm roles |
| Identity Providers | Add/remove external IdPs (Google, Microsoft, GitHub, generic OIDC, SAML); configure group claim mapping so federated users inherit correct roles |
| Active Sessions | View and revoke SSO sessions across all clients |
| Clients | Read-only list of OIDC clients registered in the realm |

### Cloud Accounts Backend

Task 2 adds tenant-scoped REST CRUD at `/api/v1/cloud-accounts`. AWS, GCP, and Azure credential payloads are accepted only on create or explicit rotation, validated against provider-specific field allowlists (complete AWS key pair or role ARN, GCP service-account JSON, complete Azure service-principal fields), encrypted with AES-256-GCM using tenant/account authenticated data, and stored as encryption version + nonce + ciphertext. Read/list responses contain metadata and credential version only; no secret field is serialized. Names are unique per tenant and create supports `Idempotency-Key`; account creation and replay binding commit in one transaction, and replay rows cascade when an account is deleted.

The 32-byte master key is resolved from `CLM_CLOUD_ACCOUNT_MASTER_KEY` or a `file://` config reference. Production startup rejects a missing or incorrectly sized key; non-production starts with the cloud-account/deploy routes disabled when no key is configured. Offline validation returns `status: "unverified"` after decrypting and parsing the tenant-owned blob. It **does not call or claim support from a live provider**. Deploy/list-infrastructure/teardown paths bind the account to the caller tenant and provider; deployment jobs retain `cloud_account_id` in their current process-local state. AWS role-ARN accounts additionally require the control-plane runtime to have source credentials and `sts:AssumeRole` permission; static-key accounts never fall back to the runtime credential chain.

| Type | Backend storage | Validation/support status |
|------|-----------------|---------------------------|
| AWS | AES-256-GCM encrypted credential blob | Offline structural validation only; existing AWS provisioner code is not Task 2 live-provider evidence |
| GCP | AES-256-GCM encrypted credential blob | Offline structural validation only; provisioning remains planned |
| Azure | AES-256-GCM encrypted credential blob | Offline structural validation only; provisioning remains planned |
| On-Premise Linux | Not accepted by this Task 2 backend | Existing UI concept only; no backend SSH validation or live support claim |

The old browser `localStorage` implementation may still exist in the unchanged portal and must not be treated as the authoritative backend. No portal files were changed by Task 2.

## Task 2 Security Backend Status

**Status: Code Ready, offline validated; not live-provider verified.**

- Authentication has no seed/default tenant fallback. Non-platform identities require a non-nil UUID `tenant_id`; every identity requires a valid UUID `sub`; platform roles may omit tenant but an invalid supplied tenant is rejected. HMAC validation is skipped only when its secret is empty. Issuer, audience, JWKS URL, algorithms, HTTP timeout, and key-cache TTL are configurable and enforced. JWKS status/body limits and negative caching for unknown key IDs bound outbound work.
- Tenant ownership is enforced through explicit scoped repository/service methods and API checks. Cross-tenant reads/mutations return denial/not-found, while all-tenant user and satellite enumeration is an explicit platform-admin operation. `admin` and `super_admin` may select a target tenant explicitly; tenant roles are pinned to their token tenant. Local role assignment has a privilege ceiling: tenant roles cannot create platform admins, `admin` cannot create `super_admin`, and unknown roles are rejected by service and database constraints. Identity realm administration remains a platform-admin-only all-tenant surface; platform mutations with no token tenant require `X-CLM-Tenant-ID` for attribution.
- `control_plane_audit_events` is tenant-scoped and append-only. PostgreSQL triggers reject UPDATE and DELETE. Protected successful mutations buffer the HTTP response until the audit append succeeds. Caller request IDs are syntax/length checked and replaced before mutation when unsafe. On audit failure, the client receives `500` plus `X-Audit-Write-Failed: true` and the failure is logged. **This is response fail-closed, not transaction atomicity:** a domain mutation may already have committed before the audit append fails, and no rollback is claimed. A tenantless platform request rejected before any target is resolved preserves its original 4xx and logs the missing audit scope rather than creating a falsely attributed event; no domain mutation occurred in that case. Audit metadata uses a small top-level allowlist, recursively rejects secret-bearing keys, is size-bounded, and never includes credentials, tokens, authorization headers, or raw request/response bodies.
- `GET /api/v1/audits` supports tenant-scoped filtering for auditor, tenant_admin, and platform-admin roles. Platform admins must select a tenant when their token has none.
- Cloud-account behavior and limitations are documented above. Migration `003_tenant_security.sql` is wired into Make, Compose initialization, and the integration-test runner.

Offline tests cover invalid/missing auth claims, issuer/audience/algorithm enforcement, JWKS caching/fetch failure, cross-tenant access, encryption/redaction/provider binding, audit metadata safety, and visible audit-write failure. PostgreSQL trigger/integration coverage requires `CLM_TEST_DB_DSN`; no live cloud-provider support is claimed by these tests.

---

## API Reference

### Public (no auth)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/login` | Email/password login (JWT) |
| POST | `/api/v1/auth/register` | Self-service signup |
| GET | `/api/v1/auth/sso/providers` | Available SSO methods |
| GET | `/api/v1/auth/{google,microsoft,github}/login` | OAuth redirects |
| GET | `/api/v1/auth/{google,microsoft,github}/callback` | OAuth callbacks |
| GET | `/api/v1/auth/oidc/login` | Generic OIDC redirect |
| GET | `/api/v1/auth/oidc/callback` | Generic OIDC callback |
| GET | `/api/v1/auth/saml/login` | SAML redirect |
| POST | `/api/v1/auth/saml/callback` | SAML assertion consumer |
| GET | `/health` | Liveness check |

### Protected (Bearer JWT)

#### Tenants

| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| POST | `/api/v1/tenants` | super_admin, admin | Create tenant |
| GET | `/api/v1/tenants` | super_admin, admin | List all |
| GET | `/api/v1/tenants/:id` | any | Get details |
| PATCH | `/api/v1/tenants/:id` | super_admin, admin, tenant_admin | Update plan/status/name |
| DELETE | `/api/v1/tenants/:id` | super_admin | Delete (409 if has dependents; `?force=true` cascades) |
| POST | `/api/v1/tenants/:id/sso` | super_admin, admin, tenant_admin | Configure per-tenant SSO |
| GET | `/api/v1/tenants/:id/sso` | super_admin, admin, tenant_admin | Read per-tenant SSO config |
| DELETE | `/api/v1/tenants/:id/sso` | super_admin, admin, tenant_admin | Remove per-tenant SSO config |

> The route table above is kept honest by an audit that diffs it against the routes gin actually registers at boot (`grep GIN-debug` in the server log). At the last check all 63 documented routes existed among the 72 registered.

#### Users

| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| POST | `/api/v1/users` | super_admin, admin, tenant_admin | Create user (role privilege ceiling enforced) |
| GET | `/api/v1/users` | super_admin, admin, tenant_admin, auditor | List users |
| GET | `/api/v1/users/:id` | any | Get user |
| PATCH | `/api/v1/users/:id/role` | super_admin, admin, tenant_admin | Change role (cannot assign above caller) |
| DELETE | `/api/v1/users/:id` | super_admin, admin, tenant_admin | Disable |

#### Gateways

| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| POST | `/api/v1/satellites` | operator+ | Register |
| GET | `/api/v1/satellites` | any | List |
| GET | `/api/v1/satellites/:id` | any | Get details |
| PATCH | `/api/v1/satellites/:id` | super_admin, tenant_admin | Update |
| DELETE | `/api/v1/satellites/:id` | super_admin, tenant_admin | Unregister |

#### Gateway Deployment

| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| POST | `/api/v1/satellites/deploy` | super_admin, admin, tenant_admin | Development-only legacy cloud bootstrap; production returns 503 until secure credential + tenant-bound mTLS injection is implemented |
| GET | `/api/v1/satellites/deploy` | super_admin, admin, tenant_admin | List jobs |
| GET | `/api/v1/satellites/deploy/:job_id` | super_admin, admin, tenant_admin | Job status + steps |
| GET | `/api/v1/satellites/deploy/:job_id/logs` | super_admin, admin, tenant_admin | Job logs |
| POST | `/api/v1/satellites/deploy/:job_id/cancel` | super_admin, admin, tenant_admin | Cancel |
| POST | `/api/v1/satellites/deploy/validate-credentials` | super_admin, admin, tenant_admin | Offline structural check of stored account; returns `unverified` |
| POST | `/api/v1/satellites/deploy/list-infra` | super_admin, admin, tenant_admin | List available infra |
| DELETE | `/api/v1/satellites/:id/teardown` | super_admin, admin, tenant_admin | Destroy |
| POST | `/api/v1/satellites/:id/upgrade` | super_admin, admin, tenant_admin | Upgrade |

#### Cloud Accounts and Audit

| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| POST | `/api/v1/cloud-accounts` | super_admin, admin, tenant_admin | Create metadata and encrypted credentials; supports `Idempotency-Key` |
| GET | `/api/v1/cloud-accounts` | super_admin, admin, tenant_admin | List redacted tenant accounts |
| GET | `/api/v1/cloud-accounts/:id` | super_admin, admin, tenant_admin | Read redacted account metadata |
| PATCH | `/api/v1/cloud-accounts/:id` | super_admin, admin, tenant_admin | Rename without changing credentials |
| POST | `/api/v1/cloud-accounts/:id/rotate` | super_admin, admin, tenant_admin | Replace encrypted credentials |
| POST | `/api/v1/cloud-accounts/:id/validate` | super_admin, admin, tenant_admin | Offline validation; always reports `unverified` |
| DELETE | `/api/v1/cloud-accounts/:id` | super_admin, admin, tenant_admin | Delete tenant-owned account |
| GET | `/api/v1/audits` | super_admin, admin, tenant_admin, auditor | Filter immutable events for one authorized tenant |

#### Gateway Communication (secured Task 3 transport; Code Ready)

These routes are outside human JWT authentication, but they are not public/anonymous. Every machine route requires a per-satellite bearer credential bound to the route satellite and tenant. Production additionally requires a verified client certificate whose URI/DNS SAN binds that identity; malformed or oversized authorization headers are rejected and expensive scrypt verification has a bounded in-process concurrency limit. The legacy unauthenticated path exists only behind `gateway.allow_insecure_legacy: true` in a non-production environment. Command delivery uses durable leases and delivery attempts; duplicate side effects are suppressed by command ID plus execution generation, where automatic lease reclamation preserves the generation and only an authorized operator retry increments it.

The existing cloud deployment adapters cannot yet inject this one-time credential and tenant-bound client identity. Therefore `POST /api/v1/satellites/deploy` fails closed with HTTP 503 in production; pre-provision the satellite identity and mTLS material instead. Development retains the legacy deployment orchestration explicitly for compatibility.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/satellites/:id/heartbeat` | Authenticated health report |
| GET | `/api/v1/satellites/:id/commands` | Atomically claim leased commands; pulling does not complete or clear work |
| POST | `/api/v1/satellites/:id/commands/:cmd_id/ack` | Acknowledge a lease token and enter running state |
| POST | `/api/v1/satellites/:id/commands/:cmd_id/lease` | Renew a valid running lease |
| POST | `/api/v1/satellites/:id/commands/:cmd_id/result` | Idempotently report a lease-bound final/retryable result |

#### Identity Management (Keycloak Admin)

| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/v1/identity/status` | super_admin, admin | Realm stats |
| GET | `/api/v1/identity/users` | super_admin, admin | List realm users with roles |
| POST | `/api/v1/identity/users` | super_admin, admin | Create realm user |
| GET | `/api/v1/identity/users/:id` | super_admin, admin | User detail + sessions |
| PUT | `/api/v1/identity/users/:id` | super_admin, admin | Update profile |
| DELETE | `/api/v1/identity/users/:id` | super_admin, admin | Delete from realm |
| POST | `/api/v1/identity/users/:id/enabled` | super_admin, admin | Enable/disable |
| POST | `/api/v1/identity/users/:id/password` | super_admin, admin | Reset password (permanent only) |
| POST | `/api/v1/identity/users/:id/unlock` | super_admin, admin | Repair an account Keycloak refuses to authenticate |
| POST | `/api/v1/identity/users/:id/role` | super_admin, admin | Set realm role |
| POST | `/api/v1/identity/users/:id/logout` | super_admin, admin | Revoke all sessions |

#### Why created users could not log in — "Account is not fully set up"

Both portals authenticate with the OAuth **Direct Grant** (Resource Owner Password) flow, because the browser-side Keycloak JS adapter is unusable in this deployment (see Decisions). Direct Grant has no interactive step, so Keycloak refuses the grant outright — with the unhelpful `invalid_grant: "Account is not fully set up"` — whenever the account has anything outstanding. Two distinct conditions trigger it, and only one of them is visible in `requiredActions`:

| Condition | Visible in `requiredActions`? | Cause |
|---|---|---|
| `UPDATE_PASSWORD` | yes | user created with a temporary password |
| `VERIFY_PROFILE` | **no** — derived at login | `firstName` or `lastName` missing, both **required** by the realm's declarative user profile |

The second one was the everyday failure: the create-user form accepted blank names, so the account was created successfully (`201`), looked healthy in the users list, and then failed every single login. Measured behaviour:

```
first + last name  -> login 200
first name only    -> login 400  Account is not fully set up
last name only     -> login 400  Account is not fully set up
no names           -> login 400  Account is not fully set up
```

Current handling:

- `first_name` and `last_name` are **required** by `POST /identity/users`, and the rejection message states why.
- `temp_password: true` on create and `temporary: true` on password reset are **refused with `400`** rather than silently producing an account that can never sign in.
- The users table shows a **Login blocked** badge for either condition — including the invisible profile one — with the reason in the tooltip.
- **`POST /identity/users/:id/unlock`** repairs an affected account: clears pending required actions, fills the mandatory profile name (from the optional `first_name`/`last_name` body, otherwise derived from the email local part), marks the email verified and re-enables the account. It returns a `changes` array describing exactly what it altered.
- New users are stamped with a `tenant_id` attribute from the caller's tenant, so their token carries the correct tenant claim instead of falling back to the seed tenant.
| GET | `/api/v1/identity/roles` | super_admin, admin | List realm roles |
| POST | `/api/v1/identity/roles` | super_admin, admin | Create role |
| DELETE | `/api/v1/identity/roles/:name` | super_admin, admin | Delete role |
| GET | `/api/v1/identity/providers` | super_admin, admin | List identity providers |
| POST | `/api/v1/identity/providers` | super_admin, admin | Add IdP (social or OIDC) |
| PUT | `/api/v1/identity/providers/:alias` | super_admin, admin | Toggle enable/disable |
| DELETE | `/api/v1/identity/providers/:alias` | super_admin, admin | Remove IdP |
| GET | `/api/v1/identity/sessions` | super_admin, admin | All active sessions |
| DELETE | `/api/v1/identity/sessions/:sid` | super_admin, admin | Revoke session |
| GET | `/api/v1/identity/clients` | super_admin, admin | List OIDC clients |

#### Identity Management — Groups

| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| GET | `/api/v1/identity/groups` | super_admin, admin | List groups with their mapped role and member count |
| POST | `/api/v1/identity/groups` | super_admin, admin | Create a group, optionally mapping a realm role |
| DELETE | `/api/v1/identity/groups/:gid` | super_admin, admin | Delete a group (members are kept, but lose the inherited role) |
| POST | `/api/v1/identity/groups/:gid/role` | super_admin, admin | Set the group's realm role (empty string clears it) |
| GET | `/api/v1/identity/groups/:gid/members` | super_admin, admin | List group members |
| GET | `/api/v1/identity/users/:id/groups` | super_admin, admin | List a user's groups |
| PUT | `/api/v1/identity/users/:id/groups/:gid` | super_admin, admin | Add a user to a group |
| DELETE | `/api/v1/identity/users/:id/groups/:gid` | super_admin, admin | Remove a user from a group |

---

## API Versioning & Compatibility

All endpoints live under `/api/v1`. Gateways in customer networks upgrade on their own schedule, so the
control plane must tolerate older gateways indefinitely — this is the constraint that shapes the policy.

### What may change within `v1`

| Change | Allowed | Notes |
|--------|---------|-------|
| Adding an endpoint | Yes | |
| Adding an **optional** request field | Yes | Must retain existing behaviour when omitted — the `ha` block on the deploy request is the model |
| Adding a response field | Yes | Clients must ignore unknown fields; the portals do |
| Adding an enum value | Yes, with care | Existing clients must not break on an unrecognised value |
| Relaxing validation | Yes | |
| **Removing or renaming a field** | No | Breaking |
| **Making an optional field required** | No | Breaking |
| **Changing a status code or error shape** | No | Breaking — errors are `{"error": "..."}` and clients branch on it |
| **Changing default behaviour** | No | Breaking even if the signature is unchanged |

### Gateway compatibility

The three gateway-facing endpoints — heartbeat, command pull, and command result — are the most
sensitive surface in the system. A gateway running an older build must continue to register and
report health against a newer control plane.

- Heartbeat and command payloads are **additive only**; new fields (`replica_id`, `replica_count` for HA) must be optional server-side
- Never change the meaning of an existing command `type`; introduce a new type instead
- The control plane records gateway `version`; use it to gate new behaviour rather than assuming the latest

### Introducing `v2`

Only for a genuinely breaking change. Run `v1` and `v2` side by side, keep `v1` serving until fleet
telemetry shows no gateway or portal still using it, and announce removal at least one release ahead.
Because gateways sit in customer networks and cannot be force-upgraded, expect `v1` to outlive
convenience.

### Currently unversioned

`/health` and `/api/v1/health` are treated as stable operational contracts — monitoring depends on
them, so their shape should not change even additively without checking what scrapes them.

---

## Background Services

| Service | Description |
|---------|-------------|
| Gateway Health Monitor | 30s interval scan; marks gateways offline after 2m without heartbeat |

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.22 |
| HTTP Framework | Gin |
| Database | PostgreSQL 16 |
| Cache | Redis 7 |
| Identity Provider | Keycloak 24.0 (realm `clm`, Direct Grant + JWKS) |
| Auth | JWT (golang-jwt/v5), bcrypt, Keycloak RS256 + internal HS256 |
| Cloud SDKs | AWS SDK v2 (Lightsail, EC2, EKS, IAM, STS) |
| Config | Viper (YAML + env vars) |
| Logging | Zap (structured JSON) |
| Frontend | Vanilla HTML + JS SPA (zero build tooling) |
| Reverse Proxy | nginx (SPA + /api + /auth) |
| Container | Docker + Docker Compose |

---

## Directory Structure

```
saas-control-plane/
├── cmd/clm-server/main.go           # Entry point (services, identity client, router, HTTP server)
├── internal/
│   ├── api/rest/
│   │   ├── router.go                # Route definitions + dependency wiring
│   │   ├── handlers/                # auth, tenant, user, satellite, deploy, sso, identity
│   │   └── middleware/              # JWT auth (dual RS256+HS256), RBAC, request logging
│   ├── core/interfaces.go           # Repository interfaces (Tenant, User, Satellite, Cert, Approval)
│   ├── saas/
│   │   ├── identity/               # Keycloak Admin REST API client (users, roles, IdPs, sessions)
│   │   ├── deploy/                  # Lightsail + EC2 provisioners, job executor
│   │   ├── sso/                     # SAML, OIDC, Google, Microsoft federation
│   │   ├── tenant/                  # Tenant lifecycle (CRUD + guarded cascade delete)
│   │   └── users/                   # User management + credential auth
│   ├── services/gateway-health/     # Fleet heartbeat monitor
│   └── store/postgres/              # TenantRepo, UserRepo, SatelliteRepo
├── pkg/
│   ├── config/config.go             # ServerConfig, SatelliteConfig, JWTConfig
│   ├── models/                      # Tenant, User, Certificate, Satellite, Approval, Purchase
│   └── utils/logger.go             # Zap logger factory
├── configs/
│   ├── server.yaml                  # Production config template
│   └── server-test.yaml             # Test profile (Docker hostnames, debug mode)
├── web/admin-portal/
│   ├── index.html                   # SPA shell (login overlay, sidebar nav, content area)
│   └── app.js                       # All pages (dashboard, tenants, users, groups, SSO, gateways, jobs, activity, identity)
└── deploy/
    ├── Dockerfile.server             # Multi-stage Go build (monorepo module-path rewrite)
    ├── docker-compose.test.yml       # Full stack: postgres, redis, keycloak, clm-server, nginx
    ├── docker-compose.yml            # Minimal (no keycloak) for local dev
    ├── nginx-test.conf               # Reverse proxy (/ SPA, /api proxy, /auth keycloak proxy)
    ├── keycloak/clm-realm.json       # Realm import: 3 clients and 8 roles (no users/secrets)
    ├── init-seed.sql                 # Default tenant only; users are provisioned at runtime
    ├── deploy-test.sh                # One-command Lightsail deployment
    └── migrations/
        ├── 001_initial.sql           # Core schema (13 tables)
        ├── 002_sso_fields.sql        # SSO extensions
        ├── 003_tenant_security.sql   # Append-only audit + encrypted cloud accounts
        └── 004_secure_command_transport.sql # Satellite credentials + durable leases
```

---

## Configuration

### Environment Variables (clm-server)

| Variable | Default | Description |
|----------|---------|-------------|
| `CLM_CONFIG` | `configs/server.yaml` | Path to server config |
| `CLM_JWT_SECRET` | — | Internal HMAC signing key; when empty, HMAC token validation is disabled |
| `CLM_JWKS_URL` | — | External JWT signing-key endpoint; HTTPS is required in production |
| `CLM_CLOUD_ACCOUNT_MASTER_KEY` | — | Exactly 32 bytes in production; may be supplied through a `file://` config reference |
| `GIN_MODE` | `debug` | Gin mode (debug/release) |
| `KEYCLOAK_URL` | `http://keycloak:8180/auth` | Keycloak base URL |
| `KEYCLOAK_REALM` | `clm` | Managed realm |
| `KEYCLOAK_ADMIN_REALM` | `master` | Admin auth realm |
| `KEYCLOAK_ADMIN_USER` | `admin` | Keycloak admin username |
| `KEYCLOAK_ADMIN_PASS` | — | Keycloak admin password (enables Identity Management) |

### server.yaml

```yaml
port: 8080
host: "0.0.0.0"
env: "production"
log_level: "info"

database:
  host: "db.internal"
  port: 5432
  user: "clm"
  password: "${CLM_DB_PASSWORD}"
  dbname: "clm_db"
  sslmode: "verify-full"

redis:
  addr: "localhost:6379"
  password: "${CLM_REDIS_PASSWORD}"

jwt:
  secret: "${CLM_JWT_SECRET}"
  expiry_minutes: 60
  refresh_days: 7
  issuer: "clm-server"
  audience: "clm-api"
  jwks_url: "${CLM_JWKS_URL}"
  allowed_algorithms: ["HS256", "RS256"]
  jwks_cache_ttl: "5m"
  jwks_http_timeout: "5s"

cloud_accounts:
  # Prefer a mounted secret: file:///run/secrets/clm-cloud-account-key
  master_key: "${CLM_CLOUD_ACCOUNT_MASTER_KEY}"

tls:
  enabled: true
  cert_path: "/etc/clm/tls/server.crt"
  key_path: "/etc/clm/tls/server.key"
  client_auth: "required"
  client_ca_path: "/etc/clm/tls/gateway-client-ca.crt"

gateway:
  allow_insecure_legacy: false
  lease_duration: "2m"
  max_pull_commands: 10
  max_result_bytes: 262144
  retry_base: "5s"
```

`components/shared/scripts/install-server.sh` requires CA-issued server certificate/key and a gateway client-CA bundle for production, installs them read-only for the service, writes the 32-byte cloud-account key to a separate secret file, copies migrations `001` through `004`, and applies each with `ON_ERROR_STOP=1`. It does not generate a PKI or claim a successful live handshake.

---

## Deployment

### Test Environment (Live)

| Property | Value |
|----------|-------|
| URL | http://35.154.6.28 |
| Instance | `clm-saas-test` (Lightsail xlarge_3_1, 4 vCPU, 16 GB RAM, 320 GB) |
| Region | ap-south-1a |
| Static IP | `clm-saas-test-ip` (35.154.6.28) |
| AWS Profile | `test` (account 348915724759) |
| SSH | `ssh -i ~/.ssh/clm-saas-deploy.pem ubuntu@35.154.6.28` |
| Remote path | `/opt/clm/` |

### Deploy Commands

```bash
# Rebuild server (compiles inside Docker):
cd /opt/clm/deploy
sudo docker compose -f docker-compose.test.yml build clm-server
sudo docker compose -f docker-compose.test.yml up -d clm-server

# Portal update (no rebuild needed, volume-mounted):
scp web/admin-portal/app.js ubuntu@35.154.6.28:/opt/clm/web/admin-portal/app.js
scp web/admin-portal/index.html ubuntu@35.154.6.28:/opt/clm/web/admin-portal/index.html

# Full redeploy:
./deploy-test.sh
```

### Test identities

The realm import intentionally contains no users or passwords. Set deployment
secrets in `deploy/.env`, then provision isolated test users through Keycloak at
runtime; do not document or commit their credentials.

---

## Known Limitations

- ~~Keycloak realm data is not durable.~~ **Resolved** — Keycloak runs in production mode against a dedicated `keycloak` PostgreSQL database; realm data survives container recreation.
- **Identity ownership remains split.** The PostgreSQL `users` table and Keycloak realm are still unsynchronised. Task 2 removes token tenant fallback and stamps new Keycloak users with an explicit target tenant, but it does not make the two stores transactional.
- **Identity administration is platform-wide.** `/api/v1/identity` remains restricted to platform-admin roles; platform mutations without a token tenant require `X-CLM-Tenant-ID` for audit attribution. Fine-grained per-tenant Keycloak realms/groups are not implemented.
- **Cloud-account backend is Code Ready, not live-provider verified.** Credentials are encrypted server-side and redacted, but Task 2 validation is explicitly offline (`unverified`), deploy jobs are process-local, and the unchanged portal may still use its legacy `localStorage` path.
- **TLS/mTLS transport is Code Ready, not live-verified.** Production configuration fails closed unless HTTPS, gateway client-CA verification, and secure leased transport limits are configured. The checked-in Compose profile remains isolated test mode over HTTP, and no live CA chain or handshake was exercised for Task 3.
- **Runtime secrets are externally supplied.** Compose and YAML files contain only environment or secret-file references; operators must populate the ignored `deploy/.env` or runtime secret files before startup.
- **Tests exist but coverage is thin.** Repository integration tests and focused config tests exist; broader handler, router, and service coverage is still needed.
- **This tree is not under version control** — `git rev-parse` reports no repository. Code changes are therefore unrecoverable without manual backups, and dead code cannot be safely deleted. Putting it in git is a prerequisite for most cleanup work.
- **Control-plane audit is Code Ready, not transaction-atomic.** Protected JWT mutations append immutable tenant-scoped events and make write failures visible, but a domain write can commit before a failed audit append. Gateway communication routes and public authentication events are outside this middleware, and external tamper-evident export remains future work.
- **Nullable-column scan risk.** Only `tenant_repo.go` and `satellite_repo.go` guard nullable columns (`COALESCE`/`sql.Null*`). `user_repo.go`, `cert_repo.go`, `approval_repo.go`, and `purchase_repo.go` do not — the same class of bug that made tenant fetch return 404 may still be latent there.
- **Single instance, no HA, no scheduled backups.** PostgreSQL runs in a container on one Lightsail host.
- **Hardcoded host.** The Keycloak hostname `http://35.154.6.28/auth` is baked into the realm export and portal config.

---

## Recommended Improvements

Ordered by risk. Items 1–4 are prerequisites for any non-demo use.

### Critical — data durability and access

#### 1. Make Keycloak durable — **DONE**

Keycloak now runs in production mode against PostgreSQL instead of `start-dev` on embedded H2, so realm data is no longer discarded when the container is recreated.

**What was done:**

- `deploy/migrations/000_keycloak_db.sql` creates a `keycloak` database owned by `clm`. It is numbered `000` so it runs before the CLM schema. Note that `/docker-entrypoint-initdb.d` scripts only execute on an **empty** data directory, so existing deployments need the database created by hand — the file documents the one-liner.
- `deploy/docker-compose.test.yml` switches Keycloak to `command: start --import-realm` with `KC_DB=postgres` pointing at that database, and adds `depends_on: postgres (service_healthy)`.
- Credentials moved to `${POSTGRES_PASSWORD}`, `${KEYCLOAK_ADMIN_USER}`, `${KEYCLOAK_ADMIN_PASS}`, and `${PUBLIC_URL}` with test-value fallbacks, sourced from `deploy/.env` (template in `deploy/.env.example`, and `.env` is git-ignored).

**Production mode is materially stricter than `start-dev`** — three settings are required or it refuses to serve:

| Setting | Why |
|---------|-----|
| `KC_HTTP_ENABLED: "true"` | `start` refuses plain HTTP without it, and nginx terminates the connection here |
| `KC_PROXY_HEADERS: xforwarded` | Makes Keycloak trust nginx's `X-Forwarded-*` when building redirect URLs |
| `KC_HOSTNAME_URL` + `KC_HOSTNAME_STRICT: "false"` | An explicit hostname is mandatory; strict-off is the safety net |

**Verified:** `Profile prod activated`, `jdbc-postgresql` among installed features, Liquibase created **92 tables** in the `keycloak` database, and realm `clm` imported. Then a user was created through Identity Management, the container was **destroyed and recreated**, and both the user *and its password* survived — login returned `200` before and after.

**Migration side effect worth knowing:** the H2 realm data is discarded on cutover, so the realm re-imports from `clm-realm.json`. Anything created through the portal beforehand (groups, identity providers, users beyond the three seeded ones) is lost. Export the realm first if that is not acceptable.

#### 2. Reconcile the two user stores

Two independent stores are written by separate endpoints and drift. RBAC comes from the Keycloak token while tenant scoping comes from PostgreSQL, so a user can exist for authentication but not authorisation.

**Target split:**

| Concern | Owner |
|---------|-------|
| Credentials, MFA, sessions, realm roles, groups, IdP links | Keycloak |
| Tenant membership, profile, plan attribution, audit linkage | PostgreSQL `users` (keyed by Keycloak `sub`) |

**Change** — add `keycloak_id` (unique) to the `users` table and make user creation a single transaction: create in Keycloak, then insert the local row with the returned ID; roll back the Keycloak user if the local insert fails. Set `tenant_id` as a Keycloak user attribute so it lands in the token and the middleware stops falling back to the hardcoded default tenant. Deletion should be symmetric.

**Acceptance** — a user created through either path is visible from both; the token carries the correct `tenant_id`; deleting removes both records.

#### 3. Cloud Accounts encrypted backend — **CODE READY / OFFLINE TESTED**

Task 2 adds tenant-scoped PostgreSQL persistence and REST CRUD for AWS, GCP, and Azure accounts. Credential payloads are write-only, AES-256-GCM encrypted with a production-required 32-byte key, versioned, authenticated to tenant/account IDs, and never returned by read APIs. Create supports tenant-scoped idempotency keys and names are unique per tenant. Deploy and credential-validation requests use `cloud_account_id` and enforce tenant/provider ownership.

**Remaining work:** the unchanged portal may still use its legacy browser storage; deployment jobs are not durable; production key rotation/KMS envelope encryption is not implemented; no live provider validation was performed. Offline validation intentionally returns `unverified` and must not be presented as live-provider support.

**Evidence:** focused unit tests cover encryption authentication, response redaction, cross-tenant denial, idempotent replay/conflict, provider binding, unsupported encryption versions, and offline validation.

#### 4. Terminate TLS properly

A certificate lifecycle product served over plain HTTP undermines its own premise, and the gateway's deploy endpoint accepts `cert_pem`/`key_pem` in a request body.

**Change** — obtain a CA-issued certificate via **DNS-01** (not HTTP-01, which needs inbound reachability), or use an internal CA the corporate proxy already trusts. Self-signed has already been rejected by the proxy (`SSL_SELF_SIGNED`), so do not retry that path.

**Watch out** — a permanent `301` HTTP→HTTPS redirect was previously cached by browsers and broke both portals. Serve both schemes during rollout and use `302` until HTTPS is confirmed working end-to-end, including the gateway's `/saas-api/` and `/saas-auth/` proxies which point at this host.

**Acceptance** — both portals load over HTTPS; the gateway heartbeat and its two SaaS proxies continue to work; no mixed-content warnings.

### Security

#### 5. Fix tenant-scoped user creation

`POST /api/v1/users` always assigns the caller's `tenant_id` from the JWT, so a user cannot be created in another tenant. This blocks onboarding a customer organization — and it silently invalidated a test during tenant-delete verification, because a user intended for a new tenant landed in the caller's instead.

**Change** — accept an optional `tenant_id` in the request body; permit it only for `super_admin` and `admin`, and validate the tenant exists. Everyone else continues to get their own tenant. Depends on item 2 for the Keycloak side.

**Acceptance** — a `super_admin` can create a user in another tenant; a `tenant_admin` supplying a foreign `tenant_id` receives `403`.

#### 6. Rotate and externalise secrets

`configs/server-test.yaml` contains the JWT signing secret and DB password; Keycloak admin is `admin`/`admin`, and the portal's identity API is a direct path to full realm administration.

**Change** — read all secrets from the environment with no in-repo defaults, supply them via a `.env` file that is git-ignored (or a secret manager), rotate every current value, and purge them from git history (`git filter-repo`). Note that rotating the JWT secret invalidates existing internal HS256 tokens — Keycloak RS256 sessions are unaffected.

**Acceptance** — no secret literal appears anywhere in the repository; the stack starts from environment variables alone.

#### 7. Rate-limit authentication

Redis is deployed and listed as "cache/rate" but no limiter is wired up, leaving the Direct Grant flow open to password spraying.

**Change** — add per-IP and per-account limits on `/api/v1/auth/login` and the `/auth/realms/*/protocol/openid-connect/token` proxy, with exponential backoff and a temporary account lock. Keycloak also has native brute-force detection (`bruteForceProtected` in the realm), which is the cheaper first step.

**Acceptance** — repeated failures from one IP are throttled; a legitimate login still succeeds afterwards.

#### 8. Control-plane mutation audit — **CODE READY / OFFLINE TESTED**

Task 2 adds immutable tenant-scoped audit events for protected JWT mutations plus filtered reads for auditor, tenant_admin, and platform-admin roles. Database triggers reject update/delete. Events include request ID, actor/role, action, resource, outcome, source IP, timestamp, and size-bounded safe metadata; credentials, tokens, authorization values, and raw bodies are rejected.

Audit append failure replaces a buffered successful response with `500` and `X-Audit-Write-Failed: true`, but the domain mutation may already have committed. This is intentionally documented response fail-closed behavior, **not transaction atomicity**. Public authentication events, gateway communication routes, before/after summaries, external tamper-evident retention/export, and transactional outbox semantics remain future work.

### Correctness

#### 9. Audit all repositories for nullable-column scans — **DONE (live repos)**

Audited every scanned column against `information_schema.columns`. The schema has **39 nullable columns** across the CLM tables.

**A second live instance of the bug was found and fixed on the login path.** `users.password_hash` is nullable — an SSO-provisioned user has no local password — but `models.User.PasswordHash` is a plain `string`. Reading such a user failed with `converting NULL to string is unsupported`, and because `Login()` reported *every* repository error as `ErrInvalidCredentials`, the real cause was completely invisible. A passwordless user would simply have been unable to log in with no diagnosable reason.

**Changes:**

| File | Change |
|------|--------|
| `user_repo.go` | Added shared `userColumns` with `COALESCE(password_hash, '')`; `GetByID`/`GetByEmail` now share `scanUser`; returns `nil` on error instead of a half-populated struct; introduced `ErrUserNotFound` wrapping `core.ErrNotFound` |
| `satellite_repo.go` | Returns `nil` on error; `ErrSatelliteNotFound` wraps `core.ErrNotFound` (nullable columns were already `COALESCE`d) |
| `users/service.go` | `Login()` distinguishes a genuine miss from a database failure — only `core.ErrNotFound` becomes `ErrInvalidCredentials`, everything else is logged and surfaced; an empty hash is rejected explicitly. `CreateUser()` no longer swallows lookup errors, which previously let a database failure look like an available email address |

**Not changed:** `cert_repo.go`, `approval_repo.go`, and `purchase_repo.go` carry the same pattern but are **dead code** — nothing constructs them, and per decision D6 certificates, approvals, and purchasing belong to the gateway. They were left in place rather than deleted because **this tree is not under version control**, so deletion would be unrecoverable. Removing them is safe once the repo is in git.

**Acceptance met:** `TestUserRepo_GetByEmail_NullPasswordHash` and `TestTenantRepo_GetByID_NullSSOProvider` both insert `NULL` into the offending column and assert the read succeeds. Verified live: internal login works for all three users, an unknown email returns `401` (not `500`), and single-row tenant and user reads return `200`.

#### 10. Stop flattening repository errors

`core.ErrNotFound` now lets services distinguish a genuine miss from a database failure. Apply the same pattern in the user and satellite services — collapsing every error into "not found" is precisely what disguised the tenant bug as a routing problem and cost real debugging time.

**Acceptance** — a forced database error surfaces as `500` with the cause logged, not as `404`.

#### 11. Add soft delete for tenants

The guarded cascade is correct but irreversible, and two tenants plus two user accounts were lost in a handful of clicks during testing.

**Change** — add `deleted_at timestamptz` to `tenants`, exclude soft-deleted rows from `List`/`GetByID` by default, retain for a configurable window (30 days), and add a restore endpoint. Keep hard delete available to `super_admin` with `?purge=true`. Pair with item 8 so the deletion is recorded regardless.

**Acceptance** — a deleted tenant disappears from the portal but can be restored within the window, with its users intact.

### Engineering practice

#### 12. Add tests — **STARTED** (8 integration tests, runner in place)

The repository had **zero** `*_test.go` files. There are now 8 passing integration tests in `internal/store/postgres/repo_test.go`, covering exactly the bug classes that have actually occurred:

| Test | Guards against |
|------|----------------|
| `TestUserRepo_GetByEmail_NullPasswordHash` | The login-path NULL scan (item 9) |
| `TestUserRepo_GetByEmail_NotFound` | `core.ErrNotFound` semantics + no half-populated struct |
| `TestTenantRepo_GetByID_NullSSOProvider` | The original tenant NULL bug, and asserts `GetByID` and `List` agree |
| `TestTenantRepo_GetByID_NotFound` | Same for tenants |
| `TestTenantRepo_CountDependents_And_Delete` | The delete guard: counts, refusal while dependents exist, cascade leaves no orphans |
| `TestTenantRepo_DeleteMissing` | Deleting an absent tenant reports not-found |
| `TestSatelliteRepo_GetByID_NotFound` | Same for satellites |
| `TestTenantRepo_UpdateRoundTrip` | Plan/limit writes round-trip |

**These are deliberately integration tests, not unit tests with mocks.** Every bug in this list is a NULL-handling or driver-level bug; a mock returns tidy zero values and would have hidden all of them.

**Running them** — `deploy/run-tests.sh` provisions a throwaway `clm_test` database, applies the migrations, runs `go vet` and `go test` inside a `golang:1.22-alpine` container joined to the compose network, and drops the database afterwards. It never touches `clm_db`. This also gives the project a compile check, which matters because there is no local Go toolchain.

**Guard against a false green:** `TestMain` exits 0 when `CLM_TEST_DB_DSN` is unset so `go test ./...` stays usable without a database — but that would otherwise print `ok` having run nothing. It now prints a loud warning, and setting `CLM_TEST_DB_REQUIRED=1` (which the runner does) turns a missing database into a hard failure. Verified both ways: 8 passes with a DSN, 0 tests run without one.

**Still to add:** router/handler tests (the missing `DELETE` route would have been caught by a table-driven route assertion) and service-level tests with a fake repository.

#### 13. Add CI and a reproducible deploy

No `.github/workflows` exists, and manual `tar` + `scp` + `docker build` deployment has already produced **three** repo-versus-server drift incidents where the running system carried fixes the repository lacked:

| Drift incident | Symptom |
|----------------|---------|
| `Dockerfile.server` missing the monorepo module-path `sed` | Build failed outright after a fresh checkout was deployed |
| `nginx-test.conf` had a trailing slash on `proxy_pass` to Keycloak | Every realm URL returned 404 — the `/auth` prefix was being stripped |
| Gateway `internal/` restructure done by hand | Import paths unresolvable from a plain tar of the repo |

**Change** — build, vet, and test on push; publish a versioned image; deploy that artifact rather than copying source to the host and building there. The gateway now has `deploy/build-src-bundle.sh`; the control plane needs the equivalent so its deploy is reproducible from a clean clone.

**Acceptance** — a fresh clone deploys successfully with no manual edits on the server.

#### 14. Automate backups and rehearse a restore

The only backups that exist were taken by hand during a VM replacement. Schedule `pg_dump` (both `clm_db` and, after item 1, the `keycloak` database) to S3 with lifecycle retention, and verify a restore into a scratch instance. An untested backup is not a backup.

**Acceptance** — a documented restore produces a working stack with the expected row counts.

#### 15. Monitor the control plane itself

The gateway health monitor watches gateways; nothing watches the SaaS. The host was previously lost to an out-of-memory condition — 4 GB RAM with no swap, killed by a Go build running alongside Keycloak, PostgreSQL, Redis, and nginx — with no alert and no automatic recovery. It is now 16 GB with 4 GB of swap, but the gap in observability remains.

**Change** — external uptime checks on `/health` for both hosts, container restart and OOM alerting, disk and memory thresholds, and an alert when any gateway goes offline. Confirm the gateway host also has swap configured; only the control plane host was remediated.

**Acceptance** — killing the server container raises an alert within a minute.

### Product gaps

| Item | Notes |
|------|-------|
| Subscription & billing | Entire feature is unbuilt; `planLimits()` is the only entitlement enforcement today |
| AWS / GCP / Azure Marketplace | Fulfillment and metering integrations not started |
| GCP / Azure provisioning | Deploy Engine currently AWS-only in practice |
| ACME / CA issuance | Gateway records certificate requests as pending; nothing issues them |
| MFA | TOTP not implemented |
| vSatellite (Component 3) | Scaffolding only |

### Suggested sequence

| Order | Work | Rationale |
|-------|------|-----------|
| 1 | Keycloak persistence (#1) | Stops ongoing identity data loss |
| 2 | Reconcile user stores (#2, #5) | Everything downstream depends on who owns identity |
| 3 | Cloud Accounts backend + encryption (#3) | Gates any real credential use |
| 4 | TLS + secret rotation (#4, #6) | Gates any non-demo use |
| 5 | Tests, then CI (#12, #13) | Makes all later work cheaper and stops drift |
| 6 | Nullable-scan audit + audit log (#8, #9) | Removes a known latent bug class |
| 7 | Backups + monitoring (#14, #15) | Inexpensive insurance |
| 8 | Billing, marketplace, ACME, GCP/Azure | Feature work, once the platform is sound |

---

## Operational Runbook

Failure modes that have already occurred here, with the diagnosis that identified them. Check this list before deep debugging — several of these present misleadingly.

### A newly deployed page or tab "is not available" — **fixed (browser caching)**

**The most likely cause of any "feature X is missing" report.** `index.html` and `app.js` were served with **no `Cache-Control` header at all**, only `ETag`/`Last-Modified`. Browsers were therefore free to reuse a cached `app.js` indefinitely. Because the portal is a single unversioned script, one stale file hides *every* UI change deployed since it was cached.

**How to recognise it:** the server log shows the browser calling the endpoints of the *old* page while never calling the new one. The Groups tab was diagnosed this way — a browser hit `/identity/status`, `/identity/roles` and `/identity/users` (exactly the old Users tab's calls) but never `/identity/groups`, proving the loaded script predated the Groups tab. The API itself returned `200` throughout.

```bash
# Which endpoints has a real browser actually called?
docker compose logs clm-server | grep Mozilla | grep '"path":"/api/v1/identity'
```

**Fix:** dedicated `location = /index.html` and `location = /app.js` blocks in `nginx-test.conf` (and the gateway's `nginx.conf`) send `Cache-Control: no-cache, no-store, must-revalidate`. Both files are small and volume-mounted, so revalidating per load costs nothing.

**Confirm a fix reached the browser** rather than assuming a deploy is visible:

```bash
curl -sI http://35.154.6.28/app.js | grep -i cache-control   # expect no-store
curl -s  http://35.154.6.28/app.js | grep -c renderIdentityGroups
```

If a stale page is still suspected, a hard reload (Cmd/Ctrl+Shift+R) bypasses whatever the browser already holds.

### Organization tab blank; tokens missing email / tenant_id — **fixed**

The Organization tab rendered "Organization details not available", the topbar showed no email, and — more seriously — **every Keycloak user was silently assigned the seed tenant**.

**Cause: the realm import replaced Keycloak's built-in client scopes.** `clm-realm.json` supplies a custom `clientScopes` array containing only `roles`. Supplying that array *replaces* the built-ins, so `email`, `profile`, `web-origins` and friends were never created in the realm. The clients then listed `defaultClientScopes: ["openid","profile","email","roles"]`, three of which did not exist, and Keycloak skipped them with a warning that is easy to miss in a noisy startup log:

```
WARN [org.keycloak.models.utils.RepresentationToModel] Referenced client scope 'profile' doesn't exist. Ignoring
```

Access tokens therefore carried `roles` and nothing else — no `email`, no `preferred_username`, no `tenant_id`.

**Why this mattered beyond the blank page:** `middleware/auth.go` falls back to the seed tenant `a0000000-0000-0000-0000-000000000001` when `tenant_id` is absent from the token. With no mapper emitting that claim, *every* portal user resolved to the seed tenant regardless of who they were. Invisible with one tenant; a tenant-isolation defect the moment a second exists.

**Fix:** protocol mappers were added **directly to the two portal clients** (`email`, `preferred_username`, `full name`, `tenant_id`) rather than recreating the standard scopes. Client-level mappers apply regardless of scope assignment, so the token no longer depends on the scope configuration at all. The seed users gained a `tenant_id` attribute, and the realm now sets `unmanagedAttributePolicy: ENABLED` because Keycloak 24's declarative user profile otherwise rejects custom attributes.

**Two traps encountered while fixing it, both worth remembering:**

1. `PUT /admin/realms/{realm}/users/{id}` **replaces** the user representation. Sending only `{"attributes": {...}}` drops `email` and fails with `error-user-attribute-required`. Read the user, merge, write the whole record — the same partial-update trap that affected tenant `PATCH`.
2. Under production mode the **master** realm has `sslRequired=external`, so the admin API is unreachable over plain HTTP from outside. The `clm` realm sets `sslRequired=none`, which is why portal logins still work. Run admin API calls from the host against `http://localhost:8180` — localhost counts as a local request. Container-to-container calls from private IPs are unaffected, which is why Identity Management kept working.

**Also hardened:** the Organization tab now falls back to `/api/v1/tenants/me` when the claim is absent, so the page works regardless of realm mapper configuration instead of dead-ending.

### 502 from nginx after recreating a container — **now prevented**

Recreating `keycloak` or `clm-server` used to make nginx return `502` for that upstream until nginx was itself restarted.

**Cause:** nginx resolves an upstream hostname **once at startup** and caches the IP indefinitely. A recreated container gets a new IP on the Docker network, so nginx kept proxying to an address nothing was listening on.

**Distinguishing signal:** the service answers directly on the host (`curl localhost:8180/auth/realms/clm` → 200) while the same path through nginx returns 502.

**Fix applied:** `nginx-test.conf` now declares Docker's embedded DNS and routes each upstream through a variable, which forces re-resolution:

```nginx
resolver 127.0.0.11 valid=10s ipv6=off;
...
set $kc_upstream keycloak;
proxy_pass http://$kc_upstream:8180;   # no URI part, so /auth/... passes through unchanged
```

Keeping no URI component after the port matters — see the 404 case below.

**If it recurs:** `docker compose up -d --force-recreate nginx`.

### Every Keycloak realm URL returns 404

`/auth/realms/clm` returns 404 while Keycloak itself is healthy.

**Cause:** a trailing slash on `proxy_pass` in the nginx `/auth/` block. Keycloak runs with `KC_HTTP_RELATIVE_PATH=/auth` and expects to receive the prefix; a trailing slash strips it.

```nginx
proxy_pass http://keycloak:8180/;   # wrong — strips /auth
proxy_pass http://keycloak:8180;    # correct — preserves it
```

**Confirm:** `curl localhost:8180/auth/realms/clm` returns 200 from the host while `curl localhost/auth/realms/clm` returns 404.

### Go build fails immediately after deploying a clean checkout

**Cause:** `Dockerfile.server` is missing the monorepo module-path rewrite. Imports use `github.com/m2p/clm/internal/...` while `go.mod` declares `github.com/m2p/clm/saas-control-plane`. The `sed` line before `go mod tidy` is required, not optional.

### Identity Management shows "Keycloak Admin API not reachable" right after a restart

**Cause:** Keycloak's JVM takes roughly 25 seconds to import the realm and start serving. `clm-server` probes once at boot and logs a warning, then retries per request.

**Action:** wait and refresh. If it persists, check `KEYCLOAK_ADMIN_PASS` is set — the identity routes are not registered at all when it is empty.

### Host stops responding: SSH closes immediately, HTTP times out

**Cause:** out of memory. Observed with 4 GB RAM and no swap when a Go build ran alongside Keycloak, PostgreSQL, Redis, and nginx. The OOM killer took sshd with it.

**Distinguishing signal:** CPU utilisation is *low* (~14%) and burst capacity is healthy while nothing responds. Disk was only 23% used, which rules out the other common cause.

**Fix applied:** control plane host moved to 16 GB with a 4 GB swapfile persisted in `/etc/fstab`. Verify the gateway host has swap too.

**Recovery:** reboot via the Lightsail API; the instance keeps its static IP and disk.

### nginx returns 500 for every static file after a deploy

**Cause:** the deploy deleted a directory that is bind-mounted into the nginx container. Removing `web/` breaks the mount and nginx can no longer resolve any file.

**Fix:** extract in place rather than `rm -rf` the mounted path, or `docker compose up -d --force-recreate nginx` afterwards.

**Note:** this also produces false passes in verification scripts — a `grep` against a 500 error page finds no match and looks like success. Always assert the response body is the real artifact, not just that a string is absent.

### A single-row fetch 404s but the list endpoint works

**Cause:** a nullable column scanned into a non-nullable Go type. See Recommended Improvements #9. The list query typically omits the offending column, which is why only the single-row path fails.

**Confirm:**
```sql
SELECT column_name, is_nullable FROM information_schema.columns
WHERE table_name = '<table>';
```

### Verifying a fix on the live deployment

Run against the deployed stack, not just the build:

```bash
# 1. Obtain a token using runtime-provisioned test credentials.
curl -s -X POST "${PUBLIC_URL}/auth/realms/clm/protocol/openid-connect/token" \
  -d "grant_type=password&client_id=clm-admin-portal&username=${CLM_TEST_ADMIN_EMAIL}&password=${CLM_TEST_ADMIN_PASSWORD}"

# 2. Confirm the route is actually registered (gin logs every route at boot)
sudo docker compose -f docker-compose.test.yml logs clm-server | grep GIN-debug | grep <path>

# 3. Distinguish a missing route from a handler error
#    404 with no user_id in the request log  -> route not registered (gin NoRoute)
#    404 with a user_id                      -> handler returned not-found
```

A 404 carrying `user_id` in the structured log means authentication ran, so the route exists and the handler decided. Without it, the route was never registered. That distinction saves considerable time.
