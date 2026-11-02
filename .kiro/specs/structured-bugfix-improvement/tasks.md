# Implementation Plan: Structured Bug-Fix Workflow Improvement

## Overview

This plan implements the enhancement to `clm-bugfix.workflow.json`, restructuring the three-step pipeline (investigate → diagnose → fix) into a nine-step flow with a bounded review loop and a final validation gate:

```
validate-component → preflight-scan → investigate → diagnose → diagnosis-review-loop (max 3) → fix → validation-gate
```

Implementation is in **Go** (the project's language, per the design's Testing Strategy). The work spans two surfaces:

1. `.kiro/workflows/clm-bugfix.workflow.json` — the declarative workflow definition (inputs, `audit_log` concern, step prompts, the `repeat` review loop, the validation-gate step).
2. `.kiro/workflows/clm-bugfix_support.go` — pure, testable Go functions for the supporting logic (path normalization, component validation, hash computation, validation-gate logic, rollback validation, compliance checks). These are the units under test for the property-based tests.
3. `.kiro/workflows/clm-bugfix.workflow_test.go` — property-based tests (one per design correctness property) + unit tests + structural workflow tests.
4. `.kiro/workflows/clm-bugfix.integration_test.go` — integration tests (build tag `integration`) using a git repository fixture.

The plan reuses conventions from the sibling `clm-feature-dev.workflow.json` (`{{resolved_root}}` template variable, `validate-component` startup step, `audit_log` cross-cutting concern, `repeat` loop with `fileCheck` stop condition, PBT annotation format) so both workflows share a consistent operational model. Backward compatibility is preserved: existing invocations omitting `workspace_root` continue to work with documented defaults and non-blocking warnings.

Conventions for each task: it names the exact file(s) to modify, what to change, and how to verify. Property-based tests use `pgregory.net/rapid`, minimum 100 iterations (50 for round-trip parser/serializer properties), each preceded by `// Feature: structured-bugfix-improvement, Property N: <title>` and the property-type tag (invariant, round-trip, idempotence, metamorphic, error condition).

---

## Tasks

- [x] 1. Foundation: workflow inputs, path resolution, and component validation
  - [x] 1.1 Add `workspace_root` optional input and `audit_log` concern stub to the workflow JSON
    - Modify `.kiro/workflows/clm-bugfix.workflow.json`: add `"workspace_root": "string"` to `inputs`; add a top-level `audit_log` object with `path: "{{resolved_root}}/.agents/tasks/workflow-audit-log.json"`, `append_only: true`, `flush_on_halt_or_pause`, `secret_exclusion`, `record_artifact_paths`, `record_diagnosis_review_iterations`, `record_compliance_gating`, and `hash_computation` descriptions (schema filled in task 11.x).
    - Verify: JSON parses; `workspace_root` appears in `inputs` with default-of-cwd documented in description; `audit_log.path` uses `{{resolved_root}}` (no hardcoded path).
    - _Requirements: 1.1, 1.3, 1.6, 12.1, 15.4_

  - [x] 1.2 Replace all hardcoded absolute paths with `{{resolved_root}}` in every step prompt
    - Modify `.kiro/workflows/clm-bugfix.workflow.json`: in the `investigate`, `diagnose`, and `fix` step prompts, replace every occurrence of the hardcoded root `d:\cmd project\M2P-Git\CLM` and the `.agents\tasks\` artifact paths with `{{resolved_root}}` and `{{resolved_root}}/.agents/tasks/...` respectively. No step definition may contain a hardcoded absolute path.
    - Verify: grep the file for `d:\\` / `M2P-Git` / absolute Windows paths → zero matches; every artifact path references `{{resolved_root}}`.
    - _Requirements: 1.2, 1.5, 15.4_

  - [x] 1.3 Add the `validate-component` step to the workflow JSON
    - Modify `.kiro/workflows/clm-bugfix.workflow.json`: insert a `validate-component` step as the first step (`type: "step"`, `id: "validate-component"`, `timeout_ms: 5000`, `on_timeout: "fail"`, `agent: "wf-component-validator"`). Its prompt resolves `workspace_root` (default cwd + WARNING when omitted; HALT when non-existent), validates `component` against the allowlist `["saas-control-plane","satellite-gateway","vsatellite","shared"]` (case-sensitive exact match; default `saas-control-plane` + WARNING when omitted; HALT with invalid-value + valid-list log when no match, no artifacts produced), exposes `{{resolved_root}}` and `{{component}}`, and initializes the audit log.
    - Verify: step is first in `steps`; allowlist text present; exact warning strings ("workspace_root not provided; using current working directory as workspace root", "component not specified; defaulting to saas-control-plane") present; timeout configured.
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 14.1_

  - [x] 1.4 Implement WorkspaceRootResolver and ComponentValidator pure functions in Go
    - Create `.kiro/workflows/clm-bugfix_support.go`: `func ResolveWorkspaceRoot(workspaceRoot string) (resolvedRoot string, warning string, err error)` — trim all trailing `\` and `/`; omitted/empty → cwd + warning; non-existent → error. `func NormalizePath(p string) string` — strip trailing separators. `func ValidateComponent(component string) (component string, warning string, err error)` — exact, case-sensitive allowlist match; omitted → "saas-control-plane" + warning; no-match → error including received value and full allowlist. Export the allowlist via `var ValidComponents = [...]string{...}`.
    - Verify: `go vet ./...` and `go build ./...` pass for the package; functions are pure (no I/O except `os.Getwd`).
    - _Requirements: 1.1, 1.3, 1.4, 1.6, 2.1–2.5, 13.1, 13.2_

  - [ ]* 1.5 Write property test for path normalization idempotence
    - **Property 2: Path Normalization Idempotence** — `normalize(normalize(x)) == normalize(x)` for paths with 0–10 trailing separators of mixed `\` and `/`.
    - Add to `.kiro/workflows/clm-bugfix.workflow_test.go` using `rapid`: generator of random paths with trailing separators; assert idempotence. Tag `// Feature: structured-bugfix-improvement, Property 2: Path Normalization Idempotence` (invariant).
    - **Validates: Requirements 1.6**

  - [ ]* 1.6 Write property test for component + workspace-root input validation
    - **Property 3: Component and Workspace Root Input Validation** — allowlisted strings accepted without warning; non-allowlisted rejected; workspace_root of 1–4096 chars resolving to an existing dir accepted; non-existent rejected.
    - Add `TestComponentValidationExactMatch` / `TestWorkspaceRootValidation` using `rapid` generating allowlisted and random strings, and random workspace_root lengths. (error condition / invariant)
    - **Validates: Requirements 1.1, 1.4, 2.2, 2.3**

  - [ ]* 1.7 Write property test that all artifact paths derive from workspace root
    - **Property 1: All Artifact Paths Derived from Workspace Root** — every artifact path, prompt path reference, and validation working directory in the workflow JSON is `{{resolved_root}}` + relative sub-path; audit log path likewise; no hardcoded absolute path.
    - Add `TestAllPathsDerivedFromRoot`: parse the workflow JSON, walk all string fields, assert none contains a hardcoded absolute path and all artifact references use `{{resolved_root}}`. (invariant)
    - **Validates: Requirements 1.2, 1.5, 15.4**

  - [ ]* 1.8 Write property test for backward compatibility
    - **Property 22: Backward Compatibility Invariant** — an invocation valid before enhancement (only `bug_report` + `component`) executes without a non-zero exit due to missing new inputs; `workspace_root` optional defaulting to cwd; the three existing artifact filenames unchanged; no new artifact reuses those three names.
    - Add `TestBackwardCompatibility`: invoke with only the two original inputs, assert defaults applied (no error), assert existing filenames preserved, assert new artifact names differ from the three. (invariant)
    - **Validates: Requirements 13.1, 13.3, 13.4**

  - [ ]* 1.9 Write unit tests for root/component defaults and halts
    - Add to `.kiro/workflows/clm-bugfix.workflow_test.go`: `TestWorkspaceRootOmittedDefaultsToCwd` (cwd used + warning emitted), `TestWorkspaceRootNonExistentHalts` (HALT before any step), `TestWorkspaceRootTrailingSeparatorNormalized`, `TestComponentOmittedDefaultsToControlPlane` (default + warning), `TestComponentInvalidHalts` (HALT, no artifacts).
    - _Requirements: 1.3, 1.4, 1.6, 2.4, 13.2_

