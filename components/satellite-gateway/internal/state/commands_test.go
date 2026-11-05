package state

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestCommandJournalSurvivesRestart(t *testing.T) {
	store, err := New(t.TempDir(), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	journal, err := NewCommandJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	want := CommandResult{CommandID: id, ExecutionGeneration: 1, Status: "succeeded", Result: "done", CompletedAt: time.Now().UTC()}
	if err := journal.Put(want); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewCommandJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.Get(id, 1)
	if !ok {
		t.Fatal("processed command was lost after restart")
	}
	if got.Status != want.Status || got.Result != want.Result {
		t.Fatalf("got=%+v want=%+v", got, want)
	}
}

func TestCommandJournalDuplicateLookupReturnsOutcome(t *testing.T) {
	journal, _ := NewCommandJournal(nil)
	id := uuid.New()
	first := CommandResult{CommandID: id, ExecutionGeneration: 1, Status: "succeeded", Result: "first"}
	if err := journal.Put(first); err != nil {
		t.Fatal(err)
	}
	got, ok := journal.Get(id, 1)
	if !ok || got.Result != "first" {
		t.Fatalf("duplicate lookup failed: %+v", got)
	}
}

func TestCommandJournalCorruptionFailsClosed(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := os.WriteFile(filepath.Join(dir, commandCollection+".json"), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCommandJournal(store); err == nil {
		t.Fatal("corrupt command journal was accepted")
	}
}

func TestCommandJournalScopesDuplicateSuppressionToExecutionGeneration(t *testing.T) {
	journal, err := NewCommandJournal(nil)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	result := CommandResult{CommandID: id, ExecutionGeneration: 0, Status: "succeeded", Result: "done"}
	if err := journal.Put(result); err != nil {
		t.Fatal(err)
	}
	// Lease expiry increments the delivery attempt but not the execution
	// generation, so automatic redelivery must replay the saved outcome.
	if _, ok := journal.Get(id, 0); !ok {
		t.Fatal("automatic redelivery was not suppressed")
	}
	// Only an explicit control-plane retry increments the generation and permits
	// the side effect to run again.
	if _, ok := journal.Get(id, 1); ok {
		t.Fatal("a new execution generation was incorrectly suppressed")
	}
}

func TestCommandJournalDeleteSurvivesRestart(t *testing.T) {
	store, err := New(t.TempDir(), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	journal, err := NewCommandJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := journal.Put(CommandResult{CommandID: id, ExecutionGeneration: 2, Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Delete(id, 2); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewCommandJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Get(id, 2); ok {
		t.Fatal("confirmed result remained in the journal after restart")
	}
}

func TestCommandJournalConcurrentWritesSurviveRestart(t *testing.T) {
	store, err := New(t.TempDir(), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	journal, err := NewCommandJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	const count = 16
	ids := make([]uuid.UUID, count)
	var wg sync.WaitGroup
	for i := range ids {
		ids[i] = uuid.New()
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			if err := journal.Put(CommandResult{CommandID: id, ExecutionGeneration: 1, Status: "succeeded"}); err != nil {
				t.Errorf("Put: %v", err)
			}
		}(ids[i])
	}
	wg.Wait()
	reloaded, err := NewCommandJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, ok := reloaded.Get(id, 1); !ok {
			t.Fatalf("concurrent result %s was lost", id)
		}
	}
}
