const fs = require('fs');
const path = require('path');
const reg = require('../features/registry.js');
const outPath = path.join(__dirname, '../CLM-400-FEATURE-TRACEABILITY.md');

const dbTableMap = {
  'domain-01-core-contract-repository': 'contracts / contract_repository',
  'domain-02-specialized-agreements': 'specialized_agreements / contract_terms',
  'domain-03-legal-operations-financial-terms': 'financial_terms / payment_schedules',
  'domain-04-document-intelligence-infrastructure': 'document_indexes / parsed_tokens',
  'domain-05-advanced-enterprise-clm-capabilities': 'enterprise_contracts / delegation_rules',
  'domain-06-section-a-contract-intake-request-management': 'contract_intake_requests',
  'domain-07-section-b-advanced-contract-authoring': 'contract_clauses / authoring_sessions',
  'domain-08-section-c-advanced-negotiation-management': 'negotiation_sessions / redlines',
  'domain-09-section-d-advanced-legal-operations': 'legal_matters / legal_holds',
  'domain-10-section-e-corporate-procurement-management': 'procurement_vendors / purchase_orders',
  'domain-11-section-f-financial-commercial-management': 'invoices / three_way_matches',
  'domain-12-section-g-advanced-obligation-performance-management': 'contract_obligations / sla_metrics',
  'domain-13-section-h-advanced-risk-compliance': 'risk_assessments / compliance_policies',
  'domain-14-section-i-advanced-contract-intelligence-ai': 'ai_inference_logs / semantic_embeddings',
  'domain-15-section-j-corporate-governance-enterprise-administration': 'governance_policies / delegations',
  'domain-16-section-k-digital-signature-certificate-management': 'digital_signatures / pki_certificates',
  'domain-17-section-l-advanced-repository-document-intelligence': 'document_metadata / ocr_extractions',
  'domain-18-section-m-enterprise-integration-automation': 'integration_webhooks / sync_events',
  'domain-19-section-n-advanced-enterprise-reporting': 'analytics_reports / executive_dashboards',
  'domain-20-section-o-enterprise-security-technical-administration': 'security_access_reviews / audit_events_worm'
};

const rbacMap = {
  'domain-01': 'CLM_LEGAL_USER / DOA Tier 1',
  'domain-02': 'CLM_LEGAL_SPECIALIST / DOA Tier 2',
  'domain-03': 'CLM_FINANCE_ANALYST / DOA Tier 2',
  'domain-04': 'CLM_LEGAL_OPS / DOA Tier 1',
  'domain-05': 'CLM_ENTERPRISE_ADMIN / DOA Tier 3',
  'domain-06': 'CLM_INTAKE_REQUESTER / DOA Tier 1',
  'domain-07': 'CLM_CONTRACT_AUTHOR / DOA Tier 2',
  'domain-08': 'CLM_NEGOTIATOR / DOA Tier 2',
  'domain-09': 'CLM_LEGAL_COUNSEL / DOA Tier 3',
  'domain-10': 'CLM_PROCUREMENT_OFFICER / DOA Tier 2',
  'domain-11': 'CLM_CFO / DOA Tier 4',
  'domain-12': 'CLM_OBLIGATION_MANAGER / DOA Tier 2',
  'domain-13': 'CLM_COMPLIANCE_OFFICER / DOA Tier 3',
  'domain-14': 'CLM_AI_REVIEWER / DOA Tier 2',
  'domain-15': 'CLM_GOVERNANCE_BOARD / DOA Tier 4',
  'domain-16': 'CLM_SIGNATORY / DOA Tier 3',
  'domain-17': 'CLM_DOCUMENT_SPECIALIST / DOA Tier 1',
  'domain-18': 'CLM_SYSTEM_INTEGRATOR / DOA Tier 3',
  'domain-19': 'CLM_EXECUTIVE_LEADER / DOA Tier 4',
  'domain-20': 'CLM_SECURITY_ADMIN / DOA Tier 4'
};

