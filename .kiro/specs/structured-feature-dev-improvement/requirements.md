# Requirements Document

## Introduction

This document specifies requirements for improving the CLM Structured Feature Development workflow (`clm-feature-dev.workflow.json`). The current workflow covers the pipeline: design → design-review loop → plan → implement+review loop → validate, and produces three artifacts: `feature-design.md`, `feature-plan.md`, and `feature-summary.md`.

Ten identified gaps reduce the workflow's reliability, auditability, and correctness. This enhancement addresses all ten gaps across six functional areas:

1. **Path Resolution** — Replace hardcoded absolute paths with workspace-root-relative references so the workflow is portable across developer machines and CI environments.
2. **Input Validation** — Validate the `component` input before work begins so invalid names fail fast rather than silently defaulting.
3. **Planning Enhancements** — Group plan tasks by CLM lifecycle stage and require property-based tests for new algorithmic logic, making cross-stage blast radius visible.
4. **Validation Gate Improvements** — Extend the final validation step to check for unresolved design questions, missing migration scripts, and missing compliance documentation for approval/e-signature paths.
5. **Artifact Quality** — Add a rollback section to `feature-summary.md` and a per-revision changelog to `feature-design.md` so reviewers can see exactly what changed between iterations.
6. **Loop Management** — Guard the approval stop condition against accidental overwrite with a version counter, and emit an escalation artifact when any loop exhausts its maximum iterations.

The workflow must remain backward-compatible: existing workflow invocations that omit the new fields must continue to function with documented defaults and non-breaking warnings.

---

## Glossary

- **Workflow**: The JSON pipeline definition in `.kiro/workflows/clm-feature-dev.workflow.json` that orchestrates agents through the design–plan–implement–validate lifecycle.
- **Workspace_Root**: The canonical root directory of the CLM project, supplied at workflow invocation time via the `workspace_root` input variable. No path inside workflow artifacts may be hardcoded to an absolute value.
- **Component**: One of the four valid CLM subsystems — `saas-control-plane`, `satellite-gateway`, `vsatellite`, `shared`. Any value outside this set is invalid.
- **CLM_Stage**: One of the 26 named Contract Lifecycle Management stages (Request Intake & Triage through AI Contract Intelligence). Stages are enumerated in the project's `clm-project-context.md`.
- **Approval_File**: The JSON sentinel file (e.g., `feature-design-approved.json`) used as the stop condition for a review loop.
- **Version_Counter**: An integer field stored inside the Approval_File that increments on every agent-initiated write, preventing a later write from silently overwriting a manual approval.
- **Escalation_Artifact**: A Markdown file written when a loop exhausts its maximum iteration count, explaining which loop exhausted, how many iterations ran, the last reviewer verdict, and the recommended next human action.
- **Pre-Flight_Scan**: A read-only codebase inspection executed before the design step that detects in-progress work (uncommitted changes, open branches, or in-flight PRs) that may conflict with the incoming feature.
- **Design_Changelog**: A versioned section appended to `feature-design.md` on every revision, recording the iteration number, timestamp, agent identity, and a structured diff summary of what changed.
- **Rollback_Section**: A required section in `feature-summary.md` that enumerates every artifact produced, every schema migration applied, and the exact steps needed to revert the feature.
- **Property_Based_Test**: A test that verifies a logical property holds across a generated range of inputs, as opposed to a fixed example test. Required for any new algorithmic logic introduced by a feature.
- **Migration_Script**: A versioned SQL or schema-migration file required whenever a feature introduces database schema changes.
- **Compliance_Doc**: Structured documentation required whenever a feature touches the Approval Workflow or Electronic Signature CLM stages, covering GDPR, PCI-DSS, SOC 2, and HIPAA impact analysis.
- **Multi-Tenant_Isolation**: The architectural constraint that no tenant's data or processing may be accessible to another tenant. Non-negotiable across all CLM components.
- **Zero-Egress_AI**: The constraint that AI processing must occur inside the customer's VPC via `satellite-gateway` and must not send data to external endpoints.
- **Immutable_Audit_Trail**: The constraint that every CLM state transition must produce a tamper-evident audit log entry.