- [x] 2. Pre-flight codebase scanner
  - [x] 2.1 Add the `preflight-scan` step to the workflow JSON
    - Modify `.kiro/workflows/clm-bugfix.workflow.json`: insert `preflight-scan` as the second step (`id: "preflight-scan"`, `timeout_ms: 30000`, `on_timeout: "fail"`, `agent: "wf-preflight-scanner"`), executing after `validate-component` and before `investigate`. Prompt runs a read-only scan of the target component directory and the artifact output directory; writes `preflight-clean.json` (clean) or `preflight-conflicts.md` (conflict/unknown) and HALTs on conflict/error; honors a prior `preflight-acknowledged.json` to skip the pause.
    - Verify: step ordering validate-component → preflight-scan → investigate; timeout 30000; read-only language present.
    - _Requirements: 3.1, 14.1, 14.2_

  - [x] 2.2 Implement git-status detection, severity classification, and clean/conflict output
    - In `.kiro/workflows/clm-bugfix_support.go`: `func ScanUncommittedChanges(componentDir string) ([]Conflict, error)` using `git status --porcelain` (severity `low` for untracked `?`, `medium` for modified/staged). `func WritePreflightResult(artifactDir string, conflicts []Conflict, status string) error` writing `preflight-clean.json` `{"conflicts":[],"status":"clean"}` when empty or `preflight-conflicts.md` listing each conflict with type, path, severity.
    - Verify: unit-call returns expected severities; clean case writes `preflight-clean.json` with the exact schema.
    - _Requirements: 3.2, 3.3, 3.4_

  - [x] 2.3 Implement branch-divergence detection and stale-artifact glob detection
    - In `.kiro/workflows/clm-bugfix_support.go`: `func ScanUnmergedBranches(componentDir string) ([]Conflict, error)` via `git branch --no-merged <default-branch>` (severity `high`). `func ScanPriorRunArtifacts(artifactDir string) ([]Conflict, error)` globbing `bug-*.md`, `bug-*.json`, `feature-*.md`, `feature-*.json`, `plan-output.md`, `spec-*.md` (severity `medium`, type `prior-run-artifact`).
    - Verify: each detector returns conflicts of the correct type/severity; glob covers all six patterns.
    - _Requirements: 3.2_

  - [x] 2.4 Implement acknowledgement bypass and large-workspace scoping
    - In `.kiro/workflows/clm-bugfix_support.go`: `func IsAcknowledged(artifactDir string) bool` reads `preflight-acknowledged.json` `{"acknowledged":true}` → skip conflict-pause. `func ShouldScopeToComponent(workspaceRoot, component string) bool` returns true when tracked-file count ≥ 10,001; when true, `ScanUncommittedChanges`/`ScanPriorRunArtifacts` limit their scope to the component directory subtree.
    - Verify: acknowledged file skips pause; >10K files triggers component-subtree scoping (verifiable independent of elapsed time).
    - _Requirements: 3.7, 14.4_

  - [x] 2.5 Implement timeout enforcement and git-command failure handling
    - In `.kiro/workflows/clm-bugfix_support.go`: wrap each git/glob call with the configured timeout; on any read error or git command failure, write `preflight-conflicts.md` with `{"status":"unknown"}` and return a halt error (do not proceed). Surface which operation timed out and elapsed time.
    - Verify: simulated git failure writes `preflight-conflicts.md` status=unknown and halts; timeout error names the operation.
    - _Requirements: 3.6, 14.5_

  - [ ]* 2.6 Write property test for pre-flight scan read-only behavior
    - **Property 4: Pre-Flight Scan Is Read-Only** — after a scan (clean/conflict/error), tracked file checksums, git branch state, and existing artifacts are unchanged; scanner creates/modifies/deletes nothing.
    - Add `TestPreflightScanReadOnly` using `rapid` over random workspace states; compare pre/post file checksums and branch refs. (invariant)
    - **Validates: Requirements 3.5**

  - [ ]* 2.7 Write property test for pre-flight conflict listing completeness
    - **Property 5: Pre-Flight Conflict Listing Completeness** — every detected conflict appears in `preflight-conflicts.md` with type (`uncommitted-change`/`unmerged-branch`/`prior-run-artifact`), file path, and severity (`low`/`medium`/`high`); none omitted.
    - Add `TestConflictListingComplete` generating random conflict sets; assert the listing is a superset of detected conflicts with correct attributes. (invariant)
    - **Validates: Requirements 3.4**

  - [ ]* 2.8 Write property test for pre-flight scoping on large workspaces
    - **Property 23: Pre-Flight Scan Scoping for Large Workspaces** — for ≥10,001 tracked files, the inspected file set is a subset of the component directory subtree, verifiable independent of elapsed time.
    - Add `TestPreflightScopingLargeWorkspace` generating workspaces >10K files; assert inspected paths ⊆ component subtree regardless of timing. (invariant)
    - **Validates: Requirements 14.4**

  - [ ]* 2.9 Write unit tests for pre-flight outputs and halts
    - Add: `TestPreflightCleanWritesCleanJson`, `TestPreflightAcknowledgedSkipsPause`, `TestPreflightReadErrorHaltsUnknown`.
    - _Requirements: 3.3, 3.6, 3.7_

