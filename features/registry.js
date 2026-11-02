/**
 * Master Registry for all 400 CLM Enterprise Features
 */
const fs = require('fs');
const path = require('path');

const masterFeatures = [
  {
    "id": 1,
    "code": "CLM-REQ-001",
    "name": "Contract Repository Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/001-contract-repository-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 2,
    "code": "CLM-REQ-002",
    "name": "Contract Request Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/002-contract-request-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 3,
    "code": "CLM-REQ-003",
    "name": "Contract Creation",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/003-contract-creation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 4,
    "code": "CLM-REQ-004",
    "name": "Contract Drafting",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/004-contract-drafting",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 5,
    "code": "CLM-REQ-005",
    "name": "Contract Templates Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/005-contract-templates-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 6,
    "code": "CLM-REQ-006",
    "name": "Clause Library Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/006-clause-library-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 7,
    "code": "CLM-REQ-007",
    "name": "Clause Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/007-clause-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 8,
    "code": "CLM-REQ-008",
    "name": "Contract Authoring",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/008-contract-authoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 9,
    "code": "CLM-REQ-009",
    "name": "Contract Negotiation",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/009-contract-negotiation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 10,
    "code": "CLM-REQ-010",
    "name": "Contract Collaboration",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/010-contract-collaboration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 11,
    "code": "CLM-REQ-011",
    "name": "Contract Review",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/011-contract-review",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 12,
    "code": "CLM-REQ-012",
    "name": "Contract Approval Workflow",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/012-contract-approval-workflow",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 13,
    "code": "CLM-REQ-013",
    "name": "Contract Signing",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/013-contract-signing",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 14,
    "code": "CLM-REQ-014",
    "name": "Electronic Signature (E-Signature)",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/014-electronic-signature-e-signature",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 15,
    "code": "CLM-REQ-015",
    "name": "Digital Signature",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/015-digital-signature",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 16,
    "code": "CLM-REQ-016",
    "name": "Contract Execution",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/016-contract-execution",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 17,
    "code": "CLM-REQ-017",
    "name": "Contract Version Control",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/017-contract-version-control",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 18,
    "code": "CLM-REQ-018",
    "name": "Contract Amendment Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/018-contract-amendment-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 19,
    "code": "CLM-REQ-019",
    "name": "Contract Renewal Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/019-contract-renewal-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 20,
    "code": "CLM-REQ-020",
    "name": "Contract Expiration Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/020-contract-expiration-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 21,
    "code": "CLM-REQ-021",
    "name": "Contract Termination Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/021-contract-termination-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 22,
    "code": "CLM-REQ-022",
    "name": "Contract Obligation Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/022-contract-obligation-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 23,
    "code": "CLM-REQ-023",
    "name": "Contract Compliance Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/023-contract-compliance-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 24,
    "code": "CLM-REQ-024",
    "name": "Contract Risk Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/024-contract-risk-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 25,
    "code": "CLM-REQ-025",
    "name": "Contract Performance Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/025-contract-performance-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 26,
    "code": "CLM-REQ-026",
    "name": "Contract Monitoring",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/026-contract-monitoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 27,
    "code": "CLM-REQ-027",
    "name": "Contract Tracking",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/027-contract-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 28,
    "code": "CLM-REQ-028",
    "name": "Contract Alerts & Notifications",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/028-contract-alerts-notifications",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 29,
    "code": "CLM-REQ-029",
    "name": "Contract Calendar",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/029-contract-calendar",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 30,
    "code": "CLM-REQ-030",
    "name": "Contract Search",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/030-contract-search",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 31,
    "code": "CLM-REQ-031",
    "name": "Advanced Contract Search",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/031-advanced-contract-search",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 32,
    "code": "CLM-REQ-032",
    "name": "Contract Analytics",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/032-contract-analytics",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 33,
    "code": "CLM-REQ-033",
    "name": "Contract Reporting",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/033-contract-reporting",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 34,
    "code": "CLM-REQ-034",
    "name": "Contract Dashboard",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/034-contract-dashboard",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 35,
    "code": "CLM-REQ-035",
    "name": "Contract Audit Trail",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/035-contract-audit-trail",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 36,
    "code": "CLM-REQ-036",
    "name": "Contract History Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/036-contract-history-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 37,
    "code": "CLM-REQ-037",
    "name": "Contract Metadata Management",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/037-contract-metadata-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 38,
    "code": "CLM-REQ-038",
    "name": "Contract Classification",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/038-contract-classification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 39,
    "code": "CLM-REQ-039",
    "name": "Contract Categorization",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/039-contract-categorization",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 40,
    "code": "CLM-REQ-040",
    "name": "Contract Data Extraction",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/040-contract-data-extraction",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 41,
    "code": "CLM-REQ-041",
    "name": "AI-Powered Contract Analysis",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/041-ai-powered-contract-analysis",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 42,
    "code": "CLM-REQ-042",
    "name": "AI Contract Summarization",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/042-ai-contract-summarization",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 43,
    "code": "CLM-REQ-043",
    "name": "AI Risk Detection",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/043-ai-risk-detection",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 44,
    "code": "CLM-REQ-044",
    "name": "AI Clause Identification",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/044-ai-clause-identification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 45,
    "code": "CLM-REQ-045",
    "name": "AI Contract Comparison",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/045-ai-contract-comparison",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 46,
    "code": "CLM-REQ-046",
    "name": "AI Contract Generation",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/046-ai-contract-generation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 47,
    "code": "CLM-REQ-047",
    "name": "AI Compliance Checking",
    "domain": "Core Contract Repository & Lifecycle",
    "domainId": "domain-01-core-contract-repository",
    "folder": "domain-01-core-contract-repository/047-ai-compliance-checking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 48,
    "code": "CLM-REQ-048",
    "name": "Vendor Contract Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/048-vendor-contract-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 49,
    "code": "CLM-REQ-049",
    "name": "Customer Contract Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/049-customer-contract-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 50,
    "code": "CLM-REQ-050",
    "name": "Supplier Agreement Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/050-supplier-agreement-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 51,
    "code": "CLM-REQ-051",
    "name": "Employee Agreement Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/051-employee-agreement-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 52,
    "code": "CLM-REQ-052",
    "name": "NDA Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/052-nda-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 53,
    "code": "CLM-REQ-053",
    "name": "MSA Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/053-msa-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 54,
    "code": "CLM-REQ-054",
    "name": "SOW Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/054-sow-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 55,
    "code": "CLM-REQ-055",
    "name": "SLA Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/055-sla-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 56,
    "code": "CLM-REQ-056",
    "name": "Purchase Agreement Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/056-purchase-agreement-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 57,
    "code": "CLM-REQ-057",
    "name": "Sales Agreement Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/057-sales-agreement-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 58,
    "code": "CLM-REQ-058",
    "name": "Employment Contract Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/058-employment-contract-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 59,
    "code": "CLM-REQ-059",
    "name": "Lease Agreement Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/059-lease-agreement-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 60,
    "code": "CLM-REQ-060",
    "name": "Partnership Agreement Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/060-partnership-agreement-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 61,
    "code": "CLM-REQ-061",
    "name": "Licensing Agreement Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/061-licensing-agreement-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 62,
    "code": "CLM-REQ-062",
    "name": "Service Agreement Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/062-service-agreement-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 63,
    "code": "CLM-REQ-063",
    "name": "Procurement Contract Management",
    "domain": "Specialized Agreement Portfolio",
    "domainId": "domain-02-specialized-agreements",
    "folder": "domain-02-specialized-agreements/063-procurement-contract-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 64,
    "code": "CLM-REQ-064",
    "name": "Legal Matter Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/064-legal-matter-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 65,
    "code": "CLM-REQ-065",
    "name": "Dispute Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/065-dispute-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 66,
    "code": "CLM-REQ-066",
    "name": "Obligation Tracking",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/066-obligation-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 67,
    "code": "CLM-REQ-067",
    "name": "Financial Terms Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/067-financial-terms-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 68,
    "code": "CLM-REQ-068",
    "name": "Payment Terms Tracking",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/068-payment-terms-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 69,
    "code": "CLM-REQ-069",
    "name": "Contract Budget Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/069-contract-budget-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 70,
    "code": "CLM-REQ-070",
    "name": "Spend Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/070-spend-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 71,
    "code": "CLM-REQ-071",
    "name": "Vendor Risk Assessment",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/071-vendor-risk-assessment",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 72,
    "code": "CLM-REQ-072",
    "name": "Third-Party Risk Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/072-third-party-risk-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 73,
    "code": "CLM-REQ-073",
    "name": "Regulatory Compliance",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/073-regulatory-compliance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 74,
    "code": "CLM-REQ-074",
    "name": "Policy Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/074-policy-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 75,
    "code": "CLM-REQ-075",
    "name": "Delegation of Authority (DOA)",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/075-delegation-of-authority-doa",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 76,
    "code": "CLM-REQ-076",
    "name": "Role-Based Access Control (RBAC)",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/076-role-based-access-control-rbac",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 77,
    "code": "CLM-REQ-077",
    "name": "User Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/077-user-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 78,
    "code": "CLM-REQ-078",
    "name": "Team Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/078-team-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 79,
    "code": "CLM-REQ-079",
    "name": "Department Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/079-department-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 80,
    "code": "CLM-REQ-080",
    "name": "Multi-Level Approval Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/080-multi-level-approval-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 81,
    "code": "CLM-REQ-081",
    "name": "Workflow Automation",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/081-workflow-automation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 82,
    "code": "CLM-REQ-082",
    "name": "Escalation Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/082-escalation-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 83,
    "code": "CLM-REQ-083",
    "name": "Task Management",
    "domain": "Legal Operations & Financial Terms",
    "domainId": "domain-03-legal-operations-financial-terms",
    "folder": "domain-03-legal-operations-financial-terms/083-task-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 84,
    "code": "CLM-REQ-084",
    "name": "Document Management",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/084-document-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 85,
    "code": "CLM-REQ-085",
    "name": "Document Versioning",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/085-document-versioning",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 86,
    "code": "CLM-REQ-086",
    "name": "Document Comparison",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/086-document-comparison",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 87,
    "code": "CLM-REQ-087",
    "name": "Document Upload & Download",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/087-document-upload-download",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 88,
    "code": "CLM-REQ-088",
    "name": "OCR Document Processing",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/088-ocr-document-processing",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 89,
    "code": "CLM-REQ-089",
    "name": "Email Integration",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/089-email-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 90,
    "code": "CLM-REQ-090",
    "name": "ERP Integration",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/090-erp-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 91,
    "code": "CLM-REQ-091",
    "name": "CRM Integration",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/091-crm-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 92,
    "code": "CLM-REQ-092",
    "name": "HRMS Integration",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/092-hrms-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 93,
    "code": "CLM-REQ-093",
    "name": "Procurement Integration",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/093-procurement-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 94,
    "code": "CLM-REQ-094",
    "name": "API Management",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/094-api-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 95,
    "code": "CLM-REQ-095",
    "name": "Webhooks",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/095-webhooks",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 96,
    "code": "CLM-REQ-096",
    "name": "Single Sign-On (SSO)",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/096-single-sign-on-sso",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 97,
    "code": "CLM-REQ-097",
    "name": "Multi-Factor Authentication (MFA)",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/097-multi-factor-authentication-mfa",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 98,
    "code": "CLM-REQ-098",
    "name": "Data Encryption",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/098-data-encryption",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 99,
    "code": "CLM-REQ-099",
    "name": "Security Management",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/099-security-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 100,
    "code": "CLM-REQ-100",
    "name": "Audit & Governance",
    "domain": "Document Intelligence & System Integration",
    "domainId": "domain-04-document-intelligence-infrastructure",
    "folder": "domain-04-document-intelligence-infrastructure/100-audit-governance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 101,
    "code": "CLM-REQ-101",
    "name": "Contract Intelligence",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/101-contract-intelligence",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 102,
    "code": "CLM-REQ-102",
    "name": "Autonomous Contract Workflows",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/102-autonomous-contract-workflows",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 103,
    "code": "CLM-REQ-103",
    "name": "AI Legal Assistant",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/103-ai-legal-assistant",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 104,
    "code": "CLM-REQ-104",
    "name": "Generative AI Contract Drafting",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/104-generative-ai-contract-drafting",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 105,
    "code": "CLM-REQ-105",
    "name": "Natural Language Contract Search",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/105-natural-language-contract-search",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 106,
    "code": "CLM-REQ-106",
    "name": "Contract Benchmarking",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/106-contract-benchmarking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 107,
    "code": "CLM-REQ-107",
    "name": "Risk Scoring Engine",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/107-risk-scoring-engine",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 108,
    "code": "CLM-REQ-108",
    "name": "Obligation Intelligence",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/108-obligation-intelligence",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 109,
    "code": "CLM-REQ-109",
    "name": "Contract Portfolio Management",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/109-contract-portfolio-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 110,
    "code": "CLM-REQ-110",
    "name": "Contract Repository Migration",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/110-contract-repository-migration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 111,
    "code": "CLM-REQ-111",
    "name": "Supplier Onboarding",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/111-supplier-onboarding",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 112,
    "code": "CLM-REQ-112",
    "name": "Vendor Performance Monitoring",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/112-vendor-performance-monitoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 113,
    "code": "CLM-REQ-113",
    "name": "Contract Leakage Detection",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/113-contract-leakage-detection",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 114,
    "code": "CLM-REQ-114",
    "name": "Revenue Leakage Detection",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/114-revenue-leakage-detection",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 115,
    "code": "CLM-REQ-115",
    "name": "Contract Lifecycle Automation",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/115-contract-lifecycle-automation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 116,
    "code": "CLM-REQ-116",
    "name": "Global Contract Management",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/116-global-contract-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 117,
    "code": "CLM-REQ-117",
    "name": "Multi-Language Contract Support",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/117-multi-language-contract-support",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 118,
    "code": "CLM-REQ-118",
    "name": "Multi-Currency Support",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/118-multi-currency-support",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 119,
    "code": "CLM-REQ-119",
    "name": "Multi-Entity Contract Management",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/119-multi-entity-contract-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 120,
    "code": "CLM-REQ-120",
    "name": "Contract Self-Service Portal",
    "domain": "Advanced Enterprise CLM Capabilities",
    "domainId": "domain-05-advanced-enterprise-clm-capabilities",
    "folder": "domain-05-advanced-enterprise-clm-capabilities/120-contract-self-service-portal",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 121,
    "code": "CLM-REQ-121",
    "name": "Contract Intake Portal",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/121-contract-intake-portal",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 122,
    "code": "CLM-REQ-122",
    "name": "Contract Request Forms",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/122-contract-request-forms",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 123,
    "code": "CLM-REQ-123",
    "name": "Dynamic Request Forms",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/123-dynamic-request-forms",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 124,
    "code": "CLM-REQ-124",
    "name": "Contract Request Classification",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/124-contract-request-classification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 125,
    "code": "CLM-REQ-125",
    "name": "Request Prioritization",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/125-request-prioritization",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 126,
    "code": "CLM-REQ-126",
    "name": "Request Assignment",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/126-request-assignment",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 127,
    "code": "CLM-REQ-127",
    "name": "Request Queue Management",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/127-request-queue-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 128,
    "code": "CLM-REQ-128",
    "name": "Request Status Tracking",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/128-request-status-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 129,
    "code": "CLM-REQ-129",
    "name": "Legal Request Management",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/129-legal-request-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 130,
    "code": "CLM-REQ-130",
    "name": "Business Request Management",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/130-business-request-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 131,
    "code": "CLM-REQ-131",
    "name": "Self-Service Contract Requests",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/131-self-service-contract-requests",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 132,
    "code": "CLM-REQ-132",
    "name": "Contract Request Validation",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/132-contract-request-validation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 133,
    "code": "CLM-REQ-133",
    "name": "Duplicate Request Detection",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/133-duplicate-request-detection",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 134,
    "code": "CLM-REQ-134",
    "name": "Request SLA Management",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/134-request-sla-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 135,
    "code": "CLM-REQ-135",
    "name": "Request Escalation",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/135-request-escalation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 136,
    "code": "CLM-REQ-136",
    "name": "Contract Request Cancellation",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/136-contract-request-cancellation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 137,
    "code": "CLM-REQ-137",
    "name": "Contract Request Reassignment",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/137-contract-request-reassignment",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 138,
    "code": "CLM-REQ-138",
    "name": "Request-to-Contract Conversion",
    "domain": "Section A: Contract Intake & Request Management",
    "domainId": "domain-06-section-a-contract-intake-request-management",
    "folder": "domain-06-section-a-contract-intake-request-management/138-request-to-contract-conversion",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 139,
    "code": "CLM-REQ-139",
    "name": "Conditional Document Generation",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/139-conditional-document-generation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 140,
    "code": "CLM-REQ-140",
    "name": "Dynamic Clause Insertion",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/140-dynamic-clause-insertion",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 141,
    "code": "CLM-REQ-141",
    "name": "Automated Document Assembly",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/141-automated-document-assembly",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 142,
    "code": "CLM-REQ-142",
    "name": "Contract Data Mapping",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/142-contract-data-mapping",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 143,
    "code": "CLM-REQ-143",
    "name": "Contract Field Validation",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/143-contract-field-validation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 144,
    "code": "CLM-REQ-144",
    "name": "Template Approval",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/144-template-approval",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 145,
    "code": "CLM-REQ-145",
    "name": "Template Version Governance",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/145-template-version-governance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 146,
    "code": "CLM-REQ-146",
    "name": "Clause Approval Workflow",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/146-clause-approval-workflow",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 147,
    "code": "CLM-REQ-147",
    "name": "Clause Alternative Management",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/147-clause-alternative-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 148,
    "code": "CLM-REQ-148",
    "name": "Clause Fallback Management",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/148-clause-fallback-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 149,
    "code": "CLM-REQ-149",
    "name": "Contract Playbook Management",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/149-contract-playbook-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 150,
    "code": "CLM-REQ-150",
    "name": "Negotiation Playbook",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/150-negotiation-playbook",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 151,
    "code": "CLM-REQ-151",
    "name": "Legal Language Standardization",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/151-legal-language-standardization",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 152,
    "code": "CLM-REQ-152",
    "name": "Contract Language Translation",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/152-contract-language-translation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 153,
    "code": "CLM-REQ-153",
    "name": "Contract Document Watermarking",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/153-contract-document-watermarking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 154,
    "code": "CLM-REQ-154",
    "name": "Contract Generation Rules Engine",
    "domain": "Section B: Advanced Contract Authoring",
    "domainId": "domain-07-section-b-advanced-contract-authoring",
    "folder": "domain-07-section-b-advanced-contract-authoring/154-contract-generation-rules-engine",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 155,
    "code": "CLM-REQ-155",
    "name": "Counterparty Collaboration Portal",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/155-counterparty-collaboration-portal",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 156,
    "code": "CLM-REQ-156",
    "name": "External Negotiation Workspace",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/156-external-negotiation-workspace",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 157,
    "code": "CLM-REQ-157",
    "name": "Redline Management",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/157-redline-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 158,
    "code": "CLM-REQ-158",
    "name": "Redline Comparison",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/158-redline-comparison",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 159,
    "code": "CLM-REQ-159",
    "name": "Negotiation Comment Tracking",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/159-negotiation-comment-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 160,
    "code": "CLM-REQ-160",
    "name": "Negotiation History",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/160-negotiation-history",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 161,
    "code": "CLM-REQ-161",
    "name": "Negotiation Round Management",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/161-negotiation-round-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 162,
    "code": "CLM-REQ-162",
    "name": "Negotiation Status Dashboard",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/162-negotiation-status-dashboard",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 163,
    "code": "CLM-REQ-163",
    "name": "Negotiation Deadline Tracking",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/163-negotiation-deadline-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 164,
    "code": "CLM-REQ-164",
    "name": "Counterparty Change Tracking",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/164-counterparty-change-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 165,
    "code": "CLM-REQ-165",
    "name": "Negotiation Approval Rules",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/165-negotiation-approval-rules",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 166,
    "code": "CLM-REQ-166",
    "name": "Negotiation Risk Identification",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/166-negotiation-risk-identification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 167,
    "code": "CLM-REQ-167",
    "name": "Negotiation Outcome Analytics",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/167-negotiation-outcome-analytics",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 168,
    "code": "CLM-REQ-168",
    "name": "Contract Negotiation Chat",
    "domain": "Section C: Advanced Negotiation Management",
    "domainId": "domain-08-section-c-advanced-negotiation-management",
    "folder": "domain-08-section-c-advanced-negotiation-management/168-contract-negotiation-chat",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 169,
    "code": "CLM-REQ-169",
    "name": "Legal Matter Intake",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/169-legal-matter-intake",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 170,
    "code": "CLM-REQ-170",
    "name": "Legal Matter Assignment",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/170-legal-matter-assignment",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 171,
    "code": "CLM-REQ-171",
    "name": "Legal Case Tracking",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/171-legal-case-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 172,
    "code": "CLM-REQ-172",
    "name": "Legal Team Workload Management",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/172-legal-team-workload-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 173,
    "code": "CLM-REQ-173",
    "name": "Outside Counsel Management",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/173-outside-counsel-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 174,
    "code": "CLM-REQ-174",
    "name": "Legal Spend Tracking",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/174-legal-spend-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 175,
    "code": "CLM-REQ-175",
    "name": "Legal Document Privilege Management",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/175-legal-document-privilege-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 176,
    "code": "CLM-REQ-176",
    "name": "Legal Hold Management",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/176-legal-hold-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 177,
    "code": "CLM-REQ-177",
    "name": "Litigation Support",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/177-litigation-support",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 178,
    "code": "CLM-REQ-178",
    "name": "Legal Opinion Management",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/178-legal-opinion-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 179,
    "code": "CLM-REQ-179",
    "name": "Legal Research Repository",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/179-legal-research-repository",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 180,
    "code": "CLM-REQ-180",
    "name": "Legal Knowledge Management",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/180-legal-knowledge-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 181,
    "code": "CLM-REQ-181",
    "name": "Legal Department Performance Analytics",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/181-legal-department-performance-analytics",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 182,
    "code": "CLM-REQ-182",
    "name": "Legal Service Request Management",
    "domain": "Section D: Advanced Legal Operations",
    "domainId": "domain-09-section-d-advanced-legal-operations",
    "folder": "domain-09-section-d-advanced-legal-operations/182-legal-service-request-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 183,
    "code": "CLM-REQ-183",
    "name": "Purchase Requisition Integration",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/183-purchase-requisition-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 184,
    "code": "CLM-REQ-184",
    "name": "Purchase Order Integration",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/184-purchase-order-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 185,
    "code": "CLM-REQ-185",
    "name": "Source-to-Contract Management",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/185-source-to-contract-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 186,
    "code": "CLM-REQ-186",
    "name": "Supplier Selection Management",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/186-supplier-selection-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 187,
    "code": "CLM-REQ-187",
    "name": "Supplier Prequalification",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/187-supplier-prequalification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 188,
    "code": "CLM-REQ-188",
    "name": "Supplier Due Diligence",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/188-supplier-due-diligence",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 189,
    "code": "CLM-REQ-189",
    "name": "Supplier Contract Negotiation",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/189-supplier-contract-negotiation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 190,
    "code": "CLM-REQ-190",
    "name": "Supplier Onboarding",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/190-supplier-onboarding",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 191,
    "code": "CLM-REQ-191",
    "name": "Supplier Offboarding",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/191-supplier-offboarding",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 192,
    "code": "CLM-REQ-192",
    "name": "Supplier Contract Scorecards",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/192-supplier-contract-scorecards",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 193,
    "code": "CLM-REQ-193",
    "name": "Supplier Performance Evaluation",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/193-supplier-performance-evaluation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 194,
    "code": "CLM-REQ-194",
    "name": "Supplier Spend Analysis",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/194-supplier-spend-analysis",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 195,
    "code": "CLM-REQ-195",
    "name": "Supplier Relationship Management",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/195-supplier-relationship-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 196,
    "code": "CLM-REQ-196",
    "name": "Supplier Insurance Certificate Tracking",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/196-supplier-insurance-certificate-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 197,
    "code": "CLM-REQ-197",
    "name": "Supplier License Tracking",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/197-supplier-license-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 198,
    "code": "CLM-REQ-198",
    "name": "Supplier Certification Expiry Alerts",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/198-supplier-certification-expiry-alerts",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 199,
    "code": "CLM-REQ-199",
    "name": "Supplier Contract Compliance",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/199-supplier-contract-compliance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 200,
    "code": "CLM-REQ-200",
    "name": "Supplier Diversity Tracking",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/200-supplier-diversity-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 201,
    "code": "CLM-REQ-201",
    "name": "Multi-Tier Supplier Risk Mapping",
    "domain": "Section E: Corporate Procurement Management",
    "domainId": "domain-10-section-e-corporate-procurement-management",
    "folder": "domain-10-section-e-corporate-procurement-management/201-multi-tier-supplier-risk-mapping",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 202,
    "code": "CLM-REQ-202",
    "name": "Contract Pricing Management",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/202-contract-pricing-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 203,
    "code": "CLM-REQ-203",
    "name": "Pricing Schedule Management",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/203-pricing-schedule-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 204,
    "code": "CLM-REQ-204",
    "name": "Price Escalation Management",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/204-price-escalation-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 205,
    "code": "CLM-REQ-205",
    "name": "Discount Management",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/205-discount-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 206,
    "code": "CLM-REQ-206",
    "name": "Rebate Management",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/206-rebate-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 207,
    "code": "CLM-REQ-207",
    "name": "Tax Management",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/207-tax-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 208,
    "code": "CLM-REQ-208",
    "name": "Currency Exchange Management",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/208-currency-exchange-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 209,
    "code": "CLM-REQ-209",
    "name": "Contract Revenue Recognition Integration",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/209-contract-revenue-recognition-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 210,
    "code": "CLM-REQ-210",
    "name": "Milestone Payment Management",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/210-milestone-payment-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 211,
    "code": "CLM-REQ-211",
    "name": "Invoice Matching",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/211-invoice-matching",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 212,
    "code": "CLM-REQ-212",
    "name": "Three-Way Matching Integration",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/212-three-way-matching-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 213,
    "code": "CLM-REQ-213",
    "name": "Financial Commitment Tracking",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/213-financial-commitment-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 214,
    "code": "CLM-REQ-214",
    "name": "Contract Cost Forecasting",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/214-contract-cost-forecasting",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 215,
    "code": "CLM-REQ-215",
    "name": "Contract Profitability Analysis",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/215-contract-profitability-analysis",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 216,
    "code": "CLM-REQ-216",
    "name": "Budget Allocation",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/216-budget-allocation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 217,
    "code": "CLM-REQ-217",
    "name": "Payment Reconciliation",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/217-payment-reconciliation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 218,
    "code": "CLM-REQ-218",
    "name": "Financial Variance Analysis",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/218-financial-variance-analysis",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 219,
    "code": "CLM-REQ-219",
    "name": "Contract Financial Forecasting",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/219-contract-financial-forecasting",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 220,
    "code": "CLM-REQ-220",
    "name": "Contract Cost Optimization",
    "domain": "Section F: Financial & Commercial Management",
    "domainId": "domain-11-section-f-financial-commercial-management",
    "folder": "domain-11-section-f-financial-commercial-management/220-contract-cost-optimization",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 221,
    "code": "CLM-REQ-221",
    "name": "Obligation Creation from Contract",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/221-obligation-creation-from-contract",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 222,
    "code": "CLM-REQ-222",
    "name": "Obligation Ownership Assignment",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/222-obligation-ownership-assignment",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 223,
    "code": "CLM-REQ-223",
    "name": "Obligation Calendar",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/223-obligation-calendar",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 224,
    "code": "CLM-REQ-224",
    "name": "Obligation Dependency Management",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/224-obligation-dependency-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 225,
    "code": "CLM-REQ-225",
    "name": "Obligation Evidence Collection",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/225-obligation-evidence-collection",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 226,
    "code": "CLM-REQ-226",
    "name": "Obligation Completion Verification",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/226-obligation-completion-verification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 227,
    "code": "CLM-REQ-227",
    "name": "Obligation Breach Detection",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/227-obligation-breach-detection",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 228,
    "code": "CLM-REQ-228",
    "name": "Contract Milestone Management",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/228-contract-milestone-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 229,
    "code": "CLM-REQ-229",
    "name": "Deliverable Acceptance Management",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/229-deliverable-acceptance-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 230,
    "code": "CLM-REQ-230",
    "name": "Service Performance Tracking",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/230-service-performance-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 231,
    "code": "CLM-REQ-231",
    "name": "SLA Breach Management",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/231-sla-breach-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 232,
    "code": "CLM-REQ-232",
    "name": "KPI Monitoring",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/232-kpi-monitoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 233,
    "code": "CLM-REQ-233",
    "name": "Penalty Calculation",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/233-penalty-calculation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 234,
    "code": "CLM-REQ-234",
    "name": "Service Credit Management",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/234-service-credit-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 235,
    "code": "CLM-REQ-235",
    "name": "Warranty Tracking",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/235-warranty-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 236,
    "code": "CLM-REQ-236",
    "name": "Insurance Obligation Tracking",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/236-insurance-obligation-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 237,
    "code": "CLM-REQ-237",
    "name": "Performance Guarantee Tracking",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/237-performance-guarantee-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 238,
    "code": "CLM-REQ-238",
    "name": "Contract Value Realization",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/238-contract-value-realization",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 239,
    "code": "CLM-REQ-239",
    "name": "Post-Execution Compliance Verification",
    "domain": "Section G: Advanced Obligation & Performance Management",
    "domainId": "domain-12-section-g-advanced-obligation-performance-management",
    "folder": "domain-12-section-g-advanced-obligation-performance-management/239-post-execution-compliance-verification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 240,
    "code": "CLM-REQ-240",
    "name": "Contract Risk Register",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/240-contract-risk-register",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 241,
    "code": "CLM-REQ-241",
    "name": "Risk Assessment Questionnaire",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/241-risk-assessment-questionnaire",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 242,
    "code": "CLM-REQ-242",
    "name": "Risk Scoring Configuration",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/242-risk-scoring-configuration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 243,
    "code": "CLM-REQ-243",
    "name": "Risk Mitigation Planning",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/243-risk-mitigation-planning",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 244,
    "code": "CLM-REQ-244",
    "name": "Risk Ownership Assignment",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/244-risk-ownership-assignment",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 245,
    "code": "CLM-REQ-245",
    "name": "Risk Acceptance Workflow",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/245-risk-acceptance-workflow",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 246,
    "code": "CLM-REQ-246",
    "name": "Risk Exception Management",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/246-risk-exception-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 247,
    "code": "CLM-REQ-247",
    "name": "Regulatory Change Monitoring",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/247-regulatory-change-monitoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 248,
    "code": "CLM-REQ-248",
    "name": "Regulatory Obligation Mapping",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/248-regulatory-obligation-mapping",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 249,
    "code": "CLM-REQ-249",
    "name": "Data Privacy Compliance",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/249-data-privacy-compliance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 250,
    "code": "CLM-REQ-250",
    "name": "GDPR Compliance Tracking",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/250-gdpr-compliance-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 251,
    "code": "CLM-REQ-251",
    "name": "Industry-Specific Compliance",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/251-industry-specific-compliance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 252,
    "code": "CLM-REQ-252",
    "name": "Sanctions Screening Integration",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/252-sanctions-screening-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 253,
    "code": "CLM-REQ-253",
    "name": "Anti-Bribery Compliance",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/253-anti-bribery-compliance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 254,
    "code": "CLM-REQ-254",
    "name": "Conflict of Interest Declaration",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/254-conflict-of-interest-declaration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 255,
    "code": "CLM-REQ-255",
    "name": "Business Continuity Compliance",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/255-business-continuity-compliance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 256,
    "code": "CLM-REQ-256",
    "name": "Cybersecurity Contract Assessment",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/256-cybersecurity-contract-assessment",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 257,
    "code": "CLM-REQ-257",
    "name": "Data Processing Agreement Management",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/257-data-processing-agreement-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 258,
    "code": "CLM-REQ-258",
    "name": "Cross-Border Contract Compliance",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/258-cross-border-contract-compliance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 259,
    "code": "CLM-REQ-259",
    "name": "Compliance Evidence Repository",
    "domain": "Section H: Advanced Risk & Compliance",
    "domainId": "domain-13-section-h-advanced-risk-compliance",
    "folder": "domain-13-section-h-advanced-risk-compliance/259-compliance-evidence-repository",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 260,
    "code": "CLM-REQ-260",
    "name": "AI Contract Classification",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/260-ai-contract-classification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 261,
    "code": "CLM-REQ-261",
    "name": "AI Contract Data Extraction",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/261-ai-contract-data-extraction",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 262,
    "code": "CLM-REQ-262",
    "name": "AI Metadata Extraction",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/262-ai-metadata-extraction",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 263,
    "code": "CLM-REQ-263",
    "name": "AI Obligation Extraction",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/263-ai-obligation-extraction",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 264,
    "code": "CLM-REQ-264",
    "name": "AI Entity Recognition",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/264-ai-entity-recognition",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 265,
    "code": "CLM-REQ-265",
    "name": "AI Date Extraction",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/265-ai-date-extraction",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 266,
    "code": "CLM-REQ-266",
    "name": "AI Financial Term Extraction",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/266-ai-financial-term-extraction",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 267,
    "code": "CLM-REQ-267",
    "name": "AI Clause Deviation Detection",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/267-ai-clause-deviation-detection",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 268,
    "code": "CLM-REQ-268",
    "name": "AI Legal Language Analysis",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/268-ai-legal-language-analysis",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 269,
    "code": "CLM-REQ-269",
    "name": "AI Contract Risk Prediction",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/269-ai-contract-risk-prediction",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 270,
    "code": "CLM-REQ-270",
    "name": "AI Contract Compliance Analysis",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/270-ai-contract-compliance-analysis",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 271,
    "code": "CLM-REQ-271",
    "name": "AI Contract Portfolio Intelligence",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/271-ai-contract-portfolio-intelligence",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 272,
    "code": "CLM-REQ-272",
    "name": "AI Contract Relationship Mapping",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/272-ai-contract-relationship-mapping",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 273,
    "code": "CLM-REQ-273",
    "name": "AI Duplicate Contract Detection",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/273-ai-duplicate-contract-detection",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 274,
    "code": "CLM-REQ-274",
    "name": "AI Contract Anomaly Detection",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/274-ai-contract-anomaly-detection",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 275,
    "code": "CLM-REQ-275",
    "name": "AI Negotiation Recommendations",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/275-ai-negotiation-recommendations",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 276,
    "code": "CLM-REQ-276",
    "name": "AI Renewal Recommendations",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/276-ai-renewal-recommendations",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 277,
    "code": "CLM-REQ-277",
    "name": "AI Contract Q&A",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/277-ai-contract-q-a",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 278,
    "code": "CLM-REQ-278",
    "name": "AI Legal Research Assistant",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/278-ai-legal-research-assistant",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 279,
    "code": "CLM-REQ-279",
    "name": "AI Contract Knowledge Graph",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/279-ai-contract-knowledge-graph",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 280,
    "code": "CLM-REQ-280",
    "name": "AI Human-in-the-Loop Review",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/280-ai-human-in-the-loop-review",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 281,
    "code": "CLM-REQ-281",
    "name": "AI Confidence Scoring",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/281-ai-confidence-scoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 282,
    "code": "CLM-REQ-282",
    "name": "AI Hallucination Verification",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/282-ai-hallucination-verification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 283,
    "code": "CLM-REQ-283",
    "name": "AI Model Governance",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/283-ai-model-governance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 284,
    "code": "CLM-REQ-284",
    "name": "AI Usage Monitoring",
    "domain": "Section I: Advanced Contract Intelligence & AI",
    "domainId": "domain-14-section-i-advanced-contract-intelligence-ai",
    "folder": "domain-14-section-i-advanced-contract-intelligence-ai/284-ai-usage-monitoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 285,
    "code": "CLM-REQ-285",
    "name": "Corporate Entity Management",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/285-corporate-entity-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 286,
    "code": "CLM-REQ-286",
    "name": "Subsidiary Management",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/286-subsidiary-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 287,
    "code": "CLM-REQ-287",
    "name": "Business Unit Management",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/287-business-unit-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 288,
    "code": "CLM-REQ-288",
    "name": "Legal Entity Hierarchy",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/288-legal-entity-hierarchy",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 289,
    "code": "CLM-REQ-289",
    "name": "Contract Ownership Transfer",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/289-contract-ownership-transfer",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 290,
    "code": "CLM-REQ-290",
    "name": "Contract Delegation Management",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/290-contract-delegation-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 291,
    "code": "CLM-REQ-291",
    "name": "Signatory Authority Management",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/291-signatory-authority-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 292,
    "code": "CLM-REQ-292",
    "name": "Authorized Signatory Registry",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/292-authorized-signatory-registry",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 293,
    "code": "CLM-REQ-293",
    "name": "Power of Attorney Management",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/293-power-of-attorney-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 294,
    "code": "CLM-REQ-294",
    "name": "Corporate Policy Enforcement",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/294-corporate-policy-enforcement",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 295,
    "code": "CLM-REQ-295",
    "name": "Approval Authority Matrix",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/295-approval-authority-matrix",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 296,
    "code": "CLM-REQ-296",
    "name": "Segregation of Duties",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/296-segregation-of-duties",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 297,
    "code": "CLM-REQ-297",
    "name": "Conflict-of-Interest Controls",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/297-conflict-of-interest-controls",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 298,
    "code": "CLM-REQ-298",
    "name": "Contract Exception Register",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/298-contract-exception-register",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 299,
    "code": "CLM-REQ-299",
    "name": "Governance Committee Management",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/299-governance-committee-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 300,
    "code": "CLM-REQ-300",
    "name": "Enterprise Contract Governance",
    "domain": "Section J: Corporate Governance & Enterprise Administration",
    "domainId": "domain-15-section-j-corporate-governance-enterprise-administration",
    "folder": "domain-15-section-j-corporate-governance-enterprise-administration/300-enterprise-contract-governance",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 301,
    "code": "CLM-REQ-301",
    "name": "Digital Certificate Registry",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/301-digital-certificate-registry",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 302,
    "code": "CLM-REQ-302",
    "name": "Digital Certificate Issuance Tracking",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/302-digital-certificate-issuance-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 303,
    "code": "CLM-REQ-303",
    "name": "Digital Certificate Expiry Monitoring",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/303-digital-certificate-expiry-monitoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 304,
    "code": "CLM-REQ-304",
    "name": "Certificate Revocation Checking",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/304-certificate-revocation-checking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 305,
    "code": "CLM-REQ-305",
    "name": "Public Key Infrastructure (PKI) Integration",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/305-public-key-infrastructure-pki-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 306,
    "code": "CLM-REQ-306",
    "name": "Certificate Authority Integration",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/306-certificate-authority-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 307,
    "code": "CLM-REQ-307",
    "name": "Signatory Identity Verification",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/307-signatory-identity-verification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 308,
    "code": "CLM-REQ-308",
    "name": "Signer Authentication",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/308-signer-authentication",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 309,
    "code": "CLM-REQ-309",
    "name": "OTP Verification",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/309-otp-verification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 310,
    "code": "CLM-REQ-310",
    "name": "Multi-Factor Signer Authentication",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/310-multi-factor-signer-authentication",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 311,
    "code": "CLM-REQ-311",
    "name": "Electronic Seal Management",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/311-electronic-seal-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 312,
    "code": "CLM-REQ-312",
    "name": "Digital Timestamp Verification",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/312-digital-timestamp-verification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 313,
    "code": "CLM-REQ-313",
    "name": "Signature Validation",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/313-signature-validation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 314,
    "code": "CLM-REQ-314",
    "name": "Signature Evidence Management",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/314-signature-evidence-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 315,
    "code": "CLM-REQ-315",
    "name": "Non-Repudiation Evidence",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/315-non-repudiation-evidence",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 316,
    "code": "CLM-REQ-316",
    "name": "Long-Term Signature Validation",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/316-long-term-signature-validation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 317,
    "code": "CLM-REQ-317",
    "name": "Signing Order Management",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/317-signing-order-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 318,
    "code": "CLM-REQ-318",
    "name": "Multi-Party Signing",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/318-multi-party-signing",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 319,
    "code": "CLM-REQ-319",
    "name": "Remote Signing",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/319-remote-signing",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 320,
    "code": "CLM-REQ-320",
    "name": "Signing Session Tracking",
    "domain": "Section K: Digital Signature & Certificate Management",
    "domainId": "domain-16-section-k-digital-signature-certificate-management",
    "folder": "domain-16-section-k-digital-signature-certificate-management/320-signing-session-tracking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 321,
    "code": "CLM-REQ-321",
    "name": "Contract Folder Hierarchy",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/321-contract-folder-hierarchy",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 322,
    "code": "CLM-REQ-322",
    "name": "Related Contract Linking",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/322-related-contract-linking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 323,
    "code": "CLM-REQ-323",
    "name": "Parent-Child Contract Relationships",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/323-parent-child-contract-relationships",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 324,
    "code": "CLM-REQ-324",
    "name": "Master Agreement Linking",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/324-master-agreement-linking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 325,
    "code": "CLM-REQ-325",
    "name": "Contract Portfolio Mapping",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/325-contract-portfolio-mapping",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 326,
    "code": "CLM-REQ-326",
    "name": "Document Retention Scheduling",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/326-document-retention-scheduling",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 327,
    "code": "CLM-REQ-327",
    "name": "Legal Document Archiving",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/327-legal-document-archiving",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 328,
    "code": "CLM-REQ-328",
    "name": "Secure Document Disposal",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/328-secure-document-disposal",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 329,
    "code": "CLM-REQ-329",
    "name": "Document Classification Rules",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/329-document-classification-rules",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 330,
    "code": "CLM-REQ-330",
    "name": "Document Access Expiry",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/330-document-access-expiry",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 331,
    "code": "CLM-REQ-331",
    "name": "Document Redaction",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/331-document-redaction",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 332,
    "code": "CLM-REQ-332",
    "name": "Sensitive Information Detection",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/332-sensitive-information-detection",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 333,
    "code": "CLM-REQ-333",
    "name": "Document Integrity Verification",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/333-document-integrity-verification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 334,
    "code": "CLM-REQ-334",
    "name": "Document Hash Verification",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/334-document-hash-verification",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 335,
    "code": "CLM-REQ-335",
    "name": "Document OCR Quality Validation",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/335-document-ocr-quality-validation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 336,
    "code": "CLM-REQ-336",
    "name": "Bulk Contract Import",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/336-bulk-contract-import",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 337,
    "code": "CLM-REQ-337",
    "name": "Bulk Contract Export",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/337-bulk-contract-export",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 338,
    "code": "CLM-REQ-338",
    "name": "Contract Migration Validation",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/338-contract-migration-validation",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 339,
    "code": "CLM-REQ-339",
    "name": "Contract Data Deduplication",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/339-contract-data-deduplication",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 340,
    "code": "CLM-REQ-340",
    "name": "Contract Repository Health Monitoring",
    "domain": "Section L: Advanced Repository & Document Intelligence",
    "domainId": "domain-17-section-l-advanced-repository-document-intelligence",
    "folder": "domain-17-section-l-advanced-repository-document-intelligence/340-contract-repository-health-monitoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 341,
    "code": "CLM-REQ-341",
    "name": "SAP Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/341-sap-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 342,
    "code": "CLM-REQ-342",
    "name": "Oracle ERP Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/342-oracle-erp-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 343,
    "code": "CLM-REQ-343",
    "name": "Microsoft Dynamics Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/343-microsoft-dynamics-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 344,
    "code": "CLM-REQ-344",
    "name": "Salesforce Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/344-salesforce-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 345,
    "code": "CLM-REQ-345",
    "name": "Workday Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/345-workday-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 346,
    "code": "CLM-REQ-346",
    "name": "ServiceNow Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/346-servicenow-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 347,
    "code": "CLM-REQ-347",
    "name": "Coupa Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/347-coupa-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 348,
    "code": "CLM-REQ-348",
    "name": "SAP Ariba Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/348-sap-ariba-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 349,
    "code": "CLM-REQ-349",
    "name": "Microsoft 365 Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/349-microsoft-365-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 350,
    "code": "CLM-REQ-350",
    "name": "SharePoint Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/350-sharepoint-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 351,
    "code": "CLM-REQ-351",
    "name": "Microsoft Teams Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/351-microsoft-teams-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 352,
    "code": "CLM-REQ-352",
    "name": "Outlook Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/352-outlook-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 353,
    "code": "CLM-REQ-353",
    "name": "Gmail Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/353-gmail-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 354,
    "code": "CLM-REQ-354",
    "name": "Adobe Acrobat Sign Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/354-adobe-acrobat-sign-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 355,
    "code": "CLM-REQ-355",
    "name": "DocuSign Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/355-docusign-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 356,
    "code": "CLM-REQ-356",
    "name": "REST API Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/356-rest-api-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 357,
    "code": "CLM-REQ-357",
    "name": "GraphQL API Support",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/357-graphql-api-support",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 358,
    "code": "CLM-REQ-358",
    "name": "Event-Driven Integration",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/358-event-driven-integration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 359,
    "code": "CLM-REQ-359",
    "name": "Webhook Event Management",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/359-webhook-event-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 360,
    "code": "CLM-REQ-360",
    "name": "Integration Error Monitoring",
    "domain": "Section M: Enterprise Integration & Automation",
    "domainId": "domain-18-section-m-enterprise-integration-automation",
    "folder": "domain-18-section-m-enterprise-integration-automation/360-integration-error-monitoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 361,
    "code": "CLM-REQ-361",
    "name": "Contract Cycle Time Analytics",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/361-contract-cycle-time-analytics",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 362,
    "code": "CLM-REQ-362",
    "name": "Contract Approval Bottleneck Analytics",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/362-contract-approval-bottleneck-analytics",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 363,
    "code": "CLM-REQ-363",
    "name": "Contract Negotiation Analytics",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/363-contract-negotiation-analytics",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 364,
    "code": "CLM-REQ-364",
    "name": "Contract Renewal Forecasting",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/364-contract-renewal-forecasting",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 365,
    "code": "CLM-REQ-365",
    "name": "Contract Expiry Forecasting",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/365-contract-expiry-forecasting",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 366,
    "code": "CLM-REQ-366",
    "name": "Contract Compliance Analytics",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/366-contract-compliance-analytics",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 367,
    "code": "CLM-REQ-367",
    "name": "Contract Risk Heatmaps",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/367-contract-risk-heatmaps",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 368,
    "code": "CLM-REQ-368",
    "name": "Supplier Contract Analytics",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/368-supplier-contract-analytics",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 369,
    "code": "CLM-REQ-369",
    "name": "Customer Contract Analytics",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/369-customer-contract-analytics",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 370,
    "code": "CLM-REQ-370",
    "name": "Department-Wise Contract Analytics",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/370-department-wise-contract-analytics",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 371,
    "code": "CLM-REQ-371",
    "name": "Contract Value Leakage Analysis",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/371-contract-value-leakage-analysis",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 372,
    "code": "CLM-REQ-372",
    "name": "Contract Spend Leakage Analysis",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/372-contract-spend-leakage-analysis",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 373,
    "code": "CLM-REQ-373",
    "name": "Contract Performance Benchmarking",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/373-contract-performance-benchmarking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 374,
    "code": "CLM-REQ-374",
    "name": "Contract Portfolio Health Score",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/374-contract-portfolio-health-score",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 375,
    "code": "CLM-REQ-375",
    "name": "Executive Governance Reports",
    "domain": "Section N: Advanced Enterprise Reporting",
    "domainId": "domain-19-section-n-advanced-enterprise-reporting",
    "folder": "domain-19-section-n-advanced-enterprise-reporting/375-executive-governance-reports",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 376,
    "code": "CLM-REQ-376",
    "name": "Tenant Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/376-tenant-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 377,
    "code": "CLM-REQ-377",
    "name": "Multi-Tenant Architecture",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/377-multi-tenant-architecture",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 378,
    "code": "CLM-REQ-378",
    "name": "Environment Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/378-environment-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 379,
    "code": "CLM-REQ-379",
    "name": "IP Allowlisting",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/379-ip-allowlisting",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 380,
    "code": "CLM-REQ-380",
    "name": "Session Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/380-session-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 381,
    "code": "CLM-REQ-381",
    "name": "Device Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/381-device-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 382,
    "code": "CLM-REQ-382",
    "name": "Privileged Access Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/382-privileged-access-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 383,
    "code": "CLM-REQ-383",
    "name": "Attribute-Based Access Control",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/383-attribute-based-access-control",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 384,
    "code": "CLM-REQ-384",
    "name": "Field-Level Permissions",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/384-field-level-permissions",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 385,
    "code": "CLM-REQ-385",
    "name": "Document-Level Permissions",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/385-document-level-permissions",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 386,
    "code": "CLM-REQ-386",
    "name": "Dynamic Data Masking",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/386-dynamic-data-masking",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 387,
    "code": "CLM-REQ-387",
    "name": "Data Loss Prevention",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/387-data-loss-prevention",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 388,
    "code": "CLM-REQ-388",
    "name": "Security Incident Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/388-security-incident-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 389,
    "code": "CLM-REQ-389",
    "name": "Security Event Monitoring",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/389-security-event-monitoring",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 390,
    "code": "CLM-REQ-390",
    "name": "Immutable Audit Logs",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/390-immutable-audit-logs",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 391,
    "code": "CLM-REQ-391",
    "name": "Backup Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/391-backup-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 392,
    "code": "CLM-REQ-392",
    "name": "Disaster Recovery",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/392-disaster-recovery",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 393,
    "code": "CLM-REQ-393",
    "name": "Business Continuity Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/393-business-continuity-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 394,
    "code": "CLM-REQ-394",
    "name": "Data Residency Configuration",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/394-data-residency-configuration",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 395,
    "code": "CLM-REQ-395",
    "name": "Data Sovereignty Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/395-data-sovereignty-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 396,
    "code": "CLM-REQ-396",
    "name": "API Rate Limiting",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/396-api-rate-limiting",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 397,
    "code": "CLM-REQ-397",
    "name": "API Key Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/397-api-key-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 398,
    "code": "CLM-REQ-398",
    "name": "Secrets Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/398-secrets-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 399,
    "code": "CLM-REQ-399",
    "name": "Security Policy Management",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/399-security-policy-management",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  },
  {
    "id": 400,
    "code": "CLM-REQ-400",
    "name": "Enterprise Access Review",
    "domain": "Section O: Enterprise Security & Technical Administration",
    "domainId": "domain-20-section-o-enterprise-security-technical-administration",
    "folder": "domain-20-section-o-enterprise-security-technical-administration/400-enterprise-access-review",
    "status": "active",
    "compliance": "ISO-27001 / SOC-2 / NIST-CSF / GDPR"
  }
];

class FeatureRegistry {
    constructor() {
        this.features = masterFeatures;
        this.cache = new Map();
    }

    getAll() {
        return this.features.map(feature => ({
            ...feature,
            status: "registered",
            implementationStatus: "NOT_STARTED"
        }));
    }

    count() {
        return this.features.length;
    }

    getById(id) {
        const meta = this.features.find(f => f.id === Number(id));
        if (!meta) return null;
        if (!this.cache.has(meta.id)) {
            const modPath = path.resolve(__dirname, meta.folder, 'index.js');
            this.cache.set(meta.id, require(modPath));
        }
        return this.cache.get(meta.id);
    }

    getByDomain(domainId) {
        return this.getAll().filter(feature => feature.domainId === domainId);
    }

    execute(id, context = {}) {
        const feature = this.getById(id);
        if (!feature) {
            throw new Error(`Feature with ID ${id} not found in CLM registry.`);
        }
        const result = feature.execute(context);
        return {
            ...result,
            verification: {
                level: "registry-smoke",
                businessCapabilityVerified: false
            }
        };
    }
}

module.exports = new FeatureRegistry();