---

## Requirements

### Requirement 1: Workspace-Root-Relative Path Resolution

**User Story:** As a CLM developer, I want all workflow artifact paths to be derived from a single `workspace_root` input variable, so that the workflow runs correctly on any machine or CI environment without manual path edits.

#### Acceptance Criteria

1. THE Workflow SHALL accept a `workspace_root` input variable that specifies the absolute path to the CLM project root, where the value is a string of 1 to 4096 characters.
2. THE Workflow SHALL construct every artifact path by joining `workspace_root` with a relative sub-path using the host operating system's path separator; no workflow step definition SHALL contain a hardcoded absolute path.
3. WHEN `workspace_root` is not supplied by the caller, THE Workflow SHALL default to the current working directory and emit a non-blocking warning message indicating that `workspace_root` was not provided to the workflow log before executing any step.
4. WHEN `workspace_root` is supplied but does not resolve to an existing directory, THE Workflow SHALL halt immediately with an error message indicating the unresolvable path before executing any step.
5. THE Workflow SHALL apply the `workspace_root` resolution rule uniformly to all file-check stop conditions, all agent prompt path references, and all output file paths.
6. WHEN `workspace_root` is supplied and contains one or more trailing path separator characters, THE Workflow SHALL normalize the value by removing all trailing path separators before constructing any artifact path.

---

### Requirement 2: Component Input Validation

**User Story:** As a CLM developer, I want the workflow to reject unknown component names at startup, so that mistyped or unsupported components are caught before any design or code work is done.

#### Acceptance Criteria

1. THE Workflow SHALL define an explicit allowlist of valid component values: `saas-control-plane`, `satellite-gateway`, `vsatellite`, `shared`.
2. WHEN the `component` input is provided and its value exactly matches one of the four allowlisted values (case-sensitive, no leading or trailing whitespace), THE Workflow SHALL proceed to the next pipeline step without emitting any warning.
3. WHEN the `component` input is provided and its value does not exactly match any allowlisted value, THE Workflow SHALL halt immediately before executing any pipeline step, log a message containing the invalid value received and the complete list of valid values, and produce no output artifacts.
4. WHEN the `component` input is omitted, THE Workflow SHALL emit a WARNING-level log entry stating "component not specified; defaulting to saas-control-plane" and proceed using `saas-control-plane` as the component value.
5. THE Component_Validator SHALL complete input validation before the design step executes, so that no design work begins for an invalid component.

---

### Requirement 3: Pre-Flight Codebase Scan

**User Story:** As a CLM developer, I want the workflow to scan the target component for conflicting in-progress work before designing, so that the new feature does not unknowingly overlap with uncommitted changes or open branches.

#### Acceptance Criteria

1. THE Workflow SHALL include a pre-flight scan step that executes after input validation and before the design step.
2. WHEN the pre-flight scan executes, THE Pre_Flight_Scanner SHALL inspect the target component's directory for: (a) uncommitted file changes (modified, staged, or untracked files relative to HEAD), (b) local branches that have diverged from the default branch and not been merged, and (c) files matching the pattern `feature-*.md`, `feature-*.json`, `bug-*.md`, `bug-*.json`, `plan-output.md`, or `spec-*.md` in the workflow artifact output directory from a prior incomplete run.
3. WHEN no conflicts are detected, THE Pre_Flight_Scanner SHALL write a `preflight-clean.json` artifact with `{"conflicts": [], "status": "clean"}` and allow the design step to proceed.
4. WHEN one or more conflicts are detected, THE Pre_Flight_Scanner SHALL write a `preflight-conflicts.md` artifact listing each conflict with its type (uncommitted-change, unmerged-branch, or prior-run-artifact), file path, and severity (low, medium, or high), halt the pipeline with a non-zero exit code, and require explicit human acknowledgement before the pipeline can resume.
5. THE Pre_Flight_Scanner SHALL NOT modify any source files, branches, or workflow artifacts during its scan.
6. IF the pre-flight scan encounters a read error on any file or git command, THEN THE Pre_Flight_Scanner SHALL log the specific error, write a `preflight-conflicts.md` artifact with `{"status": "unknown"}`, and halt the pipeline rather than proceeding.
7. WHEN a prior `preflight-conflicts.md` artifact exists and human acknowledgement has been recorded (a `preflight-acknowledged.json` file with `{"acknowledged": true}` is present), THE Workflow SHALL skip the conflict-pause step and proceed to the design step.

