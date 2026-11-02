# Implementation Plan: Structured Feature Dev Improvement

## Overview

Enhance `clm-feature-dev.workflow.json` and its associated agent prompts across ten reliability gaps organized into nine implementation groups. The changes are purely to workflow JSON configuration and embedded agent prompt strings — no application source code is introduced. Property-based and integration tests for the new logic are written in Go using `pgregory.net/rapid`, co-located with the workflow tooling tests.

All changes are backward-compatible: callers that omit new optional inputs continue to work with documented defaults.

---

## Tasks

- [x] 1. Foundation — Workspace-root portability in workflow JSON
  - [x] 1.1 Add `workspace_root` input field to `clm-feature-dev.workflow.json` and replace all hardcoded absolute paths with `{{resolved_root}}` template variable
    - Open `.kiro/workflows/clm-feature-dev.workflow.json`
    - Add `"workspace_root": "string"` under the `"inputs"` object alongside `feature_request` and `component`
    - Replace every occurrence of `d:\\cmd project\\M2P-Git\\CLM` (and any variant) in `"path"`, `"prompt"`, and `"escalationFile"` values with `{{resolved_root}}`
    - Verify: JSON parses cleanly (`jq . < clm-feature-dev.workflow.json`); grep for hardcoded drive-letter paths returns zero results
    - _Requirements: 1.1, 1.2, 1.5_

  - [x] 1.2 Add ComponentValidator step `validate-component` at index 0 of the `steps` array with allowlist enforcement and exact log message format
    - Insert a new step object `{"type": "step", "id": "validate-component", "timeout_ms": 5000, "on_timeout": "fail", "agent": "wf-component-validator", "prompt": "..."}` as the first element of `steps`
    - Prompt must instruct the agent to: (a) accept component values from `["saas-control-plane", "satellite-gateway", "vsatellite", "shared"]`; (b) emit `WARNING: component not specified; defaulting to saas-control-plane` when omitted; (c) emit `ERROR: Invalid component '<value>'. Valid values: saas-control-plane, satellite-gateway, vsatellite, shared` and HALT when value is outside the allowlist
    - Verify: step appears at index 0 in `steps` array; `timeout_ms` is 5000; log message strings match exactly the format in the design
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_

  - [x] 1.3 Implement WorkspaceRootResolver normalization logic in the `validate-component` prompt
    - Extend the `validate-component` prompt to also handle workspace root: strip trailing `\` or `/` separators from `workspace_root`, check directory existence, default to CWD with warning `WARNING: workspace_root not provided; using current working directory as workspace root` when omitted, HALT with error when path does not exist
    - Expose resolved root as `{{resolved_root}}` for downstream steps
    - Verify: prompt explicitly covers all four branches (omitted → warning + CWD, valid → normalize + proceed, non-existent → HALT, trailing sep → strip); audit log records the resolved root value
    - _Requirements: 1.1, 1.3, 1.4, 1.6, 11.2_

- [x] 2. Checkpoint — Foundation complete
  - Confirm `clm-feature-dev.workflow.json` parses as valid JSON, contains `workspace_root` in `inputs`, has `validate-component` as `steps[0]`, and contains zero hardcoded absolute paths. Ask the user if any questions arise.

- [ ] 3. Pre-Flight Scanner
  - [x] 3.1 Add `preflight-scan` step at index 1 of `steps` array and wire it into the workflow JSON
    - Insert `{"type": "step", "id": "preflight-scan", "timeout_ms": 30000, "on_timeout": "fail", "agent": "wf-preflight-scanner", "prompt": "..."}` immediately after `validate-component`
    - Verify: step appears at index 1; `timeout_ms` is 30000; design step remains at index 2
    - _Requirements: 3.1_

  - [~] 3.2 Implement git status check — uncommitted/staged/untracked file detection with severity classification
    - Write the `preflight-scan` prompt section that runs `git status --porcelain` and classifies each changed file: severity `medium` for files under `components/<component>/`, severity `low` for all other paths; collect results as `Conflict` objects with fields `type`, `file_path`, `severity`
    - When no conflicts found, write `{{resolved_root}}/.agents/tasks/preflight-clean.json` with schema `{"conflicts": [], "status": "clean", "scanned_at": "<ISO-8601>", "workspace_root": "...", "component": "..."}`
    - When conflicts found, write `{{resolved_root}}/.agents/tasks/preflight-conflicts.md` in the Markdown table format from the design
    - Verify: prompt covers the clean path (writes `preflight-clean.json`) and the conflict path (writes `preflight-conflicts.md`); severity mapping matches the design table
    - _Requirements: 3.2, 3.3, 3.4_

  - [~] 3.3 Implement branch divergence detection and stale artifact glob detection
    - Extend the `preflight-scan` prompt to also run `git branch -vv` to find local branches diverged from the default branch and not merged — classify as severity `high`, type `unmerged-branch`
    - Scan `{{resolved_root}}/.agents/tasks/` for files matching globs `feature-*.md`, `feature-*.json`, `bug-*.md`, `bug-*.json`, `plan-output.md`, `spec-*.md` — classify as severity `low`, type `prior-run-artifact`
    - Merge all three conflict sources (git status, branch, stale artifacts) into a single list before writing output
    - Verify: prompt handles all three detection types; table in `preflight-conflicts.md` contains all three `type` values
    - _Requirements: 3.2_

  - [~] 3.4 Implement `preflight-acknowledged.json` bypass and large-workspace scoping
    - At the start of the `preflight-scan` prompt, instruct the agent to check whether `{{resolved_root}}/.agents/tasks/preflight-acknowledged.json` exists with `{"acknowledged": true}`; if so, skip all conflict detection and proceed directly to the design step
    - When the tracked file count (via `git ls-files | wc -l`) exceeds 10,000, scope the git status scan to `{{resolved_root}}/components/{{component}}/` rather than the full workspace root
    - Verify: prompt includes the acknowledgement check as the first conditional; scoping logic is gated on the 10,000-file threshold
    - _Requirements: 3.5, 3.7, 12.4_

  - [~] 3.5 Implement timeout enforcement and git command failure handling in the preflight prompt
    - Instruct the agent to: (a) catch any git command failure, write `preflight-conflicts.md` with `{"status": "unknown", "error": "<message>", "scanned_at": "..."}`, and HALT; (b) enforce 30-second total scan budget — if exceeded, emit `ERROR: PreFlightScanner timed out after <ms>ms (limit: 30000ms)` and HALT
    - The scanner must not create, modify, or delete source files or prior workflow artifacts at any point
    - Verify: prompt includes explicit instructions for both error branches; read-only constraint is stated
    - _Requirements: 3.5, 3.6, 12.2, 12.3, 12.5_

- [~] 4. Checkpoint — Pre-flight complete
  - Confirm `preflight-scan` is at index 1, `design` at index 2, both have `timeout_ms`, and the prompt covers clean / conflict / acknowledged / scoped-scan / git-error paths. Ask the user if any questions arise.

- [ ] 5. Versioned Approval & Escalation
  - [x] 5.1 Update both `repeat` step `stopCondition.fileCheck` paths to use `{{resolved_root}}` and document the versioned ApprovalFile schema
    - In both `design-review-loop` and `implement-review-loop`, replace hardcoded paths in `stopCondition.fileCheck.path` with `{{resolved_root}}/.agents/tasks/<filename>`
    - Add a comment block (or embedded prompt instruction) documenting the ApprovalFile schema: `{"approved": bool, "version": int, "last_written_by": string, "last_written_at": ISO-8601}`
    - Verify: both stop conditions use `{{resolved_root}}`; no hardcoded paths remain
    - _Requirements: 9.1, 1.2_

  - [~] 5.2 Implement VersionedApprovalWrite logic in the design-reviewer and code-reviewer agent prompts
    - Update `design-reviewer` and `code-reviewer` prompts to follow the read-before-write protocol: (1) read current `version` from the approval file; (2) if file unreadable, abort write and emit error; (3) detect version conflict (current version ≠ agent's last recorded version) → emit `WARN: Approval file version conflict on '<path>': expected version <n>, found <m>. Not modifying file. Pipeline paused.` and PAUSE; (4) otherwise write `version + 1`, `last_written_by`, `last_written_at`, and `approved` field; retry up to 3 times on atomic write failure; PAUSE after 3 failures
    - Verify: prompts enumerate all four branches (read fail, conflict, success, retry exhausted)
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5_

  - [~] 5.3 Add `onExhaustion.writeEscalation` to both repeat steps with filename format and append-with-separator behavior
    - Add `"onExhaustion": {"writeEscalation": true, "escalationFile": "{{resolved_root}}/.agents/tasks/{loop-id}-escalation-{{timestamp_utc}}.md"}` to both `design-review-loop` and `implement-review-loop`
    - Update both reviewer agent prompts to produce the escalation artifact content matching the design schema: loop identifier, max iterations, iterations executed, final verdict, unresolved findings, `recommended_next_action` from vocabulary `{manual-approve, revise-and-retry, escalate-to-team, abandon}`
    - Instruct agents to append below `---` separator if the escalation file already exists (never overwrite)
    - Verify: both repeat steps have `onExhaustion.writeEscalation: true`; escalation filename uses `{loop-id}` and `{{timestamp_utc}}`; append behavior is explicitly stated in the prompt
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5_

- [~] 6. Checkpoint — Versioned approval and escalation complete
  - Confirm stop conditions use `{{resolved_root}}`, reviewer prompts include the read-before-write protocol, and both repeat steps have `onExhaustion` with `writeEscalation: true`. Ask the user if any questions arise.

- [ ] 7. Planner Enhancements
  - [x] 7.1 Update planner agent prompt to read CLM stages and produce blast-radius summary table with stage-grouped task list
    - Rewrite the `plan` step prompt to instruct the agent to: (a) read the 26 CLM stage names from `{{resolved_root}}/.kiro/steering/clm-project-context.md`; (b) identify every stage touched by the approved design (criteria: stage's data models, APIs, UI components, or business rules are read, written, created, or modified); (c) produce a blast-radius summary table at the top of `feature-plan.md` with columns: stage name, task count, impact description (≤20 words); (d) group all tasks under their associated CLM stage heading
    - If no stage is touched, include an explicit `### No CLM Stage Impact` section instead of omitting grouping
    - Verify: prompt instructs reading from `clm-project-context.md`; blast-radius table format matches the design; ungrouped tasks are not permitted
    - _Requirements: 4.1, 4.2, 4.3, 4.4_

  - [~] 7.2 Add Cross-Stage section logic and `[COMPLIANCE-REQUIRED]` injection for stages 11 and 12
    - Extend the planner prompt to: (a) place any task touching two or more distinct stages under a `### Cross-Stage` section with all affected stage names listed on that task entry; (b) annotate any task touching stage 11 (Approval Workflow) or stage 12 (Electronic Signature) with `[COMPLIANCE-REQUIRED]` and add a compliance documentation task as the immediately preceding entry in that stage's section
    - Verify: prompt explicitly names stage 11 and stage 12 as compliance triggers; cross-stage logic is distinct from single-stage grouping
    - _Requirements: 4.2, 4.5_

  - [x] 7.3 Add planner guard: halt if approved design document is absent
    - At the top of the planner prompt, instruct the agent to check for `{{resolved_root}}/.agents/tasks/feature-design-approved.json` with `"approved": true`; if absent or `approved != true`, HALT with message `ERROR: Planning requires an approved design document. feature-design-approved.json not found or approved is false.` and produce no plan artifact
    - Verify: prompt check is the first action; error message matches the exact wording; no partial plan is written on failure
    - _Requirements: 4.6_

