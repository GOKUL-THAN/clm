# DevSecOps and Standards Status

**Assessment date:** 2026-11-06

**Repository:** `GOKUL-THAN/clm`

**Branch:** `audit/phase0-ci-workflow`

**Assessment type:** Source/configuration review; not a certification or audit opinion
**Production readiness:** Not established

## Evidence boundary

This register records what is present in the repository and which checks have
actually run. A manifest is not proof that infrastructure is deployed. The
last all-green CI run before the changes described in this assessment was
[run 42153501338](https://github.com/GOKUL-THAN/clm/actions/runs/42153501338)
on commit `4262e6d`. After the infrastructure and CI changes, run
[42253960553](https://github.com/GOKUL-THAN/clm/actions/runs/42253960553)
passed all nine jobs on commit `403d2d7`, including dedicated
ServiceAccount/token assertions, Kubernetes policy checks, Terraform
formatting/provider validation, SBOM upload, and SLSA provenance attestation
for the SBOM. The attestation was downloaded and verified with GitHub CLI,
constraining both repository and signer workflow.

No cloud apply, Kubernetes deployment, branch-protection change, restore
exercise, compliance assessment, or production operation was performed.

## DevSecOps verification

| Area | Repository evidence | Verified result | Remaining gap |
|---|---|---|---|
| Docker | Multi-stage Dockerfiles for the SaaS server, satellite gateway, and vSatellite; runtime stages declare a non-root `clm` user | CI run `42253960553` built the SaaS and gateway images and passed its configured Trivy critical-severity scan | vSatellite image was not included in that image-scan job; base-image pinning, provenance, runtime policy, and deployment behavior are not independently validated |
| Kubernetes | Restricted Pod Security labels and non-root, read-only-root-filesystem, no-privilege-escalation settings in the control-plane and gateway workloads; dedicated namespace ServiceAccounts with API-token automount disabled; NetworkPolicy manifests | YAML syntax, NetworkPolicy and pod-security assertions, ServiceAccount definitions, and pod-level account/token settings passed in CI run `42253960553` | No Role/RoleBinding grants are added because no Kubernetes API client use was found in the Go workloads; cloud IAM/workload identity must be provisioned separately; no cluster/CNI deployment test |
| Terraform | AWS, Azure, and GCP configuration; partial S3, AzureRM, and GCS remote backends declared; AWS now specifies private RDS access, provider-managed master-password storage, S3 public-access blocking, versioning, TLS-only bucket access, and required Object Lock retention inputs | Terraform formatting passed; provider initialization with `-backend=false` and `terraform validate` passed for all three modules in CI run `42253960553` | Backend buckets/accounts/containers must be provisioned and passed through backend config with restrictive IAM, encryption, versioning, and locking; no plan/apply, drift check, or cloud validation; Azure/GCP controls need separate review |
| CI/CD | `.github/workflows/ci.yml` runs Go formatting, vet, tests, race tests, builds, Gosec, govulncheck, Gitleaks, Trivy filesystem/IaC scan, Syft SBOM generation and upload, PostgreSQL 15 RLS tests, container scans, Kubernetes policy assertions, and Terraform validation | CI run `42253960553` passed all nine jobs, including SBOM upload and attestation | Branch protection, required-check settings, deployment approvals, and rollback gates are GitHub/environment configuration and remain unverified |
| Dependencies | `govulncheck` runs for each staged Go module; Trivy scans the filesystem | All three module dependency checks passed in run `42253960553` | No independent Node dependency SCA inventory or scheduled vulnerability-response exercise was verified |
| Source code | Gosec runs on each Go module; Gitleaks scans Git history | Gosec and full-history Gitleaks passed in run `42253960553` | SAST coverage of JavaScript and other languages is not established |
| Container scanning | Trivy scans built SaaS and gateway images and fails for critical findings | Passed in run `42253960553` | Threshold is critical-only; no signed image policy or verified vSatellite image scan |
| SBOM | Syft generates CycloneDX JSON; the workflow uploads it as a GitHub Actions artifact | Run `42253960553` uploaded `clm-filesystem-sbom-403d2d77c991c58058b6d8e7e79b08c18dfb17b2` (69,087 bytes); artifact expiration is not set by the workflow and follows repository policy | The SBOM describes the source filesystem, not the built images; artifact is not a permanent archive |
| Provenance | GitHub SLSA provenance attestation is generated for the SBOM on trusted workflow runs | `gh attestation verify` passed for the downloaded artifact with repository `GOKUL-THAN/clm` and signer workflow `.github/workflows/ci.yml`; predicate type is `https://slsa.dev/provenance/v1` and subject SHA-256 is `a9acc74106849ac80ab427e0b0b64b8bd4c9d6cd4694891d20399a46816ad87c` | Container images/release artifacts are not attested; an SBOM attestation does not establish an independent SLSA level or release provenance |
| Secrets | Gitleaks full-history scan; AWS RDS master password is configured for provider-managed storage | Gitleaks passed in run `42253960553` | No end-to-end secret-manager rotation, application consumption, or cloud IAM validation |
| Monitoring | Prometheus, Grafana dashboard, and alerting configuration exist under `deploy/observability` | Configuration presence only | No live telemetry, alert delivery, SLO, or incident-response exercise |
| Backup | Cloud database backup configuration exists in provider IaC | Configuration presence only | No current backup job or restore validation evidence |
| Disaster recovery | DR and recovery documents exist | Documentation only | No measured RPO/RTO or isolated restore/failover exercise in this assessment |
| Network | Kubernetes NetworkPolicies exist; a former control-plane ingress rule admitted `0.0.0.0/0` on gRPC port 9090 | Source fix restricts the peer to labeled gateway pods in `clm-in-vpc`; its CI invariant check passed in run `42253960553` | Must be tested with the selected CNI and real namespace labels; external egress allowlists remain deployment-specific |

## Standards applicability and evidence register

Applicability is conditional on deployment, customer, jurisdiction, and data
scope. “Evidence present” means only that repository controls or tests exist;
it does not mean that all requirements of a standard are met. Control-level
mapping requires authoritative source material and an appropriately scoped
assessment.

| Standard | Intended application | Applicability | Current evidence/status | External or remaining validation |
|---|---|---|---|---|
| OWASP ASVS 5.0 | Application security verification | Applicable to web/API assurance | Go security and auth tests exist; no complete ASVS 5.0 control assessment | Control-by-control independent verification |
| OWASP API Security Top 10 | API security | Applicable to exposed APIs | Go auth, tenant-isolation, and API tests exist | Route inventory, BOLA/BFLA and abuse-case testing across all APIs |
| OWASP Top 10 | Web application security | Applicable to web portals | CI checks JavaScript syntax; application security controls exist in source | Full web threat tests, frontend SAST, and manual verification |
| OWASP SAMM | Security maturity | Optional maturity model; useful for program improvement | No measured SAMM assessment | Scoped maturity assessment and evidence |
| NIST SSDF SP 800-218 | Secure software development | Applicable as a development-practice reference | CI includes reviewable builds, SAST, SCA, secret and container scanning | Organization-wide practice evidence, provenance, vulnerability disclosure, and governance |
| NIST SP 800-53 Rev. 5 | Security/privacy control catalog | Conditional on customer/regulatory boundary | Partial technical controls exist | System categorization, control selection, implementation statements, and assessor evidence |
| NIST Cybersecurity Framework 2.0 | Cybersecurity governance | Applicable as a voluntary risk framework | No current organizational profile/assessment | Govern/Identify/Protect/Detect/Respond/Recover evidence and ownership |
| ISO/IEC 27001:2022 | Information security management | Conditional on organization and assurance objectives | Repository controls alone do not establish an ISMS | Scope, risk treatment, operating evidence, and accredited certification where sought |
| ISO/IEC 27002:2022 | Information-security control guidance | Conditional | Some technical controls exist | Applicability statement and control-level operating evidence |
| ISO/IEC 27701 | Privacy information management | Conditional on privacy role and scope | No verified PIMS assessment | Controller/processor role determination, privacy controls, and independent assessment |
| ISO/IEC 25010 | Software product quality | Relevant quality model | Go unit/integration checks and CI gates exist | Defined quality model, measurable requirements, and product-level evaluation |
| ISO/IEC 27017 | Cloud-security guidance | Conditional on cloud service scope | IaC contains selected cloud safeguards | Cloud shared-responsibility assessment and operational evidence |
| ISO/IEC 27018 | Public-cloud PII protection | Conditional on public-cloud PII processing | No verified PII control mapping | Data-flow, processor, deletion, disclosure, and independent evidence |
| ISO 22301 | Business continuity management | Conditional on organizational continuity objectives | DR documents and backup configuration exist | Exercised BCP/DR, measured objectives, and management-system assessment |
| ISO/IEC 20000-1 | IT service management | Conditional on service-management scope | No verified service-management system evidence | Defined service processes, metrics, and independent assessment |
| ISO 31000 | Risk-management guidance | Applicable as a risk framework | Risk registers/reports exist; operational currency is not established | Ownership, acceptance, review cadence, and evidence freshness |
| GDPR | Personal data protection | Conditional on processing personal data in territorial scope | Privacy-related documentation exists | Legal applicability, records of processing, rights workflows, DPIA where required, and counsel review |
| PCI DSS 4.0.1 | Payment-card data security | Only if account data enters scope | No evidence this product processes cardholder data | Confirm scope; if applicable, segmentation and QSA-led assessment |
| SOC 2 Trust Services Criteria | Independent service assurance | Conditional on customer assurance commitments | No independent SOC 2 report verified | Defined system boundary, period of operation, controls, and CPA examination |
| HIPAA | Regulated ePHI protection | Only for applicable covered-entity/business-associate workflows | No ePHI scope established | Legal role/scope determination, BAA where required, safeguards, and assessment |
| eIDAS | EU electronic identification and trust services | Conditional on signature service, jurisdiction, and claims | Signature/PKI code exists; no qualified trust-service evidence | Legal classification, trust-service/provider validation, and qualified-service assessment if claimed |
| ESIGN Act | U.S. electronic signatures | Conditional on U.S. transactions and applicable exceptions | No end-to-end signature consent/evidence assessment | Transaction-specific legal review and signature workflow verification |
| RFC 5280 | X.509 certificate and PKI profile | Applicable to X.509 validation paths | Certificate handling and tests exist | Independent path-validation and policy review for supported profiles |
| RFC 3161 | Trusted timestamp protocol | Conditional on timestamp feature/provider | No live TSA verification established | Provider integration, token validation, and long-term evidence tests |
| FIPS 140-3 | Cryptographic-module validation | Only where required by deployment/customer policy | No validated module certificate established; repository labels are not evidence | Verify exact module, version, operating environment, and certificate; do not claim validation otherwise |
| SLSA | Software supply-chain integrity | Applicable as a build-integrity objective | GitHub-hosted workflow emitted a verifiable SLSA provenance statement for the SBOM | Container-image/release provenance and an independent SLSA level assessment are not established |
| WCAG 2.2 | Accessibility | Applicable to user-facing portals | No accessibility audit evidence in this assessment | Automated and manual WCAG 2.2 AA evaluation with assistive technology |
| OpenAPI | API description/contract | Applicable to stable external APIs | No complete, verified API contract was established in this review | Inventory routes, publish versioned contract, and run conformance tests |
| ACME RFC 8555 | Automated certificate management | Conditional on ACME adapters being enabled | ACME support is referenced in project materials | Interoperability tests against an isolated ACME server, including account/key lifecycle and failure handling |

## Release decision

**Status: DEVELOPMENT.** The current repository has useful automated security
checks and some hardened deployment configuration; the current CI run passed.
External deployment controls, container provenance, permanent evidence
retention, restore evidence, accessibility, and independent standards
assessments remain open. No certification or production-readiness claim is made.
