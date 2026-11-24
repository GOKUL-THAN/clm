# CLM Standards Applicability Matrix

**Assessment date:** 2026-11-06  
**Purpose:** Applicability register, not a compliance opinion or certification.

Applicability depends on deployment, jurisdiction, customer, data classes, and
contracted service scope. A source control or CI check is not equivalent to
control effectiveness, an independent assessment, or certification. No
standard below is certified by this report.

| Standard / framework | Potential applicability | Evidence in this assessment | Current state | Remaining evidence |
|---|---|---|---|---|
| OWASP ASVS 5.0 | Web/API product assurance | Selected Go tests and CI checks; no complete ASVS mapping | `NOT_VERIFIED` | Versioned control-by-control scope and evidence |
| OWASP API Security Top 10 | Exposed APIs | Selected tenant/auth tests only | `NOT_VERIFIED` | API inventory and authorization/abuse-case testing |
| OWASP Top 10 | Web application | Syntax and scenario checks | `NOT_VERIFIED` | Full web threat tests, frontend SAST, and manual tests |
| OWASP LLM Top 10 | Only if AI features are in scope | Selected in-process prompt-injection scenarios | `NOT_VERIFIED` | Tenant retrieval, tool use, exfiltration, data governance, and adversarial evaluations |
| CWE | Vulnerability taxonomy | Used to describe historical findings | `NOT_VERIFIED` | Complete current-source analysis and finding validation |
| NIST SSDF SP 800-218 | Secure development practices | CI includes configured source/dependency/secret/container checks | `NOT_VERIFIED` | Organization-wide process evidence and governance |
| NIST Cybersecurity Framework 2.0 | Organizational cybersecurity governance | Repository controls only | `NOT_VERIFIED` | Organizational implementation profile, outcomes, and evidence |
| NIST AI RMF | If AI functionality is offered | Local scenario evidence only | `NOT_VERIFIED` | Govern/map/measure/manage evidence and provider/data risk review |
| NIST SP 800-53 Rev. 5 | If selected by customer/regulatory scope | No complete control mapping | `NOT_VERIFIED` | Applicable baseline, control owners, implementation/effectiveness evidence |
| ISO/IEC 27001:2022 | ISMS scope chosen by organization | No ISMS certification evidence | `NOT_VERIFIED` | ISMS operation, internal audit, and accredited certification |
| ISO/IEC 27002:2022 | Security-control guidance | No control-by-control assessment | `NOT_VERIFIED` | Applicable control selection and evidence |
| ISO/IEC 27017 | Cloud service scope | IaC and CI checks only | `NOT_VERIFIED` | Shared-responsibility and operational cloud control assessment |
| ISO/IEC 27018 | Personal data in public cloud | No privacy control assessment | `NOT_VERIFIED` | Data scope, processor practices, and control evidence |
| ISO/IEC 27701 | Privacy management system | No PIMS assessment | `NOT_VERIFIED` | PIMS scope, roles, controls, and audit evidence |
| SOC 2 Trust Services Criteria | Customer assurance demand | CI evidence only | `NOT_VERIFIED` | Defined scope, operating evidence, and CPA examination; no report shown |
| GDPR | If processing personal data subject to GDPR | No legal or operational compliance assessment | `NOT_VERIFIED` | Controller/processor roles, lawful basis, rights, DPIA, retention, and operations |
| PCI DSS 4.0.1 | Only if payment-account data is in scope | No payment data scope established | `NOT_APPLICABLE` pending scope determination | Confirm whether cardholder data enters/stores/transits the service |
| HIPAA | Only if regulated PHI and covered-entity/business-associate role apply | No PHI scope established | `NOT_APPLICABLE` pending scope determination | Confirm service role, BAA, safeguards, and evidence if applicable |
| eIDAS / applicable electronic-signature laws | If signatures are offered for regulated/legal use | Local signature tests only; no QTSP/QSCD evidence | `NOT_VERIFIED` | Jurisdiction-specific legal review, trusted provider, identity and evidence controls |
| RFC 5280 / RFC 3161 | If certificate paths/timestamps are claimed | Local cryptographic scenarios only | `NOT_VERIFIED` | Independent chain and timestamp validation against actual trust services |
| FIPS 140-3 | Only for a requirement to use validated crypto modules | No applicable CMVP module certificate reviewed | `NOT_VERIFIED` | Module identity/boundary and current CMVP certificate evidence |
| SLSA | Supply-chain integrity target | CI generated SBOM provenance; no full level assessment | `NOT_VERIFIED` | Target level, build isolation, source/dependency integrity, and artifact provenance |
| CIS Benchmarks / Kubernetes Pod Security | Depends on deployed platform | CI policy assertions and manifests | `NOT_VERIFIED` | Runtime cluster, CNI, configuration, and benchmark validation |
| ISO 22301 / NIST SP 800-34 | Business continuity and recovery scope | DR documents only; no drill | `NOT_VERIFIED` | Approved continuity plan and measured recovery exercise |
| WCAG 2.2 | Web accessibility obligations/target | No accessibility audit in this verification | `NOT_VERIFIED` | Automated and manual assistive-technology assessment |

`NOT_APPLICABLE` above is provisional until the product's data and deployment
scope is formally determined. No legal conclusion is provided.