- [ ] 8. Coder & Reviewer PBT
  - [x] 8.1 Update coder agent prompt with PBT requirement, qualifying function definition, library selection, annotation format, and round-trip minimum
    - Update the `implement` step prompt to add a PBT enforcement section that instructs: (a) any new function whose primary logic includes sorting, filtering, parsing, serialization, cryptographic transformation, state-machine transition, or data aggregation over a collection requires at least one property-based test; (b) use `pgregory.net/rapid` or `github.com/leanovate/gopter` for Go, `fast-check` for TypeScript/JS, `hypothesis` for Python; (c) annotate each PBT with an inline comment immediately before the test function: `// Property: <type> — <description>`, `// Validates: <requirement ref>`, `// Type: invariant | round-trip | idempotence | metamorphic | error condition`; (d) for parsers/serializers, assert `parse(format(x)) == x` with a minimum of 50 automatically generated inputs; (e) general PBT minimum is 100 iterations
    - Verify: prompt names all seven qualifying operations; all three PBT libraries are listed by language; annotation format matches the design exactly; round-trip (50) and general (100) minimums are both stated
    - _Requirements: 5.1, 5.2, 5.3, 5.5_

  - [~] 8.2 Update code-reviewer agent prompt with PBT blocking check
    - Extend the `code-reviewer` step prompt to instruct: any new function meeting the qualifying definition in task 8.1 that lacks a corresponding property-based test with the required annotation comment must be listed as a blocking finding; the review output must not approve (`approved: true`) while any such finding remains open
    - Verify: prompt explicitly states that missing PBT = blocking finding; the blocking finding prevents approval
    - _Requirements: 5.6_

