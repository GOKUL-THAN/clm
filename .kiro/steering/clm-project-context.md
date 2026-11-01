# CLM Project Context

## Project Identity
**Name:** Enterprise Contract & Certificate Lifecycle Management (CLM) Platform  
**Architecture:** Zero-Trust Hybrid SaaS Control Plane + Customer In-VPC Satellite Gateway  
**Model:** Multi-tenant, Customer-Controlled Encryption, Zero-Egress AI

## What This System Does
Unified enterprise orchestration across:
- **Contract Lifecycle** — 26 stages from intake to termination, AI-assisted intelligence
- **Machine Identity / TLS Certificate Lifecycle** — PKI governance, X.509 management, renewal automation

## Component Map
| Path | Role |
|------|------|
| `components/saas-control-plane/` | Central SaaS orchestration, multi-tenant control, API gateway |
| `components/satellite-gateway/` | Customer in-VPC gateway, zero-egress AI processing |
| `components/vsatellite/` | Virtual satellite variant for cloud-native deployments |
| `components/shared/` | Shared libraries, types, utilities across components |
| `deploy/` | Infrastructure-as-code, Helm charts, deployment manifests |
| `scripts/` | Operational and CI/CD automation scripts |
| `compliance/` | Compliance evidence, audit artifacts |
| `docs/` | Architecture docs, API specs, runbooks |

## Key Architectural Constraints
- **Multi-tenant isolation** is non-negotiable — every change must respect tenant boundaries
- **Zero-egress AI** — AI processing happens inside the customer's VPC via satellite gateway
- **Cryptographic integrity** — contracts use PKI digital signatures with X.509 timestamping
- **Compliance frameworks** — GDPR, PCI-DSS, SOC 2, HIPAA requirements apply throughout
- **Immutable audit trail** — all state transitions must be audit-logged

## 26 Contract Lifecycle Stages
1. Request Intake & Triage → 2. Contract Intake → 3. Contract Creation → 4. Drafting →
5. Authoring → 6. Template Management → 7. Clause Library → 8. Negotiation →
9. Collaboration → 10. Review → 11. Approval Workflow → 12. Electronic Signature →
13. Digital Signature → 14. Execution → 15. Repository → 16. Monitoring →
17. Obligation Management → 18. Compliance Management → 19. Risk Management →
20. Amendment → 21. Renewal → 22. Expiration → 23. Termination →
24. Analytics → 25. Reporting → 26. AI Contract Intelligence

## Development Standards
- All API changes must be backward-compatible unless a breaking change is explicitly approved
- Security-sensitive code (auth, crypto, tenant isolation) requires a design review step before implementation
- Database schema changes require a migration script
- New features touching the approval workflow or e-signature path need compliance documentation
- Run `make test` to verify; `make lint` for style checks

## Workflow Mode Reference
Four structured modes are available for development work on this project:

| Mode | Trigger phrase | Purpose |
|------|---------------|---------|
| **Feature** | `feature:` or `/feature` | Full structured feature development with design + implement + review |
| **Plan** | `plan:` or `/plan` | Plan-only — break down into tasks, no code changes |
| **Bugfix** | `bug:` or `/bug` | Investigate → diagnose → fix workflow |
| **Spec** | `spec:` or `/spec` | SpecFast — clarify requirements then auto-generate requirements doc, design, and tasks |
