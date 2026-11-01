# CLM — 26-STAGE CONTRACT LIFECYCLE & 14-DEPARTMENT SPECIFICATION
**Platform:** Enterprise Contract & Certificate Lifecycle Management (CLM) SaaS Platform  
**Document Reference:** CLM-CORE-LIFECYCLE-2026.1  
**Assessment Date:** 2026-10-31  
**Author:** Principal Software Architect & Enterprise Workflow Specialist  
**Classification:** Enterprise Core Lifecycle Architecture Standard  

---

## 1. Executive Summary

This specification provides the authoritative, exhaustive operational definition for all **26 Contract Lifecycle Stages** across **8 Macro-Stages**, and reconciles the architecture with the **14 Corporate Departments** implemented in the platform.

Every stage is defined with deterministic entry criteria, exit criteria, responsible departments, RBAC permissions, target SLAs, audit logging requirements, and automated failure handling procedures.

---

## 2. Macro-Stage & Departmental Topology

### 2.1 The 8 Macro-Stages
```
[1. Intake] ──► [2. Authoring] ──► [3. Negotiation] ──► [4. Approval]
     │                                                        │
     ▼                                                        ▼
[8. Archival] ◄── [7. Post-Execution] ◄── [6. Execution] ◄── [5. Signature]
```

### 2.2 The 14 Verified Corporate Departments
The platform embeds tailored workflows, permission scopes, and agreement types across 14 enterprise departments:

| # | Department Code | Department Name | Key Agreements Managed | Primary Lifecycle Stages |
| :-: | :--- | :--- | :--- | :--- |
| **1** | `HR` | Human Resources | Offer Letters, Executive NDAs, Contractor Agreements | Stages 1–4, 11–16, 21 |
| **2** | `FIN` | Finance | Credit Facilities, Debt Instruments, Milestone Billing | Stages 11–13, 19–20, 23 |
| **3** | `PROC` | Procurement | Master Supply Agreements (MSA), POs, Sourcing RFPs | Stages 1–5, 8–10, 19–20, 23 |
| **4** | `SALES` | Sales / Commercial | Customer MSAs, Order Forms, Reseller Agreements, SLAs | Stages 1–10, 14–16, 24 |
| **5** | `LEGAL` | Legal Operations | Dispute Settlements, IP Licensing, Retainers, Playbooks | Stages 2–10, 22, 25–26 |
| **6** | `IT` | Information Technology | Software Licenses, SaaS Subscriptions, Cloud IaaS/PaaS | Stages 1–5, 11–13, 21–22 |
| **7** | `SEC` | Security & Privacy | Data Processing Agreements (DPA), Security Addenda | Stages 4–6, 11–13, 21 |
| **8** | `OPS` | Operations | Logistics Master Agreements, Equipment Maintenance | Stages 1–4, 21–22 |
| **9** | `MFG` | Manufacturing | Tooling Contracts, OEM Supply Agreements, Warranties | Stages 1–4, 8–10, 21–23 |
| **10** | `FAC` | Facilities | Real Estate Leases, Servicing, Tenant Improvements | Stages 1–4, 11–13, 21, 24 |
| **11** | `MKTG` | Marketing | Agency Retainers, Sponsorships, Trademark Licensing | Stages 1–4, 11–13, 21 |
| **12** | `COMPL` | Compliance | Regulatory Filings, Statutory Audit Engagements | Stages 5, 11–13, 21, 26 |
| **13** | `ADMIN` | Corporate Administration| DOA Policies, Power of Attorney, Signatory Registries | Stages 11–13, 14–16 |
| **14** | `EXEC` | Executive Management | Joint Ventures, M&A Agreements, Board Charters | Stages 11–13, 14–16, 26 |

---

## 3. The 26 Detailed Contract Lifecycle Stages

