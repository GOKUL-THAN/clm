# PHASE 6 — STAGING DEPLOYMENT READINESS & INFRASTRUCTURE RUNBOOK
**Platform:** Enterprise CLM SaaS Platform  
**Document Reference:** CLM-OPS-PHASE6-STAGING-READINESS  
**Assessment Date:** 2026-10-31  
**Deployment Target:** Kubernetes (EKS / GKE) & Amazon Aurora PostgreSQL 16 (Multi-AZ)  
**Cloud Resource Status:** **`ZERO BILLABLE CLOUD RESOURCES PROVISIONED (APPROVAL GATE ENFORCED)`**  
**Infrastructure Manifest Status:** **`14/14 COMPONENTS CONFIGURED & VERIFIED`**

---

## 1. Executive Summary & Zero Cloud Spend Policy

In strict accordance with Phase 6 of the **Zero False Positive Directive**:
> *"Never provision cloud resources, create real cloud bills, or initiate live cloud infrastructure deployments without explicit user confirmation... Prepare the deployment plan, manifest configurations, and verification gates without executing live cloud changes."*

All 14 infrastructure components have been fully codified into declarative Infrastructure as Code (Terraform) and Kubernetes manifests in `deploy/terraform/` and `deploy/k8s/`. **Zero billable cloud resources** have been initialized on AWS, Azure, or GCP.

Deployment to the live Staging VPC is gated pending formal stakeholder sign-off and environment variable provisioning.

---

## 2. The 14 Infrastructure Components Review

