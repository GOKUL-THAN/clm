// Package workflows contains the pure, testable supporting logic for the
// CLM Structured Bug-Fix workflow (clm-bugfix.workflow.json).
//
// The workflow itself is a declarative JSON pipeline definition that drives AI
// agents through the investigate → diagnose → fix lifecycle. The functions in
// this file implement the deterministic, side-effect-free pieces of that
// pipeline — workspace root resolution, path normalization, and component
// input validation — so they can be exercised by unit and property-based
// tests independently of agent execution.
//
// Conventions mirror the sibling clm-feature-dev.workflow.json: the
// {{resolved_root}} template variable, the validate-component startup step,
// and the audit_log cross-cutting concern.
//
// This file implements task 1.4 of the structured-bugfix-improvement spec.
package workflows

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ValidComponents is the exhaustive allowlist of CLM subsystems accepted by the
// workflow's component input. Matching is case-sensitive and exact; callers
// must not trim or otherwise normalize the input before comparison.
//
// Requirement 2.1.
var ValidComponents = []string{
	"saas-control-plane",
	"satellite-gateway",
	"vsatellite",
	"shared",
}

// defaultComponent is the component used when the caller omits the component
// input, preserving backward compatibility with pre-enhancement invocations
// (Requirements 2.4, 13.1).
const defaultComponent = "saas-control-plane"

// Exact warning strings emitted by the resolvers. The text is part of the
// workflow's contract and is asserted verbatim by the workflow tests
// (Requirements 1.3, 2.4, 13.2).
const (
	// workspaceRootWarning is returned when workspace_root is omitted or empty,
	// signaling that the current working directory is used instead.
	workspaceRootWarning = "workspace_root not provided; using current working directory as workspace root"

	// componentDefaultWarning is returned when the component input is omitted,
	// signaling that the default component is being used.
	componentDefaultWarning = "component not specified; defaulting to saas-control-plane"
)

// NormalizePath strips every trailing path separator (both '\' and '/') from
// the right end of p until none remain. It is a pure function: it performs no
// I/O and does not touch the filesystem.
//
// Normalization is idempotent — NormalizePath(NormalizePath(p)) == NormalizePath(p)
// for all p — which is the contract verified by design Property 2
// (Requirement 1.6).
//
// Examples (showing actual string content):
//
//	NormalizePath("/foo/bar/")     -> "/foo/bar"
//	NormalizePath(`C:\project\`)   -> `C:\project`
//	NormalizePath("/foo///")       -> "/foo"
//	NormalizePath("bar")           -> "bar"
func NormalizePath(p string) string {
	return strings.TrimRight(p, "\\/")
}

// ResolveWorkspaceRoot resolves the workspace_root workflow input to an
// existing directory path, applying the rules of Requirement 1.
//
// Behavior:
//
//   - If workspaceRoot is empty (omitted), the current working directory is
//     used and a non-blocking warning is returned in warning; err is nil.
//     (Requirements 1.3, 13.1, 13.2)
//
//   - Otherwise workspaceRoot is normalized by stripping all trailing path
//     separators. (Requirement 1.6)
//
//   - If the normalized path does not resolve to an existing directory, an
//     error is returned and both resolvedRoot and warning are empty; the
//     pipeline is expected to HALT on this error before any step executes.
//     (Requirement 1.4)
//
//   - If the normalized path resolves to an existing directory, the normalized
//     path is returned with an empty warning and nil error.
//
// The only I/O performed is os.Getwd (for the omitted case) and os.Stat (for the
// directory existence check), matching the task's purity contract.
func ResolveWorkspaceRoot(workspaceRoot string) (resolvedRoot string, warning string, err error) {
	// Branch A — omitted or empty: default to the current working directory
	// and emit a non-blocking warning (Requirements 1.3, 13.2).
	if workspaceRoot == "" {
		cwd, getwdErr := os.Getwd()
		if getwdErr != nil {
			// os.Getwd failing is exceptional (e.g. the process cwd was
			// removed). Surface it as an error rather than silently
			// proceeding with an empty root.
			return "", "", fmt.Errorf("could not determine current working directory: %w", getwdErr)
		}
		return cwd, workspaceRootWarning, nil
	}

	// Branch B/D — provided: normalize trailing separators first (Requirement 1.6).
	normalized := NormalizePath(workspaceRoot)

	// Verify the normalized path resolves to an existing directory
	// (Requirement 1.4). A path that does not exist or is not a directory
	// both fail this check with the same message.
	info, statErr := os.Stat(normalized)
	if statErr != nil {
		return "", "", fmt.Errorf("workspace_root does not resolve to existing directory: %s", normalized)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("workspace_root does not resolve to existing directory: %s", normalized)
	}

	return normalized, "", nil
}

// ValidateComponent validates the component workflow input against the
// ValidComponents allowlist before any pipeline step executes (Requirement 2).
//
// Behavior:
//
//   - If component is empty (omitted), the default component
//     "saas-control-plane" is returned with a non-blocking warning and nil
//     error (Requirements 2.4, 13.1 backward compatibility).
//
//   - If component exactly matches (case-sensitive, no trimming) one of the
//     allowlisted values, it is returned unchanged with an empty warning and
//     nil error (Requirement 2.2).
//
//   - If component does not match any allowlisted value, an error is returned
//     containing the invalid value received and the complete list of valid
//     values (Requirement 2.3); the pipeline is expected to HALT and produce
//     no output artifacts.
//
// No normalization or trimming is applied to the input before matching; the
// comparison is byte-for-byte exact, so e.g. "saas-control-plane " (with a
// trailing space) does not match and yields an error.
func ValidateComponent(component string) (resolvedComponent string, warning string, err error) {
	// Branch A — omitted or empty: default and warn (Requirement 2.4).
	if component == "" {
		return defaultComponent, componentDefaultWarning, nil
	}

	// Branch B — exact, case-sensitive allowlist match (Requirement 2.2).
	for _, valid := range ValidComponents {
		if component == valid {
			return component, "", nil
		}
	}

	// Branch C — no match: HALT with the received value and the full allowlist
	// (Requirement 2.3). Built from ValidComponents so the message stays in
	// sync with the allowlist.
	return "", "", fmt.Errorf("Invalid component '%s'. Valid values: %s", component, strings.Join(ValidComponents, ", "))
}

// ---------------------------------------------------------------------------
// Audit Logging — Task 11.1
// Implements the append-only AuditLogger and run-entry schema.
// Requirements: 12.1, 12.2, 12.3, 12.4, 15.1, 15.2, 15.3
// ---------------------------------------------------------------------------

// AuditStepEntry records a single pipeline step's execution metadata.
// It is embedded in AuditRunEntry.Steps.
type AuditStepEntry struct {
	// StepID is the workflow step identifier (e.g., "validate-component").
	StepID string `json:"step_id"`
	// StepStart is the step start timestamp in ISO-8601 UTC to second precision.
	StepStart string `json:"step_start"`
	// StepEnd is the step end timestamp in ISO-8601 UTC to second precision.
	StepEnd string `json:"step_end"`
	// Outcome is one of: "success", "paused", "failed".
	Outcome string `json:"outcome"`
}

