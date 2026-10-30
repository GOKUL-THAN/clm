// Package audit gives the gateway an append-only record of who did what.
//
// Every other durable store in this component (state.Store, credstore) is a
// full-snapshot JSON file: load everything at startup, rewrite the whole file
// on every change. That is the right shape for state that is small and
// read-mostly, which is why it was reused for the certificate inventory and
// for credentials. An audit trail is neither: it only grows, a rewrite of the
// whole history on every event would get slower as the log does, and — the
// part that actually matters — an audit record must not be revisable. A
// snapshot store that holds the log as a JSON array invites exactly the "load
// it, drop the entry you want gone, save it back" pattern an audit trail
// exists to prevent.
//
// So this is a separate, deliberately simpler mechanism: one JSON object per
// line, appended and fsynced, never rewritten. The file is the source of
// truth for the full history and for export; a bounded in-memory ring buffer
// serves the query API without reading a growing file on every request.
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Event is one recorded action.
type Event struct {
	ID         string    `json:"id"`
	Time       time.Time `json:"time"`
	Actor      string    `json:"actor"`                 // caller email/username, or "unknown"
	Role       string    `json:"role,omitempty"`        // the role the caller held at the time
	Action     string    `json:"action"`                // e.g. "certificate.issue", "approval.approve"
	TargetType string    `json:"target_type,omitempty"` // certificate, approval, ca_account, credential, deployment, policy
	TargetID   string    `json:"target_id,omitempty"`
	// TargetLabel is the human-readable identity of the target (a domain, an
	// account name) — a UUID means nothing to a reviewer six months later.
	TargetLabel   string `json:"target_label,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Result        string `json:"result"` // success, failure, denied
	// Detail carries the error message on failure, or a short description of
	// what changed on success. Never a secret value or a private key: callers
	// must not put credential material here.
	Detail     string `json:"detail,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
}

const (
	ResultSuccess = "success"
	ResultFailure = "failure"
	// ResultDenied is for an authorization refusal, kept distinct from a
	// business-logic failure because "someone tried and was refused" is a
	// different signal than "someone was allowed and it went wrong".
	ResultDenied = "denied"
)

// maxMemory bounds the ring buffer used for the query API. The file on disk
// keeps every event ever recorded; this only bounds how far back a live query
// can see without going to the export path.
const maxMemory = 20000

// Logger appends events to a JSONL file and serves recent ones from memory.
//
// A zero-value Logger (returned when dir is empty) accepts every call and does
// nothing, the same convention state.Store uses, so callers do not need to
// branch on whether auditing is configured.
type Logger struct {
	dir    string
	path   string
	file   *os.File
	logger *zap.Logger

	mu     sync.Mutex
	recent []*Event // ring buffer, oldest first, capped at maxMemory
}

// New opens (or creates) the audit log under dir. An empty dir disables
// auditing rather than failing, consistent with how state persistence itself
// is made optional — a gateway with no state.dir configured has nowhere
// durable to put this either.
func New(dir string, logger *zap.Logger) (*Logger, error) {
	l := &Logger{logger: logger}
	if strings.TrimSpace(dir) == "" {
		logger.Warn("audit trail is disabled: no directory configured " +
			"(set state.dir so actions taken on this gateway are recorded)")
		return l, nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating audit dir %s: %w", dir, err)
	}
	l.dir = dir
	l.path = filepath.Join(dir, "audit.jsonl")

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening audit log %s: %w", l.path, err)
	}
	l.file = f

	if err := l.loadRecent(); err != nil {
		logger.Error("could not load recent audit history; the log itself is unaffected",
			zap.Error(err))
	}

	logger.Info("audit trail enabled", zap.String("path", l.path), zap.Int("in_memory", len(l.recent)))
	return l, nil
}

// Enabled reports whether events are actually being recorded.
func (l *Logger) Enabled() bool { return l != nil && l.file != nil }

