// Package credstore holds named sets of credentials for CA accounts and for
// deployment targets.
//
// Both needed the same thing and neither had it. CA accounts could only come from
// environment variables read at startup, and a deployment had no way to carry the
// credentials for its target at all — the deploy endpoint took certificate
// material and nothing else, so an AWS, GCP or Azure target had nothing to
// authenticate with.
//
// Two rules the rest of the code depends on:
//
//   - A credential value is never returned by any read path. List and Redacted
//     return key names and masked presence only. Reveal exists for the one caller
//     that genuinely needs the value — code building a provider or deployer — and
//     is not reachable from the API.
//   - Files are written 0600 in the state directory. This is not encryption at
//     rest: anything with root on the host, or a copy of the volume, can read
//     them. That limit is documented rather than papered over.
package credstore

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/m2p/clm/internal/satellite/state"
	"go.uber.org/zap"
)

// Set is one named credential bundle.
type Set struct {
	Name string `json:"name"`
	// Kind groups sets by what they are for: "ca" or "deploy".
	Kind string `json:"kind"`
	// Service is the vendor or target type the credentials belong to, for example
	// "godaddy", "aws-acm", "gcp-lb", or "azure-keyvault".
	Service    string            `json:"service"`
	Values     map[string]string `json:"values"`
	SecretKeys []string          `json:"secret_keys"`
	CreatedBy  string            `json:"created_by,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// Store persists credential sets.
type Store struct {
	mu     sync.RWMutex
	sets   map[string]*Set
	store  *state.Store
	logger *zap.Logger
}

const collection = "credentials"

// ErrConcurrentUpdate means the caller edited a stale representation and must
// reload before retrying rather than overwriting a newer credential change.
var ErrConcurrentUpdate = errors.New("credential set changed since it was loaded")

// New opens the store and loads anything already saved.
func New(st *state.Store, logger *zap.Logger) *Store {
	s := &Store{sets: map[string]*Set{}, store: st, logger: logger}

	if st.Enabled() {
		var snap struct {
			Sets []*Set `json:"sets"`
		}
		if err := st.Load(collection, &snap); err != nil {
			logger.Error("loading credentials failed", zap.Error(err))
		} else {
			for _, set := range snap.Sets {
				if set == nil || normalise(set.Name) == "" {
					continue
				}
				loaded := cloneSet(set)
				loaded.Name = normalise(loaded.Name)
				loaded.Kind = strings.ToLower(strings.TrimSpace(loaded.Kind))
				loaded.Service = strings.ToLower(strings.TrimSpace(loaded.Service))
				s.sets[loaded.Name] = loaded
			}
			if len(s.sets) > 0 {
				logger.Info("credential sets restored", zap.Int("count", len(s.sets)))
			}
		}
	} else {
		logger.Warn("state persistence is disabled, so credential sets will be lost " +
			"on restart and CA accounts registered through the API will not survive")
	}
	return s
}

// Enabled reports whether mutations can be durably persisted. Runtime APIs must
// reject writes when this is false instead of acknowledging memory-only secrets.
func (s *Store) Enabled() bool {
	return s != nil && s.store != nil && s.store.Enabled()
}

func normalise(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func cloneSet(set *Set) *Set {
	if set == nil {
		return nil
	}
	out := *set
	out.Values = make(map[string]string, len(set.Values))
	for k, v := range set.Values {
		out.Values[k] = v
	}
	out.SecretKeys = append([]string(nil), set.SecretKeys...)
	return &out
}

// Put creates or updates a set and retains omitted values. It is used by
// callers such as CA account updates whose forms cannot read existing secrets.
func (s *Store) Put(set *Set, secretKeys map[string]bool) error {
	return s.put(set, secretKeys, true, nil)
}

// Replace creates or updates a set using the supplied values as the complete
// desired state. API handlers use this after explicitly merging retained
// write-only secrets and applying remove_keys.
func (s *Store) Replace(set *Set, secretKeys map[string]bool) error {
	return s.put(set, secretKeys, false, nil)
}

// ReplaceIfUnmodified applies a complete desired state only when updatedAt
// still matches the persisted set. This prevents two portal edits from silently
// overwriting one another.
func (s *Store) ReplaceIfUnmodified(set *Set, secretKeys map[string]bool, updatedAt time.Time) error {
	return s.put(set, secretKeys, false, &updatedAt)
}

// put writes atomically. A successful return means the updated snapshot has
// been durably persisted; credential changes must not rely on a debounced
// best-effort write.
func (s *Store) put(set *Set, secretKeys map[string]bool, merge bool, expectedUpdatedAt *time.Time) error {
	if set == nil || normalise(set.Name) == "" {
		return fmt.Errorf("a credential set name is required")
	}

	stored := cloneSet(set)
	stored.Name = normalise(stored.Name)
	stored.Kind = strings.ToLower(strings.TrimSpace(stored.Kind))
	stored.Service = strings.ToLower(strings.TrimSpace(stored.Service))
	if stored.Values == nil {
		stored.Values = map[string]string{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	previous, existed := s.sets[stored.Name]
	if expectedUpdatedAt != nil && (!existed || !previous.UpdatedAt.Equal(*expectedUpdatedAt)) {
		return ErrConcurrentUpdate
	}
	if existed {
		stored.CreatedAt = previous.CreatedAt
		if stored.CreatedBy == "" {
			stored.CreatedBy = previous.CreatedBy
		}
		if merge {
			// Omitted values are retained so an API or form that cannot read a
			// secret never has to resend it. Blank secret input also means
			// "keep existing".
			for k, v := range previous.Values {
				if _, provided := stored.Values[k]; !provided {
					stored.Values[k] = v
				} else if strings.TrimSpace(stored.Values[k]) == "" && secretKeys[k] {
					stored.Values[k] = v
				}
			}
		}
	} else {
		stored.CreatedAt = time.Now().UTC()
	}
	stored.UpdatedAt = time.Now().UTC()

	// Secret classification is monotonic for retained fields. Previously, an
	// update that omitted a custom secret kept its value but dropped its
	// secret_keys entry, causing the next list response to expose it.
	secretSet := map[string]bool{}
	for k, isSecret := range secretKeys {
		if isSecret {
			secretSet[k] = true
		}
	}
	if existed {
		for _, k := range previous.SecretKeys {
			if _, retained := stored.Values[k]; retained {
				secretSet[k] = true
			}
		}
	}
	stored.SecretKeys = stored.SecretKeys[:0]
	for k := range secretSet {
		stored.SecretKeys = append(stored.SecretKeys, k)
	}
	sort.Strings(stored.SecretKeys)

	s.sets[stored.Name] = stored
	if err := s.persistNowLocked(); err != nil {
		if existed {
			s.sets[stored.Name] = previous
		} else {
			delete(s.sets, stored.Name)
		}
		return fmt.Errorf("persisting credential set %q: %w", stored.Name, err)
	}

	s.logger.Info("credential set stored",
		zap.String("name", stored.Name), zap.String("kind", stored.Kind),
		zap.String("service", stored.Service), zap.Int("secret_keys", len(stored.SecretKeys)))
	return nil
}

// Reveal returns independent copies of the full values and metadata. Only code
// constructing a provider or deployer may call this; never wire it directly to
// an HTTP response.
func (s *Store) Reveal(name string) (map[string]string, *Set, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	set, ok := s.sets[normalise(name)]
	if !ok {
		return nil, nil, fmt.Errorf("no credential set named %q", name)
	}
	copy := cloneSet(set)
	return copy.Values, copy, nil
}

// Redacted returns write-only metadata with every credential value replaced,
// safe for an API response.
func (s *Store) Redacted(name string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	set, ok := s.sets[normalise(name)]
	if !ok {
		return nil, fmt.Errorf("no credential set named %q", name)
	}
	return redact(set), nil
}

func redact(set *Set) map[string]any {
	values := map[string]string{}
	keys := make([]string, 0, len(set.Values))
	for k, v := range set.Values {
		keys = append(keys, k)
		// All credential values are write-only at the API boundary, including
		// identifiers such as access-key IDs and tenant/client IDs. Callers only
		// need key presence to configure a reference.
		if strings.TrimSpace(v) == "" {
			values[k] = ""
		} else {
			values[k] = "••••••••"
		}
	}
	sort.Strings(keys)

	return map[string]any{
		"name": set.Name, "kind": set.Kind, "service": set.Service,
		"keys": keys, "values": values,
		"secret_keys": append([]string(nil), set.SecretKeys...),
		"created_by":  set.CreatedBy,
		"created_at":  set.CreatedAt, "updated_at": set.UpdatedAt,
	}
}

// List returns every set, redacted, optionally filtered by kind.
func (s *Store) List(kind string) []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.sets))
	for n := range s.sets {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		set := s.sets[n]
		if kind != "" && !strings.EqualFold(set.Kind, kind) {
			continue
		}
		out = append(out, redact(set))
	}
	return out
}

// Delete removes a set. A successful return means the removal is durable.
func (s *Store) Delete(name string) error {
	name = normalise(name)

	s.mu.Lock()
	defer s.mu.Unlock()
	previous, ok := s.sets[name]
	if !ok {
		return fmt.Errorf("no credential set named %q", name)
	}
	delete(s.sets, name)
	if err := s.persistNowLocked(); err != nil {
		s.sets[name] = previous
		return fmt.Errorf("persisting credential removal %q: %w", name, err)
	}

	s.logger.Info("credential set removed", zap.String("name", name))
	return nil
}

// Names returns the stored set names.
func (s *Store) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.sets))
	for n := range s.sets {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// snapshotLocked builds an immutable snapshot while s.mu is held.
func (s *Store) snapshotLocked() map[string]any {
	names := make([]string, 0, len(s.sets))
	for name := range s.sets {
		names = append(names, name)
	}
	sort.Strings(names)
	sets := make([]*Set, 0, len(names))
	for _, name := range names {
		sets = append(sets, cloneSet(s.sets[name]))
	}
	return map[string]any{"sets": sets}
}

func (s *Store) persistNowLocked() error {
	if s.store == nil || !s.store.Enabled() {
		return nil
	}
	return s.store.SaveNow(collection, s.snapshotLocked())
}

// FlushState writes synchronously, for shutdown.
func (s *Store) FlushState() {
	if s.store == nil || !s.store.Enabled() {
		return
	}
	s.mu.RLock()
	snapshot := s.snapshotLocked()
	s.mu.RUnlock()

	if err := s.store.SaveNow(collection, snapshot); err != nil {
		s.logger.Error("final credential save failed", zap.Error(err))
	}
}
