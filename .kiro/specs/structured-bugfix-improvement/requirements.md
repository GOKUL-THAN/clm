# Requirements Document

## Introduction

This document specifies requirements for improving the CLM Structured Bug-Fix workflow (`clm-bugfix.workflow.json`). The current workflow covers the pipeline: investigate (read-only) → diagnose (root cause + fix plan) → fix (apply + tests), and produces three artifacts: `bug-investigation.md`, `bug-diagnosis.md`, and `bug-fix-summary.md`.

Twelve identified gaps reduce the workflow's portability, correctness, traceability, and compliance posture. This enhancement addresses all twelve gaps across ten functional areas:

1. **Path Resolution** — Replace hardcoded absolute paths with a `workspace_root` input variable so the workflow is portable across developer machines and CI environments.
2. **Input Validation** — Validate the `component` input against an allowlist before work begins so invalid names fail fast rather than silently defaulting.
3. **Pre-Flight Scan** — Scan the target component for conflicting in-progress work before investigating, so that the bugfix does not unknowingly overlap with uncommitted changes or open branches.
4. **Investigation Enhancements** — Require a failing reproduction test before diagnosis proceeds, and tag the investigation report with the affected CLM lifecycle stages so blast radius is visible early.
5. **Diagnosis Enhancements** — Add a review step after diagnosis to catch incorrect root cause identifications before code is changed.
6. **Fix Step Enhancements** — Require property-based tests for algorithmic regression-test logic and document a rollback procedure in the bug-fix summary.
7. **Validation Gate Improvements** — Extend the fix step's validation beyond `make test` + `make lint` to verify that the reproduction test now passes, that diagnosis-specified regression tests were added, and that compliance documentation exists when the bug touches the approval workflow or electronic signature path.
8. **Artifact Quality** — Add a rollback section to `bug-fix-summary.md` so the on-call team can revert the fix under incident conditions.
9. **Loop Management** — Emit an escalation artifact when the investigation cannot confirm a root cause, so that the pipeline never proceeds to diagnosis without a confirmed cause.
10. **Auditability** — Produce a tamper-evident audit record of every workflow run so that the pipeline's behavior can be verified during a compliance review.

The workflow must remain backward-compatible: existing workflow invocations that omit the new fields must continue to function with documented defaults and non-blocking warnings.

---

## Glossary