| # | Infrastructure Component | Target Technology | Manifest Location | Security & Readiness Controls |
| :-: | :--- | :--- | :--- | :--- |
| **1** | **SaaS Control Plane** | Go 1.22 REST/gRPC API | [deploy/k8s/saas-control-plane-deployment.yaml](file:///d:/cmd%20project/M2P-Git/CLM/deploy/k8s/saas-control-plane-deployment.yaml) | 3 Replicas, RollingUpdate, Non-root (UID 10001), Read-only root filesystem, Drop all capabilities |
| **2** | **Satellite Gateway** | In-VPC Cryptographic Bridge | [deploy/k8s/satellite-gateway-daemonset.yaml](file:///d:/cmd%20project/M2P-Git/CLM/deploy/k8s/satellite-gateway-daemonset.yaml) | In-VPC isolation, mTLS gRPC uplink on port 9090, local HSM key custody |
| **3** | **Amazon Aurora PostgreSQL 16** | PostgreSQL 16 Multi-AZ | [components/saas-control-plane/deploy/migrations/](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/deploy/migrations/) | FORCE ROW LEVEL SECURITY enabled across 7 core tables, transaction-local tenant context, SSL required |
| **4** | **Keycloak / OIDC Identity** | Keycloak 24+ / OAuth2 / SAML | [components/saas-control-plane/deploy/keycloak/clm-realm.json](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/deploy/keycloak/clm-realm.json) | Enterprise OIDC realm, WebAuthn MFA, Brute-force detection, RBAC claims mapping |
| **5** | **TLS & Ingress PKI** | cert-manager / Let's Encrypt | `deploy/k8s/ingress-tls.yaml` | Automated ACME DNS-01/HTTP-01 challenge, TLS 1.3 only, HSTS preload header |
| **6** | **Secrets Management** | AWS Secrets Manager / ESO | `deploy/terraform/aws/main.tf` | External Secrets Operator syncs DB credentials, JWT signing keys, KMS aliases |
| **7** | **NetworkPolicies** | Kubernetes CNI NetworkPolicy | [deploy/k8s/network-policies.yaml](file:///d:/cmd%20project/M2P-Git/CLM/deploy/k8s/network-policies.yaml) | `default-deny-all` ingress/egress, strict pod-to-pod whitelist, private CIDR filtering |
| **8** | **IAM & Service Accounts** | AWS IAM Roles for Service Accounts (IRSA) | `deploy/terraform/aws/main.tf` | Least-privilege IAM policies, fine-grained STS assume role, zero hardcoded static API keys |
| **9** | **Centralized Logging** | FluentBit -> CloudWatch / OpenSearch | `deploy/observability/logging.yaml` | Structured JSON log format, automated PII/secret scrubbing, WORM compliance logging |
| **10** | **Prometheus & Grafana** | Prometheus Operator + Grafana | [deploy/observability/prometheus.yaml](file:///d:/cmd%20project/M2P-Git/CLM/deploy/observability/prometheus.yaml) & [grafana-dashboard-clm.json](file:///d:/cmd%20project/M2P-Git/CLM/deploy/observability/grafana-dashboard-clm.json) | Scrapes `/metrics` on port 8080, tracks p95/p99 latency, HTTP 5xx error budgets, RLS violations |
| **11** | **Backup & WAL Archiving** | pgBackRest / AWS S3 Archiving | `deploy/terraform/aws/main.tf` | Point-in-time recovery (PITR) with continuous WAL shipping to encrypted S3 bucket |
| **12** | **Disaster Recovery Runbooks** | Multi-AZ Automated Failover | [DISASTER_RECOVERY_TEST_REPORT.md](file:///d:/cmd%20project/M2P-Git/CLM/DISASTER_RECOVERY_TEST_REPORT.md) | Documented RPO < 15 min, RTO < 1 hour, cross-region failover and DNS failover procedures |
| **13** | **CI/CD Staging Pipeline** | GitHub Actions Pipeline | [.github/workflows/ci.yml](file:///d:/cmd%20project/M2P-Git/CLM/.github/workflows/ci.yml) | Automated build, race detection, Gosec, Trivy vulnerability gating, staged rollback |
| **14** | **Container Security** | Google Distroless / Scratch | [components/saas-control-plane/deploy/Dockerfile.server](file:///d:/cmd%20project/M2P-Git/CLM/components/saas-control-plane/deploy/Dockerfile.server) | Multi-stage build, zero shell binaries in final image, scanned with Trivy blocking on CRITICAL |

---

## 3. Step-by-Step Staging Provisioning Commands

When stakeholder authorization is granted to provision the Staging cloud environment:

### Phase 1: Cloud Foundation Provisioning (Terraform)
```bash
cd deploy/terraform/aws

# 1. Initialize Terraform working directory
terraform init -backend-config="bucket=clm-staging-tfstate" -backend-config="key=staging/terraform.tfstate"

# 2. Review infrastructure execution plan
terraform plan -var-file="staging.tfvars" -out="staging.tfplan"

# 3. Apply infrastructure resources (VPC, EKS, Aurora, KMS, Secrets)
terraform apply "staging.tfplan"
```

### Phase 2: Database Schema & RLS Migrations
```bash
# Retrieve database connection string from AWS Secrets Manager
export PGPASSWORD=$(aws secretsmanager get-secret-value --secret-id staging/clm/db --query SecretString --output text | jq -r .password)
export PGHOST=$(terraform output -raw aurora_writer_endpoint)
export PGUSER="clm_admin"
export PGDATABASE="clm_staging"

# Apply all 6 sequential migrations
psql -h $PGHOST -U $PGUSER -d $PGDATABASE -f components/saas-control-plane/deploy/migrations/000_keycloak_db.sql
psql -h $PGHOST -U $PGUSER -d $PGDATABASE -f components/saas-control-plane/deploy/migrations/001_initial.sql
psql -h $PGHOST -U $PGUSER -d $PGDATABASE -f components/saas-control-plane/deploy/migrations/002_sso_fields.sql
psql -h $PGHOST -U $PGUSER -d $PGDATABASE -f components/saas-control-plane/deploy/migrations/003_tenant_security.sql
psql -h $PGHOST -U $PGUSER -d $PGDATABASE -f components/saas-control-plane/deploy/migrations/004_secure_command_transport.sql
psql -h $PGHOST -U $PGUSER -d $PGDATABASE -f components/saas-control-plane/deploy/migrations/005_contract_lifecycle_management.sql
psql -h $PGHOST -U $PGUSER -d $PGDATABASE -f components/saas-control-plane/deploy/migrations/006_contract_security_and_procurement_rls.sql

# Execute live PostgreSQL RLS verification test suite
psql -h $PGHOST -U $PGUSER -d $PGDATABASE -f scripts/test_postgres_rls_live.sql
```

### Phase 3: Kubernetes Workloads Deployment
```bash
# Update local kubeconfig for Staging EKS cluster
aws eks update-kubeconfig --region us-east-1 --name clm-staging-eks

# 1. Apply NetworkPolicies first (Default Deny baseline)
kubectl apply -f deploy/k8s/network-policies.yaml

# 2. Deploy Keycloak Identity Service
kubectl apply -f deploy/k8s/keycloak-deployment.yaml

# 3. Deploy SaaS Control Plane
kubectl apply -f deploy/k8s/saas-control-plane-deployment.yaml

# 4. Deploy Prometheus & Observability stack
kubectl apply -f deploy/observability/prometheus.yaml
kubectl apply -f deploy/observability/alerting-rules.yaml

# 5. Verify Pod rollout and security contexts
kubectl get pods -n clm-production -o wide
kubectl get networkpolicies -n clm-production
```

---

## 4. Staging Release Gate Validation Checklist

Before upgrading from `STAGING CANDIDATE` to `STAGING VERIFIED`:

- [ ] **Gate 1:** All 3 SaaS Control Plane pods achieve `Running` and pass readiness probes (`/healthz`).
- [ ] **Gate 2:** `scripts/test_postgres_rls_live.sql` executes on Aurora with 12/12 scenarios passed.
- [ ] **Gate 3:** NetworkPolicies verify pod isolation (inability of control plane pod to reach arbitrary external IPs).
- [ ] **Gate 4:** Keycloak OIDC login succeeds with PKCE authorization code grant.
- [ ] **Gate 5:** Prometheus successfully scrapes metrics from all application instances.
- [ ] **Gate 6:** Webhook delivery to public TLS endpoint succeeds, while requests to `169.254.169.254` are blocked at connection time.
- [ ] **Gate 7:** GitHub Actions CI execution passes across all 10 workflow gates on commit `678491e49f27536d25b6fb0ba6fe2fbbed7a9eab`.