### STAGE 1: Intake Request Submission
* **Stage ID:** `STAGE-01` | **Macro-Stage:** `1. Intake`
* **Responsible Department:** All 14 Departments (Originating Business Unit)
* **RBAC Permissions:** `contract:create_request`, `clm_business_user`
* **Entry Criteria:** Business user completes intake questionnaire (counterparty, estimated value, commercial intent).
* **Exit Criteria:** System generates unique Request UUID and assigns triage SLA timer.
* **Target SLA:** Instantaneous (< 5 seconds).
* **Audit Requirements:** Log `request_created` event with actor, timestamp, IP, and intake form JSON.
* **Failure Handling:** Return form validation errors with field-level diagnostics.

### STAGE 2: Triage & Qualification
* **Stage ID:** `STAGE-02` | **Macro-Stage:** `1. Intake`
* **Responsible Department:** `LEGAL`, `PROC`, or `SALES` (Deal Desk)
* **RBAC Permissions:** `contract:triage`, `legal_counsel`, `procurement_manager`
* **Entry Criteria:** Request UUID exists in `contract_requests` with status `pending_triage`.
* **Exit Criteria:** Assignment of risk tier (Tier 1-Low, Tier 2-Medium, Tier 3-High) and selection of contracting track (Standard Template vs. Third-Party Paper).
* **Target SLA:** 4 business hours.
* **Audit Requirements:** Log `request_triaged` event with assigned attorney and risk classification.
* **Failure Handling:** Escalate to Legal Operations Lead if unassigned after 4 hours.

### STAGE 3: Template & Clause Selection
* **Stage ID:** `STAGE-03` | **Macro-Stage:** `2. Authoring`
* **Responsible Department:** `LEGAL`
* **RBAC Permissions:** `contract:author`, `legal_counsel`, `contract_specialist`
* **Entry Criteria:** Triaged request approved for drafting.
* **Exit Criteria:** System merges intake variables into standard pre-approved template from `contract_templates`.
* **Target SLA:** 1 business hour.
* **Audit Requirements:** Log `template_selected` event with template ID and version digest.
* **Failure Handling:** Prompt for fallback clause library lookup if intake parameters match non-standard conditions.

### STAGE 4: Dynamic Drafting & Assembly
* **Stage ID:** `STAGE-04` | **Macro-Stage:** `2. Authoring`
* **Responsible Department:** Originating Department & `LEGAL`
* **RBAC Permissions:** `contract:edit`, `legal_counsel`, `dept_author`
* **Entry Criteria:** Template assembled with populated business variables.
* **Exit Criteria:** Initial baseline draft completed; preliminary terms confirmed by contract author.
* **Target SLA:** 2 business days.
* **Audit Requirements:** Log `draft_assembled` event with word count and preliminary clause inventory.
* **Failure Handling:** Auto-save drafts to local draft store; provide restore checkpoints.

### STAGE 5: AI Semantic Risk Evaluation
* **Stage ID:** `STAGE-05` | **Macro-Stage:** `2. Authoring`
* **Responsible Department:** `SEC`, `COMPL`, `LEGAL`
* **RBAC Permissions:** `contract:ai_analyze`, `security_officer`, `legal_counsel`
* **Entry Criteria:** Draft content submitted for automated pre-review.
* **Exit Criteria:** AI contract intelligence engine scans text, evaluates deviation against standard playbook, checks for indirect prompt injection, and generates risk score (0–100).
* **Target SLA:** < 30 seconds.
* **Audit Requirements:** Log `ai_analysis_completed` event with model version, risk score, and detected clauses.
* **Failure Handling:** If indirect prompt injection detected, flag `CRITICAL` risk and require mandatory human review.