- **Workflow**: The JSON pipeline definition in `.kiro/workflows/clm-bugfix.workflow.json` that orchestrates agents through the investigate → diagnose → fix lifecycle.
- **Workspace_Root**: The canonical root directory of the CLM project, supplied at workflow invocation time via the `workspace_root` input variable. No path inside workflow artifacts may be hardcoded to an absolute value.
- **Component**: One of the four valid CLM subsystems — `saas-control-plane`, `satellite-gateway`, `vsatellite`, `shared`. Any value outside this set is invalid.
- **CLM_Stage**: One of the 26 named Contract Lifecycle Management stages (Request Intake & Triage through AI Contract Intelligence). Stages are enumerated in the project's `clm-project-context.md`.
- **Pre-Flight_Scan**: A read-only codebase inspection executed before the investigate step that detects in-progress work (uncommitted changes, open branches, or in-flight PRs) that may conflict with the incoming bugfix.
- **Reproduction_Test**: A test written during the investigation step that reproduces the reported bug and fails before the fix is applied. The test must fail for the same reason the bug manifests in production.
- **Bug_Condition_Test**: Synonym for Reproduction_Test. The term emphasizes that the test encodes the bug's triggering condition as a falsifiable property.
- **Property_Based_Test**: A test that verifies a logical property holds across a generated range of inputs, as opposed to a fixed example test. Required for any algorithmic logic introduced or modified by a bugfix.
- **Root_Cause_Hypothesis**: A candidate explanation for a bug's underlying cause, stated in the investigation report with supporting evidence from the code and a method to confirm it.
- **Bug_Investigation_Report**: The `bug-investigation.md` artifact produced by the investigate step, containing the bug summary, code path, suspect root causes, recent changes, and blast radius.
- **Bug_Diagnosis_Report**: The `bug-diagnosis.md` artifact produced by the diagnose step, containing the confirmed root cause, fix approach, fix tasks, regression risk, compliance impact, and test cases to add.
- **Bug_Fix_Summary**: The `bug-fix-summary.md` artifact produced by the fix step, containing the root cause, fix applied, tests added, verification output, and follow-up items.
- **Diagnosis_Review**: A review step that inspects the Bug_Diagnosis_Report for correctness before the fix step proceeds, catching incorrect root cause identifications.
- **Escalation_Artifact**: A Markdown file written when the investigation cannot confirm a root cause, explaining which hypotheses were ruled out and recommending a human action.
- **Rollback_Section**: A required section in `bug-fix-summary.md` that enumerates every file changed by the fix and the exact steps needed to revert the fix.
- **Compliance_Doc**: Structured documentation required whenever a bugfix touches the Approval Workflow (stage 11) or Electronic Signature (stage 12) CLM stages, covering GDPR, PCI-DSS, SOC 2, and HIPAA impact analysis.
- **Migration_Script**: A versioned SQL or schema-migration file required whenever a bugfix introduces database schema changes.
- **Multi-Tenant_Isolation**: The architectural constraint that no tenant's data or processing may be accessible to another tenant. Non-negotiable across all CLM components.
- **Zero-Egress_AI**: The constraint that AI processing must occur inside the customer's VPC via `satellite-gateway` and must not send data to external endpoints.
- **Cryptographic_Integrity**: The constraint that contracts and certificates use PKI digital signatures with X.509 timestamping; any bugfix touching signature or certificate code must preserve this integrity.
- **Immutable_Audit_Trail**: The constraint that every CLM state transition must produce a tamper-evident audit log entry.
- **Audit_Log_Record**: A JSON object appended to `workflow-audit-log.json` that records a single workflow run's steps, timestamps, inputs, and a SHA-256 integrity hash.

---

## Requirements

### Requirement 1: Workspace-Root-Relative Path Resolution

**User Story:** As a CLM developer, I want all workflow artifact paths to be derived from a single `workspace_root` input variable, so that the bugfix workflow runs correctly on any machine or CI environment without manual path edits.

#### Acceptance Criteria

1. THE Workflow SHALL accept a `workspace_root` input variable that specifies the absolute path to the CLM project root, where the value is a string of 1 to 4096 characters.
2. THE Workflow SHALL construct every artifact path by joining `workspace_root` with a relative sub-path using the host operating system's path separator; no workflow step definition SHALL contain a hardcoded absolute path.
3. WHEN `workspace_root` is not supplied by the caller, THE Workflow SHALL default to the current working directory and emit a non-blocking warning message indicating that `workspace_root` was not provided to the workflow log before executing any step.
4. WHEN `workspace_root` is supplied but does not resolve to an existing directory, THE Workflow SHALL halt immediately with an error message indicating the unresolvable path before executing any step.
5. THE Workflow SHALL apply the `workspace_root` resolution rule uniformly to all agent prompt path references, all output file paths (`bug-investigation.md`, `bug-diagnosis.md`, `bug-fix-summary.md`), and all validation command working directories.
6. WHEN `workspace_root` is supplied and contains one or more trailing path separator characters, THE Workflow SHALL normalize the value by removing all trailing path separators before constructing any artifact path.

---

### Requirement 2: Component Input Validation

**User Story:** As a CLM developer, I want the workflow to reject unknown component names at startup, so that mistyped or unsupported components are caught before any investigation work is done.

#### Acceptance Criteria