---

### Requirement 4: CLM Lifecycle Stage Grouping in the Plan

**User Story:** As a CLM developer, I want the implementation plan to group tasks by the CLM lifecycle stages they affect, so that cross-stage blast radius is visible before any code is written.

#### Acceptance Criteria

1. WHEN planning begins, THE Planner SHALL identify every CLM lifecycle stage that the feature touches, where a stage is considered touched if the approved design document explicitly references that stage's data models, APIs, UI components, or business rules as being read, written, created, or modified by the feature.
2. THE Planner SHALL group all tasks in `feature-plan.md` under their associated CLM stage heading; a task that satisfies the touch criterion in criterion 1 for two or more distinct stage headings SHALL appear under a "Cross-Stage" section with the names of all affected stages listed on the task entry.
3. IF the feature touches no named CLM stage, THEN THE Planner SHALL include an explicit "No CLM Stage Impact" section in `feature-plan.md` rather than omitting stage grouping.
4. WHEN planning begins, THE Planner SHALL produce a blast-radius summary table at the top of `feature-plan.md` listing each affected stage name, the count of tasks assigned to that stage, and a description of the impact of no more than 20 words.
5. WHEN a task satisfies the touch criterion in criterion 1 for the Approval Workflow stage (stage 11) or the Electronic Signature stage (stage 12), THE Planner SHALL annotate that task with a `[COMPLIANCE-REQUIRED]` marker; IF the task touches both stage 11 and stage 12, THE Planner SHALL add a single combined compliance documentation task referencing both stages as the immediately preceding entry; IF the task touches only one of those two stages, THE Planner SHALL add one compliance documentation task referencing that stage as the immediately preceding entry.
6. IF the approved design document is absent or has not been marked as approved (no `feature-design-approved.json` with `"approved": true` exists) when planning begins, THEN THE Planner SHALL halt and emit an error message indicating that planning requires an approved design document, and SHALL produce no plan artifact.

---

### Requirement 5: Property-Based Tests for Algorithmic Logic

**User Story:** As a CLM developer, I want the coder step to require property-based tests for any new algorithmic logic, so that edge cases in contract and certificate processing are caught before the feature ships.

#### Acceptance Criteria

1. WHEN a coder step introduces a new function or method whose primary logic consists of one or more of the following operations — sorting, filtering, parsing, serialization, cryptographic transformation, state-machine transition, or data aggregation over a collection — THE Coder SHALL write at least one property-based test for that function using a property-based testing library that automatically generates test inputs.
2. THE Coder SHALL annotate each property-based test with a structured inline comment immediately preceding the test function that names the correctness property being verified; the comment SHALL identify the property as exactly one of: invariant, round-trip, idempotence, metamorphic, or error condition.
3. WHEN a coder step introduces a parser or serializer for any data format used in the CLM platform, THE Coder SHALL write a round-trip property test asserting that `parse(format(x)) == x` for a minimum of 50 automatically generated inputs per test run.
4. IF a new function interacts only with external services (AWS, database, Keycloak) and contains none of the operations listed in criterion 1, THEN THE Coder MAY substitute an integration test in place of a property-based test, provided the integration test covers at least one successful execution path and at least one failure or error path, with an inline comment explaining the substitution.
5. THE Coder SHALL use a property-based testing library that generates inputs automatically and pseudo-randomly from a defined input domain; a manually enumerated parameterized test does not satisfy the property-based test requirement.
6. THE Code_Reviewer SHALL verify that every new function meeting the definition in criterion 1 has a corresponding property-based test; any function lacking a required test SHALL be listed as a blocking finding in the code review output.

---

### Requirement 6: Extended Validation Gate

**User Story:** As a CLM developer, I want the validation gate to verify unresolved design questions, migration scripts, and compliance documentation before closing a feature, so that no feature ships with incomplete artifacts.