### STAGE 6: Version Seal & Baseline Lock
* **Stage ID:** `STAGE-06` | **Macro-Stage:** `2. Authoring`
* **Responsible Department:** `LEGAL`
* **RBAC Permissions:** `contract:lock_version`, `legal_counsel`
* **Entry Criteria:** Authoring completed and internal risk score within allowable threshold (< 40).
* **Exit Criteria:** Compute SHA-256 content digest; write immutable snapshot to `contract_versions` as Version 1.0.
* **Target SLA:** Instantaneous.
* **Audit Requirements:** Log `version_locked` event with version number and SHA-256 master digest.
* **Failure Handling:** Reject version locking if master digest computation fails.

### STAGE 7: External Negotiation Workspace Setup
* **Stage ID:** `STAGE-07` | **Macro-Stage:** `3. Negotiation`
* **Responsible Department:** `SALES`, `PROC`, `LEGAL`
* **RBAC Permissions:** `negotiation:create_session`, `deal_desk`, `procurement_manager`
* **Entry Criteria:** Version 1.0 locked; counterparty negotiator contact confirmed.
* **Exit Criteria:** Generate one-time cryptographically signed negotiation token with 14-day expiry.
* **Target SLA:** 2 hours.
* **Audit Requirements:** Log `negotiation_session_created` event with token hash and recipient email.
* **Failure Handling:** Regenerate token upon authenticated request if expired.

### STAGE 8: External Redlining & Counterparty Review
* **Stage ID:** `STAGE-08` | **Macro-Stage:** `3. Negotiation`
* **Responsible Department:** Counterparty & `LEGAL`
* **RBAC Permissions:** `negotiation:submit_redline`, `external_counterparty`
* **Entry Criteria:** Counterparty accesses secure workspace via valid one-time token.
* **Exit Criteria:** Proposed modifications submitted and parsed into atomic clause-level redlines in `contract_redlines`.
* **Target SLA:** 5 business days.
* **Audit Requirements:** Log `redline_proposed` event with clause ID, before/after diff, and source IP.
* **Failure Handling:** Reject redline payloads exceeding 5MB or containing malicious HTML/script injections.

### STAGE 9: Redline Reconciliation & Resolution
* **Stage ID:** `STAGE-09` | **Macro-Stage:** `3. Negotiation`
* **Responsible Department:** `LEGAL`, `SALES`, `PROC`
* **RBAC Permissions:** `negotiation:resolve_redline`, `legal_counsel`
* **Entry Criteria:** Redline batch submitted by counterparty.
* **Exit Criteria:** Every proposed redline marked `accepted`, `rejected`, or `counter_proposed`.
* **Target SLA:** 2 business days.
* **Audit Requirements:** Log `redline_resolved` event for each item with attorney rationale.
* **Failure Handling:** Require escalation to General Counsel for rejected core indemnification/liability terms.

### STAGE 10: Clean Draft Finalization
* **Stage ID:** `STAGE-10` | **Macro-Stage:** `3. Negotiation`
* **Responsible Department:** `LEGAL`
* **RBAC Permissions:** `contract:finalize_draft`, `legal_counsel`
* **Entry Criteria:** All redlines resolved and mutual agreement confirmed by both parties.
* **Exit Criteria:** Clean, final execution version compiled; SHA-256 digest computed and locked as Final Draft.
* **Target SLA:** 4 business hours.
* **Audit Requirements:** Log `clean_draft_finalized` event with final document digest.
* **Failure Handling:** Abort finalization if unresolved redlines remain in database.

### STAGE 11: DOA Threshold & Routing Engine
* **Stage ID:** `STAGE-11` | **Macro-Stage:** `4. Approval`
* **Responsible Department:** `FIN`, `ADMIN`
* **RBAC Permissions:** `approval:route`, `system_orchestrator`
* **Entry Criteria:** Clean draft finalized; financial value and department evaluated.
* **Exit Criteria:** System queries `delegation_of_authority_rules` and builds required approver chain:
  - Tier 1: Department Manager (up to \$50,000)
  - Tier 2: Vice President / Director (up to \$250,000)
  - Tier 3: CFO / CEO (up to \$1,000,000+)
  - Tier 4: Board of Directors (> \$10,000,000)