1. THE Workflow SHALL define an explicit allowlist of valid component values: `saas-control-plane`, `satellite-gateway`, `vsatellite`, `shared`.
2. WHEN the `component` input is provided and its value exactly matches one of the four allowlisted values (case-sensitive, no leading or trailing whitespace), THE Workflow SHALL proceed to the next pipeline step without emitting any warning.
3. WHEN the `component` input is provided and its value does not exactly match any allowlisted value, THE Workflow SHALL halt immediately before executing any pipeline step, log a message containing the invalid value received and the complete list of valid values, and produce no output artifacts.
4. WHEN the `component` input is omitted, THE Workflow SHALL emit a WARNING-level log entry stating "component not specified; defaulting to saas-control-plane" and proceed using `saas-control-plane` as the component value.
5. THE Component_Validator SHALL complete input validation before the investigate step executes, so that no investigation work begins for an invalid component.

---

### Requirement 3: Pre-Flight Codebase Scan

**User Story:** As a CLM developer, I want the workflow to scan the target component for conflicting in-progress work before investigating, so that the bugfix does not unknowingly overlap with uncommitted changes or open branches.

#### Acceptance Criteria

1. THE Workflow SHALL include a pre-flight scan step that executes after input validation and before the investigate step.
2. WHEN the pre-flight scan executes, THE Pre_Flight_Scanner SHALL inspect the target component's directory for: (a) uncommitted file changes (modified, staged, or untracked files relative to HEAD), (b) local branches that have diverged from the default branch and have not been merged, and (c) files matching the pattern `bug-*.md`, `bug-*.json`, `feature-*.md`, `feature-*.json`, `plan-output.md`, or `spec-*.md` in the workflow artifact output directory from a prior incomplete run.
3. WHEN no conflicts are detected, THE Pre_Flight_Scanner SHALL write a `preflight-clean.json` artifact with `{"conflicts": [], "status": "clean"}` and allow the investigate step to proceed.
4. WHEN one or more conflicts are detected, THE Pre_Flight_Scanner SHALL write a `preflight-conflicts.md` artifact listing each conflict with its type (uncommitted-change, unmerged-branch, or prior-run-artifact), file path, and severity (low, medium, or high), halt the pipeline with a non-zero exit code, and require explicit human acknowledgement before the pipeline can resume.
5. THE Pre_Flight_Scanner SHALL NOT modify any source files, branches, or workflow artifacts during its scan.
6. IF the pre-flight scan encounters a read error on any file or git command, THEN THE Pre_Flight_Scanner SHALL log the specific error, write a `preflight-conflicts.md` artifact with `{"status": "unknown"}`, and halt the pipeline rather than proceeding.
7. WHEN a prior `preflight-conflicts.md` artifact exists and human acknowledgement has been recorded (a `preflight-acknowledged.json` file with `{"acknowledged": true}` is present), THE Workflow SHALL skip the conflict-pause step and proceed to the investigate step.

---

### Requirement 4: Bug Condition Exploration Test

**User Story:** As a CLM developer, I want the investigation step to produce a failing test that reproduces the bug before diagnosis proceeds, so that the root cause analysis is grounded in a reproducible failure rather than a hypothesis alone.

#### Acceptance Criteria

1. WHEN the investigate step completes, THE Investigator SHALL write a Reproduction_Test that reproduces the reported bug condition, where the test asserts the expected (correct) behavior and fails when executed against the current unfixed codebase.
2. THE Reproduction_Test SHALL be written to a file whose path is recorded in `bug-investigation.md` under a `## Reproduction Test` section containing the file path relative to `workspace_root` and the exact command to execute the test.
3. WHEN the Reproduction_Test is executed against the current unfixed codebase, THE Reproduction_Test SHALL fail with an assertion error or runtime exception that corresponds to the reported bug condition; the Investigator SHALL record the actual failure output in `bug-investigation.md`.
4. IF the Investigator cannot reproduce the bug after attempting at least two distinct reproduction strategies, THEN THE Investigator SHALL record each attempted strategy and its result in `bug-investigation.md`, flag the bug as `unreproducible`, and halt the pipeline before the diagnose step.
5. THE Workflow SHALL NOT proceed to the diagnose step unless `bug-investigation.md` contains either a `## Reproduction Test` section with a passing-reference file path or an `unreproducible` flag; the absence of both SHALL halt the pipeline with an error indicating that a reproduction test is required.
6. THE Reproduction_Test SHALL use the same test framework already configured in the target component (the framework invoked by `make test`); the Investigator SHALL NOT introduce a new test framework or test runner dependency.

