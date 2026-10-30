// Package state gives the gateway durable storage for the data it must not lose
// on restart.
//
// Everything the gateway tracked used to live in Go maps, so a restart wiped the
// certificate inventory, every approval decision, and the whole vSatellite fleet
// including agent credentials — which meant every agent had to be re-enrolled by
// hand. Discovery could repopulate certificates; approvals and enrolments were
// simply gone.
//
// This is a JSON snapshot store rather than an embedded database on purpose. The
// binary is built with CGO_ENABLED=0, so SQLite is out without a pure-Go driver,
// and the data here is small, read-mostly and always loaded in full. A snapshot
// per collection keeps the format inspectable and the failure modes obvious.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Store persists named collections as JSON files in a directory.
//
// A zero-value Store with no directory is valid and does nothing, so callers do
// not need to branch on whether persistence is configured.
type Store struct {
	dir    string
	logger *zap.Logger

	mu sync.Mutex
	// pending coalesces rapid saves. A discovery scan upserts hundreds of
	// certificates in a burst; writing the file for each one would be pointless
	// churn, so writes are debounced per collection.
	pending   map[string]*time.Timer
	sequence  map[string]uint64
	committed map[string]uint64

	// writeMu serializes temp-file replacement across synchronous and debounced
	// writes. Per-collection sequence numbers then prevent an older timer from
	// replacing a newer safety checkpoint after it finally acquires this lock.
	writeMu sync.Mutex
}

// SaveDelay is how long a save is deferred to absorb a burst of changes.
const SaveDelay = 750 * time.Millisecond

// New opens a store rooted at dir. An empty dir returns a disabled store that
// silently accepts calls, which is how persistence stays optional.
func New(dir string, logger *zap.Logger) (*Store, error) {
	s := &Store{
		dir: dir, logger: logger,
		pending: map[string]*time.Timer{}, sequence: map[string]uint64{}, committed: map[string]uint64{},
	}
	if dir == "" {
		logger.Warn("state persistence is disabled; the inventory and fleet will " +
			"be lost on restart (set state.dir to enable it)")
		return s, nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating state dir %s: %w", dir, err)
	}
	// Fail fast on an unwritable directory rather than discovering it on the
	// first save, by which point the caller has no way to report it.
	probe := filepath.Join(dir, ".write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return nil, fmt.Errorf("state dir %s is not writable: %w", dir, err)
	}
	_ = os.Remove(probe)

	logger.Info("state persistence enabled", zap.String("dir", dir))
	return s, nil
}

// Enabled reports whether snapshots are actually written.
func (s *Store) Enabled() bool { return s != nil && s.dir != "" }

// Dir returns the snapshot directory, empty when disabled.
func (s *Store) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name+".json")
}

// Load reads a collection into v. A missing file is not an error: first start is
// the normal case, and treating it as a failure would block startup.
func (s *Store) Load(name string, v any) error {
	if !s.Enabled() {
		return nil
	}
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", name, err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, v); err != nil {
		// A corrupt snapshot must not stop the gateway from starting. Keep the
		// bad file for inspection and carry on empty.
		bad := s.path(name) + ".corrupt"
		_ = os.Rename(s.path(name), bad)
		s.logger.Error("state snapshot was unreadable and has been set aside",
			zap.String("collection", name), zap.String("kept_at", bad), zap.Error(err))
		return nil
	}
	s.logger.Info("state snapshot loaded",
		zap.String("collection", name), zap.Int("bytes", len(data)))
	return nil
}

// SaveNow writes a collection immediately and durably.
//
// The write goes to a temporary file which is then renamed over the target.
// Rename within a directory is atomic, so a crash mid-write leaves the previous
// snapshot intact instead of a half-written file that would fail to parse.
func (s *Store) SaveNow(name string, v any) error {
	if !s.Enabled() {
		return nil
	}
	s.mu.Lock()
	if s.sequence == nil {
		s.sequence = map[string]uint64{}
	}
	s.sequence[name]++
	sequence := s.sequence[name]
	s.mu.Unlock()
	return s.saveVersion(name, v, sequence)
}

func (s *Store) saveVersion(name string, v any, sequence uint64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.Lock()
	if s.committed == nil {
		s.committed = map[string]uint64{}
	}
	if sequence < s.committed[name] {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", name, err)
	}

	target := s.path(name)
	tmp, err := os.CreateTemp(s.dir, "."+name+"-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", name, err)
	}
	tmpName := tmp.Name()

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("writing %s: %w", name, err)
	}
	// fsync before rename: without it the rename can be visible while the
	// contents are not yet on disk, which is exactly the truncated-file case the
	// atomic rename is meant to prevent.
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("syncing %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("closing %s: %w", name, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("setting mode on %s: %w", name, err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replacing %s: %w", name, err)
	}

	s.mu.Lock()
	if sequence > s.committed[name] {
		s.committed[name] = sequence
	}
	s.mu.Unlock()
	return nil
}

// Save schedules a write, coalescing calls that arrive within SaveDelay.
//
// snapshot is called at write time rather than taking a value now, so the write
// captures the collection's state then and the caller does not have to build a
// copy for every mutation.
func (s *Store) Save(name string, snapshot func() any) {
	if !s.Enabled() {
		return
	}
	s.mu.Lock()
	if s.sequence == nil {
		s.sequence = map[string]uint64{}
	}
	s.sequence[name]++
	sequence := s.sequence[name]
	if t, ok := s.pending[name]; ok {
		t.Stop()
	}
	s.pending[name] = time.AfterFunc(SaveDelay, func() {
		s.mu.Lock()
		delete(s.pending, name)
		s.mu.Unlock()

		if err := s.saveVersion(name, snapshot(), sequence); err != nil {
			s.logger.Error("saving state snapshot failed",
				zap.String("collection", name), zap.Error(err))
		}
	})
	s.mu.Unlock()
}

// Flush writes any pending save immediately. Called on shutdown so a clean stop
// does not lose the last few seconds of changes.
func (s *Store) Flush() {
	if !s.Enabled() {
		return
	}
	s.mu.Lock()
	timers := make([]*time.Timer, 0, len(s.pending))
	for _, t := range s.pending {
		timers = append(timers, t)
	}
	s.mu.Unlock()

	// Firing a pending timer early is not possible, so wait for the debounce
	// window plus a margin for the writes themselves.
	if len(timers) > 0 {
		time.Sleep(SaveDelay + 500*time.Millisecond)
	}
}

// LoadStrict reads security-sensitive state and fails startup if it is corrupt.
// Command deduplication cannot safely fall back to an empty journal because that
// would repeat previously completed side effects.
func (s *Store) LoadStrict(name string, v any) error {
	if !s.Enabled() {
		return nil
	}
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", name, err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("decoding security-sensitive state %s: %w", name, err)
	}
	return nil
}
