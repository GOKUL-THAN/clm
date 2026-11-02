# Apply audit_log configuration and prompt instructions to the workflow JSON
$filePath = "d:\400 clm\CLM\.kiro\workflows\clm-feature-dev.workflow.json"
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$raw = [System.IO.File]::ReadAllText($filePath, [System.Text.Encoding]::UTF8)

# === Change 1: Insert audit_log object between inputs and steps ===
$old1 = @"
  "inputs": {
    "feature_request": "prompt",
    "component": "string",
    "workspace_root": "string"
  },
  "steps": [
"@

$em = [char]0x2014

$auditLogBlock = @"
  "inputs": {
    "feature_request": "prompt",
    "component": "string",
    "workspace_root": "string"
  },
  "audit_log": {
    "path": "{{resolved_root}}/.agents/tasks/workflow-audit-log.json",
    "description": "Cross-cutting audit logging concern. Not a separate agent step. Runs as pre/post hooks around every step. The audit log is append-only across pipeline runs in the same output directory.",
    "format": "Top-level JSON array. Each element is one run entry appended as a separate JSON object. Prior run records are never overwritten or truncated.",
    "append_only": true,
    "flush_on_halt_or_pause": "Before any HALT or PAUSE, flush the partial audit log capturing all steps executed up to and including the failing or pausing step before the pipeline halts.",
    "schema": {
      "run_id": "UUID ${em} unique identifier for this run",
      "run_start": "ISO-8601 timestamp (UTC, accurate to the second, e.g. 2024-01-15T10:00:00Z)",
      "run_end": "ISO-8601 timestamp (UTC, accurate to the second)",
      "workspace_root": "string ${em} the resolved absolute path of the workspace root used for this run",
      "component": "string ${em} the validated component value used for this run",
      "feature_request": "string ${em} the feature_request input value",
      "steps": [
        {
          "step_id": "string ${em} the step identifier",
          "step_start": "ISO-8601 timestamp (UTC, accurate to the second)",
          "step_end": "ISO-8601 timestamp (UTC, accurate to the second)",
          "outcome": "success | paused | failed"
        }
      ],
      "approval_file_events": [
        {
          "file": "string ${em} path to the approval file written",
          "written_at": "ISO-8601 timestamp (UTC, accurate to the second)",
          "version_written": "integer ${em} the version number written"
        }
      ],
      "artifacts_written": [
        "string ${em} resolved absolute paths of all artifact files written during the run"
      ],
      "hash": "sha256:<hex> ${em} SHA-256 integrity hash"
    },
    "secret_exclusion": "Any input whose name ends in '_secret', '_token', '_password', or '_key', or that is marked \"secret\": true in the workflow input definition, MUST be omitted from the log. Only feature_request, workspace_root, and component input values are recorded in the run entry.",
    "hash_computation": "SHA-256 hash is computed over the JSON-serialized run entry with the \"hash\" field set to \"\" (empty string). The resulting digest is written into the \"hash\" field as \"sha256:<hex>\". This enables deterministic re-computation for integrity verification.",
    "write_failure_handling": "If the workflow-audit-log.json file cannot be written due to a filesystem error, surface ERROR: Audit logging failed: <reason> and do NOT report the run as successfully completed.",
    "record_artifact_paths": true,
    "record_approval_file_events": true
  },
  "steps": [
"@

if ($raw.Contains($old1)) {
    $raw = $raw.Replace($old1, $auditLogBlock)
    Write-Output "Change 1 applied: audit_log object inserted between inputs and steps"
} else {
    Write-Output "ERROR: Could not find match for Change 1 (inputs/steps boundary)"
    exit 1
}

# === Change 2: Insert audit logging section in validate-component prompt ===
# The match is within a JSON string value, so \n are literal backslash-n
$old2 = '## Completion\n\nIf both steps succeed'

$auditPromptText = '## Audit Logging\n\nAs the first pipeline step, initialize the workflow audit log run entry in {{resolved_root}}/.agents/tasks/workflow-audit-log.json (configured under the top-level `audit_log` object). The audit log is a top-level JSON array; this run entry is appended as a new object ' + [char]0x2014 + ' never overwrite prior run records.\n\nThe run entry must contain: run_id (UUID), run_start (ISO-8601), run_end (ISO-8601, set when the run completes or halts), workspace_root, component, feature_request, the ordered list of steps executed (each with step_id, step_start, step_end, and outcome as one of success/paused/failed), approval_file_events (each with file path, written_at, and version_written), artifacts_written (resolved absolute paths), and a SHA-256 integrity hash.\n\nSecret exclusion: Any input whose name ends in ''_secret'', ''_token'', ''_password'', or ''_key'', or that is marked \"secret\": true in the workflow input definition MUST be omitted. Only feature_request, workspace_root, and component are recorded.\n\nSHA-256 hash computation: Compute the hash over the JSON-serialized run entry with the \"hash\" field set to \"\" (empty string), then write the resulting digest into the \"hash\" field as \"sha256:<hex>\".\n\nBefore any HALT or PAUSE, flush the partial audit log with steps executed so far. If the audit log file cannot be written due to a filesystem error, surface ERROR: Audit logging failed: <reason> and do NOT report the run as successfully completed.\n\n## Completion\n\nIf both steps succeed'

if ($raw.Contains($old2)) {
    $raw = $raw.Replace($old2, $auditPromptText)
    Write-Output "Change 2 applied: audit logging section inserted in validate-component prompt"
} else {
    Write-Output "ERROR: Could not find match for Change 2 (## Completion in validate-component prompt)"
    exit 1
}

# Write the file back as UTF-8 without BOM
[System.IO.File]::WriteAllText($filePath, $raw, $utf8NoBom)
Write-Output "File written successfully"

# Validate JSON
try {
    $null = $raw | ConvertFrom-Json
    Write-Output "JSON validation: PASSED"
} catch {
    Write-Output "JSON validation: FAILED - $_"
    exit 1
}