// AuditRunEntry is one run record appended to workflow-audit-log.json.
//
// The Hash field is a SHA-256 integrity value computed over the canonical JSON
// serialization of this struct with Hash set to "" (empty string). The result
// is stored as "sha256:<lowercasehex>" (Requirement 12.1, 15.1).
//
// Fields are declared in alphabetical json-tag order so that encoding/json's
// struct-field-declaration-order marshaling produces a deterministic canonical
// JSON serialization without a custom marshaler. Any future field addition
// MUST be inserted in alphabetical json-tag position to preserve this
// guarantee. The tags are intentionally sorted: artifacts_written,
// bug_report_id, component, hash, run_end, run_id, run_start, steps,
// workspace_root.
type AuditRunEntry struct {
	// ArtifactsWritten holds the resolved absolute paths of all artifact files
	// written during the run (Requirement 12.4).
	ArtifactsWritten []string `json:"artifacts_written"`
	// BugReportID is the identifier of the bug report being fixed.
	BugReportID string `json:"bug_report_id"`
	// Component is the validated component value for this run.
	Component string `json:"component"`
	// Hash is "sha256:<hex>" computed over this entry with Hash set to "".
	// Stored last so callers can compute it before finalizing the entry
	// (Requirements 12.1, 15.1, 15.2).
	Hash string `json:"hash"`
	// RunEnd is the run end timestamp (set on completion or halt).
	RunEnd string `json:"run_end"`
	// RunID is a unique identifier (e.g., UUID) for this pipeline run.
	RunID string `json:"run_id"`
	// RunStart is the run start timestamp in ISO-8601 UTC.
	RunStart string `json:"run_start"`
	// Steps is the ordered list of pipeline steps executed in this run.
	// Each step records its id, start/end timestamps, and outcome.
	// On a partial run (HALT/PAUSE), all steps up to and including the
	// failing/pausing step are included (Requirement 12.2).
	Steps []AuditStepEntry `json:"steps"`
	// WorkspaceRoot is the resolved absolute path used for this run.
	WorkspaceRoot string `json:"workspace_root"`
}

// ComputeAuditHash computes the SHA-256 integrity hash over the canonical JSON
// serialization of entry with entry.Hash set to "" (empty string).
//
// The canonical form uses encoding/json with struct field tags sorted
// alphabetically (the fields on AuditRunEntry are declared in tag-alphabetical
// order, so json.Marshal produces a deterministic byte sequence). No
// insignificant whitespace is included (json.Marshal compact output).
//
// Returns "sha256:<lowercasehex>" (Requirements 12.1, 15.1, 15.2).
func ComputeAuditHash(entry AuditRunEntry) (string, error) {
	// Ensure the hash field is empty so the computation is over content only.
	entry.Hash = ""

	canonical, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("audit hash: failed to marshal entry: %w", err)
	}

	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// AppendAuditEntry appends entry to the append-only workflow-audit-log.json at