---

### Requirement 5: CLM Lifecycle Stage Tagging in Investigation

**User Story:** As a CLM developer, I want the investigation report to map the bug to the affected CLM lifecycle stages, so that cross-stage blast radius is visible before diagnosis and fix work begins.

#### Acceptance Criteria

1. WHEN the investigate step produces `bug-investigation.md`, THE Investigator SHALL include a `## CLM Lifecycle Stage Impact` section listing every CLM_Stage affected by the bug, where a stage is considered affected if the bug's code path reads, writes, creates, modifies, or deletes that stage's data models, APIs, UI components, or business rules.
2. THE `## CLM Lifecycle Stage Impact` section SHALL list each affected stage by its numbered name (e.g., "Stage 11: Approval Workflow") and SHALL include, for each stage, a description of the impact of no more than 20 words.
3. IF the bug touches no named CLM_Stage, THEN THE Investigator SHALL include an explicit "No CLM Stage Impact" entry in the `## CLM Lifecycle Stage Impact` section rather than omitting the section.
4. WHEN the bug touches the Approval Workflow stage (stage 11) or the Electronic Signature stage (stage 12), THE Investigator SHALL annotate that stage entry with a `[COMPLIANCE-REQUIRED]` marker in the `## CLM Lifecycle Stage Impact` section.
5. WHEN the bug touches cryptographic signature, certificate, or X.509 timestamping code, THE Investigator SHALL annotate the affected stage entry with a `[CRYPTO-IMPACT]` marker, regardless of which numbered stage the cryptographic code belongs to.
6. THE Investigator SHALL record the stage impact determination before the diagnose step begins, so that the diagnosis can reference the confirmed stages.

---

### Requirement 6: Diagnosis Review Step

**User Story:** As a CLM developer, I want a review step to inspect the diagnosis before the fix is applied, so that an incorrect root cause identification is caught before code is changed.

#### Acceptance Criteria

1. THE Workflow SHALL include a diagnosis review step that executes after the diagnose step produces `bug-diagnosis.md` and before the fix step begins.
2. WHEN the diagnosis review step executes, THE Diagnosis_Reviewer SHALL inspect `bug-diagnosis.md` for: (a) internal consistency between the confirmed root cause and the fix tasks, (b) completeness of the fix task list relative to the confirmed root cause, (c) correctness of the regression test case list relative to the reproduction test, and (d) presence of the compliance/audit impact section.
3. WHEN the Diagnosis_Reviewer finds no blocking issues, THE Diagnosis_Reviewer SHALL write a `bug-diagnosis-reviewed.json` artifact with `{"reviewed": true, "verdict": "approved", "findings": []}` and allow the fix step to proceed.
4. WHEN the Diagnosis_Reviewer finds one or more blocking issues, THE Diagnosis_Reviewer SHALL write a `bug-diagnosis-reviewed.json` artifact with `{"reviewed": true, "verdict": "rejected", "findings": [...]}` where each finding includes a severity (blocking or advisory), a description, and a reference to the section and line in `bug-diagnosis.md` that must be corrected, and SHALL route the diagnosis back to the diagnose step for revision.
5. THE Workflow SHALL allow the diagnosis revise-review loop to iterate up to a maximum of 3 times; WHEN the loop reaches 3 iterations without an approved verdict, THE Workflow SHALL write an escalation artifact and transition to a paused state.
6. THE Diagnosis_Reviewer SHALL verify that the confirmed root cause in `bug-diagnosis.md` is consistent with the reproduction test recorded in `bug-investigation.md`; IF the root cause does not explain the reproduction test failure, THEN THE Diagnosis_Reviewer SHALL flag this as a blocking finding.
7. THE Diagnosis_Reviewer SHALL NOT modify `bug-diagnosis.md`; the diagnose step is solely responsible for applying revisions.