- [ ] 9. Extended Validation Gate
  - [~] 9.1 Implement open-question scan in the validator prompt
    - Rewrite the `validate` step prompt to add an open-question scan as the first check: scan `feature-design.md` line by line for `TODO`, `TBD`, `OPEN:`, `??`, and for items in open-questions sections that lack `[RESOLVED]` or `[ANSWERED]`; if any are found, report each by line number and text in `feature-validation-failed.json`
    - Verify: prompt lists all five markers; `[RESOLVED]`/`[ANSWERED]` exemption is explicit; line number reporting is required
    - _Requirements: 6.1_

  - [~] 9.2 Implement migration script check in the validator prompt
    - Extend the validator prompt to: when `[MIGRATION-REQUIRED]` is present in `feature-plan.md`, derive the feature slug (feature_request lowercased, spaces → hyphens, special chars stripped) and check `{{resolved_root}}/deploy/migrations/` for a filename containing the slug (case-insensitive); fail the gate if not found
    - Verify: prompt derives the slug with the exact transformation rule; case-insensitive match is stated; both pass (migration exists) and fail (migration absent) paths are covered
    - _Requirements: 6.2_

  - [~] 9.3 Implement compliance doc check in the validator prompt
    - Extend the validator prompt to: when `[COMPLIANCE-REQUIRED]` is present in `feature-plan.md`, check `{{resolved_root}}/compliance/` for a filename containing the feature slug (case-insensitive); fail the gate if not found
    - Verify: case-insensitive slug match is stated; failure detail message identifies the slug and directory searched
    - _Requirements: 6.3_

  - [~] 9.4 Implement consolidated validation result files: `feature-validation-passed.json` and `feature-validation-failed.json`
    - Extend the validator prompt to collect all check results (open-question scan, migration check, compliance check, `make test`, `make lint`, plan-tasks-complete) and: (a) if all pass, write `feature-validation-passed.json` with run timestamp, feature name, and list of checks each with `"result": "pass"`; (b) if any fail, write `feature-validation-failed.json` with run timestamp, feature name, all check results, and `"detail"` field for each failed check; never write both files in the same run; never write `feature-validation-passed.json` on any failure
    - The prompt must instruct running `make test` and `make lint` from `{{resolved_root}}` and capturing full output on non-zero exit
    - Verify: mutual-exclusion rule is explicit; all six checks are listed; `detail` field is required for failures; full command output capture is stated
    - _Requirements: 6.4, 6.5, 6.6_