* **Target SLA:** Instantaneous (< 5 seconds).
* **Audit Requirements:** Log `approval_chain_generated` event with required approvers and dollar thresholds.
* **Failure Handling:** Route to General Counsel and CFO if contract value is uncapped.

### STAGE 12: Segregation of Duties (SoD) Enforcement
* **Stage ID:** `STAGE-12` | **Macro-Stage:** `4. Approval`
* **Responsible Department:** `COMPL`, `ADMIN`
* **RBAC Permissions:** `approval:sod_verify`, `system_security`
* **Entry Criteria:** Approver attempts to approve contract.
* **Exit Criteria:** Verification passes rule `Requester != Approver` (ARC-004). Approver confirmed distinct from author.
* **Target SLA:** Instantaneous (< 1 second).
* **Audit Requirements:** Log `sod_check_passed` or `sod_violation_blocked` event.
* **Failure Handling:** Halt approval with HTTP 403 Forbidden: "Segregation of duties violation: Author cannot approve."

### STAGE 13: Multi-Tier Approval Execution
* **Stage ID:** `STAGE-13` | **Macro-Stage:** `4. Approval`
* **Responsible Department:** Designee per DOA Tier (`FIN`, `LEGAL`, `EXEC`)
* **RBAC Permissions:** `approval:execute`, `designated_approver`
* **Entry Criteria:** Contract pending approval at current tier level.
* **Exit Criteria:** Digital approval signature recorded; status transitions to `approved`.
* **Target SLA:** 24 hours per approval tier.
* **Audit Requirements:** Log `approval_granted` event with approver UUID, tier, timestamp, and comments.
* **Failure Handling:** Send reminder alert at 12 hours; auto-escalate to secondary delegate at 24 hours.

### STAGE 14: Signing Route Configuration
* **Stage ID:** `STAGE-14` | **Macro-Stage:** `5. Signature`
* **Responsible Department:** `LEGAL`, `ADMIN`
* **RBAC Permissions:** `signature:configure`, `contract_administrator`
* **Entry Criteria:** All required DOA approvals granted.
* **Exit Criteria:** Signer sequence (serial vs. parallel), email addresses, and auth methods defined in `contract_signatures`.
* **Target SLA:** 2 business hours.
* **Audit Requirements:** Log `signing_route_configured` event with signer recipient roster.
* **Failure Handling:** Validate signer email domains against organizational whitelist.

### STAGE 15: Signer Identity Authentication
* **Stage ID:** `STAGE-15` | **Macro-Stage:** `5. Signature`
* **Responsible Department:** Counterparty & External Signers
* **RBAC Permissions:** `signature:authenticate`, `external_signer`
* **Entry Criteria:** Signer accesses signing portal via unique cryptographic URL.
* **Exit Criteria:** Signer completes MFA challenge (SMS/Email OTP or corporate OIDC SSO).
* **Target SLA:** 48 hours to complete.
* **Audit Requirements:** Log `signer_authenticated` event with auth method, IP address, and user agent.
* **Failure Handling:** Invalidate session after 3 failed OTP attempts; notify contract owner.

### STAGE 16: In-VPC Hardware / PKI Signature Execution
* **Stage ID:** `STAGE-16` | **Macro-Stage:** `5. Signature`
* **Responsible Department:** Customer In-VPC Satellite Gateway
* **RBAC Permissions:** `signature:sign`, `satellite_enclave_role`
* **Entry Criteria:** Signer clicks "Accept & Sign".
* **Exit Criteria:** Customer In-VPC HSM / KMS creates detached RSA-PSS or ECDSA digital signature over document digest.
* **Target SLA:** < 5 seconds.
* **Audit Requirements:** Log `signature_executed` event with detached signature hash and key ID.
* **Failure Handling:** Abort if signature verification fails bit-flip tamper check.