---

### Requirement 7: Investigation Failure Escalation

**User Story:** As a CLM developer, I want an escalation artifact to be produced when the investigation cannot confirm a root cause, so that the pipeline never proceeds to diagnosis without a confirmed cause.

#### Acceptance Criteria

1. WHEN the investigate step completes and all Root_Cause_Hypotheses listed in `bug-investigation.md` have been ruled out (each marked as "ruled out" with a reason), THE Investigator SHALL write an `investigation-escalation.md` escalation artifact before halting the pipeline.
2. THE Escalation_Artifact SHALL include: the bug report summary, the complete list of hypotheses investigated and the reason each was ruled out, the reproduction test status (reproduced, unreproducible, or not attempted), the CLM lifecycle stages examined, and a recommended next action drawn from: manual-investigation, gather-more-logs, escalate-to-domain-expert, or abandon.
3. WHEN the Escalation_Artifact is written, THE Workflow SHALL transition to a paused state and SHALL NOT proceed to the diagnose step.
4. THE Escalation_Artifact SHALL be written to the workflow artifact output directory with a filename in the format `investigation-escalation-{ISO-8601-timestamp-in-seconds-UTC}.md`.
5. WHEN an escalation artifact file already exists at the computed filename from a prior run, THE Workflow SHALL append the new escalation event content below a horizontal rule (`---`) separator rather than overwriting the existing file.
6. IF at least one Root_Cause_Hypothesis in `bug-investigation.md` is marked as "confirmed" (not "ruled out"), THEN THE Workflow SHALL proceed to the diagnose step without writing an escalation artifact.

---

### Requirement 8: Property-Based Regression Tests

**User Story:** As a CLM developer, I want the fix step to require property-based tests for any algorithmic logic introduced or modified by the bugfix, so that edge cases in contract and certificate processing are caught before the fix ships.

#### Acceptance Criteria

1. WHEN the fix step introduces or modifies a function or method whose primary logic consists of one or more of the following operations — sorting, filtering, parsing, serialization, cryptographic transformation, state-machine transition, or data aggregation over a collection — THE Fixer SHALL write at least one Property_Based_Test for that function using a property-based testing library that automatically generates test inputs.
2. THE Fixer SHALL annotate each Property_Based_Test with a structured inline comment immediately preceding the test function that names the correctness property being verified; the comment SHALL identify the property as exactly one of: invariant, round-trip, idempotence, metamorphic, or error condition.
3. WHEN the fix step introduces or modifies a parser or serializer for any data format used in the CLM platform, THE Fixer SHALL write a round-trip property test asserting that `parse(format(x)) == x` for a minimum of 50 automatically generated inputs per test run.
4. IF a modified function interacts only with external services (AWS, database, Keycloak) and contains none of the operations listed in criterion 1, THEN THE Fixer MAY substitute an integration test in place of a Property_Based_Test, provided the integration test covers at least one successful execution path and at least one failure or error path, with an inline comment explaining the substitution.
5. THE Fixer SHALL use a property-based testing library that generates inputs automatically and pseudo-randomly from a defined input domain; a manually enumerated parameterized test does not satisfy the Property_Based_Test requirement.
6. WHEN the diagnosis specifies regression test cases in `bug-diagnosis.md`, THE Fixer SHALL add each listed case as a test; any test case listed in the diagnosis that corresponds to an algorithmic operation defined in criterion 1 SHALL be implemented as a Property_Based_Test rather than a fixed example test.

---

### Requirement 9: Rollback Section in Bug-Fix Summary

**User Story:** As a CLM operator, I want `bug-fix-summary.md` to include a rollback section, so that the on-call team can revert the fix safely under incident conditions without needing to reconstruct the steps manually.

#### Acceptance Criteria