- [~] 10. Checkpoint — Validation gate complete
  - Confirm validator prompt covers all six checks, mutual-exclusion of result files, and the feature-slug derivation rule. Ask the user if any questions arise.

- [ ] 11. Artifact Quality — Design Changelog
  - [x] 11.1 Update design agent prompt to append `## Changelog` section on first creation
    - Extend the `design` step prompt to instruct: after writing `feature-design.md`, append a `## Changelog` section with a single initial entry formatted as `### Iteration 1 — <ISO-8601 timestamp to the second> — agent:design` followed by `Initial design created.`
    - Verify: prompt states append (not insert or overwrite); timestamp format is ISO-8601 accurate to the second; agent identity is `agent:design`
    - _Requirements: 7.1_

  - [~] 11.2 Update design-revise agent prompt for per-execution changelog append with all edge cases
    - Extend the `design-revise` step prompt to instruct: (a) before modifying `feature-design.md`, if no `## Changelog` section exists, append one; (b) after all modifications, append exactly one new entry: `### Iteration <n+1> — <ISO-8601> — agent:design-revise` with a structured list of section headings added, modified, or removed; (c) if the revision produced no substantive changes (whitespace/punctuation/formatting only), append the entry with note `No substantive changes` rather than omitting it; (d) never modify or remove prior changelog entries; (e) the limit is one entry per step execution regardless of how many sections changed
    - Verify: prompt covers all five sub-cases; append-only invariant is stated; one-entry-per-execution cap is explicit
    - _Requirements: 7.2, 7.3, 7.4, 7.5, 7.6_