// logPath. The file stores a top-level JSON array; if it does not exist, it is
// created with the entry as the sole element. Existing entries are never
// overwritten or deleted (Requirements 12.3, 15.3).
//
// The write is performed atomically: the updated array is marshaled to a
// temporary file in the same directory, then renamed over logPath. This
// prevents a partial write from corrupting the log.
//
// Returns an error if the file cannot be read or written. Callers must surface
// "ERROR: Audit logging failed: <reason>" and must NOT report the run as
// successfully completed (Requirement 12.7).
func AppendAuditEntry(logPath string, entry AuditRunEntry) error {
	// Step 1 — read the existing records (or start with an empty slice).
	var records []json.RawMessage

	existing, readErr := os.ReadFile(logPath)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			// A real I/O error — the file exists but cannot be read.
			return fmt.Errorf("audit log: read failed: %w", readErr)
		}
		// File does not exist yet; start with an empty slice.
		records = []json.RawMessage{}
	} else {
		// File exists — unmarshal the current top-level array.
		if err := json.Unmarshal(existing, &records); err != nil {
			return fmt.Errorf("audit log: existing content is not a valid JSON array: %w", err)
		}
	}

	// Step 2 — marshal the new entry.
	entryBytes, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("audit log: failed to marshal new entry: %w", err)
	}

	// Step 3 — append the new entry and marshal the updated array.
	records = append(records, json.RawMessage(entryBytes))

	updated, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("audit log: failed to marshal updated log array: %w", err)
	}

	// Step 4 — atomic write: write to a temp file then rename.
	// The temp file lives in the same directory so rename is atomic on
	// POSIX systems (same filesystem). On Windows, os.Rename replaces the
	// destination atomically at the filesystem level.
	tmpPath := logPath + ".tmp"
	if err := os.WriteFile(tmpPath, updated, 0o644); err != nil {
		return fmt.Errorf("audit log: write to temp file failed: %w", err)
	}
	if err := os.Rename(tmpPath, logPath); err != nil {
		// Best-effort cleanup of the temp file; ignore the error.
		_ = os.Remove(tmpPath)
		return fmt.Errorf("audit log: atomic rename failed: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Task 11.2 — canonical hash verification
// Implements VerifyAuditHash for tamper-detection on stored audit records.
// Requirements: 15.1, 15.2
// ---------------------------------------------------------------------------

// VerifyAuditHash verifies the stored SHA-256 integrity hash in entry against
// a freshly computed hash of the same entry's content.
//
// The stored hash is entry.Hash. The recomputed hash is obtained by calling
// ComputeAuditHash, which sets entry.Hash = "" on a local copy before
// serializing, so the computation is over content only — identical to what
// was computed when the record was originally written.
//
// Return values:
//
//   - (true,  nil)   — stored hash matches recomputed hash; record is intact.
//   - (false, nil)   — stored hash differs from recomputed hash; the record
//     has been tampered with. Callers SHOULD report the record as tampered
//     and identify it by RunStart and BugReportID (Requirement 15.2).
//   - (false, error) — the recomputation itself failed (e.g., marshaling
//     error); the integrity status is unknown. The caller SHOULD treat this
//     as a verification failure and investigate the underlying error.
//
// This function never modifies entry; it operates on value copies only.
//
// Requirements: 15.1, 15.2
func VerifyAuditHash(entry AuditRunEntry) (bool, error) {
	stored := entry.Hash

	recomputed, err := ComputeAuditHash(entry)
	if err != nil {
		return false, fmt.Errorf("audit hash verification: recomputation failed: %w", err)
	}

	return stored == recomputed, nil
}

// ---------------------------------------------------------------------------
// Pre-Flight Scanner — Tasks 2.2–2.5
// Implements read-only workspace inspection before the investigate step.
// Requirements: 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 14.4, 14.5
// ---------------------------------------------------------------------------

// scanTimeout is the per-git-command timeout used by all scanner operations.
// Requirement 14.5 — timeout enforcement; surface which operation timed out.
const scanTimeout = 25 * time.Second

// Conflict represents a single conflict detected during the pre-flight scan.
type Conflict struct {
	// Type is one of: "uncommitted-change", "unmerged-branch", "prior-run-artifact".
	Type string `json:"type"`
	// FilePath is the affected file or branch name (absolute or relative to
	// the workspace root, depending on context).
	FilePath string `json:"file_path"`
	// Severity is one of: "low", "medium", "high".
	Severity string `json:"severity"`
}

// preflightCleanPayload is the exact schema written to preflight-clean.json
// when no conflicts are detected (Requirement 3.3).
type preflightCleanPayload struct {
	Conflicts []Conflict `json:"conflicts"`
	Status    string     `json:"status"`
}

// preflightUnknownPayload is written to preflight-conflicts.md when a read
// error or git command failure prevents a reliable scan (Requirement 3.6).
type preflightUnknownPayload struct {
	Status string `json:"status"`
}

// acknowledgedPayload is the schema of preflight-acknowledged.json
// (Requirement 3.7).
type acknowledgedPayload struct {
	Acknowledged bool `json:"acknowledged"`
}

// runGitCommand executes a git command in dir with the package-level
// scanTimeout. It returns (stdout, nil) on success or ("", error) on timeout
// or non-zero exit. The error message names the operation and, on timeout,
// includes the elapsed time — satisfying Requirement 14.5.
func runGitCommand(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("git %s timed out after %s", strings.Join(args, " "), elapsed.Round(time.Millisecond))
	}
	if err != nil {
		return "", fmt.Errorf("git %s failed (%s): %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// ScanUncommittedChanges runs `git status --porcelain` in scanDir and returns
// a Conflict for each changed file. Files whose path starts with componentDir
// receive severity "medium"; all others receive "low".
//
// The porcelain format is "<XY> <path>" where XY is a two-character status
// code. The file path begins at index 3 of each line.
//
// Requirement 3.2
func ScanUncommittedChanges(scanDir, componentDir string) ([]Conflict, error) {
	out, err := runGitCommand(scanDir, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("ScanUncommittedChanges: %w", err)
	}

	var conflicts []Conflict
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			// Empty line or too short — skip.
			continue
		}
		// porcelain format: "XY path" — status is first 2 chars, path starts at index 3.
		filePath := line[3:]

		severity := "low"
		// Files under the component directory are elevated to "medium"
		// (Requirement 3.2; task 2.2: severity = "medium" if
		// strings.HasPrefix(filePath, componentDir), else "low").
		if strings.HasPrefix(filePath, componentDir) {
			severity = "medium"
		}

		conflicts = append(conflicts, Conflict{
			Type:     "uncommitted-change",
			FilePath: filePath,
			Severity: severity,
		})
	}
	return conflicts, nil
}

// ScanUnmergedBranches runs `git branch --no-merged HEAD` in repoDir and
// returns a Conflict per unmerged branch with severity "high". The current
// branch line (prefixed with "* ") is trimmed to its bare name before being
// recorded; git typically does not include the current branch in
// --no-merged output, but we handle the prefix defensively.
//
// Requirement 3.2
func ScanUnmergedBranches(repoDir string) ([]Conflict, error) {
	out, err := runGitCommand(repoDir, "branch", "--no-merged", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("ScanUnmergedBranches: %w", err)
	}

	var conflicts []Conflict
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		// Strip the "* " current-branch marker if present (defensive).
		name = strings.TrimPrefix(name, "* ")
		conflicts = append(conflicts, Conflict{
			Type:     "unmerged-branch",
			FilePath: name,
			Severity: "high",
		})
	}
	return conflicts, nil
}

// artifactGlobPatterns is the exhaustive set of patterns used by
// ScanPriorRunArtifacts to detect stale artifacts from a prior incomplete run.
// Requirement 3.2.
var artifactGlobPatterns = []string{
	"bug-*.md",
	"bug-*.json",
	"feature-*.md",
	"feature-*.json",
	"plan-output.md",
	"spec-*.md",
}

// ScanPriorRunArtifacts globs artifactDir for stale artifact files matching
// the six patterns defined in artifactGlobPatterns. Each match is returned as
// a Conflict with type "prior-run-artifact" and severity "medium".
//
// Requirement 3.2
func ScanPriorRunArtifacts(artifactDir string) ([]Conflict, error) {
	var conflicts []Conflict
	for _, pattern := range artifactGlobPatterns {
		matches, err := filepath.Glob(filepath.Join(artifactDir, pattern))
		if err != nil {
			// filepath.Glob returns an error only for a malformed pattern,
			// which cannot happen with our static patterns. Surface it anyway.
			return nil, fmt.Errorf("ScanPriorRunArtifacts: glob pattern %q: %w", pattern, err)
		}
		for _, match := range matches {
			conflicts = append(conflicts, Conflict{
				Type:     "prior-run-artifact",
				FilePath: match,
				Severity: "medium",
			})
		}
	}
	return conflicts, nil
}

// IsAcknowledged reads preflight-acknowledged.json in artifactDir and returns
// true if and only if the file can be parsed and its "acknowledged" field is
// true. Any I/O error, missing file, or malformed JSON causes it to return
// false so the pipeline defaults to the safe (halt-on-conflict) path.
//
// Requirement 3.7
func IsAcknowledged(artifactDir string) bool {
	path := filepath.Join(artifactDir, "preflight-acknowledged.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var payload acknowledgedPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}
	return payload.Acknowledged
}

// ShouldScopeToComponent returns true when the workspace has 10,001 or more
// git-tracked files, determined by running `git ls-files | wc -l` equivalent
// (using `git ls-files` piped through line counting) in workspaceRoot. When
// true, callers MUST scope their git commands and glob operations to the
// component directory subtree — this scoping is mandatory and independently
// verifiable regardless of elapsed scan time (Requirement 14.4).
//
// On any command failure the error is returned and the caller should treat the
// workspace as small (scope=false) to avoid incorrectly skipping files, while
// surfacing the error for diagnostics (Requirement 14.5).
//
// Requirement 14.4
func ShouldScopeToComponent(workspaceRoot string) (bool, error) {
	// Use `git ls-files --others --cached --exclude-standard` which lists all
	// tracked files. We count lines by running through a context-guarded exec.
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-files")
	cmd.Dir = workspaceRoot

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	if ctx.Err() == context.DeadlineExceeded {
		return false, fmt.Errorf("ShouldScopeToComponent: git ls-files timed out after %s", elapsed.Round(time.Millisecond))
	}
	if err != nil {
		return false, fmt.Errorf("ShouldScopeToComponent: git ls-files failed: %w", err)
	}

	// Count non-empty lines in the output — each line is one tracked file.
	count := 0
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}

	return count >= 10001, nil
}

// WritePreflightResult writes the scan result artifact to artifactDir.
//
// When conflicts is empty and status is "clean", it writes
// preflight-clean.json with the exact schema {"conflicts":[],"status":"clean"}
// (Requirement 3.3).
//
// When conflicts is non-empty or status is not "clean" (e.g., "unknown" or
// "conflict"), it writes preflight-conflicts.md as a Markdown table with
// columns: # | Type | File Path | Severity. A metadata header is prepended
// with the scan timestamp, workspace root, and component (Requirement 3.4).
//
// Requirements 3.3, 3.4
func WritePreflightResult(artifactDir string, conflicts []Conflict, status, scannedAt, workspaceRoot, component string) error {
	if len(conflicts) == 0 && status == "clean" {
		return writePreflightClean(artifactDir)
	}
	return writePreflightConflicts(artifactDir, conflicts, status, scannedAt, workspaceRoot, component)
}

// writePreflightClean writes preflight-clean.json (Requirements 3.3).
func writePreflightClean(artifactDir string) error {
	payload := preflightCleanPayload{
		Conflicts: []Conflict{},
		Status:    "clean",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("writePreflightClean: marshal failed: %w", err)
	}
	path := filepath.Join(artifactDir, "preflight-clean.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writePreflightClean: write failed: %w", err)
	}
	return nil
}