### STAGE 17: RFC 3161 Timestamp Token Sealing
* **Stage ID:** `STAGE-17` | **Macro-Stage:** `5. Signature`
* **Responsible Department:** Signature Engine & External TSA
* **RBAC Permissions:** `signature:timestamp`, `system_pki`
* **Entry Criteria:** Digital signature generated.
* **Exit Criteria:** Obtain signed TimeStampResp from RFC 3161 Time-Stamping Authority; embed token in evidence bundle.
* **Target SLA:** < 10 seconds.
* **Audit Requirements:** Log `timestamp_token_embedded` event with TSA certificate serial number.
* **Failure Handling:** Fall back to secondary TSA endpoint if primary is unreachable.

### STAGE 18: Evidence Bundle Compilation
* **Stage ID:** `STAGE-18` | **Macro-Stage:** `5. Signature`
* **Responsible Department:** `LEGAL`, `SEC`
* **RBAC Permissions:** `signature:compile_evidence`, `system_orchestrator`
* **Entry Criteria:** All designated signers have executed signatures and timestamps.
* **Exit Criteria:** Serialization of all signatures, cert chains, audit logs, and TSA tokens into an immutable JSON-LD/CMS evidence bundle.
* **Target SLA:** < 1 minute.
* **Audit Requirements:** Log `evidence_bundle_compiled` event with bundle SHA-256 digest.
* **Failure Handling:** Quarantine agreement if evidence bundle integrity check fails.

### STAGE 19: Execution & Activation
* **Stage ID:** `STAGE-19` | **Macro-Stage:** `6. Execution`
* **Responsible Department:** `LEGAL`, `ADMIN`
* **RBAC Permissions:** `contract:activate`, `contract_administrator`
* **Entry Criteria:** Evidence bundle compiled and validated.
* **Exit Criteria:** Status transitioned from `pending_signature` to `executed` / `active`. Effective date recorded.
* **Target SLA:** Instantaneous.
* **Audit Requirements:** Log `contract_activated` event; notify all internal and external parties.
* **Failure Handling:** Revert to `pending_signature` if activation transaction fails.

### STAGE 20: ERP & Financial Ledger Synchronization
* **Stage ID:** `STAGE-20` | **Macro-Stage:** `6. Execution`
* **Responsible Department:** `FIN`, `PROC`
* **RBAC Permissions:** `integration:sync_erp`, `financial_systems`
* **Entry Criteria:** Contract active and possesses financial value > \$0.
* **Exit Criteria:** Push commitments, cost centers, and line items to SAP / NetSuite via HMAC-signed webhook.
* **Target SLA:** < 5 minutes.
* **Audit Requirements:** Log `erp_synced` event with ERP purchase order ID and response code.
* **Failure Handling:** Retry via exponential backoff (1m, 5m, 15m, 1h); route to Dead-Letter Queue (DLQ) if failed.

### STAGE 21: Post-Execution Obligation Extraction
* **Stage ID:** `STAGE-21` | **Macro-Stage:** `7. Post-Execution`
* **Responsible Department:** `OPS`, `IT`, `SEC`
* **RBAC Permissions:** `obligation:initialize`, `operations_manager`
* **Entry Criteria:** Contract activated.
* **Exit Criteria:** Contract deliverables and compliance obligations parsed into atomic tasks in `contract_obligations`.
* **Target SLA:** 24 hours.
* **Audit Requirements:** Log `obligations_initialized` event with task count and due dates.
* **Failure Handling:** Provide manual task creation interface for custom legal commitments.

