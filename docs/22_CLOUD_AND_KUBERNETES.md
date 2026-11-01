# CLM-SPEC-22: CLOUD & KUBERNETES DEPLOYMENT SPECIFICATION
## EKS, AKS, GKE, Restricted Pod Security Standards, and Zero-Trust NetworkPolicies

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-OPS-022  
**Classification:** Enterprise Cloud Engineering Standard  

---

### 1. Multi-Cloud Infrastructure (AWS, Azure, GCP)

The platform is designed to deploy across all major cloud hyper-scalers using standardized Terraform modules in `deploy/terraform/`:
* **AWS:** EKS managed Kubernetes cluster, AWS KMS customer master key, RDS PostgreSQL Multi-AZ, and S3 with Object Lock.
* **Azure:** AKS cluster, Azure Key Vault Premium (HSM-backed), Azure Database for PostgreSQL Flexible Server with Zone Redundancy.
* **GCP:** GKE Autopilot/Standard cluster, Cloud KMS HSM protection level, and Cloud SQL for PostgreSQL 15 Regional HA.

---

### 2. Kubernetes Pod Security Standards (Restricted)

All Kubernetes workloads in `deploy/k8s/` enforce the Kubernetes **Restricted Pod Security Standard**:
```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 10001
  runAsGroup: 10001
  fsGroup: 10001
  seccompProfile:
    type: RuntimeDefault
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
```
* **No Root Execution:** Containers run under dedicated unprivileged system users (UID 10001 for Control Plane, 10002 for Satellite).
* **Immutable Filesystem:** Root filesystems are mounted read-only; transient files are written solely to bounded in-memory `emptyDir` volumes at `/tmp`.

---

### 3. Zero-Trust NetworkPolicies

Declared in `deploy/k8s/network-policies.yaml`:
* **Default Deny:** All ingress and egress traffic is dropped by default across the namespace.
* **Micro-Segmentation:** The Control Plane pod can only contact PostgreSQL on TCP port 5432, Keycloak on port 8080, and CoreDNS on UDP/TCP 53.
* **In-VPC Isolation:** Satellite Gateways only accept inbound traffic from customer-approved internal proxies and outbound traffic to the pinned SaaS control plane gRPC port 9090.