// writePreflightConflicts writes preflight-conflicts.md as a Markdown table
// (Requirement 3.4). When status is "unknown" (git read error), an additional
// JSON snippet is embedded after the table for machine readability.
func writePreflightConflicts(artifactDir string, conflicts []Conflict, status, scannedAt, workspaceRoot, component string) error {
	var sb strings.Builder

	sb.WriteString("# Pre-Flight Scan: Conflicts Detected\n\n")
	sb.WriteString("| Key | Value |\n")
	sb.WriteString("|-----|-------|\n")
	sb.WriteString("| Scanned At | " + scannedAt + " |\n")
	sb.WriteString("| Workspace Root | " + workspaceRoot + " |\n")
	sb.WriteString("| Component | " + component + " |\n")
	sb.WriteString("| Status | " + status + " |\n\n")

	if status == "unknown" {
		// Embed a machine-readable unknown-status marker so that automated
		// tooling can detect the "could not scan" case (Requirement 3.6).
		unknownJSON, _ := json.Marshal(preflightUnknownPayload{Status: "unknown"})
		sb.WriteString("```json\n")
		sb.WriteString(string(unknownJSON))
		sb.WriteString("\n```\n\n")
		sb.WriteString("> **Warning:** The pre-flight scan could not complete due to a read error. ")
		sb.WriteString("The workspace state is unknown. Resolve the error and re-run.\n\n")
	}

	if len(conflicts) > 0 {
		sb.WriteString("## Detected Conflicts\n\n")
		sb.WriteString("| # | Type | File Path | Severity |\n")
		sb.WriteString("|---|------|-----------|----------|\n")
		for i, c := range conflicts {
			sb.WriteString("| " + strconv.Itoa(i+1) + " | " + c.Type + " | " + c.FilePath + " | " + c.Severity + " |\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Resolution\n\n")
	sb.WriteString("To acknowledge these conflicts and proceed, create `preflight-acknowledged.json` in the artifact directory:\n\n")
	sb.WriteString("```json\n{\"acknowledged\": true}\n```\n\n")
	sb.WriteString("Then re-run the workflow.\n")

	path := filepath.Join(artifactDir, "preflight-conflicts.md")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("writePreflightConflicts: write failed: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Diagnosis Review — Tasks 5.2–5.3
// Implements DiagnosisReviewer checks and verdict writing.
// Requirements: 6.2, 6.3, 6.4, 6.6
// ---------------------------------------------------------------------------

// Finding is a single reviewer finding in a diagnosis review.
// Severity is "blocking" or "advisory". Blocking findings prevent the fix
// step from proceeding; advisory findings are informational warnings.
// (Requirements 6.3, 6.4)
type Finding struct {
	Severity    string `json:"severity"`    // "blocking" | "advisory"
	Description string `json:"description"` // human-readable explanation
	Section     string `json:"section"`     // markdown section heading in bug-diagnosis.md
	Line        int    `json:"line"`        // line number within that section, or 0 if unknown
}

// DiagnosisReviewVerdict is written to bug-diagnosis-reviewed.json by the
// DiagnosisReviewer after each review iteration.
//
// Approved variant:  {reviewed:true, verdict:"approved", findings:[], iteration:N, reviewed_at:...}
// Rejected variant:  {reviewed:true, verdict:"rejected", findings:[...],  iteration:N, reviewed_at:...}
//
// (Requirements 6.3, 6.4)
type DiagnosisReviewVerdict struct {
	Reviewed   bool      `json:"reviewed"`
	Verdict    string    `json:"verdict"`      // "approved" | "rejected" | "pending"
	Findings   []Finding `json:"findings"`
	Iteration  int       `json:"iteration"`
	ReviewedAt string    `json:"reviewed_at"` // ISO-8601 UTC
}

// WriteReviewVerdict marshals verdict to JSON and writes it atomically to path
// (temp-file + rename). Callers supply the fully-qualified destination path,
// e.g. `{{resolved_root}}/.agents/tasks/bug-diagnosis-reviewed.json`.
//
// The write is atomic on both POSIX and Windows: the JSON is written to a
// sibling `.tmp` file first, then os.Rename replaces the destination.
//
// (Requirements 6.3, 6.4)
func WriteReviewVerdict(path string, verdict DiagnosisReviewVerdict) error {
	data, err := json.Marshal(verdict)
	if err != nil {
		return fmt.Errorf("WriteReviewVerdict: marshal failed: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("WriteReviewVerdict: write to temp file failed: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("WriteReviewVerdict: atomic rename failed: %w", err)
	}
	return nil
}

// CheckDiagnosisConsistency performs a structural consistency check on the
// diagnosis document against the investigation document. It is a pure
// content-level check — no files are read by this function; callers pass the
// file contents directly so the function remains side-effect-free and easily
// testable.
//
// A blocking Finding is returned when either of the following conditions holds:
//
//  1. The `## Confirmed Root Cause` section is absent or contains no non-blank
//     content after the heading — the root cause must be stated.
//     (Requirement 6.6: root cause must explain the reproduction test failure)
//
//  2. The `## Test Cases to Add` section is absent from the diagnosis — the
//     diagnosis must enumerate the regression tests to add.
//     (Requirement 6.2.c: check regression test list completeness)
//
// Returns nil when both sections are present with content, indicating no
// structural blocking findings were detected.
//
// (Requirements 6.2, 6.6)
func CheckDiagnosisConsistency(diagnosisContent, investigationContent string) []Finding {
	var findings []Finding

	// Check 1 — `## Confirmed Root Cause` must be present and non-empty.
	rootCauseContent := extractSectionContent(diagnosisContent, "## Confirmed Root Cause")
	if rootCauseContent == "" {
		findings = append(findings, Finding{
			Severity:    "blocking",
			Description: "Section '## Confirmed Root Cause' is absent or empty; the root cause must be stated so that it can be verified against the reproduction test failure.",
			Section:     "Confirmed Root Cause",
			Line:        0,
		})
	}

	// Check 2 — `## Test Cases to Add` must be present (may be a list or
	// prose, but the section heading must exist).
	testCasesContent := extractSectionContent(diagnosisContent, "## Test Cases to Add")
	if testCasesContent == "" {
		findings = append(findings, Finding{
			Severity:    "blocking",
			Description: "Section '## Test Cases to Add' is absent or empty; the diagnosis must enumerate the regression tests to add.",
			Section:     "Test Cases to Add",
			Line:        0,
		})
	}

	// investigationContent is accepted as a parameter for future structural
	// cross-checks (e.g., verify the root cause mentions the same code path as
	// the reproduction test) but no additional automated check is performed at
	// this structural level — deeper semantic consistency requires agent
	// reasoning and is handled in the workflow step prompt.
	_ = investigationContent

	if len(findings) == 0 {
		return nil
	}
	return findings
}

// extractSectionContent returns the text that follows the given markdown
// section heading (e.g., "## Confirmed Root Cause") up to (but not including)
// the next `##`-level heading or the end of the document. The heading itself
// is not included. The returned string is trimmed of leading/trailing
// whitespace; it is empty when the section is absent or contains no non-blank
// content.
func extractSectionContent(document, heading string) string {
	lines := strings.Split(document, "\n")
	var inSection bool
	var buf strings.Builder

	for _, line := range lines {
		if strings.TrimSpace(line) == strings.TrimSpace(heading) {
			inSection = true
			continue
		}
		if inSection {
			// Stop at the next `##`-level (or higher) heading.
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") {
				break
			}
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}

	return strings.TrimSpace(buf.String())
}

// ---------------------------------------------------------------------------
// Investigation Escalation — Tasks 6.1–6.3
// Implements EscalationWriter for all-hypotheses-ruled-out scenarios.
// Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6
// ---------------------------------------------------------------------------

// HypothesisRecord is one root-cause hypothesis extracted from the
// investigation report.
type HypothesisRecord struct {
	Hypothesis     string `json:"hypothesis"`
	Status         string `json:"status"`           // "confirmed" | "ruled out: <reason>"
	RuledOutReason string `json:"ruled_out_reason"` // populated when Status starts with "ruled out"
}

// EscalationPayload holds the structured content written to
// investigation-escalation-*.md. All fields are required; empty slices are
// valid for CLMStagesExamined and Hypotheses.
//
// ReproductionStatus must be one of: "reproduced", "unreproducible",
// "not_attempted".
//
// RecommendedAction must be one of: "manual-investigation",
// "gather-more-logs", "escalate-to-domain-expert", "abandon".
//
// (Requirement 7.2)
type EscalationPayload struct {
	BugSummary         string
	Hypotheses         []HypothesisRecord
	ReproductionStatus string   // "reproduced" | "unreproducible" | "not_attempted"
	CLMStagesExamined  []string
	RecommendedAction  string   // "manual-investigation" | "gather-more-logs" | "escalate-to-domain-expert" | "abandon"
}

// hypothesisStatusPrefix is the prefix that marks a hypothesis as ruled out in
// the investigation document. The check is case-insensitive and prefix-based
// so that "ruled out: ..." and "Ruled Out: ..." both match.
const hypothesisStatusRuledOut = "ruled out"

// hypothesisStatusConfirmed is the exact marker that flags a confirmed
// hypothesis. The check is case-insensitive.
const hypothesisStatusConfirmed = "confirmed"

// ShouldEscalate inspects the raw text content of bug-investigation.md and
// returns true when every Root_Cause_Hypothesis it contains is marked
// "ruled out" (and therefore no hypothesis is "confirmed").
//
// Detection heuristic: lines that contain the word "Hypothesis" (case-
// insensitive) followed on the same or the next non-blank line by a status
// token are examined. If at least one such line contains the token "confirmed"
// (case-insensitive), the function returns false (proceed to diagnose).
// If zero "confirmed" status tokens are found among hypothesis lines, the
// function returns true (write escalation artifact).
//
// The heuristic is intentionally simple and document-format-agnostic so it
// can be exercised by property-based tests without requiring a full markdown
// parser.
//
// (Requirements 7.1, 7.6)
func ShouldEscalate(investigationContent string) bool {
	lines := strings.Split(investigationContent, "\n")
	foundHypothesis := false

	for i, line := range lines {
		lower := strings.ToLower(line)

		// Detect hypothesis lines: lines that contain "hypothesis" or
		// lines inside a hypotheses list/table row.
		if !strings.Contains(lower, "hypothesis") {
			continue
		}
		foundHypothesis = true

		// Check the current line and the next three lines for a status token.
		window := lines[i:]
		if len(window) > 4 {
			window = window[:4]
		}
		for _, wline := range window {
			if strings.Contains(strings.ToLower(wline), hypothesisStatusConfirmed) {
				// At least one hypothesis is confirmed — no escalation needed.
				return false
			}
		}
	}

	// If we found hypothesis entries but none were confirmed, escalate.
	// If no hypothesis entries were found at all, do not escalate (there is
	// nothing to rule out).
	return foundHypothesis
}

// WriteEscalationArtifact renders payload as a Markdown document and writes it
// to artifactDir with the filename format:
//
//	investigation-escalation-{timestamp}.md
//
// where timestamp is the provided timestamp string with colons replaced by
// hyphens for cross-platform filename compatibility (Requirement 7.4).
//
// If a file at the computed path already exists (from a prior run), the new
// content is appended below a `---` horizontal rule separator rather than
// overwriting the existing file, preserving prior run content (Requirement 7.5).
//
// timestamp should be an ISO-8601 second-precision UTC string such as
// "2024-01-15T10:45:00Z"; the function normalises it to a safe filename by
// replacing all colons with hyphens, producing e.g.
// "investigation-escalation-2024-01-15T10-45-00Z.md".
//
// (Requirements 7.1, 7.2, 7.3, 7.4, 7.5)
func WriteEscalationArtifact(artifactDir string, payload EscalationPayload, timestamp string) error {
	// Build a cross-platform filename by replacing colons with hyphens.
	safeTimestamp := strings.ReplaceAll(timestamp, ":", "-")
	filename := "investigation-escalation-" + safeTimestamp + ".md"
	path := filepath.Join(artifactDir, filename)

	// Render the markdown content for the new escalation event.
	content := renderEscalationMarkdown(payload, timestamp)

	// If the file already exists from a prior run, append below a --- separator
	// rather than overwriting (Requirement 7.5).
	existing, readErr := os.ReadFile(path)
	if readErr == nil {
		// File exists — append below a horizontal rule.
		combined := string(existing) + "\n---\n\n" + content
		if err := os.WriteFile(path, []byte(combined), 0o644); err != nil {
			return fmt.Errorf("WriteEscalationArtifact: append to existing file failed: %w", err)
		}
		return nil
	}

	// File does not exist (or could not be read — treat as new).
	if !os.IsNotExist(readErr) {
		// A real read error on an existing file — surface it.
		return fmt.Errorf("WriteEscalationArtifact: read existing file failed: %w", readErr)
	}

	// Write a new file.
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("WriteEscalationArtifact: write failed: %w", err)
	}
	return nil
}

// renderEscalationMarkdown renders the EscalationPayload as a Markdown
// document suitable for human review. All required fields from Requirement 7.2
// are included.
func renderEscalationMarkdown(payload EscalationPayload, writtenAt string) string {
	var sb strings.Builder

	sb.WriteString("# Investigation Escalation\n\n")
	sb.WriteString("| Key | Value |\n")
	sb.WriteString("|-----|-------|\n")
	sb.WriteString("| Written At | " + writtenAt + " |\n")
	sb.WriteString("| Reproduction Status | " + payload.ReproductionStatus + " |\n")
	sb.WriteString("| Recommended Action | " + payload.RecommendedAction + " |\n\n")

	sb.WriteString("## Bug Report Summary\n\n")
	sb.WriteString(payload.BugSummary + "\n\n")

	sb.WriteString("## Hypotheses Investigated\n\n")
	if len(payload.Hypotheses) == 0 {
		sb.WriteString("_No hypotheses were recorded._\n\n")
	} else {
		sb.WriteString("| # | Hypothesis | Status | Ruled-Out Reason |\n")
		sb.WriteString("|---|------------|--------|------------------|\n")
		for i, h := range payload.Hypotheses {
			sb.WriteString("| " + strconv.Itoa(i+1) + " | " + h.Hypothesis + " | " + h.Status + " | " + h.RuledOutReason + " |\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## CLM Lifecycle Stages Examined\n\n")
	if len(payload.CLMStagesExamined) == 0 {
		sb.WriteString("_No CLM lifecycle stages were identified as examined._\n\n")
	} else {
		for _, stage := range payload.CLMStagesExamined {
			sb.WriteString("- " + stage + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Recommended Next Action\n\n")
	sb.WriteString("`" + payload.RecommendedAction + "`\n\n")
	sb.WriteString("This escalation was triggered because all investigated root-cause hypotheses were ruled out. ")
	sb.WriteString("No diagnosis step will be executed until a human reviews and provides a new hypothesis or additional context.\n")

	return sb.String()
}

// ---------------------------------------------------------------------------
// Task 11.3 — Partial-run capture and filesystem error handling
// Requirements: 12.2, 12.7
// ---------------------------------------------------------------------------

// FinalizePartialAuditEntry updates an in-progress run entry before a HALT or
// PAUSE event. It performs three actions atomically from the caller's
// perspective:
//
//  1. Sets entry.RunEnd to the current UTC time in RFC3339 format.
//  2. Computes the SHA-256 integrity hash over the (now-final) entry content
//     and sets entry.Hash (Requirements 12.1, 15.1).
//  3. Appends the finalized entry to the append-only audit log at logPath
//     (Requirement 12.3).
//
// If any step fails, the error is wrapped with the prefix
// "Audit logging failed: <reason>" so callers can surface it verbatim as
// "ERROR: Audit logging failed: <reason>" per Requirement 12.7. Callers MUST
// NOT report the run as successfully completed when this function returns a
// non-nil error.
//
// The entry's Steps slice should already contain all pipeline steps executed
// up to and including the failing or pausing step before this function is
// called (Requirement 12.2).
//
// (Requirements 12.2, 12.7)
func FinalizePartialAuditEntry(logPath string, entry AuditRunEntry) error {
	// Step 1 — stamp the run end time.
	entry.RunEnd = time.Now().UTC().Format(time.RFC3339)

	// Step 2 — compute the integrity hash over the finalized content.
	hash, err := ComputeAuditHash(entry)
	if err != nil {
		return fmt.Errorf("Audit logging failed: compute hash: %w", err)
	}
	entry.Hash = hash

	// Step 3 — append the finalized entry to the log.
	if err := AppendAuditEntry(logPath, entry); err != nil {
		return fmt.Errorf("Audit logging failed: %s", err.Error())
	}

	return nil
}

// ---------------------------------------------------------------------------
// Task 7.4 — Rollback Section Validator
// Implements ValidateRollbackSection for bug-fix-summary.md.
// Requirements: 9.1, 9.2, 9.3, 9.4, 9.5
// ---------------------------------------------------------------------------

// ValidateRollbackSection checks that the content of bug-fix-summary.md
// contains a valid ## Rollback section. It returns a non-nil error describing
// every violation found, or nil if all checks pass.
//
// Rules enforced:
//
//  1. A `## Rollback` heading must be present as a top-level section
//     (Requirement 9.1).
//
//  2. The section body must contain at least one numbered list item of the
//     form `N. <action>` (where N is a positive integer) (Requirement 9.5).
//
//  3. The count of numbered list items must be between 1 and 20 inclusive
//     (Requirement 9.5 — procedure of 1–20 steps).
//
// The special "No rollback actions required." single-step is accepted as a
// valid numbered step, satisfying Requirement 9.8.
//
// (Requirements 9.1, 9.2, 9.3, 9.4, 9.5)
func ValidateRollbackSection(summaryContent string) error {
	// Step 1 — locate the `## Rollback` heading.
	rollbackContent := extractSectionContent(summaryContent, "## Rollback")
	if rollbackContent == "" {
		return fmt.Errorf("ValidateRollbackSection: '## Rollback' section is absent or empty (Requirement 9.1)")
	}

	// Step 2 — count numbered list items: lines matching /^\d+\. .+/
	lines := strings.Split(rollbackContent, "\n")
	stepCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Look for lines starting with one or more digits followed by ". "
		if isNumberedListItem(trimmed) {
			stepCount++
		}
	}

	// Step 3 — must have at least one numbered step (Req 9.5).
	if stepCount == 0 {
		return fmt.Errorf("ValidateRollbackSection: '## Rollback' section contains no numbered procedure steps; " +
			"at least one step is required (Requirement 9.5). " +
			"Use '1. No rollback actions required.' when no changes were made (Requirement 9.8)")
	}

	// Step 4 — must have at most 20 numbered steps (Req 9.5).
	if stepCount > 20 {
		return fmt.Errorf("ValidateRollbackSection: '## Rollback' section contains %d numbered steps; "+
			"the maximum allowed is 20 (Requirement 9.5)", stepCount)
	}

	return nil
}

// isNumberedListItem returns true if the trimmed line starts with one or more
// ASCII digits followed by ". " (i.e., is a Markdown ordered list item).
func isNumberedListItem(line string) bool {
	// Find the first ". " in the line and verify everything before it is digits.
	dotSpace := strings.Index(line, ". ")
	if dotSpace <= 0 {
		return false
	}
	prefix := line[:dotSpace]
	for _, ch := range prefix {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	// There must be at least one character of content after ". ".
	rest := strings.TrimSpace(line[dotSpace+2:])
	return rest != ""
}

// ---------------------------------------------------------------------------
// Validation Gate — Tasks 9.2–9.3
// Implements the validation gate logic for the extended post-fix checks.
// Requirements: 10.1, 10.2, 10.3, 10.5, 10.6, 10.7
// ---------------------------------------------------------------------------

// GateCheck holds the result of a single validation gate check.
// (Requirement 10.5, 10.6)
type GateCheck struct {
	Name   string `json:"name"`
	Result string `json:"result"` // "pass" | "fail"
	Detail string `json:"detail,omitempty"`
}

// BugfixValidationResult is written to either bugfix-validation-passed.json
// (allPassed == true) or bugfix-validation-failed.json (allPassed == false)
// at the end of the validation gate. The two files are mutually exclusive.
// (Requirements 10.5, 10.6)
type BugfixValidationResult struct {
	BugReportID  string      `json:"bug_report_id"`
	RunTimestamp string      `json:"run_timestamp"` // ISO-8601
	Checks       []GateCheck `json:"checks"`
}

// CheckReproductionTestPresence checks that bug-investigation.md contains
// EITHER a ## Reproduction Test section with a non-empty file_path entry,
// OR an `unreproducible: true` flag anywhere in the document.
//
// Returns a GateCheck with Result "pass" if either condition is met, or
// "fail" with a description otherwise.
//
// (Requirements 10.1, 10.7)
func CheckReproductionTestPresence(investigationContent string) GateCheck {
	// Branch A — unreproducible flag anywhere in the document (Req 10.7).
	if strings.Contains(investigationContent, "unreproducible: true") {
		return GateCheck{
			Name:   "reproduction-test-presence",
			Result: "pass",
			Detail: "bug flagged as unreproducible; at least two reproduction strategies expected per Req 10.7",
		}
	}

	// Branch B — ## Reproduction Test section with a non-empty file_path line.
	sectionContent := extractSectionContent(investigationContent, "## Reproduction Test")
	if sectionContent == "" {
		return GateCheck{
			Name:   "reproduction-test-presence",
			Result: "fail",
			Detail: "bug-investigation.md does not contain a '## Reproduction Test' section or an 'unreproducible: true' flag (Requirement 10.1)",
		}
	}

	// Look for a "file_path:" key with a non-empty value within the section.
	for _, line := range strings.Split(sectionContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "file_path:") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "file_path:"))
			if value != "" && value != `""` && value != "''" {
				return GateCheck{
					Name:   "reproduction-test-presence",
					Result: "pass",
				}
			}
		}
	}

	return GateCheck{
		Name:   "reproduction-test-presence",
		Result: "fail",
		Detail: "'## Reproduction Test' section found but contains no non-empty 'file_path:' entry (Requirement 10.1)",
	}
}

// CheckRegressionTestsPresent verifies that bug-diagnosis.md contains a
// ## Test Cases to Add section with at least one non-blank list item.
//
// This is a structural check: it verifies that the diagnosis author recorded
// regression test cases. File existence checks (verifying the test was actually
// written to disk) are integration-level concerns handled by the full validation
// gate runner.
//
// Returns a GateCheck with Result "pass" if the section exists and is non-empty,
// or "fail" with a description otherwise.
//
// (Requirement 10.2)
func CheckRegressionTestsPresent(diagnosisContent string) GateCheck {
	sectionContent := extractSectionContent(diagnosisContent, "## Test Cases to Add")
	if sectionContent == "" {
		return GateCheck{
			Name:   "regression-tests-present",
			Result: "fail",
			Detail: "bug-diagnosis.md does not contain a '## Test Cases to Add' section or the section is empty (Requirement 10.2)",
		}
	}

	// Verify at least one non-blank item exists in the section.
	for _, line := range strings.Split(sectionContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return GateCheck{
				Name:   "regression-tests-present",
				Result: "pass",
			}
		}
	}

	return GateCheck{
		Name:   "regression-tests-present",
		Result: "fail",
		Detail: "'## Test Cases to Add' section found but contains no non-blank items (Requirement 10.2)",
	}
}

// WriteValidationResult writes the validation result to either
// bugfix-validation-passed.json (when allPassed is true) or
// bugfix-validation-failed.json (when allPassed is false).
//
// The two files are mutually exclusive: a passed run never writes the failed
// file, and a failed run never writes the passed file (Requirements 10.5, 10.6).
//
// The write is atomic (temp-file + rename) to prevent partial writes from
// corrupting the artifact.
//
// (Requirements 10.5, 10.6)
func WriteValidationResult(artifactDir string, result BugfixValidationResult, allPassed bool) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("WriteValidationResult: marshal failed: %w", err)
	}

	var filename string
	if allPassed {
		filename = "bugfix-validation-passed.json"
	} else {
		filename = "bugfix-validation-failed.json"
	}

	destPath := filepath.Join(artifactDir, filename)
	tmpPath := destPath + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("WriteValidationResult: write to temp file failed: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("WriteValidationResult: atomic rename failed: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Compliance Gate — Tasks 10.1–10.2
// Implements compliance and crypto-impact checks for the validation gate.
// Requirements: 11.1, 11.2, 11.3, 11.4
// ---------------------------------------------------------------------------

// requiredComplianceSections is the exhaustive set of section headings that
// every Compliance_Doc must contain (Requirements 11.1, 11.2).
var requiredComplianceSections = []string{
	"## GDPR Impact",
	"## PCI-DSS Impact",
	"## SOC 2 Impact",
	"## HIPAA Impact",
}

// CheckComplianceDoc verifies that a compliance documentation file exists in
// complianceDir whose filename contains bugID as a case-insensitive substring,
// and that the file contains all four required sections.
//
// If no such file exists, the check fails with a description identifying the
// missing file.
//
// If the file exists but is missing one or more of the four required sections
// (## GDPR Impact, ## PCI-DSS Impact, ## SOC 2 Impact, ## HIPAA Impact), the
// check fails and lists the absent sections.
//
// Returns a GateCheck with Result "pass" when both conditions are satisfied.
//
// (Requirements 11.1, 11.2)
func CheckComplianceDoc(complianceDir, bugID string) GateCheck {
	entries, err := os.ReadDir(complianceDir)
	if err != nil {
		return GateCheck{
			Name:   "compliance-doc-present",
			Result: "fail",
			Detail: fmt.Sprintf("could not read compliance directory '%s': %s (Requirement 11.1)", complianceDir, err.Error()),
		}
	}

	// Find a file whose name contains bugID (case-insensitive).
	lowerBugID := strings.ToLower(bugID)
	var matchedPath string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.Contains(strings.ToLower(entry.Name()), lowerBugID) {
			matchedPath = filepath.Join(complianceDir, entry.Name())
			break
		}
	}

	if matchedPath == "" {
		return GateCheck{
			Name:   "compliance-doc-present",
			Result: "fail",
			Detail: fmt.Sprintf("no compliance document found in '%s' whose filename contains '%s' (case-insensitive) (Requirement 11.1)", complianceDir, bugID),
		}
	}

	// File found — verify it contains all four required sections.
	content, err := os.ReadFile(matchedPath)
	if err != nil {
		return GateCheck{
			Name:   "compliance-doc-present",
			Result: "fail",
			Detail: fmt.Sprintf("compliance document found at '%s' but could not be read: %s (Requirement 11.1)", matchedPath, err.Error()),
		}
	}

	docText := string(content)
	var missingSections []string
	for _, section := range requiredComplianceSections {
		if !strings.Contains(docText, section) {
			missingSections = append(missingSections, section)
		}
	}

	if len(missingSections) > 0 {
		return GateCheck{
			Name:   "compliance-doc-present",
			Result: "fail",
			Detail: fmt.Sprintf("compliance document '%s' is missing required sections: %s (Requirement 11.2)",
				matchedPath, strings.Join(missingSections, ", ")),
		}
	}

	return GateCheck{
		Name:   "compliance-doc-present",
		Result: "pass",
	}
}

// CheckCryptoImpactAssessment verifies that diagnosisContent contains a
// ## Cryptographic Integrity Impact Assessment section with non-empty content.
//
// Returns a GateCheck with Result "pass" if the section is present and
// non-empty, or "fail" with a description otherwise.
//
// (Requirement 11.3)
func CheckCryptoImpactAssessment(diagnosisContent string) GateCheck {
	sectionContent := extractSectionContent(diagnosisContent, "## Cryptographic Integrity Impact Assessment")
	if sectionContent == "" {
		return GateCheck{
			Name:   "crypto-impact-assessment",
			Result: "fail",
			Detail: "bug-diagnosis.md does not contain a '## Cryptographic Integrity Impact Assessment' section or the section is empty; " +
				"a [CRYPTO-IMPACT] marker was detected in bug-investigation.md (Requirement 11.3)",
		}
	}

	return GateCheck{
		Name:   "crypto-impact-assessment",
		Result: "pass",
	}
}

// HasComplianceMarker returns true if investigationContent contains either
// [COMPLIANCE-REQUIRED] or [CRYPTO-IMPACT]. Used to decide whether compliance
// gate checks need to run.
//
// When this returns false, the validator SHALL skip all compliance gating checks
// and SHALL NOT fail the gate for missing compliance documentation (Requirement 11.4).
//
// (Requirement 11.4)
func HasComplianceMarker(investigationContent string) bool {
	return strings.Contains(investigationContent, "[COMPLIANCE-REQUIRED]") ||
		strings.Contains(investigationContent, "[CRYPTO-IMPACT]")
}

// HasCryptoMarker returns true if investigationContent contains [CRYPTO-IMPACT].
// Used to decide whether the cryptographic integrity impact assessment check
// should run against bug-diagnosis.md.
//
// (Requirement 11.3)
func HasCryptoMarker(investigationContent string) bool {
	return strings.Contains(investigationContent, "[CRYPTO-IMPACT]")
}

// ---------------------------------------------------------------------------
// Task 11.4 — Audit Log Rotation and Workflow Version Recording
// Requirements: 15.5, 15.6
// ---------------------------------------------------------------------------

// maxAuditLogRecords is the threshold at which the audit log file is rotated.
// When appending a new run entry would bring the total record count to this
// value or beyond, the current file is kept unchanged and a new rotated file
// is created to hold the rotation-event entry and the caller's entry.
// (Requirement 15.5)
const maxAuditLogRecords = 10000

// nextRotatedLogPath returns the path for the next rotated audit log file,
// computed by scanning for existing rotated files and choosing the lowest
// unused three-digit numeric suffix.
//
// The rotated filename replaces the ".json" suffix of logPath with "-NNN.json"
// (e.g. "workflow-audit-log.json" → "workflow-audit-log-001.json",
// "workflow-audit-log-002.json", …). The suffix counter starts at 1 and
// increments until an unused path is found.
//
// (Requirement 15.5)
func nextRotatedLogPath(logPath string) string {
	base := strings.TrimSuffix(logPath, ".json")
	if base == logPath {
		// logPath has no ".json" suffix — append the counter directly.
		base = logPath
	}
	for i := 1; i <= 999; i++ {
		candidate := fmt.Sprintf("%s-%03d.json", base, i)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	// Fallback: use a timestamp suffix to avoid an infinite loop in the
	// pathological case where all 999 numeric suffixes are taken.
	return fmt.Sprintf("%s-%s.json", base, time.Now().UTC().Format("20060102T150405Z"))
}

// rotationMarkerEntry builds the synthetic run entry written as the first
// record in a newly rotated log file (Requirement 15.5). The rotation event
// uses RunID "rotation" and records the original log path in BugReportID.
func rotationMarkerEntry(originalLogPath string) AuditRunEntry {
	now := time.Now().UTC().Format(time.RFC3339)
	return AuditRunEntry{
		ArtifactsWritten: []string{},
		BugReportID:      "log-rotated-from: " + originalLogPath,
		Component:        "",
		Hash:             "",
		RunEnd:           now,
		RunID:            "rotation",
		RunStart:         now,
		Steps: []AuditStepEntry{
			{
				StepID:    "rotation",
				StepStart: now,
				StepEnd:   now,
				Outcome:   "success",
			},
		},
		WorkspaceRoot: "",
	}
}

// AppendAuditEntryWithRotation extends AppendAuditEntry with automatic file
// rotation when the log reaches maxAuditLogRecords records.
//
// If appending the entry to logPath would result in more than
// maxAuditLogRecords records, the function:
//
//  1. Keeps the existing logPath file unchanged (preserves prior records).
//  2. Determines the next available rotated path: the ".json" suffix of
//     logPath is replaced with "-NNN.json" (three-digit zero-padded counter,
//     starting from 001) choosing the lowest suffix whose file does not yet
//     exist.
//  3. Writes a synthetic rotation-event entry as the first record in the new
//     file, with RunID "rotation", StepID "rotation", Outcome "success", and
//     BugReportID "log-rotated-from: <original-logPath>".
//  4. Appends the caller's entry after the rotation-event entry in the new
//     file.
//
// When the existing record count is below the threshold, AppendAuditEntry is
// called directly with no rotation.
//
// The rotation-event entry and the caller's entry are both written atomically
// as a two-element array via the same temp-file + rename mechanism used by
// AppendAuditEntry.
//
// (Requirement 15.5)
func AppendAuditEntryWithRotation(logPath string, entry AuditRunEntry) error {
	// Step 1 — read the existing record count (or treat a missing file as 0).
	var existing []json.RawMessage

	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return fmt.Errorf("AppendAuditEntryWithRotation: read failed: %w", readErr)
		}
		// File does not exist — count is 0, no rotation needed.
		existing = []json.RawMessage{}
	} else {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("AppendAuditEntryWithRotation: existing log is not a valid JSON array: %w", err)
		}
	}

	// Step 2 — rotation check: if current count has already reached the
	// threshold, write to a new rotated file instead.
	if len(existing) >= maxAuditLogRecords {
		rotatedPath := nextRotatedLogPath(logPath)

		// Build the rotation marker entry.
		marker := rotationMarkerEntry(logPath)

		// Marshal both the marker and the caller's entry.
		markerBytes, err := json.Marshal(marker)
		if err != nil {
			return fmt.Errorf("AppendAuditEntryWithRotation: marshal rotation marker: %w", err)
		}
		entryBytes, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("AppendAuditEntryWithRotation: marshal entry: %w", err)
		}

		// Produce a two-element (or more) array for the rotated file.
		newRecords := []json.RawMessage{
			json.RawMessage(markerBytes),
			json.RawMessage(entryBytes),
		}
		newData, err := json.Marshal(newRecords)
		if err != nil {
			return fmt.Errorf("AppendAuditEntryWithRotation: marshal rotated array: %w", err)
		}

		// Atomic write to the rotated path.
		tmpPath := rotatedPath + ".tmp"
		if err := os.WriteFile(tmpPath, newData, 0o644); err != nil {
			return fmt.Errorf("AppendAuditEntryWithRotation: write rotated temp file: %w", err)
		}
		if err := os.Rename(tmpPath, rotatedPath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("AppendAuditEntryWithRotation: rename rotated file: %w", err)
		}

		// logPath is left untouched (Requirement 15.5: prior file preserved).
		return nil
	}

	// Step 3 — no rotation needed: delegate to AppendAuditEntry.
	return AppendAuditEntry(logPath, entry)
}