- [x] 3. Investigation enhancements (reproduction test + CLM stage tagging)
  - [x] 3.1 Enhance the investigate step with the ReproductionTestWriter
    - Modify `.kiro/workflows/clm-bugfix.workflow.json` `investigate` prompt: after read-only analysis, write a Reproduction_Test using the target component's existing test framework (the one `make test` invokes — Go `testing` package; do NOT introduce a new framework). The test asserts the expected correct behavior and fails on the unfixed codebase. Record in `bug-investigation.md` a `## Reproduction Test` section with the test file path (relative to `workspace_root`), the test function name, the exact execution command, the pre-fix failure output, and `strategies_attempted`.
    - Verify: prompt text requires a failing reproduction test, references the existing framework, and mandates the `## Reproduction Test` section with path + command.
    - _Requirements: 4.1, 4.2, 4.3, 4.6_

  - [x] 3.2 Add unreproducible handling and the reproduction-test gate
    - Modify the `investigate` prompt: if the bug cannot be reproduced after ≥2 distinct strategies, record each attempt and result, set the `unreproducible` flag, and HALT before the diagnose step. Add an explicit gate statement: the workflow SHALL NOT proceed to diagnose unless `bug-investigation.md` contains a `## Reproduction Test` section with a file path or an `unreproducible` flag; absence of both halts with "reproduction test required".
    - Verify: prompt encodes the ≥2-strategy rule, the `unreproducible` flag, the HALT-before-diagnose, and the gate condition.
    - _Requirements: 4.4, 4.5_

  - [x] 3.3 Add the CLMStageTagger to the investigate step
    - Modify the `investigate` prompt: include a `## CLM Lifecycle Stage Impact` section listing every affected CLM_Stage (a stage is affected if the bug's code path reads/writes/creates/modifies/deletes that stage's data models, APIs, UI, or business rules). Each entry uses the numbered name (e.g., "Stage 11: Approval Workflow") with an impact description ≤20 words. If no stage is affected, include an explicit "No CLM Stage Impact" entry. Stage impact must be recorded before the diagnose step begins.
    - Verify: prompt mandates the section, numbered names, ≤20-word descriptions, and the no-impact fallback.
    - _Requirements: 5.1, 5.2, 5.3, 5.6_

  - [x] 3.4 Add `[COMPLIANCE-REQUIRED]` and `[CRYPTO-IMPACT]` markers
    - Modify the `investigate` prompt: annotate Approval Workflow (stage 11) and Electronic Signature (stage 12) entries with `[COMPLIANCE-REQUIRED]`; annotate cryptographic signature, certificate, or X.509 timestamping code entries with `[CRYPTO-IMPACT]` regardless of stage number.
    - Verify: prompt text defines both markers and their trigger conditions.
    - _Requirements: 5.4, 5.5_

  - [ ]* 3.5 Write property test for the reproduction-test gate
    - **Property 6: Reproduction Test Gate** — workflow does not proceed to diagnose unless the report has a `## Reproduction Test` section with a file path or an `unreproducible` flag; when unreproducible after ≥2 strategies, the report carries the flag and the pipeline halts before diagnose.
    - Add `TestReproductionTestGate` generating random investigation reports; assert gate logic. (invariant)
    - **Validates: Requirements 4.4, 4.5**

  - [ ]* 3.6 Write property test for stage impact format and compliance markers
    - **Property 7: CLM Stage Impact Format and Compliance Markers** — a `## CLM Lifecycle Stage Impact` section is always present; entries use numbered names with ≤20-word descriptions; stage 11/12 entries carry `[COMPLIANCE-REQUIRED]`; crypto entries carry `[CRYPTO-IMPACT]`.
    - Add `TestStageImpactFormat` generating random stage sets/descriptions; assert format and marker rules. (invariant)
    - **Validates: Requirements 5.1, 5.2, 5.4, 5.5**

  - [ ]* 3.7 Write unit test for the no-stage explicit entry
    - Add `TestNoStageImpactExplicitEntry`: bug with no stage impact → explicit "No CLM Stage Impact" entry present (section not omitted).
    - _Requirements: 5.3_

