# ENTERPRISE CLM PLATFORM — PRODUCTION DEPLOYMENT & SRE RUNBOOK

**Target Clouds:** AWS, Microsoft Azure, Google Cloud Platform (GCP)  
**Container Orchestration:** Kubernetes (EKS / AKS / GKE) v1.28+  
**Security Standard:** Pod Security Standard `restricted` / CIS Kubernetes Benchmark Level 2  
**Document Version:** 4.0.0-ENTERPRISE  

---

### 1. KUBERNETES DEPLOYMENT MANIFEST (RESTRICTED POD SECURITY)

Production deployments run with strictly non-root privileges, read-only root filesystems, and dropped Linux capabilities:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: clm-api-gateway
  namespace: clm-production
  labels:
    app.kubernetes.io/name: clm-api-gateway
    app.kubernetes.io/part-of: enterprise-clm
spec:
  replicas: 4
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 0
  selector:
    matchLabels:
      app: clm-api-gateway
  template:
    metadata:
      labels:
        app: clm-api-gateway
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 10001
        runAsGroup: 10001
        fsGroup: 10001
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: gateway
          image: enterprise-clm/gateway:4.0.0
          imagePullPolicy: IfNotPresent
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - ALL
          ports:
            - containerPort: 4000
              name: http
          env:
            - name: NODE_ENV
              value: "production"
            - name: PORT
              value: "4000"
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: clm-db-credentials
                  key: connection_string
          resources:
            requests:
              cpu: "1000m"
              memory: "2Gi"
            limits:
              cpu: "4000m"
              memory: "8Gi"
          livenessProbe:
            httpGet:
              path: /api/v1/identity/status
              port: 4000
            initialDelaySeconds: 15
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /api/v1/identity/status
              port: 4000
            initialDelaySeconds: 5
            periodSeconds: 5
```

---

### 2. IN-VPC SATELLITE GATEWAY DEPLOYMENT (DOCKER COMPOSE)

For customer on-premises or isolated VPC deployment:

```yaml
version: '3.8'

services:
  clm-satellite-gateway:
    image: enterprise-clm/satellite-gateway:4.0.0
    restart: always
    network_mode: "host"
    environment:
      - SATELLITE_ID=sat-vpc-us-east-01
      - CLOUD_UPLINK_URL=https://clm-control-plane.enterprise.com
      - HSM_PKCS11_LIB=/opt/cloudhsm/lib/libcloudhsm_pkcs11.so
      - HSM_PIN_SECRET=/run/secrets/hsm_pin
      - LOCAL_SQLCIPHER_PATH=/var/data/satellite_cache.db
      - RFC3161_TSA_URL=https://timestamp.digicert.com
    volumes:
      - /etc/cloudhsm:/etc/cloudhsm:ro
      - /opt/cloudhsm/lib:/opt/cloudhsm/lib:ro
      - satellite_data:/var/data
    secrets:
      - hsm_pin
      - mtls_client_cert
      - mtls_client_key

volumes:
  satellite_data:
    driver: local

secrets:
  hsm_pin:
    file: ./secrets/hsm_pin.txt
  mtls_client_cert:
    file: ./certs/satellite_mtls.crt
  mtls_client_key:
    file: ./certs/satellite_mtls.key
```

---

### 3. SRE OBSERVABILITY, LOGGING & PROMETHEUS METRICS

The platform exports Prometheus metrics and OpenTelemetry traces across all operations:

| Metric Name | Type | Description | Alert Threshold |
|:---|:---|:---|:---|
| `clm_api_requests_total` | Counter | Total incoming REST API requests by method, route, and status. | 5xx error rate > 0.5% |
| `clm_api_request_duration_seconds` | Histogram | Latency histogram per endpoint. | p95 latency > 100ms |
| `clm_rls_violations_total` | Counter | Attempted cross-tenant query blocks by PostgreSQL kernel. | > 0 triggers critical alert |
| `clm_worm_audit_blocks_total` | Counter | Monotonically increasing WORM audit block sequence. | Rate stall > 5m |
| `clm_satellite_queue_depth` | Gauge | Number of signing events queued locally during network outage. | Queue > 5,000 |

#### Sample Prometheus Alert Rule
```yaml
groups:
  - name: clm_security_alerts
    rules:
      - alert: TenantIsolationViolationAttempt
        expr: increase(clm_rls_violations_total[5m]) > 0
        for: 0m
        labels:
          severity: critical
        annotations:
          summary: "CRITICAL: Kernel RLS blocked cross-tenant data access attempt."
          description: "Tenant context mismatch detected in PostgreSQL RLS layer. Potential security breach or misconfigured query."
```

---

### 4. DISASTER RECOVERY & BACKUP RUNBOOK

1. **RPO Target**: 0 seconds (Synchronous Multi-AZ WAL replication).
2. **RTO Target**: < 15 minutes.
3. **Database Restore Runbook**:
   ```bash
   # 1. Stop application gateway
   kubectl scale deployment clm-api-gateway --replicas=0 -n clm-production

   # 2. Execute Point-in-Time Recovery from immutable WAL archive
   pg_restore --clean --if-exists -h $PGHOST -U clm_migrator_role -d clm_enterprise s3://clm-backup-vault/wal-archives/pitr-target.dump

   # 3. Verify WORM audit chain integrity
   node scripts/verify_scenarios_a_to_h.js

   # 4. Restart gateway pods
   kubectl scale deployment clm-api-gateway --replicas=4 -n clm-production
   ```

