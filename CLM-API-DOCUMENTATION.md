# ENTERPRISE CLM PLATFORM — REST API & OPENAPI 3.0 SPECIFICATION

**API Version:** v1 (Enterprise Edition)  
**Host:** `https://clm-api.enterprise.domain` (Port 4000/443)  
**Security Schemes:** OAuth 2.0 Bearer JWT, Mutual TLS 1.3, HMAC-SHA256  
**Documentation Format:** OpenAPI 3.0.3 Conforming  

---

### 1. AUTHENTICATION & MULTI-TENANCY HEADERS

All requests to the Enterprise CLM API Gateway require authentication tokens and explicit tenant context headers:

| Header Name | Type | Description |
|:---|:---|:---|
| `Authorization` | `Bearer <JWT>` | Signed OIDC / Keycloak JWT containing user identity, roles, and session claims. |
| `X-Tenant-ID` | `UUID` | Target tenant UUID. Validated by API gateway and bound to PostgreSQL `app.current_tenant_id`. |
| `X-Signature` | `Hex String` | HMAC-SHA256 signature for automated ERP/webhook integrations. |
| `X-Timestamp` | `ISO 8601` | Nonce timestamp to prevent replay attacks (tolerance: ±300s). |

---

### 2. CORE API ENDPOINTS CATALOG

#### 2.1 400-Feature Modular Registry & Execution

##### `GET /api/v1/features`
Retrieves all 400 enterprise features with metadata, operational domains, and compliance mapping.
* **Query Parameters:**
  * `domain_id` (optional): Filter by domain (e.g., `domain-01-core-contract-repository`).
  * `limit` (optional, default 400): Maximum results to return.
* **Response (200 OK):**
```json
{
  "total": 400,
  "features": [
    {
      "id": 1,
      "code": "CLM-REQ-001",
      "name": "Contract Repository Management",
      "domain": "Core Contract Repository & Lifecycle",
      "domainId": "domain-01-core-contract-repository",
      "status": "registered",
      "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
    }
  ]
}
```

##### `GET /api/v1/features/:id`
Retrieves single feature details, parameter schemas, and execution constraints.

##### `POST /api/v1/features/:id/execute`
Executes feature business logic within isolated tenant boundaries.
* **Request Body:**
```json
{
  "tenant_id": "tenant-enterprise-01",
  "data": {
    "operation": "SYNC_REPOSITORY",
    "parameters": { "sync_mode": "FULL" }
  }
}
```
* **Response (200 OK):**
```json
{
  "success": true,
  "feature": "Contract Repository Management",
  "code": "CLM-REQ-001",
  "tenant_id": "tenant-enterprise-01",
  "result": {
    "message": "Contract Repository Management executed successfully within isolated tenant context.",
    "audit": {
      "feature_id": 1,
      "record_hash": "a1b2c3d4e5f6... (64-char SHA-256)",
      "timestamp": "2026-11-03T10:00:00.000Z"
    }
  }
}
```

##### `GET /api/v1/domains`
Returns all 20 operational domains with feature distribution statistics.

---

#### 2.2 Contract Lifecycle & Operational Endpoints

##### `GET /api/v1/contracts`
Lists contracts governed by tenant RLS filtering. Supports filtering by department, lifecycle stage, counterparty, and status.

##### `POST /api/v1/contract-requests`
Submits a new contract intake request for legal triage.

##### `POST /api/v1/contract-requests/:id/convert`
Triages an intake request and initiates dynamic contract drafting (Macro-Stage 1 $\rightarrow$ Stage 2).

##### `GET /api/v1/clauses`
Retrieves pre-approved standard playbook clauses from the central clause repository.

##### `POST /api/v1/redlines`
Submits an atomic counterparty redline diff with proposed additions/deletions.

##### `POST /api/v1/redlines/:id/accept`
Resolves and commits a proposed redline to the contract working draft.

##### `GET /api/v1/approvals/pending`
Retrieves pending contract approvals requiring authorized signoff according to Delegation of Authority (DOA) thresholds.

##### `POST /api/v1/approvals/:id/signoff`
Records authorized signoff. Enforces Segregation of Duties (SoD Policy ARC-004: contract author cannot self-approve).

##### `POST /api/v1/signatures/in-vpc`
Dispatches document digest to customer In-VPC Satellite Gateway for detached RSA-PSS / ECDSA hardware signing.

##### `POST /api/v1/signatures/verify`
Zero-knowledge cryptographic verification of detached digital signature against original document digest.

##### `GET /api/v1/obligations`
Retrieves active post-execution contractual obligations, SLA targets, and financial milestones.

##### `POST /api/v1/obligations`
Submits milestone fulfillment proof and updates obligation compliance state.

##### `POST /api/v1/financials/three-way-match`
Triggers automated three-way reconciliation between Purchase Order (PO), Goods Receipt, and Vendor Invoice.

##### `GET /api/v1/clm-reports/summary`
Retrieves executive portfolio analytics, cycle times, risk scores, and department health metrics.

##### `POST /api/v1/ai-contracts/:id/ask`
Queries zero-egress local contract intelligence engine for clause analysis and risk evaluation. Includes prompt injection defense filter.

---

#### 2.3 SDLC Workflows Engine Endpoints

##### `POST /api/v1/workflows/:type`
Executes an automated SDLC governance workflow.
* **Path Parameters:**
  * `:type`: `feature` | `bugfix` | `spec`
* **Request Body:**
```json
{
  "taskId": "CLM-SDLC-901",
  "title": "Automated Validation Run",
  "parameters": { "strictMode": true }
}
```
* **Response (200 OK):**
```json
{
  "success": true,
  "workflow": "clm-feature-dev",
  "status": "COMPLETED",
  "phases_executed": ["requirements", "architecture", "implementation", "validation"],
  "worm_record": {
    "block_id": "WF-2026-8812",
    "sha256": "8f4a... (64-char hash)"
  }
}
```

##### `GET /api/v1/workflows/audit-log`
Returns the append-only audit trail of all SDLC workflow executions.

---

### 3. ERROR CODES & PROTOCOL RESPONSES

| Status Code | Meaning | Description |
|:---|:---|:---|
| `200 OK` | Success | Request processed and tenant context satisfied. |
| `201 Created` | Created | Resource successfully created and registered in ledger. |
| `400 Bad Request` | Validation Error | Payload failed schema validation or parameter missing. |
| `401 Unauthorized` | Invalid Auth | Missing or expired JWT token. |
| `403 Forbidden` | Access Denied | Segregation of Duties violation (e.g., self-approval ARC-004) or DOA tier insufficient. |
| `404 Not Found` | Not Found | Resource ID or Feature ID does not exist in registry. |
| `409 Conflict` | Version Conflict | Optimistic locking failure or concurrent redline collision. |
| `422 Unprocessable` | Semantic Error | AI prompt injection detected or business rule violation. |
| `500 Server Error` | Internal Failure | Internal processing failure; zero sensitive details leaked. |