1. THE Fixer SHALL write a `## Rollback` section in `bug-fix-summary.md` as a required top-level section, not an optional appendix.
2. THE Rollback_Section SHALL enumerate every file created or modified by the fix with its path relative to `workspace_root`, and for each file SHALL indicate whether it was created (did not exist before the fix) or modified (existed before the fix).
3. WHEN the fix includes database schema migrations, THE Rollback_Section SHALL list each migration script by filename and SHALL specify the exact rollback command or down-migration script filename required to undo it.
4. WHEN the fix modifies configuration files or deployment manifests, THE Rollback_Section SHALL specify, for each changed key, both the prior value that must be restored and the value introduced by the fix.
5. THE Rollback_Section SHALL include a numbered, step-ordered procedure containing between 1 and 20 steps that an operator can execute sequentially to revert the fix to the pre-fix baseline state, where each step contains exactly one discrete action.
6. IF a rollback step requires a service or component to be unavailable during execution, THEN THE Rollback_Section SHALL annotate that step with a `[DOWNTIME]` marker.
7. IF a rollback step permanently destroys or makes irrecoverable any data records, files, or state, THEN THE Rollback_Section SHALL annotate that step with a `[DATA-LOSS-RISK]` marker.
8. IF the fix modifies no files, includes no migrations, and modifies no configuration files or deployment manifests, THEN THE Rollback_Section SHALL contain a single numbered step with the statement "No rollback actions required." This single step satisfies the numbered-procedure requirement of criterion 5.

---

### Requirement 10: Extended Validation Gate

**User Story:** As a CLM developer, I want the fix step's validation to verify that the reproduction test now passes, that diagnosis-specified regression tests were added, and that compliance/audit impact was addressed, so that no bugfix closes with incomplete verification.

#### Acceptance Criteria

1. WHEN the fix step's validation gate runs, THE Validator SHALL execute the Reproduction_Test recorded in `bug-investigation.md` against the fixed codebase; IF the Reproduction_Test does not pass, THEN THE Validator SHALL fail the gate and include the test output in the failure report.
2. WHEN the validation gate runs, THE Validator SHALL verify that every regression test case listed in `bug-diagnosis.md` has a corresponding test file in the codebase; IF any listed case is missing, THEN THE Validator SHALL fail the gate and list each missing case by its diagnosis description.
3. WHEN the validation gate runs and `bug-diagnosis.md` contains a compliance/audit impact section that is non-empty, THE Validator SHALL verify that the compliance or audit impact identified in the diagnosis has been addressed by the fix; IF the diagnosis states a compliance impact and the fix does not include a corresponding remediation, THEN THE Validator SHALL fail the gate and describe the unaddressed impact.
4. WHEN the validation gate runs, THE Validator SHALL execute both `make test` and `make lint` from `workspace_root` regardless of whether either command exits with a non-zero code; IF either command exits with a non-zero code, THEN THE Validator SHALL fail the gate and include the full output of both commands (whether passing or failing) in the failure report.
5. IF all validation checks pass, THEN THE Validator SHALL write `bugfix-validation-passed.json` containing: the run timestamp (ISO-8601), the list of checks performed (each with name and result "pass"), and the bug report identifier.
6. IF any validation checks fail, THEN THE Validator SHALL write `bugfix-validation-failed.json` containing: the run timestamp (ISO-8601), the list of all checks performed (each with name and result "pass" or "fail"), the failure detail for each failed check, and the bug report identifier; the Validator SHALL NOT write `bugfix-validation-passed.json`.
7. WHEN the validation gate runs and `bug-investigation.md` flags the bug as `unreproducible`, THE Validator SHALL skip the reproduction test check (criterion 1) and SHALL verify that `bug-investigation.md` documents at least two attempted reproduction strategies instead.

---

### Requirement 11: Compliance Gating for Approval and E-Signature Paths

**User Story:** As a CLM compliance officer, I want the workflow to require compliance documentation whenever a bugfix touches the approval workflow or electronic signature path, so that regulatory impact is documented before the fix ships.

#### Acceptance Criteria