#### Acceptance Criteria

1. WHEN the validation gate runs, THE Validator SHALL inspect `feature-design.md` for any open-question markers (lines containing "TODO", "TBD", "OPEN:", or "??", or any item in an open-questions section that does not contain a `[RESOLVED]` or `[ANSWERED]` marker); IF any are found, THEN THE Validator SHALL fail the gate and list each unresolved item by line number and text.
2. WHEN the validation gate runs and the feature plan contains tasks annotated with `[MIGRATION-REQUIRED]`, THE Validator SHALL verify that a migration script exists in `deploy/migrations/` whose filename contains the feature name slug as a case-insensitive substring; IF no such script exists, THEN THE Validator SHALL fail the gate.
3. WHEN the validation gate runs and the feature plan contains tasks annotated with `[COMPLIANCE-REQUIRED]`, THE Validator SHALL verify that a compliance documentation file exists in the `compliance/` directory whose filename contains the feature name slug as a case-insensitive substring; IF no such file exists, THEN THE Validator SHALL fail the gate.
4. WHEN the validation gate runs, THE Validator SHALL execute both `make test` and `make lint` from `workspace_root` regardless of whether either command exits with a non-zero code; IF either command exits with a non-zero code, THEN THE Validator SHALL fail the gate and include the full output of both commands (whether passing or failing) in the failure report.
5. IF all validation checks pass, THEN THE Validator SHALL write `feature-validation-passed.json` containing: the run timestamp (ISO-8601), the list of checks performed (each with name and result "pass"), and the feature name.
6. IF any validation checks fail, THEN THE Validator SHALL write `feature-validation-failed.json` containing: the run timestamp (ISO-8601), the list of all checks performed (each with name and result "pass" or "fail"), the failure detail for each failed check, and the feature name; the Validator SHALL NOT write `feature-validation-passed.json`.

---

### Requirement 7: Design Changelog

**User Story:** As a CLM design reviewer, I want each revision of `feature-design.md` to append a changelog entry, so that I can see exactly what changed between review iterations without diffing the raw file manually.

#### Acceptance Criteria

1. WHEN `feature-design.md` is first created, THE Design_Agent SHALL append a `## Changelog` section containing a single initial entry with: iteration number `1`, ISO-8601 timestamp accurate to the second, agent identity, and the note "Initial design created".
2. WHEN the design-revise step modifies `feature-design.md`, THE Design_Agent SHALL append a new changelog entry to the `## Changelog` section containing: the iteration number (incremented by 1 from the previous entry), an ISO-8601 timestamp accurate to the second, the agent identity, and a structured list identifying each section heading that was added, modified, or removed in that revision.
3. THE Design_Agent SHALL NOT overwrite or truncate existing changelog entries; each revision's entry SHALL be appended after all prior entries in the `## Changelog` section.
4. IF a design revision produces no changes beyond whitespace, punctuation, or formatting, THEN THE Design_Agent SHALL append a changelog entry with the iteration number, timestamp, agent identity, and the note "No substantive changes" rather than omitting the entry.
5. IF `feature-design.md` exists but does not contain a `## Changelog` section when the design-revise step runs, THEN THE Design_Agent SHALL append a `## Changelog` section to the end of the document before appending the current revision's entry.
6. THE Design_Agent SHALL append at most one changelog entry per execution of the design-revise step, even if multiple sections of `feature-design.md` are modified in that step.

---

### Requirement 8: Rollback Section in Feature Summary

**User Story:** As a CLM operator, I want `feature-summary.md` to include a rollback section, so that the on-call team can revert the feature safely under incident conditions without needing to reconstruct the steps manually.

#### Acceptance Criteria

