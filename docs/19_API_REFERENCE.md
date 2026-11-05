# CLM-SPEC-19: ENTERPRISE API REFERENCE SPECIFICATION
## RESTful Endpoints, Request/Response Schemas, Authentication, and Status Codes

**Version:** 3.4.0-Enterprise  
**Document ID:** CLM-API-019  
**Classification:** Enterprise Developer API Standard  

---

### 1. General API Conventions

* **Base URL:** `https://api.clm.enterprise.com/api/v1`
* **Transport:** TLS 1.3 mandatory.
* **Authentication:** `Authorization: Bearer <Keycloak_JWT_Token>`
* **Content Type:** `application/json; charset=utf-8`
* **Tenant Scoping:** Injected via token claims; optionally passed via `?tenant_id=<UUID>`.

---

### 2. Core Endpoints Reference

#### 2.1 Corporate Departments (14)
* **`GET /api/v1/corporate-departments`**
  * *Description:* Returns the catalog of 14 corporate departments, designated leads, contract counts, and capabilities.
  * *Response 200 OK:*
    ```json
    { "departments": [ { "code": "HR", "name": "Human Resources", "contracts_count": 1, ... } ] }
    ```

#### 2.2 Contract Repository Management
* **`GET /api/v1/contracts`**
  * *Query Params:* `status`, `stage`, `department`, `tenant_id`
  * *Response 200 OK:* `{ "contracts": [...], "count": 15 }`
* **`POST /api/v1/contracts`**
  * *Body:* `{ "title": "MSA", "type": "MSA", "total_value": 350000, "counterparty_name": "Acme Corp" }`
  * *Response 201 Created:* `{ "contract": { "id": "cnt-001", ... } }`
* **`POST /api/v1/contracts/:id/transition`**
  * *Body:* `{ "target_stage": "negotiation", "target_status": "in_negotiation" }`
  * *Response 200 OK:* `{ "contract": { ... } }`
* **`GET /api/v1/contracts/:id/versions`**
  * *Response 200 OK:* `{ "versions": [ { "version_number": 1, ... } ] }`

#### 2.3 Contract Intake Requests
* **`GET /api/v1/contract-requests`**
  * *Response 200 OK:* `{ "requests": [...] }`
* **`POST /api/v1/contract-requests/:id/convert`**
  * *Description:* Triages and converts an intake request into a formal contract draft.
  * *Response 200 OK:* `{ "contract": { "stage": "authoring", ... }, "request_id": "req-01" }`

#### 2.4 Negotiation & Redlines
* **`GET /api/v1/negotiations/:contractId/redlines`**
  * *Response 200 OK:* `{ "redlines": [...] }`
* **`POST /api/v1/negotiations/:contractId/redlines`**
  * *Body:* `{ "clause_title": "Liability", "original_text": "...", "proposed_text": "...", "author_type": "counterparty" }`
  * *Response 201 Created:* `{ "redline": { "id": "red-01", "status": "proposed" } }`
* **`PUT /api/v1/negotiations/:contractId/redlines/:redlineId/resolve`**
  * *Body:* `{ "decision": "accepted" }`
  * *Response 200 OK:* `{ "redline": { "id": "red-01", "status": "accepted" } }`

#### 2.5 Multi-Tier Approvals & Segregation of Duties
* **`GET /api/v1/approvals/pending`**
  * *Tenant scope:* Uses the authenticated tenant; platform admins may select a tenant with `?tenant_id=<UUID>`.
  * *Response 200 OK:* `{ "approvals": [...] }`
* **`GET /api/v1/approvals/:id`**
  * *Tenant scope:* The ID lookup is constrained by the authenticated or explicitly authorized tenant; a request from another tenant returns 404.
  * *Errors:* Returns 404 only when no request exists in that tenant; database or repository failures return 500.
* **`POST /api/v1/approvals`**
  * *Tenant scope:* Creates the request only in the authenticated tenant, or a platform-admin-selected tenant validated by `?tenant_id=<UUID>`.
* **`POST /api/v1/approvals/:id/approve`**
  * *Body:* `{ "comment": "Approved" }`; the approver identity comes from the authenticated token.
  * *Response 200 OK:* `{ "message": "Approval recorded", "approval": { ... } }`
  * *Response 403 Forbidden:* `{ "error": "segregation of duties: requester cannot approve own request" }`
  * *Tenant scope:* Request lookup and update both include the authorized tenant ID.
* **`POST /api/v1/approvals/:id/reject`** and **`POST /api/v1/approvals/:id/escalate`**
  * *Tenant scope:* Request lookup and update are constrained to the authorized tenant.

Approval endpoint tenant scoping is enforced in the handler, tenant-qualified repository queries, and PostgreSQL RLS policies for `approval_requests` and `approval_rules`. Repository operations set tenant context transaction-locally.

#### 2.6 Certificate Inventory & Revocation
* **`GET /api/v1/certificates/:id`**
  * *Tenant scope:* The certificate lookup is constrained to the authenticated tenant.
  * *Errors:* Returns 404 only when the certificate is absent from that tenant; repository failures return 500.
* **`POST /api/v1/certificates/:id/revoke`**
  * *Tenant scope:* The certificate lookup and update use the authenticated tenant.
  * *Errors:* Returns 404 only when the certificate is absent from that tenant; repository failures return 500.

#### 2.7 Post-Execution Obligations & Financials
* **`GET /api/v1/obligations`**
  * *Response 200 OK:* `{ "obligations": [...] }`
* **`POST /api/v1/obligations`**
  * *Body:* `{ "id": "obl-01", "status": "fulfilled", "verified_by": "audit@internal.com" }`
  * *Response 200 OK:* `{ "obligation": { "status": "fulfilled" } }`
* **`POST /api/v1/financials/three-way-match`**
  * *Body:* `{ "purchase_order_id": "PO-1", "invoice_amount": 87500.00 }`
  * *Response 200 OK:* `{ "status": "matched", "verified": true }`

#### 2.8 AI Contract Intelligence
* **`POST /api/v1/ai-contracts/:contractId/ask`**
  * *Body:* `{ "question": "What are the payment terms?" }`
  * *Response 200 OK:* `{ "answer": "Net 30 days.", "cited_clauses": ["Section 2: Commercial Terms"] }`

#### 2.9 Executive Analytics & Reporting
* **`GET /api/v1/clm-reports/summary`**
  * *Response 200 OK:* `{ "cycle_time": { "average_days": 14.2 }, "portfolio_health": { "score": 92 } }`