// loadRecent reads the tail of the log into memory so the query API has
// something to answer with immediately after a restart, rather than only
// after the next event is recorded.
//
// This reads the whole file to find the tail, which is fine at the sizes an
// audit log reaches on a single gateway; it is not the read path a query uses
// day to day, only the one-time cost at startup.
func (l *Logger) loadRecent() error {
	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var buf []*Event
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			// A single corrupt line (e.g. a partial write from a crash) must not
			// take down the rest of the history.
			l.logger.Warn("skipping unreadable audit log line", zap.Error(err))
			continue
		}
		e := ev
		buf = append(buf, &e)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(buf) > maxMemory {
		buf = buf[len(buf)-maxMemory:]
	}
	l.recent = buf
	return nil
}

// Record appends one event. It never returns an error to the caller: a
// mutating action that already happened must not be blocked or rolled back
// because the audit write failed, but the failure is logged loudly because a
// silent gap in the trail is exactly the failure mode this package exists to
// avoid.
func (l *Logger) Record(ev Event) {
	if !l.Enabled() {
		return
	}
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	if ev.Actor == "" {
		ev.Actor = "unknown"
	}
	if ev.Result == "" {
		ev.Result = ResultSuccess
	}

	data, err := json.Marshal(ev)
	if err != nil {
		l.logger.Error("could not encode audit event", zap.Error(err), zap.String("action", ev.Action))
		return
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.file.Write(data); err != nil {
		l.logger.Error("audit event was not written to disk",
			zap.Error(err), zap.String("action", ev.Action), zap.String("actor", ev.Actor))
		// Still recorded in memory below, so a query at least surfaces it for
		// this process's lifetime even though the durable copy failed.
	} else if err := l.file.Sync(); err != nil {
		// fsync failing after a successful Write means the event is not durable
		// yet. Reported the same way: loudly, without blocking the caller.
		l.logger.Error("audit event written but not synced to disk",
			zap.Error(err), zap.String("action", ev.Action))
	}

	e := ev
	l.recent = append(l.recent, &e)
	if len(l.recent) > maxMemory {
		l.recent = l.recent[len(l.recent)-maxMemory:]
	}
}

// Query filters the in-memory buffer, newest first.
type Query struct {
	Actor      string
	Action     string
	TargetType string
	Result     string
	Since      time.Time
	Until      time.Time
	Limit      int
}

// Search returns matching events, most recent first, plus the total match
// count (which can exceed len(events) when Limit truncated the result).
func (l *Logger) Search(q Query) (events []*Event, total int) {
	if !l.Enabled() {
		return nil, 0
	}
	l.mu.Lock()
	// Copy the slice header under the lock, then filter outside it: Record only
	// ever appends, so the elements already in the snapshot are never mutated
	// after the fact, but the backing array can grow concurrently.
	snapshot := l.recent
	l.mu.Unlock()

	matches := make([]*Event, 0, len(snapshot))
	for i := len(snapshot) - 1; i >= 0; i-- {
		e := snapshot[i]
		if q.Actor != "" && !strings.EqualFold(e.Actor, q.Actor) {
			continue
		}
		if q.Action != "" && !strings.Contains(strings.ToLower(e.Action), strings.ToLower(q.Action)) {
			continue
		}
		if q.TargetType != "" && !strings.EqualFold(e.TargetType, q.TargetType) {
			continue
		}
		if q.Result != "" && !strings.EqualFold(e.Result, q.Result) {
			continue
		}
		if !q.Since.IsZero() && e.Time.Before(q.Since) {
			continue
		}
		if !q.Until.IsZero() && e.Time.After(q.Until) {
			continue
		}
		matches = append(matches, e)
	}
	total = len(matches)

	limit := q.Limit
	if limit <= 0 || limit > total {
		limit = total
	}
	return matches[:limit], total
}

// Actions returns the distinct action names seen recently, for the portal's
// filter dropdown.
func (l *Logger) Actions() []string {
	if !l.Enabled() {
		return nil
	}
	l.mu.Lock()
	snapshot := l.recent
	l.mu.Unlock()

	seen := map[string]bool{}
	for _, e := range snapshot {
		seen[e.Action] = true
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// Path returns the on-disk file location, for the export handler to stream
// the full history rather than only what fits in memory.
func (l *Logger) Path() string { return l.path }

// Close flushes and closes the underlying file. Safe to call on a disabled
// logger.
func (l *Logger) Close() error {
	if !l.Enabled() {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