let lines = [];
lines.push('# ENTERPRISE CLM PLATFORM — 400 FEATURE TRACEABILITY MATRIX');
lines.push('');
lines.push('**Document Version:** 4.0.0-ENTERPRISE  ');
lines.push('**Registry inventory:** ' + reg.features.length + ' entries; feature implementation and business capability are not assessed by this report.  ');
lines.push('**Target frameworks:** ISO/IEC 27001:2022, SOC 2, NIST SP 800-53, GDPR, 21 CFR Part 11 (mapping is not certification evidence).  ');
lines.push('**Generated at:** ' + new Date().toISOString() + '  ');
lines.push('');
lines.push('This generated catalog maps feature registrations to intended domain, target schema labels, module paths, and UI location. These mappings do not prove that the schemas, authorization rules, business behavior, or integrations are implemented or verified.');
lines.push('');
lines.push("The current `test:400` suite checks registry wrappers and request validation only. It does not execute or verify feature business capabilities, and its generated execution digest is not persisted or hash-chained evidence.");
lines.push('');
lines.push('---');
lines.push('');
lines.push('## 1. OPERATIONAL DOMAIN SUMMARY');
lines.push('');
lines.push('| Operational Domain | Registered Features | Target DB Schema Label (not live-verified) | Feature Verification |');
lines.push('|:---|:---:|:---|:---:|');

const domainStats = {};
reg.features.forEach(f => {
  domainStats[f.domain] = domainStats[f.domain] || { count: 0, domainId: f.domainId };
  domainStats[f.domain].count++;
});

for (const [domName, val] of Object.entries(domainStats)) {
  const table = dbTableMap[val.domainId] || 'contracts';
  lines.push(`| ${domName} | ${val.count} | \`${table}\` | **NOT ASSESSED** |`);
}

lines.push(`| **TOTAL ALL 20 OPERATIONAL DOMAINS** | **${reg.features.length}** | **Target mappings only** | **NOT ASSESSED** |`);
lines.push('');
lines.push('---');
lines.push('');
lines.push('## 2. COMPLETE 400-FEATURE TRACEABILITY TABLE');
lines.push('');
lines.push('| # | Code | Feature Name | Operational Domain | DB Table | Service Handler | UI View | RBAC & DOA Tier | Status |');
lines.push('|---:|:---|:---|:---|:---|:---|:---|:---|:---:|');

reg.features.forEach(f => {
  const domNum = f.domainId.split('-')[1];
  const rbac = rbacMap['domain-' + domNum] || 'CLM_LEGAL_USER / DOA Tier 1';
  const dbTable = dbTableMap[f.domainId] || 'contracts';
  const handler = `CLM/features/${f.folder}/index.js`;
  const cleanName = f.name.replace(/\|/g, '-');
  lines.push(`| ${f.id} | \`${f.code}\` | **${cleanName}** | ${f.domain} | \`${dbTable}\` | [\`${f.code}\`](file:///${handler}) | \`#explorer-tab\` | ${rbac} | \`NOT_ASSESSED\` |`);
});

lines.push('');
lines.push('---');
lines.push('');
lines.push('### 3. TRACEABILITY ASSURANCE DECLARATION');
lines.push('');
lines.push('Each row corresponds to a catalog registration and module path. The current module and domain-engine smoke path is not a feature-specific business implementation; registry execution is not persistent evidence and does not establish tenant isolation, compliance, or production readiness.');
lines.push('');
lines.push('**Evidence status:** No feature-level evidence is recorded by this report.  ');
lines.push('**Generated at:** ' + new Date().toISOString());

const output = lines.join('\n');
fs.writeFileSync(outPath, output, 'utf8');
console.log('Successfully wrote CLM-400-FEATURE-TRACEABILITY.md with', reg.features.length, 'features.');
