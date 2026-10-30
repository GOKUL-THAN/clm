package api

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/m2p/clm/internal/satellite/audit"
)

// Audit trail query and export.
//
// Every other view onto the gateway answers "what is true now" — the inventory,
// the fleet, the pending approvals. This answers "what happened", which is the
// question a security review or a customer's auditor actually asks, and none of
// the other views can answer it: an approval that was later deleted, a
// certificate that failed and was pruned, a CA account that existed for a day —
// none of that survives in the current-state views.

// recordAudit is the one place handlers call into the audit log, so every event
// carries the same shape. Detail should describe what happened in a sentence a
// reviewer can read without cross-referencing anything else; it must never
// contain a secret or private key.
func (s *Server) recordAudit(r *http.Request, action, targetType, targetID, targetLabel, result, detail string) {
	claims := callerClaims(r)
	s.audit.Record(audit.Event{
		Actor:         callerName(r),
		Role:          claims.EffectiveRole(),
		Action:        action,
		TargetType:    targetType,
		TargetID:      targetID,
		TargetLabel:   targetLabel,
		CorrelationID: strings.TrimSpace(r.Header.Get("X-Correlation-ID")),
		Result:        result,
		Detail:        detail,
		RemoteAddr:    r.RemoteAddr,
	})
}

// handleAuditQuery searches the in-memory audit history.
// GET /api/v1/audit?actor=&action=&target_type=&result=&since=&until=&limit=
func (s *Server) handleAuditQuery(w http.ResponseWriter, r *http.Request) {
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden,
			"viewing the audit trail requires an admin or cert_manager role")
		return
	}
	if !s.audit.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{
			"events": []any{}, "total": 0,
			"reason": "the audit trail is disabled on this gateway: set state.dir to enable it",
		})
		return
	}

	q := audit.Query{
		Actor:      strings.TrimSpace(r.URL.Query().Get("actor")),
		Action:     strings.TrimSpace(r.URL.Query().Get("action")),
		TargetType: strings.TrimSpace(r.URL.Query().Get("target_type")),
		Result:     strings.TrimSpace(r.URL.Query().Get("result")),
	}
	if v := r.URL.Query().Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "since must be RFC3339, e.g. 2026-01-01T00:00:00Z")
			return
		}
		q.Since = t
	}
	if v := r.URL.Query().Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "until must be RFC3339")
			return
		}
		q.Until = t
	}
	q.Limit = 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q.Limit = n
		}
	}
	// A single request must not be able to pull the whole in-memory buffer (up
	// to 20,000 events) into one JSON response; the export endpoint is for a
	// full pull.
	const hardCap = 2000
	if q.Limit > hardCap {
		q.Limit = hardCap
	}

	events, total := s.audit.Search(q)
	writeJSON(w, http.StatusOK, map[string]any{
		"events":   events,
		"total":    total,
		"returned": len(events),
		"actions":  s.audit.Actions(),
	})
}

// handleAuditExport streams the durable log file rather than the in-memory
// buffer, so an export reaches back further than a live query can — the buffer
// is capped, the file on disk is not.
// GET /api/v1/audit/export?format=csv|jsonl
func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden,
			"exporting the audit trail requires an admin or cert_manager role")
		return
	}
	if !s.audit.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "the audit trail is disabled on this gateway")
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "csv"
	}
	stamp := time.Now().UTC().Format("20060102-150405")

	f, err := os.Open(s.audit.Path())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "reading the audit log failed: "+err.Error())
		return
	}
	defer f.Close()

	switch format {
	case "jsonl":
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="clm-audit-%s.jsonl"`, stamp))
		_, _ = io.Copy(w, f)

	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="clm-audit-%s.csv"`, stamp))
		cw := csv.NewWriter(w)
		defer cw.Flush()
		_ = cw.Write([]string{"id", "time", "actor", "role", "action",
			"target_type", "target_id", "target_label", "correlation_id", "result", "detail", "remote_addr"})

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var ev audit.Event
			if err := json.Unmarshal(line, &ev); err != nil {
				continue // a corrupt line must not stop the rest of the export
			}
			_ = cw.Write([]string{
				ev.ID, ev.Time.UTC().Format(time.RFC3339), ev.Actor, ev.Role, ev.Action,
				ev.TargetType, ev.TargetID, ev.TargetLabel, ev.CorrelationID, ev.Result, ev.Detail, ev.RemoteAddr,
			})
		}

	default:
		writeErr(w, http.StatusBadRequest, "format must be csv or jsonl")
	}
}