1. WHEN `bug-investigation.md` tags any CLM_Stage with a `[COMPLIANCE-REQUIRED]` marker (Approval Workflow stage 11 or Electronic Signature stage 12), THE Validator SHALL verify that a Compliance_Doc file exists in the `compliance/` directory whose filename contains the bug report identifier as a case-insensitive substring; IF no such file exists, THEN THE Validator SHALL fail the validation gate.
2. WHEN the Compliance_Doc file is required, THE Validator SHALL verify that the file contains sections addressing GDPR impact, PCI-DSS impact, SOC 2 impact, and HIPAA impact; IF any of the four sections is absent, THEN THE Validator SHALL fail the validation gate and list the missing sections.
3. WHEN `bug-investigation.md` tags any CLM_Stage with a `[CRYPTO-IMPACT]` marker, THE Validator SHALL verify that `bug-diagnosis.md` includes a cryptographic integrity impact assessment describing whether the fix preserves PKI digital signature integrity and X.509 timestamping correctness; IF the assessment is absent, THEN THE Validator SHALL fail the validation gate.
4. WHEN `bug-investigation.md` contains no `[COMPLIANCE-REQUIRED]` or `[CRYPTO-IMPACT]` markers, THE Validator SHALL skip the compliance gating checks and SHALL NOT fail the gate for missing compliance documentation.
5. THE Compliance_Doc SHALL be written by the Fixer during the fix step, not by the Validator; the Validator's role is to verify the existence and completeness of the document.
6. WHEN the fix step begins and `bug-investigation.md` contains one or more `[COMPLIANCE-REQUIRED]` markers, THE Fixer SHALL add a compliance documentation task to the fix task list before applying any code changes, referencing each affected stage.

---

### Requirement 12: Tamper-Evident Audit Logging

**User Story:** As a CLM compliance officer, I want every bugfix workflow run to produce a tamper-evident audit record covering all steps executed, so that the pipeline's behavior can be verified during a compliance review.

#### Acceptance Criteria

1. THE Workflow SHALL produce a `workflow-audit-log.json` artifact containing a top-level array where each element represents one run and records: the run start timestamp, run end timestamp, `workspace_root` value, `component` value, the bug report identifier, the ordered list of steps executed (where each step entry records the step name, step start timestamp, step end timestamp, and step outcome as one of: `success`, `paused`, or `failed`), all non-secret input values (where a secret input is any input value sourced from a secrets store or explicitly marked as a secret-type parameter), and a SHA-256 integrity hash computed over the run entry's content, excluding the hash field itself.
2. WHEN a step fails or the pipeline is paused, THE Workflow SHALL finalize and write the audit log capturing the partial run record, including all steps executed up to and including the failing or pausing step, before halting.
3. THE `workflow-audit-log.json` SHALL be append-only across pipeline runs in the same output directory; each run SHALL be recorded as a separate JSON object appended to the top-level array, not overwriting prior run records.
4. THE audit log SHALL record the resolved absolute paths of all artifact files written during the run, including `bug-investigation.md`, `bug-diagnosis.md`, `bug-fix-summary.md`, any escalation artifacts, and any validation result files.
5. WHILE the diagnosis review step (Requirement 6) is active, THE audit log SHALL record each diagnosis review iteration with the reviewer verdict (approved or rejected) and the count of blocking findings.
6. WHILE the compliance gating (Requirement 11) is active, THE audit log SHALL record the compliance gating result and the list of compliance sections verified.
7. IF the `workflow-audit-log.json` file cannot be written due to a filesystem error, THEN THE Workflow SHALL surface an error message indicating that audit logging failed, and SHALL NOT report the run as successfully completed.
8. THE audit log SHALL record the Reproduction_Test outcome (passed before fix, failed before fix, or not attempted) as part of the investigate step's entry.

---

## Non-Functional Requirements

### Requirement 13: Backward Compatibility

**User Story:** As a CLM developer who invokes the existing bugfix workflow, I want workflow invocations that omit new optional inputs to continue working, so that I do not need to update every script or alias that calls the workflow.

#### Acceptance Criteria

