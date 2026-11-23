# Implementation Plan: CLM 400-Feature Full Implementation

## Overview
Implement all 237 missing features across 20 domains to achieve 400/400 feature coverage. Each task adds real API endpoints to dev-server.js and corresponding UI pages to the portal files, then syncs public/ to components/.

## Tasks

- [x] 1. Contract Intake Management — Add contract request lifecycle endpoints to dev-server.js: dynamic forms by type, queue with filters, stats/SLA tracking, assign, escalate, cancel, reassign, duplicate detection, validation, single request detail, PATCH update. Enhance loadContractRequests() in public/app.js with queue table, SLA countdown, priority badges, assignment, escalation buttons. Sync to components/saas-control-plane/web/admin-portal/app.js.
- [x] 2. Document Intelligence and Authoring — Add document management endpoints to dev-server.js: list, upload with dedup, detail, versions, OCR, data extraction, classify, full-text search, tag, annotate, watermark, retention, archive, folders, template approval, template versions, clause approval, clause alternatives. Add loadDocuments() to public/app.js with folder tree, search, upload, detail drawer. Add nav item to index.html. Sync.
- [x] 3. Procurement and Supplier Management — Add supplier lifecycle endpoints: requisition, purchase-order, prequalification, onboard, offboard, scorecard, performance-eval, spend-analysis, insurance, licenses, cert-expiry, diversity, risk-mapping. Enhance loadProcurementSuppliers() with scorecard bars, spend chart, expiry badges. Sync.
- [x] 4. Financial Management — Add financial endpoints: pricing, pricing-schedule, price-escalation, discount, rebate, tax, currency-exchange, revenue-recognition, milestone-payment, invoice-matching, commitments, cost-forecasting, profitability, budget-allocation, reconciliation, variance, forecasting, cost-optimization. Enhance loadContractFinancials() with tabs. Sync.
- [x] 5. Obligation and Performance Management — Add obligation lifecycle endpoints: extract from contract, assign owner, calendar view, dependencies, evidence collection, verify completion, breach detection, milestones, deliverable acceptance, KPIs, service credits, warranties, performance guarantees, value realization, post-execution compliance. Enhance loadContractObligations() with calendar, milestones, KPIs. Sync.
- [x] 6. Risk Compliance and Governance — Add risk assessment, mitigation, acceptance, exception, regulatory changes, GDPR, sanctions, anti-bribery, COI, cybersecurity assessment, DPA, cross-border, evidence repository, DOA matrix, board resolutions, entity hierarchy, regulatory filings, annual compliance endpoints. Add loadRiskCompliance() and loadGovernance() UI pages with nav items. Sync.
- [x] 7. AI Contract Intelligence — Add AI endpoints: classify, extract-metadata, extract-obligations, extract-entities, extract-dates, extract-financial, deviation, summarize, predict-risk, benchmark, similarity, anomaly, NL search, generate, portfolio-intelligence, leakage-detection, compliance-check, confidence-score, autonomous-workflows, benchmarking. Enhance loadAIContracts() with extraction results, summarization, risk prediction, NL search. Sync.
- [x] 8. Enterprise Integrations — Add integration endpoints for SAP, Oracle, Salesforce, Coupa, ServiceNow, Workday, Slack, Email with config plus sync status plus error log. Add monitor and health check. Enhance loadIntegrations() with integration cards, sync trigger, health dashboard. Sync.
- [x] 9. Advanced Reporting and Security — Add reporting endpoints: cycle-time, bottleneck, renewal-forecast, counterparty-spend, spend-analytics, value-leakage, portfolio-health, executive. Add security endpoints: IP-allowlist, devices, privileged-access, ABAC, data-masking, DLP, incidents, events, backup, DR, data-residency, API-keys, secrets, access-review. Add loadAdvancedReports() and loadSecurityCenter() UI pages with nav items. Sync.
- [x] 10. Negotiation Legal Ops and Specialized Agreements — Add negotiation history, rounds, deadlines, chat, analytics. Add legal matter intake, assignment, cases, workload, spend, research, knowledge. Add disputes, escalations, tasks, budgets, vendor-risk, third-party-risk. Add specialized agreements by type. Enhance negotiation and legal UI. Sync.
- [x] 11. Core Contract Gaps and Advanced CLM — Add contract calendar, classification, categorization, AI summarization, AI generation, AI compliance, contract intelligence dashboard, autonomous workflows, benchmarking, portfolio management, vendor performance, leakage detection, lifecycle automation, global management endpoints. Add calendar view to contracts page, AI generate button. Sync.
- [x] 12. Test suite sync commit push deploy — Create scripts/test_400_implementation.js testing all new endpoints. Add test:400-impl npm script. Run all tests. Sync public to components. Commit all changes. Push to cmd HEAD:main and origin HEAD:main.

## Task Dependency Graph

```json
{
  "waves": [
    {
      "wave": 1,
      "tasks": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11]
    },
    {
      "wave": 2,
      "tasks": [12]
    }
  ],
  "dependencies": {
    "12": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11]
  }
}
```

## Notes

- All API endpoints go in dev-server.js BEFORE the return false at end of handleMockAPI function
- Use var not let/const inside handler to match existing code style
- POST endpoints must read body with req.on data/end pattern
- Use existing mock data arrays (mockContracts, mockSuppliers, mockObligations, etc.)
- UI functions must use existing apiGet/apiPost/apiPut/apiDelete helpers
- New UI pages must be added to navigate() switch in app.js
- New nav items in index.html use onclick=navigate pattern
- After all changes verify syntax with node -e require dev-server.js and node --check public/app.js
- PowerShell uses semicolons not double-ampersand for chaining
- Use Set-Content with UTF8 encoding for file writes
- After changes to public/app.js sync to components/saas-control-plane/web/admin-portal/app.js
- After changes to public/gateway/app.js sync to components/satellite-gateway/web/gateway-ui/app.js
- Git remotes: cmd is https://github.com/GOKUL-THAN/cmd.git (Vercel), origin is https://github.com/GOKUL-THAN/clm.git

