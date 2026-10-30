package state

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const commandCollection = "processed_commands"

type CommandResult struct {
	CommandID           uuid.UUID `json:"command_id"`
	ExecutionGeneration int       `json:"execution_generation,omitempty"`
	Status              string    `json:"status"`
	Result              string    `json:"result,omitempty"`
	Error               string    `json:"error,omitempty"`
	Retryable           bool      `json:"retryable"`
	CompletedAt         time.Time `json:"completed_at"`
}

type commandExecution struct {
	commandID           uuid.UUID
	executionGeneration int
}

// CommandJournal durably records command outcomes before they are posted. A
// redelivery of the same execution generation returns its saved outcome instead
// of repeating effects. An explicit server-authorized retry increments the
// generation and is allowed to execute again.
type CommandJournal struct {
	store   *Store
	mu      sync.RWMutex
	results map[commandExecution]CommandResult
}

func NewCommandJournal(store *Store) (*CommandJournal, error) {
	j := &CommandJournal{store: store, results: make(map[commandExecution]CommandResult)}
	if store == nil {
		return j, nil
	}
	var snapshot struct {
		Results []CommandResult `json:"results"`
	}
	if err := store.LoadStrict(commandCollection, &snapshot); err != nil {
		return nil, err
	}
	for _, result := range snapshot.Results {
		j.results[commandExecution{commandID: result.CommandID, executionGeneration: result.ExecutionGeneration}] = result
	}
	return j, nil
}

// Get returns a saved outcome for the exact execution generation. Delivery
// attempt numbers may increase when a lease expires, but the generation changes
// only when an operator explicitly requests re-execution.
func (j *CommandJournal) Get(id uuid.UUID, executionGeneration int) (CommandResult, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	result, ok := j.results[commandExecution{commandID: id, executionGeneration: executionGeneration}]
	return result, ok
}

func (j *CommandJournal) Put(result CommandResult) error {
	if result.CommandID == uuid.Nil {
		return fmt.Errorf("command ID is required")
	}
	if result.ExecutionGeneration < 0 {
		return fmt.Errorf("command execution generation cannot be negative")
	}
	if result.CompletedAt.IsZero() {
		result.CompletedAt = time.Now().UTC()
	}

	// Keep the lock through the durable write. Otherwise concurrent callers can
	// persist stale snapshots out of order and silently erase a newer result.
	j.mu.Lock()
	defer j.mu.Unlock()
	key := commandExecution{commandID: result.CommandID, executionGeneration: result.ExecutionGeneration}
	previous, existed := j.results[key]
	j.results[key] = result
	if j.store != nil {
		if err := j.store.SaveNow(commandCollection, map[string]any{"results": j.snapshotLocked()}); err != nil {
			if existed {
				j.results[key] = previous
			} else {
				delete(j.results, key)
			}
			return err
		}
	}
	return nil
}

func (j *CommandJournal) Delete(id uuid.UUID, executionGeneration int) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	key := commandExecution{commandID: id, executionGeneration: executionGeneration}
	previous, existed := j.results[key]
	if !existed {
		return nil
	}
	delete(j.results, key)
	if j.store != nil {
		if err := j.store.SaveNow(commandCollection, map[string]any{"results": j.snapshotLocked()}); err != nil {
			j.results[key] = previous
			return err
		}
	}
	return nil
}

func (j *CommandJournal) snapshotLocked() []CommandResult {
	out := make([]CommandResult, 0, len(j.results))
	for _, result := range j.results {
		out = append(out, result)
	}
	return out
}
