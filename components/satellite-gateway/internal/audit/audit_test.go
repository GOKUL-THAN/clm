package audit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.uber.org/zap"
)

func testLogger() *zap.Logger { return zap.NewNop() }

func TestNewWithEmptyDirIsDisabled(t *testing.T) {
	l, err := New("", testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l.Enabled() {
		t.Error("expected a logger with no directory to be disabled")
	}
	// Every method must be a safe no-op on a disabled logger.
	l.Record(Event{Action: "test"})
	events, total := l.Search(Query{})
	if events != nil || total != 0 {
		t.Errorf("Search on a disabled logger returned %v, %d; want nil, 0", events, total)
	}
	if got := l.Actions(); got != nil {
		t.Errorf("Actions on a disabled logger = %v, want nil", got)
	}
	if err := l.Close(); err != nil {
		t.Errorf("Close on a disabled logger returned an error: %v", err)
	}
}

func TestRecordAssignsDefaults(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Close()

	l.Record(Event{Action: "certificate.issue"})

	events, total := l.Search(Query{})
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	ev := events[0]
	if ev.ID == "" {
		t.Error("expected an ID to be generated")
	}
	if ev.Time.IsZero() {
		t.Error("expected a timestamp to be set")
	}
	if ev.Actor != "unknown" {
		t.Errorf("Actor = %q, want \"unknown\" as the default", ev.Actor)
	}
	if ev.Result != ResultSuccess {
		t.Errorf("Result = %q, want %q as the default", ev.Result, ResultSuccess)
	}
}

func TestRecordPersistsAcrossRestart(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("audit file permission checks require Linux mode semantics")
	}
	dir := t.TempDir()
	l1, err := New(dir, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	l1.Record(Event{Actor: "alice", Action: "certificate.issue", TargetLabel: "example.com"})
	l1.Record(Event{Actor: "bob", Action: "certificate.revoke", TargetLabel: "example.org"})
	if err := l1.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// Simulate a restart: open a fresh Logger over the same directory.
	l2, err := New(dir, testLogger())
	if err != nil {
		t.Fatalf("unexpected error reopening: %v", err)
	}
	defer l2.Close()

	_, total := l2.Search(Query{})
	if total != 2 {
		t.Errorf("total after reopen = %d, want 2", total)
	}
}

func TestSearchFiltersByActor(t *testing.T) {
	l := newTestLogger(t)
	l.Record(Event{Actor: "alice", Action: "certificate.issue"})
	l.Record(Event{Actor: "bob", Action: "certificate.issue"})

	events, total := l.Search(Query{Actor: "alice"})
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if events[0].Actor != "alice" {
		t.Errorf("got actor %q, want alice", events[0].Actor)
	}

	// Case-insensitive.
	_, total = l.Search(Query{Actor: "ALICE"})
	if total != 1 {
		t.Errorf("actor filter should be case-insensitive, total = %d", total)
	}
}

func TestSearchFiltersByAction(t *testing.T) {
	l := newTestLogger(t)
	l.Record(Event{Actor: "a", Action: "certificate.issue"})
	l.Record(Event{Actor: "a", Action: "certificate.revoke"})
	l.Record(Event{Actor: "a", Action: "approval.approve"})

	// Substring match, so "certificate" matches both issue and revoke.
	_, total := l.Search(Query{Action: "certificate"})
	if total != 2 {
		t.Errorf("substring action filter: total = %d, want 2", total)
	}

	_, total = l.Search(Query{Action: "certificate.revoke"})
	if total != 1 {
		t.Errorf("exact action filter: total = %d, want 1", total)
	}
}

func TestSearchFiltersByTargetTypeAndResult(t *testing.T) {
	l := newTestLogger(t)
	l.Record(Event{Actor: "a", Action: "x", TargetType: "certificate", Result: ResultSuccess})
	l.Record(Event{Actor: "a", Action: "x", TargetType: "certificate", Result: ResultFailure})
	l.Record(Event{Actor: "a", Action: "x", TargetType: "approval", Result: ResultSuccess})

	_, total := l.Search(Query{TargetType: "certificate"})
	if total != 2 {
		t.Errorf("target_type filter: total = %d, want 2", total)
	}

	_, total = l.Search(Query{Result: ResultFailure})
	if total != 1 {
		t.Errorf("result filter: total = %d, want 1", total)
	}
}

func TestSearchFiltersByTimeRange(t *testing.T) {
	l := newTestLogger(t)
	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)
	l.Record(Event{Actor: "a", Action: "x", Time: old})
	l.Record(Event{Actor: "a", Action: "x", Time: recent})

	_, total := l.Search(Query{Since: time.Now().Add(-24 * time.Hour)})
	if total != 1 {
		t.Errorf("since filter: total = %d, want 1", total)
	}

	_, total = l.Search(Query{Until: time.Now().Add(-24 * time.Hour)})
	if total != 1 {
		t.Errorf("until filter: total = %d, want 1", total)
	}
}

func TestSearchNewestFirst(t *testing.T) {
	l := newTestLogger(t)
	l.Record(Event{Actor: "a", Action: "first", Time: time.Now().Add(-2 * time.Hour)})
	l.Record(Event{Actor: "a", Action: "second", Time: time.Now().Add(-1 * time.Hour)})
	l.Record(Event{Actor: "a", Action: "third", Time: time.Now()})

	events, _ := l.Search(Query{})
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	if events[0].Action != "third" || events[2].Action != "first" {
		t.Errorf("events not newest-first: %s, %s, %s",
			events[0].Action, events[1].Action, events[2].Action)
	}
}

func TestSearchRespectsLimit(t *testing.T) {
	l := newTestLogger(t)
	for i := 0; i < 10; i++ {
		l.Record(Event{Actor: "a", Action: "x"})
	}
	events, total := l.Search(Query{Limit: 3})
	if len(events) != 3 {
		t.Errorf("len(events) = %d, want 3", len(events))
	}
	if total != 10 {
		t.Errorf("total = %d, want 10 (total should reflect all matches, not the limited page)", total)
	}
}

func TestActionsReturnsDistinctSorted(t *testing.T) {
	l := newTestLogger(t)
	l.Record(Event{Actor: "a", Action: "certificate.issue"})
	l.Record(Event{Actor: "a", Action: "approval.approve"})
	l.Record(Event{Actor: "a", Action: "certificate.issue"}) // duplicate

	actions := l.Actions()
	if len(actions) != 2 {
		t.Fatalf("got %d distinct actions, want 2: %v", len(actions), actions)
	}
	if actions[0] != "approval.approve" || actions[1] != "certificate.issue" {
		t.Errorf("actions not sorted: %v", actions)
	}
}

func TestLoadRecentSkipsCorruptLines(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("audit file permission checks require Linux mode semantics")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	content := `{"id":"1","action":"good.one","actor":"a","result":"success","time":"2026-01-01T00:00:00Z"}
not valid json at all
{"id":"2","action":"good.two","actor":"a","result":"success","time":"2026-01-01T00:00:01Z"}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	l, err := New(dir, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Close()

	_, total := l.Search(Query{})
	if total != 2 {
		t.Errorf("expected the 2 valid lines to load despite the corrupt one, got total=%d", total)
	}
}

func TestPathReturnsTheLogFile(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Close()

	if l.Path() != filepath.Join(dir, "audit.jsonl") {
		t.Errorf("Path() = %q, unexpected", l.Path())
	}
}

func newTestLogger(t *testing.T) *Logger {
	t.Helper()
	l, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}