- [ ] 12. Artifact Quality — Rollback Section
  - [~] 12.1 Update validator agent prompt to write `## Rollback` section in `feature-summary.md`
    - Extend the `validate` step prompt to instruct: write a `## Rollback` section as a required top-level section (not an appendix) in `feature-summary.md` containing: (a) an enumeration of every file created or modified by the feature with path relative to `workspace_root` and status `created` or `modified`; (b) for database schema migrations, list each migration script filename and the exact rollback command or down-migration filename; (c) for changed configuration files or manifests, list each changed key with prior value and new value; (d) a numbered procedure of 1–20 sequential steps to revert the feature; annotate any step requiring downtime with `[DOWNTIME]`; annotate any step permanently destroying data with `[DATA-LOSS-RISK]`; (e) if no files were created or modified and no migrations or config changes occurred, include an explicit statement `No rollback actions required.`
    - Verify: prompt states `## Rollback` is required (not optional); step-count bounds (1–20) are explicit; both annotation markers are defined; the zero-change edge case is handled
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 8.8_

- [ ] 13. Audit Logger
  - [ ] 13.1 Implement AuditLogger cross-cutting concern: append-only `workflow-audit-log.json` with full run entry schema
    - Create a new agent prompt file or extend the workflow's shared pre/post hook instructions to maintain `{{resolved_root}}/.agents/tasks/workflow-audit-log.json` as an append-only top-level JSON array
    - Each run entry must contain: `run_id` (UUID), `run_start` (ISO-8601), `run_end` (ISO-8601), `workspace_root`, `component`, `feature_request`, `steps` array (each: `step_id`, `step_start`, `step_end`, `outcome` as `success|paused|failed`), `approval_file_events` array (each: `file`, `written_at`, `version_written`), `artifacts_written` array of absolute paths, `hash` (SHA-256 — see task 13.2)
    - Exclude any input whose name ends in `_secret`, `_token`, `_password`, or `_key`, and any input marked `"secret": true`
    - Verify: schema contains all required top-level fields; `steps`, `approval_file_events`, and `artifacts_written` are arrays; secret exclusion rule is stated
    - _Requirements: 13.1, 13.4, 13.5_

  - [~] 13.2 Implement SHA-256 integrity hash computation over run entry
    - Instruct the agent to compute the SHA-256 hash over the JSON-serialized run entry with `"hash"` set to `""` (empty string), then set the `"hash"` field to `"sha256:<hex>"` before writing
    - Verify: the two-step computation (serialize with empty hash → compute → embed) is explicitly described; the prefix format is `sha256:`
    - _Requirements: 13.1_

  - [~] 13.3 Implement partial-run capture and filesystem error handling
    - Instruct the agent to: (a) flush the partial audit log (with steps executed so far and `run_end` set to the halt timestamp) before any HALT or PAUSE operation; (b) if `workflow-audit-log.json` cannot be written due to a filesystem error, surface `ERROR: Audit logging failed: <reason>` and do NOT report the run as successfully completed
    - Verify: flush-before-halt instruction is present; filesystem error results in an explicit error surface (not silent failure); successful-run report is blocked by log write failure
    - _Requirements: 13.2, 13.3, 13.6_