1. THE Validator SHALL write a `## Rollback` section in `feature-summary.md` as a required top-level section, not an optional appendix.
2. THE Rollback section SHALL enumerate every file created or modified by the feature with its path relative to `workspace_root`, and for each file SHALL indicate whether it was created (did not exist before the feature) or modified (existed before the feature).
3. WHEN the feature includes database schema migrations, THE Rollback section SHALL list each migration script by filename and SHALL specify the exact rollback command or down-migration script filename required to undo it.
4. WHEN the feature modifies configuration files or deployment manifests, THE Rollback section SHALL specify, for each changed key, both the prior value that must be restored and the value introduced by the feature.
5. THE Rollback section SHALL include a numbered, step-ordered procedure containing between 1 and 20 steps that an operator can execute sequentially to revert the feature to the pre-feature baseline state, where each step contains exactly one discrete action.
6. IF a rollback step requires a service or component to be unavailable during execution, THEN THE Rollback section SHALL annotate that step with a `[DOWNTIME]` marker.
7. IF a rollback step permanently destroys or makes irrecoverable any data records, files, or state, THEN THE Rollback section SHALL annotate that step with a `[DATA-LOSS-RISK]` marker.
8. IF the feature modifies no files, includes no migrations, and modifies no configuration files or deployment manifests, THEN THE Rollback section SHALL contain a single numbered step with the statement "No rollback actions required." This single step satisfies the numbered-procedure requirement of criterion 5.

---

### Requirement 9: Versioned Approval Stop Condition

**User Story:** As a CLM developer, I want the loop stop condition to use a version counter so that an agent-initiated write never silently overwrites a human-set manual approval.

#### Acceptance Criteria

1. THE Workflow SHALL include an integer `version` field in every Approval_File written by an agent, with the initial value set to `1` on the first agent write and incremented by exactly `1` on each subsequent agent write.
2. WHEN an agent writes to an Approval_File, THE agent SHALL read the current `version` value first, write `version + 1` in the new content, and preserve the existing `approved` field value unless the agent explicitly intends to change it; IF the Approval_File cannot be read before writing, THEN THE agent SHALL abort the write and emit an error indicating a read failure, leaving the Approval_File unchanged.
3. WHEN a human operator manually sets `approved` to `true` in an Approval_File, THE Workflow SHALL evaluate the stop condition against the `approved` field value alone, independent of whether the `version` value matches the agent's last recorded version.
4. WHEN an agent attempts to write to an Approval_File and the file's current `version` does not equal the agent's last recorded version incremented by `1`, THE agent SHALL emit a conflict warning identifying the expected version and the actual version found, SHALL NOT modify the Approval_File, and THE Workflow SHALL transition to a paused state requiring explicit human resolution before the pipeline resumes.
5. WHEN an agent write to the Approval_File occurs, THE Workflow SHALL persist the `version` value written in its own durable state within the same atomic operation as the write to the Approval_File; IF the two writes cannot be made atomically, THEN THE Workflow SHALL treat the operation as failed and retry; IF the write has failed more than 3 times (i.e., on the 4th failed attempt), THEN THE Workflow SHALL transition to a paused state.

---

### Requirement 10: Loop Escalation Artifact

**User Story:** As a CLM developer, I want an escalation artifact to be produced whenever a review loop exhausts its maximum iterations, so that the reason for exhaustion is visible and actionable without reading through all iteration logs.

#### Acceptance Criteria

1. WHEN the design-review loop reaches its maximum iteration count without the stop condition being met, THE Workflow SHALL write a `design-review-escalation.md` escalation artifact before transitioning to a paused state (halting and awaiting human intervention).
2. WHEN the implement-review loop reaches its maximum iteration count without the stop condition being met, THE Workflow SHALL write an `impl-review-escalation.md` escalation artifact before transitioning to a paused state.
3. THE Escalation_Artifact SHALL include: the loop identifier, the maximum iteration count configured, the number of iterations executed, the reviewer verdict from the final iteration (one of: approved, rejected, or none if no verdict was produced), the complete list of reviewer findings from the final iteration that were not marked as resolved, and a recommended next action for the human reviewer drawn from: manual-approve, revise-and-retry, escalate-to-team, or abandon.
4. THE Escalation_Artifact SHALL be written to the workflow artifact output directory with a filename in the format `{loop-id}-escalation-{ISO-8601-timestamp-in-seconds-UTC}.md`.
5. WHEN an escalation artifact file already exists at the computed filename from a prior run, THE Workflow SHALL append the new escalation event content below a horizontal rule (`---`) separator rather than overwriting the existing file.