// AddWorkflowVersion injects the workflow definition version into an
// AuditRunEntry as a sentinel AuditStepEntry with StepID "workflow-version".
// The Outcome field of that step entry carries the version string.
//
// The version is extracted from workflowJSONContent by searching for the
// top-level `"version":` key and reading its string value. If no version key
// is found, or if the value cannot be read as a plain string, the version
// defaults to "unknown".
//
// Because AuditRunEntry's SHA-256 hash is computed over all fields including
// the Steps slice, this injection MUST be called before ComputeAuditHash /
// FinalizePartialAuditEntry so that the version is covered by the hash.
//
// Implementation note: a full JSON parse is avoided deliberately. The workflow
// JSON is a known-format document; a targeted string search keeps the logic
// simple, testable without a dependency on a specific JSON structure, and free
// from ordering assumptions.
//
// (Requirement 15.6)
func AddWorkflowVersion(entry *AuditRunEntry, workflowJSONContent string) {
	version := extractWorkflowVersion(workflowJSONContent)

	now := time.Now().UTC().Format(time.RFC3339)
	entry.Steps = append(entry.Steps, AuditStepEntry{
		StepID:    "workflow-version",
		StepStart: now,
		StepEnd:   now,
		Outcome:   version,
	})
}

// extractWorkflowVersion parses the `"version"` string value from the
// top-level of workflowJSONContent using lightweight string manipulation.
//
// It looks for the pattern `"version"` followed by optional whitespace, a
// colon, optional whitespace, and a double-quoted string value. The quoted
// value is extracted and returned verbatim (without the surrounding quotes).
//
// Returns "unknown" when:
//   - The `"version"` key is not present.
//   - The value is not a quoted string (e.g., it is a number or null).
//   - The quoted string is empty.
func extractWorkflowVersion(content string) string {
	// Search for the literal token `"version"` in the content.
	const key = `"version"`
	idx := strings.Index(content, key)
	if idx < 0 {
		return "unknown"
	}

	// Advance past the key.
	rest := content[idx+len(key):]

	// Skip optional whitespace and the required colon.
	rest = strings.TrimLeft(rest, " \t\r\n")
	if len(rest) == 0 || rest[0] != ':' {
		return "unknown"
	}
	rest = rest[1:] // consume ':'

	// Skip optional whitespace before the value.
	rest = strings.TrimLeft(rest, " \t\r\n")

	// Expect a double-quoted string value.
	if len(rest) == 0 || rest[0] != '"' {
		return "unknown"
	}
	rest = rest[1:] // consume opening '"'

	// Read up to the closing '"', honoring basic backslash escapes so that
	// escaped quotes inside the value string do not terminate the scan early.
	var sb strings.Builder
	for i := 0; i < len(rest); i++ {
		ch := rest[i]
		if ch == '\\' && i+1 < len(rest) {
			// Skip the escape character and include the next byte verbatim.
			// This handles common escapes (\", \\, \n, etc.) without a full
			// JSON decoder — sufficient for version strings.
			i++
			sb.WriteByte(rest[i])
			continue
		}
		if ch == '"' {
			// Closing quote found.
			break
		}
		sb.WriteByte(ch)
	}

	v := sb.String()
	if v == "" {
		return "unknown"
	}
	return v
}