### STAGE 22: Performance, Milestone & SLA Monitoring
* **Stage ID:** `STAGE-22` | **Macro-Stage:** `7. Post-Execution`
* **Responsible Department:** `OPS`, `IT`, `LEGAL`
* **RBAC Permissions:** `obligation:fulfill`, `milestone_manager`
* **Entry Criteria:** Obligations in active monitoring state.
* **Exit Criteria:** Tasks marked `fulfilled`, `in_progress`, or `overdue`. Uptime metrics and penalty credits tracked.
* **Target SLA:** Continuous / Real-time.
* **Audit Requirements:** Log `obligation_status_updated` event with fulfillment evidence.
* **Failure Handling:** Generate high-priority notification to Contract Owner 14 days prior to milestone due date.

### STAGE 23: Procurement Three-Way Invoice Matching
* **Stage ID:** `STAGE-23` | **Macro-Stage:** `7. Post-Execution`
* **Responsible Department:** `FIN`, `PROC`
* **RBAC Permissions:** `procurement:match_invoice`, `accounts_payable`
* **Entry Criteria:** Supplier invoice received against contract PO.
* **Exit Criteria:** System reconciles PO line items, Goods Receipt confirmation, and Supplier Invoice totals within tolerance (±1%).
* **Target SLA:** 2 hours.
* **Audit Requirements:** Log `three_way_match_verified` event with matched amounts and invoice ID.
* **Failure Handling:** Flag `match_discrepancy` and freeze automated payment release if variance exceeds tolerance.

### STAGE 24: Expiration & Renewal Window Notification
* **Stage ID:** `STAGE-24` | **Macro-Stage:** `7. Post-Execution`
* **Responsible Department:** `SALES`, `PROC`, `LEGAL`
* **RBAC Permissions:** `contract:renewal_radar`, `system_monitor`
* **Entry Criteria:** Contract expiration within notice window (90, 60, or 30 days).
* **Exit Criteria:** Dispatch automated renewal alerts to contract owner and business stakeholders.
* **Target SLA:** Scheduled daily scan.
* **Audit Requirements:** Log `renewal_alert_dispatched` event with remaining days and recipient list.
* **Failure Handling:** Escalate to Department Head if no renewal decision made at 30 days remaining.

### STAGE 25: Amendment, Extension & Addenda Management
* **Stage ID:** `STAGE-25` | **Macro-Stage:** `8. Archival`
* **Responsible Department:** `LEGAL`, `PROC`, `SALES`
* **RBAC Permissions:** `contract:amend`, `legal_counsel`
* **Entry Criteria:** Stakeholder requests commercial terms change or extension.
* **Exit Criteria:** Amendment contract drafted, referencing `master_agreement_id`, routing through Stages 11–19.
* **Target SLA:** Standard authoring SLA.
* **Audit Requirements:** Log `amendment_linked` event connecting amendment UUID to parent agreement.
* **Failure Handling:** Prevent amendment creation against expired or terminated parent contracts.

### STAGE 26: Immutable Archival & Litigation Hold
* **Stage ID:** `STAGE-26` | **Macro-Stage:** `8. Archival`
* **Responsible Department:** `LEGAL`, `COMPL`
* **RBAC Permissions:** `contract:archive`, `compliance_officer`
* **Entry Criteria:** Contract expired or terminated; obligations fulfilled.
* **Exit Criteria:** Verification that `litigation_hold_active == false`. Transfer evidence bundle to immutable WORM archive for regulatory retention period (e.g. 7 years).
* **Target SLA:** Instantaneous upon termination approval.
* **Audit Requirements:** Log `contract_archived` event with retention expiration date.
* **Failure Handling:** If `litigation_hold_active == true`, freeze all disposition workflows; prohibit deletion or relocation.

---

## 4. Verification & Consistency Summary

* **Stage Completeness:** All 26 stages are fully accounted for, assigned unique IDs, and mapped to specific business logic.
* **Department Reconciliation:** All 14 departments are integrated with appropriate RBAC roles and approval hierarchies.
* **Zero Missing Stages:** Verified against `docs/07_CONTRACT_LIFECYCLE_ENGINE.md` and `scripts/verify_scenarios_a_to_h.js`.