- [~] 14. Final Checkpoint — All workflow changes complete
  - Confirm `clm-feature-dev.workflow.json` is valid JSON; all nine groups of changes are present; no hardcoded absolute paths remain; `workflow-audit-log.json` is included in artifact list. Ask the user if any questions arise.

- [ ] 15. Property-Based Tests — ComponentValidator
  - [~] 15.1 Write property test `TestAllowlistedComponentAccepted` for ComponentValidator
    - File: `components/saas-control-plane/internal/workflow/validator_test.go` (create if absent)
    - Using `pgregory.net/rapid`, generate each of the four exact allowlist strings and assert `validate_component(s)` returns `Ok(s)` with no warning emitted
    - Annotate with: `// Property: invariant — allowlisted component is accepted without warning`, `// Validates: Requirements 2.2`, `// Type: invariant`
    - Minimum 100 iterations (rapid default)
    - Verify: `go test ./internal/workflow/...` passes; annotation comment is present immediately before the test function
    - _Requirements: 2.2, 5.1, 5.2_

  - [ ]* 15.2 Write property test `TestNonAllowlistedComponentRejected` for ComponentValidator
    - File: same as 15.1
    - Using `rapid.StringN` filtered to exclude the four valid values, assert `validate_component(s)` returns `Err` whose message contains both the input value and all four valid values
    - Annotate with: `// Property: error condition — non-allowlisted component is rejected with informative error`, `// Validates: Requirements 2.3`, `// Type: error condition`
    - Verify: `go test ./internal/workflow/...` passes; error message format validated in assertion
    - _Requirements: 2.3, 5.1, 5.2_

- [ ] 16. Property-Based Tests — VersionedApprovalWrite
  - [~] 16.1 Write property test `TestApprovalVersionMonotonicity`
    - File: `components/saas-control-plane/internal/workflow/approval_test.go` (create if absent)
    - Using `rapid.IntRange(1, 20)` to generate N, simulate N sequential agent writes to a temp approval file and assert: `version` after write N equals N; each write produces version exactly 1 greater than the version it read
    - Annotate with: `// Property: invariant — version field increments monotonically with each write`, `// Validates: Requirements 9.1, 9.2`, `// Type: invariant`
    - Minimum 100 iterations
    - Verify: `go test ./internal/workflow/...` passes
    - _Requirements: 9.1, 9.2_

  - [ ]* 16.2 Write property test `TestApprovalRoundTripPreservesApprovedField`
    - File: same as 16.1
    - Using `rapid.Bool()`, generate an initial `approved` value, write approval file with that value, read it back, and assert the `approved` field is unchanged
    - Annotate with: `// Property: round-trip — write-read preserves the approved field value`, `// Validates: Requirements 9.2`, `// Type: round-trip`
    - Verify: `go test ./internal/workflow/...` passes
    - _Requirements: 9.2_