---

## Non-Functional Requirements

### Requirement 11: Backward Compatibility

**User Story:** As a CLM developer who invokes the existing workflow, I want workflow invocations that omit new optional inputs to continue working, so that I do not need to update every script or alias that calls the workflow.

#### Acceptance Criteria

1. THE Workflow SHALL treat the `workspace_root` input introduced by this enhancement as optional with a default value of the current working directory, documented in the workflow's input definition alongside the field name and default value.
2. WHEN an existing workflow invocation omits `workspace_root`, THE Workflow SHALL emit a WARNING-level log entry containing the text "workspace_root not provided; using current working directory as workspace root" and proceed using the current working directory as the resolved root.
3. THE Workflow SHALL NOT remove or rename any existing input fields (`feature_request`, `component`) or change their behavior such that a workflow invocation that was valid before this enhancement produces a non-zero exit code after this enhancement is applied.
4. THE Workflow SHALL NOT change the names of any existing output artifact files (`feature-design.md`, `feature-plan.md`, `feature-summary.md`); new artifacts introduced by this enhancement SHALL use filenames that are not identical to any of these three names.

---

### Requirement 12: Pipeline Startup Performance

**User Story:** As a CLM developer, I want the pre-flight scan and input validation to complete quickly, so that the overall pipeline startup time does not increase by more than 30 seconds for a clean workspace.

#### Acceptance Criteria

1. THE Component_Validator SHALL complete input validation within 5 seconds of pipeline process start on any supported operating system.
2. WHEN the workspace has fewer than 10,000 tracked files, THE Pre_Flight_Scanner SHALL complete its scan and produce its output artifact within 30 seconds.
3. THE combined elapsed time from pipeline process start to the moment the pre-flight scan output artifact is written SHALL NOT exceed 30 seconds for a clean workspace, defined as a workspace with fewer than 10,000 tracked files and no uncommitted merge conflicts or pending rebase operations.
4. WHEN the workspace has 10,001 or more tracked files, THE Pre_Flight_Scanner SHALL limit its scan to files within the target component's directory subtree; this scoping is mandatory and independently verifiable regardless of elapsed scan time or whether the full-workspace scan would have completed within the time bound.
5. IF either the component validation or pre-flight scan exceeds its time bound, THEN THE Workflow SHALL halt, emit an error message identifying which operation timed out and its elapsed time, and preserve any partially written output artifacts for diagnostic inspection.

---

### Requirement 13: Auditability of Workflow Runs

**User Story:** As a CLM compliance officer, I want every workflow run to produce a tamper-evident audit record covering all steps executed, so that the pipeline's behavior can be verified during a compliance review.

#### Acceptance Criteria

1. THE Workflow SHALL produce a `workflow-audit-log.json` artifact containing a top-level array where each element represents one run and records: the run start timestamp, run end timestamp, `workspace_root` value, `component` value, the ordered list of steps executed (where each step entry records the step name, step start timestamp, step end timestamp, and step outcome as one of: `success`, `paused`, or `failed`), all non-secret input values (where a secret input is any input value sourced from a secrets store or explicitly marked as a secret-type parameter), and a SHA-256 integrity hash computed over the run entry's content, excluding the hash field itself.
2. WHEN a step fails or the pipeline is paused, THE Workflow SHALL finalize and write the audit log capturing the partial run record, including all steps executed up to and including the failing or pausing step, before halting.
3. THE `workflow-audit-log.json` SHALL be append-only across pipeline runs in the same output directory; each run SHALL be recorded as a separate JSON object appended to the top-level array, not overwriting prior run records.
4. THE audit log SHALL record the resolved absolute paths of all artifact files written during the run.
5. WHILE the Approval_File version counter feature (Requirement 9) is active, THE audit log SHALL record each Approval_File write event with the version number written.
6. IF the `workflow-audit-log.json` file cannot be written due to a filesystem error, THEN THE Workflow SHALL surface an error message indicating that audit logging failed, and SHALL NOT report the run as successfully completed.
