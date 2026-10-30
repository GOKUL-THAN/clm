# CLM Components

CLM is a 3-component architecture. Each component is an independent Go module with its own `go.mod`, config, and deploy artifacts.

```
components/
├── saas-control-plane/     # Component 1: Central SaaS + Admin Portal
├── satellite-gateway/      # Component 2: Per-client gateway (crypto, discovery, deploy, approve)
├── vsatellite/             # Component 3: Lightweight endpoint agent (scan, deploy) — scaffolding only
└── shared/                 # Legacy scripts + original combined dashboard (deprecated)
```

## Architecture Overview

```
┌────────────────────────────────────────────────────────────────┐
│  Component 1: SaaS Control Plane + Admin Portal                │
│  (AI Forgers Cloud — single instance, multi-tenant)            │
│                                                                │
│  Organization & Tenants, Subscription & Billing, Identity      │
│  (Keycloak), SSO, Gateway Fleet, Remote Deployment             │
└──────────────────────────────┬─────────────────────────────────┘
                               │ outbound HTTP, gateway-initiated
                               │ (heartbeat 30s + command pull)
┌──────────────────────────────▼─────────────────────────────────┐
│  Component 2: Satellite Gateway (per client location)          │
│  (Customer VPC/DC — one per location)                          │
│                                                                │
│  Gateway Portal, Crypto, Secrets, Discovery, Cert Deployment,  │
│  Approvals, Monitoring, vSatellite Coordination                │
└──────────────────────────────┬─────────────────────────────────┘
                               │ manages
         ┌─────────────────────┼─────────────────────┐
    ┌────▼────────┐     ┌──────▼──────┐     ┌───────▼───────┐
    │ Component 3 │     │ Component 3 │     │  Component 3  │
    │ vSatellite  │     │ vSatellite  │     │  vSatellite   │
    │ (VPC-A)     │     │ (VPC-B)     │     │  (On-Prem DC) │
    └─────────────┘     └─────────────┘     └───────────────┘
```

**Connection direction matters:** the gateway always dials the control plane. The control plane never opens a connection to a gateway — it queues commands that the gateway pulls on its next heartbeat. This is what allows a gateway to sit inside a customer VPC with no inbound firewall rules.

## Component Boundaries

Who owns what. Getting this wrong is the most common source of misplaced code.

| Concern | Owner | Notes |
|---------|-------|-------|
| Tenants, organizations, billing | Control Plane | Multi-tenant metadata |
| Users, groups, roles, IdPs | Control Plane (via Keycloak) | Keycloak is the identity store |
| Gateway registry + health | Control Plane | Heartbeat tracking, command queue |
| Gateway provisioning | Control Plane | Cloud APIs + on-prem SSH |
| **Certificates** | **Gateway** | Private keys never leave the gateway |
| **Certificate discovery** | **Gateway** | TLS scanning of customer networks |
| **Certificate deployment** | **Gateway** | Pluggable deployers per target type |
| **Approvals** | **Gateway** | Sign-off before acting on a change |
| **CA purchasing** | **Gateway** | CA account credentials stay local |
| **Crypto / secrets** | **Gateway** | Key generation, AWS Secrets Manager |

The control plane deliberately has **no** certificate endpoints — `/api/v1/certificates` returns 404 there by design. Certificate operations are served by the gateway's own API on port 8080.

## Module Layout

Each component builds as a standalone module. Note the two build-time transformations, which exist because the components share `internal/core` and `pkg/` but ship separately:

| Component | Module path (repo) | Module path (build) | Shared code pulled in |
|-----------|-------------------|---------------------|----------------------|
| Control Plane | `github.com/m2p/clm/saas-control-plane` | `github.com/m2p/clm` | owns `internal/core`, `pkg/` |
| Satellite Gateway | `github.com/m2p/clm/satellite-gateway` | `github.com/m2p/clm` | copies `internal/core` + `pkg/` from control plane |
| vSatellite | `github.com/m2p/clm/vsatellite` | — | not yet built |

**Both Dockerfiles rewrite the module path** with `sed` before `go mod tidy`, because imports across the tree use `github.com/m2p/clm/internal/...`. Removing that `sed` line breaks the build — this has happened twice.

**The gateway additionally re-roots `internal/`** into `internal/satellite/` at bundle time (`runtime/*.go` moves up to `internal/satellite/`, `vsatellite-coordinator` becomes `vsatellite`). This is scripted in `satellite-gateway/deploy/build-src-bundle.sh` — always build the bundle with that script rather than tarring the directory by hand, or the import paths will not resolve.

## Deploy Artifacts

| Component | Source | Deploy config | Remote path |
|-----------|--------|---------------|-------------|
| Control Plane | `saas-control-plane/{cmd,internal,pkg}` | `saas-control-plane/deploy/` | `/opt/clm/` |
| Satellite Gateway | `satellite-gateway/{cmd,internal}` + shared | `satellite-gateway/deploy/` | `/opt/clm-gateway/` |
| vSatellite | `vsatellite/cmd/` | — | — |

## Per-Component Documentation

- [`saas-control-plane/README.md`](saas-control-plane/README.md) — features, API reference, configuration, deployment, recommended improvements
- [`satellite-gateway/README.md`](satellite-gateway/README.md) — gateway capabilities and portal API
- [`vsatellite/README.md`](vsatellite/README.md) — endpoint agent (planned)