- [ ] 17. Property-Based Tests — ValidationGate open-question scan
  - [ ]* 17.1 Write property test `TestOpenQuestionScannerPrecisionRecall`
    - File: `components/saas-control-plane/internal/workflow/validation_test.go` (create if absent)
    - Using `rapid` generators, produce design document strings with a randomly chosen mix of lines containing `TODO`, `TBD`, `OPEN:`, `??`, `[RESOLVED]`, and neutral lines; assert `scan_open_questions(doc)` returns exactly the set of lines containing any of the four markers and not exempted by `[RESOLVED]`/`[ANSWERED]` — no more, no fewer
    - Annotate with: `// Property: invariant — open-question scan returns precisely the unresolved markers`, `// Validates: Requirements 6.1`, `// Type: invariant`
    - Minimum 100 iterations
    - Verify: `go test ./internal/workflow/...` passes
    - _Requirements: 6.1_

- [ ] 18. Integration Test — Full startup gate
  - [~] 18.1 Write integration test `TestStartupGateHaltsOnMissingWorkspaceRoot`
    - File: `components/saas-control-plane/internal/workflow/integration_test.go` (create if absent)
    - Mock the workspace resolution to return a non-existent path; invoke the startup gate; assert HALT before any design step executes and audit log captures the partial run
    - Verify: `go test -run TestStartupGate ./internal/workflow/...` passes
    - _Requirements: 1.4, 13.2_

  - [~] 18.2 Write integration test `TestStartupGateHaltsOnInvalidComponent`
    - File: same as 18.1
    - Provide a valid workspace root but an invalid component string; invoke the startup gate; assert HALT before `preflight-scan` executes and error message contains the invalid value and all four valid values
    - Verify: `go test -run TestStartupGate ./internal/workflow/...` passes
    - _Requirements: 2.3, 12.1_

  - [ ]* 18.3 Write integration test `TestStartupGatePausesOnPreflightConflicts`
    - File: same as 18.1
    - Mock `git status --porcelain` to return one uncommitted file in the target component; invoke the startup gate; assert PAUSE before design step, `preflight-conflicts.md` is written, `preflight-clean.json` is absent
    - Verify: `go test -run TestStartupGate ./internal/workflow/...` passes
    - _Requirements: 3.3, 3.4_

  - [ ]* 18.4 Write integration test `TestStartupGateProceedsWithAcknowledgedConflicts`
    - File: same as 18.1
    - Mock the same conflict scenario as 18.3 but pre-create `preflight-acknowledged.json` with `{"acknowledged": true}`; assert design step executes without pause
    - Verify: `go test -run TestStartupGate ./internal/workflow/...` passes
    - _Requirements: 3.7_

---

## Notes

- Tasks marked with `*` are optional and can be skipped for a faster MVP; they add test coverage but are not required for the workflow to function correctly
- All workflow changes target `.kiro/workflows/clm-feature-dev.workflow.json` and the agent prompt strings embedded in each step — no CLM application source is modified by tasks 1–14
- Test files in tasks 15–18 are co-located with the workflow tooling; if a `workflow` package does not yet exist, create it as part of task 15.1
- The `pgregory.net/rapid` module must be available in `go.mod` before running property tests
- Checkpoints (tasks 2, 4, 6, 10, 14) do not produce code — they are verification gates to catch drift before proceeding to dependent groups
- Property tests reference design correctness properties: P3/P4 → tasks 15.x, P16 → tasks 16.x, P25 → task 17.1

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2", "1.3"] },
    { "id": 1, "tasks": ["3.1", "5.1", "7.1", "7.3", "8.1", "11.1", "13.1"] },
    { "id": 2, "tasks": ["3.2", "5.2", "7.2", "8.2", "11.2", "12.1", "13.2"] },
    { "id": 3, "tasks": ["3.3", "5.3", "9.1", "13.3"] },
    { "id": 4, "tasks": ["3.4", "9.2"] },
    { "id": 5, "tasks": ["3.5", "9.3"] },
    { "id": 6, "tasks": ["9.4"] },
    { "id": 7, "tasks": ["15.1", "16.1", "18.1", "18.2"] },
    { "id": 8, "tasks": ["15.2", "16.2", "17.1", "18.3", "18.4"] }
  ]
}
```