1. THE Workflow SHALL treat the `workspace_root` input introduced by this enhancement as optional with a default value of the current working directory, documented in the workflow's input definition alongside the field name and default value.
2. WHEN an existing workflow invocation omits `workspace_root`, THE Workflow SHALL emit a WARNING-level log entry containing the text "workspace_root not provided; using current working directory as workspace root" and proceed using the current working directory as the resolved root.
3. THE Workflow SHALL NOT remove or rename any existing input fields (`bug_report`, `component`) or change their behavior such that a workflow invocation that was valid before this enhancement produces a non-zero exit code after this enhancement is applied.
4. THE Workflow SHALL NOT change the names of any existing output artifact files (`bug-investigation.md`, `bug-diagnosis.md`, `bug-fix-summary.md`); new artifacts introduced by this enhancement SHALL use filenames that are not identical to any of these three names.
5. WHEN an existing workflow invocation provides only `bug_report` and `component` (the two original inputs) with no new inputs, THE Workflow SHALL execute the full pipeline including all new steps (pre-flight scan, reproduction test, diagnosis review, extended validation, compliance gating, audit logging) using documented defaults for any values those steps require.

---

### Requirement 14: Pipeline Startup Performance

**User Story:** As a CLM developer, I want the pre-flight scan and input validation to complete quickly, so that the overall pipeline startup time does not increase by more than 30 seconds for a clean workspace.

#### Acceptance Criteria

1. THE Component_Validator SHALL complete input validation within 5 seconds of pipeline process start on any supported operating system.
2. WHEN the workspace has fewer than 10,000 tracked files, THE Pre_Flight_Scanner SHALL complete its scan and produce its output artifact within 30 seconds.
3. THE combined elapsed time from pipeline process start to the moment the pre-flight scan output artifact is written SHALL NOT exceed 30 seconds for a clean workspace, defined as a workspace with fewer than 10,000 tracked files and no uncommitted merge conflicts or pending rebase operations.
4. WHEN the workspace has 10,001 or more tracked files, THE Pre_Flight_Scanner SHALL limit its scan to files within the target component's directory subtree; this scoping is mandatory and independently verifiable regardless of elapsed scan time or whether the full-workspace scan would have completed within the time bound.
5. IF either the component validation or pre-flight scan exceeds its time bound, THEN THE Workflow SHALL halt, emit an error message identifying which operation timed out and its elapsed time, and preserve any partially written output artifacts for diagnostic inspection.

---

### Requirement 15: Auditability of Workflow Runs

**User Story:** As a CLM compliance officer, I want the audit log to be tamper-evident and independently verifiable, so that a compromised agent cannot rewrite the pipeline's execution history undetected.

#### Acceptance Criteria

1. THE SHA-256 integrity hash in each Audit_Log_Record SHALL be computed over the canonical JSON serialization of the record's content (all fields except the hash field itself) with keys sorted alphabetically and no insignificant whitespace, so that any two verifiers computing the hash over the same content produce an identical result.
2. WHEN a verifier recomputes the SHA-256 hash over a stored Audit_Log_Record's content and compares it to the stored hash field, IF the two values differ, THEN the verifier SHALL report the record as tampered and SHALL identify the record by its run start timestamp and bug report identifier.
3. THE `workflow-audit-log.json` SHALL NOT contain any mechanism for deleting or modifying a stored record; any operation that attempts to overwrite or remove an existing record SHALL fail.
4. THE audit log SHALL be written to the workflow artifact output directory under the same `workspace_root` resolution as all other artifacts; the audit log path SHALL be derived from `workspace_root` and SHALL NOT be hardcoded to an absolute path.
5. WHEN the audit log grows beyond 10,000 records in a single file, THE Workflow SHALL rotate the file by creating a new file with a numeric suffix (e.g., `workflow-audit-log-001.json`) and SHALL record the rotation event as the first entry in the new file with a `rotation` step name; the prior file SHALL be preserved without modification.
6. THE audit log SHALL record the version of the workflow definition used for each run, so that changes to the workflow definition between runs are traceable.