- [x] 4. Checkpoint - Ensure all tests pass (foundation, pre-flight, investigation)
  - Ensure all tests pass, ask the user if questions arise.

- [x] 5. Diagnosis review loop
  - [x] 5.1 Add the diagnosis-review-loop to the workflow JSON
    - Modify `.kiro/workflows/clm-bugfix.workflow.json`: insert a `repeat` step (`id: "diagnosis-review-loop"`, `maxIterations: 3`, `onMaxIterations: "pause"`, `stopCondition.fileCheck` on `{{resolved_root}}/.agents/tasks/bug-diagnosis-reviewed.json` jsonPath `verdict` == `approved`) containing two inner steps: `diagnosis-reviewer` (agent `wf-diagnosis-reviewer`) and `diagnose-revise` (agent `wf-planner`, reads the reviewed verdict and applies findings). Place it between `diagnose` and `fix`.
    - Verify: loop maxIterations 3, stopCondition present, inner steps ordered reviewer → revise.
    - _Requirements: 6.1, 6.5_

  - [x] 5.2 Implement DiagnosisReviewer checks in Go
    - In `.kiro/workflows/clm-bugfix_support.go`: `func ReviewDiagnosis(diagnosisPath, investigationPath string) (verdict string, findings []Finding, err error)` performing: (a) internal consistency between confirmed root cause and fix tasks; (b) completeness of fix tasks vs root cause; (c) regression test list correctness vs the reproduction test; (d) presence of the compliance/audit impact section; (e) root cause consistency with the reproduction test failure (blocking if it does not explain the failure). The reviewer does NOT modify `bug-diagnosis.md`.
    - Verify: a consistent diagnosis returns approved; an inconsistent root cause (doesn't explain reproduction failure) returns a blocking finding.
    - _Requirements: 6.2, 6.6_

  - [x] 5.3 Add bug-diagnosis-reviewed.json verdict writing
    - In `.kiro/workflows/clm-bugfix_support.go`: `func WriteReviewVerdict(path string, verdict string, findings []Finding, iteration int) error` writing `{reviewed:true, verdict, findings, iteration, reviewed_at}`. Approved → empty findings; rejected → findings each with severity (blocking/advisory), description, section, line.
    - Verify: approved file has empty findings; rejected file lists findings with the required fields.
    - _Requirements: 6.3, 6.4_

  - [x] 5.4 Add the diagnose-revise step and escalation on max iterations
    - Modify `.kiro/workflows/clm-bugfix.workflow.json`: `diagnose-revise` prompt reads `bug-diagnosis-reviewed.json`; if rejected, applies each finding to `bug-diagnosis.md` (the diagnose step solely applies revisions). On the 3rd rejection without approval, the reviewer writes an escalation artifact and the workflow transitions to a paused state.
    - Verify: revise prompt references the verdict file and applies findings; max-iteration escalation described.
    - _Requirements: 6.5, 6.7_

  - [ ]* 5.5 Write property test for review verdict by blocking findings
    - **Property 8: Diagnosis Review Verdict Determined by Blocking Findings** — verdict is `approved` iff the blocking-finding set is empty; a root cause that doesn't explain the reproduction failure yields a blocking finding; advisory findings never flip approved → rejected.
    - Add `TestReviewVerdictByBlockingFindings` generating random finding sets; assert verdict == approved iff no blocking findings. (invariant)
    - **Validates: Requirements 6.3, 6.4, 6.6**

  - [ ]* 5.6 Write property test for the review loop bound at three
    - **Property 9: Diagnosis Review Loop Bounded at Three Iterations** — three consecutive rejections without approval ⇒ escalation artifact + paused state; the loop never iterates more than three times.
    - Add `TestReviewLoopBoundedAtThree` generating random rejection counts (0–5); assert escalation only at 3 and no iteration >3. (invariant)
    - **Validates: Requirements 6.5**

  - [ ]* 5.7 Write property test that the reviewer does not modify the diagnosis
    - **Property 10: Diagnosis Reviewer Does Not Modify Diagnosis** — `bug-diagnosis.md` content is byte-identical before and after review; only diagnose-revise applies corrections.
    - Add `TestReviewerDoesNotModifyDiagnosis` generating random diagnosis contents; assert content unchanged after `ReviewDiagnosis`. (invariant)
    - **Validates: Requirements 6.7**

- [x] 6. Investigation failure escalation
  - [x] 6.1 Implement the EscalationWriter for all-hypotheses-ruled-out
    - In `.kiro/workflows/clm-bugfix_support.go`: `func ShouldEscalate(investigationPath string) (bool, error)` returns true iff every Root_Cause_Hypothesis is marked "ruled out". When true, write `investigation-escalation-{ISO-8601-seconds-UTC}.md` to the artifact dir and signal PAUSE (do not proceed to diagnose). If at least one hypothesis is "confirmed", return false (proceed to diagnose, no escalation).
    - Verify: all-ruled-out → escalation written + paused; one confirmed → no escalation, proceed.
    - _Requirements: 7.1, 7.3, 7.6_

  - [x] 6.2 Implement the escalation artifact content
    - In `.kiro/workflows/clm-bugfix_support.go`: the escalation content includes the bug report summary, the complete hypotheses list with each ruled-out reason, the reproduction test status (`reproduced`/`unreproducible`/`not_attempted`), the CLM stages examined, and a recommended next action from `{manual-investigation, gather-more-logs, escalate-to-domain-expert, abandon}`.
    - Verify: rendered artifact contains all required fields and a valid recommended action.
    - _Requirements: 7.2_

  - [x] 6.3 Implement append-below-separator and filename format
    - In `.kiro/workflows/clm-bugfix_support.go`: filename is `investigation-escalation-{ISO-8601-timestamp-in-seconds-UTC}.md`. If a file already exists at the computed name from a prior run, append the new content below a `---` horizontal-rule separator rather than overwriting.
    - Verify: repeat-write appends below `---` and preserves prior content.
    - _Requirements: 7.4, 7.5_

  - [ ]* 6.4 Write property test for escalation iff all ruled out
    - **Property 11: Escalation Written If and Only If All Hypotheses Ruled Out** — all ruled out ⇒ escalation written + paused; at least one confirmed ⇒ proceed to diagnose without escalation.
    - Add `TestEscalationIffAllRuledOut` generating random hypothesis sets with varying confirmed/ruled-out states; assert the iff. (invariant)
    - **Validates: Requirements 7.1, 7.6**

  - [ ]* 6.5 Write property test for escalation content, filename, and append behavior
    - **Property 12: Escalation Content, Filename, and Append Behavior** — content has all required fields; filename matches `investigation-escalation-{ISO-8601-seconds-UTC}.md`; a pre-existing file gets new content appended below `---` with prior content preserved.
    - Add `TestEscalationContentComplete` generating random timestamps/hypotheses/stages; assert content + filename + append. (invariant)
    - **Validates: Requirements 7.2, 7.3, 7.4, 7.5**

- [x] 7. Fix step enhancements (PBT + compliance + rollback)
  - [x] 7.1 Add PBT requirement to the fix step prompt
    - Modify `.kiro/workflows/clm-bugfix.workflow.json` `fix` prompt: for any introduced/modified function whose primary logic is sorting, filtering, parsing, serialization, cryptographic transformation, state-machine transition, or data aggregation over a collection, write ≥1 property-based test using `pgregory.net/rapid` (auto-generating inputs). Each PBT is preceded by a comment naming the property type exactly one of `invariant`, `round-trip`, `idempotence`, `metamorphic`, `error condition`. Parsers/serializers get a round-trip test `parse(format(x)) == x` with ≥50 generated inputs. For diagnosis-listed regression cases that are algorithmic, implement them as PBTs (not fixed examples). If a modified function only touches external services (AWS, DB, Keycloak) and has none of the qualifying ops, substitute an integration test (one success + one failure path) with an inline comment explaining the substitution.
    - Verify: prompt enumerates qualifying operations, the annotation set, the round-trip ≥50 rule, and the substitution rule.
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

  - [x] 7.2 Add compliance documentation task injection to the fix step
    - Modify the `fix` prompt: when `bug-investigation.md` contains one or more `[COMPLIANCE-REQUIRED]` markers, add a compliance documentation task to the fix task list before applying any code changes, referencing each affected stage; write a `compliance/` file whose filename contains the bug report identifier with sections for GDPR, PCI-DSS, SOC 2, and HIPAA impact. When `[CRYPTO-IMPACT]` is present, ensure `bug-diagnosis.md` includes a cryptographic integrity impact assessment (PKI signature integrity + X.509 timestamping correctness).
    - Verify: prompt injects the compliance task before code changes and names all four sections.
    - _Requirements: 11.5, 11.6_

  - [x] 7.3 Add the rollback section requirement to the fix step
    - Modify the `fix` prompt: `bug-fix-summary.md` must include a `## Rollback` top-level section enumerating every created/modified file (path relative to `workspace_root`, with created/modified indicator); each migration script with its rollback command or down-migration filename; each changed config key with prior value and fix-introduced value; a numbered procedure of 1–20 steps, one discrete action per step; `[DOWNTIME]` on downtime steps and `[DATA-LOSS-RISK]` on data-destroying steps. If no files/migrations/configs changed, a single "No rollback actions required." step.
    - Verify: prompt defines all rollback elements and the no-change fallback.
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5, 9.6, 9.7, 9.8_

  - [x] 7.4 Implement the rollback section validator in Go
    - In `.kiro/workflows/clm-bugfix_support.go`: `func ValidateRollbackSection(summaryPath string) error` checking the section is present as top-level; every changed file is enumerated with created/modified indicator; migrations list rollback commands; config keys list prior+new values; numbered procedure has 1–20 steps each with one action.
    - Verify: a complete rollback section passes; a missing or malformed one fails with a specific message.
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5_

  - [ ]* 7.5 Write property test for PBT required for algorithmic functions with annotations
    - **Property 13: Property-Based Tests Required for Algorithmic Functions with Valid Annotations** — qualifying functions get ≥1 PBT with auto-generated inputs; each has a preceding comment naming exactly one of the five property types; diagnosis-specified algorithmic regression cases are PBTs, not fixed examples.
    - Add `TestPBTRequiredForAlgorithmic` generating random function signatures with varying operation types; assert PBT presence + valid annotation. (invariant)
    - **Validates: Requirements 8.1, 8.2, 8.5, 8.6**

  - [ ]* 7.6 Write property test for round-trip parsers/serializers
    - **Property 14: Round-Trip Property for Parsers and Serializers** — a round-trip test asserts `parse(format(x)) == x` for ≥50 generated inputs per run.
    - Add `TestRoundTripForParsers` generating random data structures; assert the round-trip holds over ≥50 inputs. (round-trip)
    - **Validates: Requirements 8.3**

  - [ ]* 7.7 Write property test for rollback section completeness
    - **Property 15: Rollback Section Completeness and Well-Formedness** — `## Rollback` is a top-level section; every created/modified file enumerated (relative path + indicator); migrations have rollback commands; changed config keys have prior+new values; numbered procedure of 1–20 steps with one action each.
    - Add `TestRollbackSectionComplete` generating random file-change sets; assert completeness via `ValidateRollbackSection`. (invariant)
    - **Validates: Requirements 9.1, 9.2, 9.3, 9.4, 9.5**

  - [ ]* 7.8 Write unit tests for rollback markers and no-change fallback
    - Add `TestRollbackNoChangesSingleStep` ("No rollback actions required."), `TestDowntimeMarkerOnRollbackStep` (`[DOWNTIME]`), `TestDataLossRiskMarker` (`[DATA-LOSS-RISK]`).
    - _Requirements: 9.6, 9.7, 9.8_

- [x] 8. Checkpoint - Ensure all tests pass (diagnosis review, escalation, fix step)
  - Ensure all tests pass, ask the user if questions arise.

- [x] 9. Extended validation gate
  - [x] 9.1 Add the validation-gate step to the workflow JSON
    - Modify `.kiro/workflows/clm-bugfix.workflow.json`: append a `validation-gate` step (`id: "validation-gate"`, `agent: "wf-coder"`) as the final step after `fix`. Its prompt runs all checks: reproduction test passes (skip if `unreproducible`, then verify ≥2 strategies documented); all diagnosis-listed regression cases have corresponding test files; compliance/audit impact from diagnosis addressed; `make test` and `make lint` both run regardless of other results; compliance doc exists with all four sections when `[COMPLIANCE-REQUIRED]`; crypto assessment present in diagnosis when `[CRYPTO-IMPACT]`; write `bugfix-validation-passed.json` (all pass) or `bugfix-validation-failed.json` (any fail), mutually exclusive.
    - Verify: step is last; all seven checks described; mutual exclusivity stated.
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5, 10.6, 10.7_

  - [x] 9.2 Implement ValidationGate logic in Go
    - In `.kiro/workflows/clm-bugfix_support.go`: `func RunValidationGate(...) (allPass bool, checks []CheckResult)` executing the checks above. `make test` and `make lint` always run and their full output is captured in `detail` on failure. Reproduction test check skipped when `unreproducible` (then verify ≥2 strategies documented).
    - Verify: all-pass returns true with all `pass`; a failing check returns false with its detail.
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.7_

  - [x] 9.3 Implement passed/failed JSON mutual exclusion and unreproducible bypass
    - In `.kiro/workflows/clm-bugfix_support.go`: `func WriteValidationResult(path string, passed bool, checks []CheckResult, bugReportID string) error` writing exactly one of `bugfix-validation-passed.json` (timestamp, checks all "pass", bug_report_id) or `bugfix-validation-failed.json` (timestamp, all checks with pass/fail + failure details, bug_report_id). The two files are mutually exclusive.
    - Verify: all-pass writes only passed.json; any-fail writes only failed.json; never both, never neither.
    - _Requirements: 10.5, 10.6, 10.7_

  - [ ]* 9.4 Write property test for validation gate comprehensiveness
    - **Property 16: Validation Gate Check Comprehensiveness** — the gate runs the reproduction test (unless unreproducible), verifies regression test presence, verifies compliance/audit impact addressed (if non-empty), and runs both `make test` and `make lint` regardless of other results; for unreproducible, skips the reproduction check and verifies ≥2 strategies documented; any failure fails the gate.
    - Add `TestValidationGateComprehensive` generating random check-result combinations; assert all checks run and any failure fails. (invariant)
    - **Validates: Requirements 10.1, 10.2, 10.3, 10.4, 10.7**

  - [ ]* 9.5 Write property test for validation result file mutual exclusion
    - **Property 17: Validation Result File Mutual Exclusion** — all pass ⇒ only `bugfix-validation-passed.json`; any fail ⇒ only `bugfix-validation-failed.json` (with all checks + failure details); the two never coexist.
    - Add `TestValidationResultMutualExclusion` generating random all-pass vs any-fail sets; assert exactly one file exists. (invariant)
    - **Validates: Requirements 10.5, 10.6**

  - [ ]* 9.6 Write unit test for all-pass writing passed.json
    - Add `TestValidationAllPassWritesPassedJson`: all checks pass ⇒ `bugfix-validation-passed.json` written with all `pass` and the bug report id.
    - _Requirements: 10.5, 10.6_

- [x] 10. Compliance gating for approval and e-signature paths
  - [x] 10.1 Implement compliance doc existence and completeness checks
    - In `.kiro/workflows/clm-bugfix_support.go`: `func CheckComplianceDoc(workspaceRoot, bugReportID string) (found bool, missingSections []string)` finds a file in `{{resolved_root}}/compliance/` whose filename contains the bug report id (case-insensitive) and verifies sections GDPR, PCI-DSS, SOC 2, HIPAA are present. Active only when `[COMPLIANCE-REQUIRED]` is present; skipped (no failure) when no marker.
    - Verify: missing file → found=false, all sections missing; file present but missing a section → that section listed.
    - _Requirements: 11.1, 11.2, 11.4_

  - [x] 10.2 Implement crypto impact assessment check
    - In `.kiro/workflows/clm-bugfix_support.go`: `func CheckCryptoAssessment(diagnosisPath string) (present bool)` verifies `bug-diagnosis.md` includes a cryptographic integrity impact assessment (PKI signature integrity + X.509 timestamping) when `[CRYPTO-IMPACT]` is present; skipped when no marker.
    - Verify: marker present + assessment absent → fails; marker absent → skipped.
    - _Requirements: 11.3, 11.4_

  - [ ]* 10.3 Write property test for compliance gating active iff markers present
    - **Property 18: Compliance Gating Active If and Only If Markers Present** — `[COMPLIANCE-REQUIRED]` ⇒ compliance doc existence + four sections verified; `[CRYPTO-IMPACT]` ⇒ crypto assessment in diagnosis verified; no markers ⇒ all compliance checks skipped and no failure for missing docs.
    - Add `TestComplianceGatingActiveIffMarkers` generating random marker combinations + doc contents; assert the iff. (invariant)
    - **Validates: Requirements 11.1, 11.2, 11.3, 11.4**

  - [ ]* 10.4 Write unit tests for missing sections and crypto assessment
    - Add `TestComplianceDocMissingSections`, `TestCryptoAssessmentMissing`.
    - _Requirements: 11.2, 11.3_

- [x] 11. Tamper-evident audit logging
  - [x] 11.1 Implement the append-only AuditLogger and run-entry schema
    - In `.kiro/workflows/clm-bugfix_support.go`: `func AppendAuditRun(logPath string, entry AuditRunEntry) error` appends a JSON object to the top-level array of `workflow-audit-log.json` (creates the array if absent). `AuditRunEntry` records run_id, run_start, run_end, workspace_root, component, bug_report_id, ordered steps (step_id, step_start, step_end, outcome), diagnosis_review_iterations, compliance_gating, artifacts_written, reproduction_test_outcome, workflow_definition_version. Prior records preserved; array length grows by exactly one. Secret inputs (names ending `_secret`/`_token`/`_password`/`_key` or marked secret) omitted; only bug_report, workspace_root, component recorded.
    - Verify: append grows the array by one and leaves prior entries byte-identical.
    - _Requirements: 12.1, 12.3, 12.4, 12.5, 12.6, 12.8, 15.3, 15.4_

  - [x] 11.2 Implement SHA-256 hash computation over canonical JSON
    - In `.kiro/workflows/clm-bugfix_support.go`: `func ComputeAuditHash(entry AuditRunEntry) string` computes SHA-256 over the canonical JSON serialization (all fields except `hash`, keys sorted alphabetically, no insignificant whitespace) with `hash` set to `""`, then stores `"sha256:<hex>"` in the `hash` field. `func VerifyAuditHash(entry AuditRunEntry) (matches bool)` recomputes and compares.
    - Verify: stored hash equals a freshly recomputed hash over the same content; tampering any field makes `VerifyAuditHash` return false.
    - _Requirements: 12.1, 15.1, 15.2_

  - [x] 11.3 Implement partial-run capture and filesystem error handling
    - In `.kiro/workflows/clm-bugfix_support.go`: `func FlushPartialAuditLog(entry AuditRunEntry, outcome string) error` finalizes the entry (run_end + steps up to and including the failing/pausing step) and appends before any HALT/PAUSE. If the file cannot be written due to a filesystem error, surface `ERROR: Audit logging failed: <reason>` and do NOT report the run as successfully completed.
    - Verify: a halt mid-run records all steps executed so far; a simulated write error surfaces the ERROR and blocks success.
    - _Requirements: 12.2, 12.7_

  - [x] 11.4 Implement audit log rotation at 10,000 records and workflow-version recording
    - In `.kiro/workflows/clm-bugfix_support.go`: when the file exceeds 10,000 records, rotate to `workflow-audit-log-001.json` (numeric suffix), recording a `rotation` step as the first entry in the new file; the prior file is preserved unmodified. Record the workflow definition version used for each run.
    - Verify: at 10,001 records a new suffixed file is created with a rotation first entry; the prior file is unchanged.
    - _Requirements: 15.5, 15.6_

  - [ ]* 11.5 Write property test for the audit log append-only invariant
    - **Property 19: Audit Log Append-Only Invariant** — after appending a new run, all prior records are preserved unmodified; array length increases by exactly one; any overwrite/remove attempt fails.
    - Add `TestAuditLogAppendOnly` generating random run sequences; assert prior entries unchanged + length +1. (invariant)
    - **Validates: Requirements 12.3, 15.3**

  - [ ]* 11.6 Write property test for audit log hash tamper-evidence
    - **Property 20: Audit Log Hash Tamper-Evidence via Canonical Computation** — the SHA-256 hash is over canonical JSON (keys sorted, no insignificant whitespace) with the hash field empty; recomputation over the same content equals the stored hash; any difference ⇒ tampered.
    - Add `TestAuditLogHashTamperEvident` generating random entries and random tampered fields; assert `VerifyAuditHash` matches for untampered and flags tampered. (invariant)
    - **Validates: Requirements 12.1, 15.1, 15.2**

  - [ ]* 11.7 Write property test for audit log run metadata completeness
    - **Property 21: Audit Log Run Metadata Completeness** — each entry records all required fields (run_id, timestamps, workspace_root, component, bug_report_id, ordered steps with outcomes, artifact absolute paths, diagnosis review iterations, compliance gating, reproduction test outcome, workflow definition version); a failed/paused run finalizes with all steps up to and including the failing/pausing step.
    - Add `TestAuditLogMetadataComplete` generating random run scenarios; assert all fields present and partial-run capture correct. (invariant)
    - **Validates: Requirements 12.2, 12.4, 12.5, 12.6, 12.8, 15.6**

  - [ ]* 11.8 Write unit tests for write failure and rotation
    - Add `TestAuditLogWriteFailureSurfacesError`, `TestAuditLogRotationAtTenThousand`.
    - _Requirements: 12.7, 15.5_

- [x] 12. Checkpoint - Ensure all tests pass (validation gate, compliance, audit)
  - Ensure all tests pass, ask the user if questions arise.

- [x] 13. Integration, end-to-end wiring, and backward compatibility
  - [x] 13.1 Wire the audit_log hooks around all steps and finalize the workflow JSON
    - Modify `.kiro/workflows/clm-bugfix.workflow.json`: wire the `audit_log` cross-cutting concern as pre/post hooks around every step (validate-component, preflight-scan, investigate, diagnose, diagnosis-reviewer, diagnose-revise, fix, validation-gate), recording step boundaries, outcomes, artifact paths, diagnosis review iterations, and compliance gating. Finalize the full nine-step ordering and the `audit_log` schema (path, append_only, hash_computation, secret_exclusion, flush_on_halt_or_pause, write_failure_handling, record_artifact_paths, record_diagnosis_review_iterations, record_compliance_gating). Confirm existing artifact filenames are unchanged and new artifact names differ from the three originals.
    - Verify: `go test ./... -count=1` (structural tests) passes; `TestWorkflowJsonValidStructure`, `TestStepOrderingCorrect`, `TestAuditLogInitialized` pass; JSON is valid.
    - _Requirements: 12.1, 12.4, 12.5, 12.6, 13.3, 13.4_

  - [ ]* 13.2 Write integration test: startup gate halts on missing/non-existent workspace root
    - Add to `.kiro/workflows/clm-bugfix.integration_test.go` (build tag `integration`): invoke the workflow with a non-existent `workspace_root`; assert HALT before any step and a partial audit log with outcome `failed`.
    - _Requirements: 1.4, 13.2_

  - [ ]* 13.3 Write integration test: startup gate halts on invalid component
    - Add: invoke with an invalid `component`; assert HALT before any pipeline step, an error log containing the invalid value and the full allowlist, and no output artifacts produced.
    - _Requirements: 2.3_

  - [ ]* 13.4 Write integration test: preflight conflicts pause before investigate
    - Add: test git fixture with uncommitted changes, an unmerged branch, and a prior `bug-investigation.md`; assert `preflight-conflicts.md` written and the pipeline halts before investigate; with `preflight-acknowledged.json` present, assert the pause is skipped and investigate proceeds.
    - _Requirements: 3.4_

  - [ ]* 13.5 Write integration test: diagnosis review loop pauses after 3 rejected iterations
    - Add: drive the review loop to reject three times; assert an escalation artifact is written and the workflow transitions to a paused state; no iteration exceeds 3.
    - _Requirements: 6.5_

  - [ ]* 13.6 Write integration test: escalation written when all hypotheses ruled out
    - Add: investigation where every hypothesis is ruled out; assert `investigation-escalation-{timestamp}.md` is written, pipeline pauses, and does not proceed to diagnose; a second run appends below `---`.
    - _Requirements: 7.1, 7.3_

  - [ ]* 13.7 Write integration test: end-to-end pipeline produces all artifacts and validation passes
    - Add: a known bug; run the full pipeline; assert `bug-investigation.md` (with reproduction test + stage impact), `bug-diagnosis.md`, `bug-diagnosis-reviewed.json` (approved), `bug-fix-summary.md` (with rollback), `bugfix-validation-passed.json`, and `workflow-audit-log.json` are produced; the reproduction test passes post-fix; `make test`/`make lint` pass.
    - _Requirements: All_

  - [ ]* 13.8 Write integration test: backward-compatible invocation with only bug_report + component
    - Add: invoke with only `bug_report` and `component` (no `workspace_root`); assert a WARNING is emitted, cwd is used as root, and the full pipeline runs with documented defaults without a non-zero exit.
    - _Requirements: 13.5_

- [x] 14. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- All implementation is in **Go**. Code examples in the workflow JSON steps and in the Go support file use Go (`pgregory.net/rapid` for property-based tests; the project's `testing` package for the reproduction test).
- Tasks marked with `*` are optional test sub-tasks (unit, property-based, and integration tests). The implementer MUST implement the non-`*` sub-tasks and MUST NOT implement the `*` sub-tasks during this workflow; `*` tasks may be skipped for a faster MVP.
- Property-based tests run with a minimum of 100 iterations (50 for round-trip parser/serializer properties). Each PBT is preceded by `// Feature: structured-bugfix-improvement, Property N: <title>` and tagged with its property type (invariant, round-trip, idempotence, metamorphic, error condition).
- Each task references the specific requirements/acceptance criteria it satisfies for traceability. The 23 correctness properties from the design each map to one PBT sub-task placed next to the implementation it tests.
- Integration tests use a build tag (`integration`) and run via `go test ./... -count=1 -tags=integration`; the default `go test ./... -count=1` run covers property + unit + structural tests. CI (`go test ./... -count=1`, `go vet ./...`) discovers the new tests automatically.
- Checkpoints (`4`, `8`, `12`, `14`) are validation breaks; they are not part of the dependency graph.
- The dependency graph below schedules leaf sub-tasks into waves. Tasks writing the same file are placed in different waves to avoid conflicts; tasks in the same wave are independent and run in parallel; tasks in wave N only start after all tasks in waves 0..N-1 complete. Optional (`*`) sub-tasks are included in the graph.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.4"] },
    { "id": 1, "tasks": ["1.3", "11.1", "1.5"] },
    { "id": 2, "tasks": ["2.1", "11.2", "1.6"] },
    { "id": 3, "tasks": ["3.1", "2.2", "1.9"] },
    { "id": 4, "tasks": ["3.2", "2.3", "1.8"] },
    { "id": 5, "tasks": ["5.1", "2.4", "2.6"] },
    { "id": 6, "tasks": ["5.4", "5.2", "2.7"] },
    { "id": 7, "tasks": ["7.1", "6.1", "3.5"] },
    { "id": 8, "tasks": ["9.1", "5.3", "2.8"] },
    { "id": 9, "tasks": ["3.3", "9.2", "5.5"] },
    { "id": 10, "tasks": ["3.4", "9.3", "3.7"] },
    { "id": 11, "tasks": ["7.3", "10.1", "3.6"] },
    { "id": 12, "tasks": ["7.2", "7.4", "9.4"] },
    { "id": 13, "tasks": ["1.2", "10.2", "7.7"] },
    { "id": 14, "tasks": ["13.1", "6.2", "1.7"] },
    { "id": 15, "tasks": ["11.3", "11.6", "13.2"] },
    { "id": 16, "tasks": ["11.4", "11.7", "13.3"] },
    { "id": 17, "tasks": ["6.3", "11.8", "13.4"] },
    { "id": 18, "tasks": ["2.5", "6.5", "13.5"] },
    { "id": 19, "tasks": ["5.7", "13.6"] },
    { "id": 20, "tasks": ["6.4", "13.7"] },
    { "id": 21, "tasks": ["7.5", "13.8"] },
    { "id": 22, "tasks": ["7.6"] },
    { "id": 23, "tasks": ["5.6"] },
    { "id": 24, "tasks": ["9.5"] },
    { "id": 25, "tasks": ["9.6"] },
    { "id": 26, "tasks": ["7.8"] },
    { "id": 27, "tasks": ["10.3"] },
    { "id": 28, "tasks": ["10.4"] },
    { "id": 29, "tasks": ["2.9"] },
    { "id": 30, "tasks": ["11.5"] }
  ]
}
```
