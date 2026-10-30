package api

import (
	"fmt"

	"github.com/m2p/clm/internal/satellite/state"
	"go.uber.org/zap"
)

// Persistence for the certificate and approval inventory.
//
// The inventory was in-memory only, so a restart lost every approval decision
// and the whole certificate history. Discovery could rebuild certificates, but
// only for hosts still reachable and only after someone ran a scan — and an
// approval, once lost, is unrecoverable.

// inventorySnapshot is the on-disk shape. It is a struct rather than the bare
// maps so fields can be added later without invalidating existing snapshots.
type inventorySnapshot struct {
	Certificates []*Certificate `json:"certificates"`
	Approvals    []*Approval    `json:"approvals"`
}

const inventoryCollection = "inventory"

// AttachStore gives the inventory durable storage and loads any existing
// snapshot. Safe to call with a disabled store.
func (i *Inventory) AttachStore(st *state.Store, logger *zap.Logger) error {
	i.mu.Lock()
	i.store = st
	i.logger = logger
	i.mu.Unlock()

	if !st.Enabled() {
		return nil
	}

	var snap inventorySnapshot
	if err := st.LoadStrict(inventoryCollection, &snap); err != nil {
		return fmt.Errorf("loading inventory snapshot: %w", err)
	}

	i.mu.Lock()
	recoveryMarked := false
	for _, c := range snap.Certificates {
		if c != nil && c.ID != "" {
			// A process restart during issuance/deployment makes external state
			// indeterminate. Never silently retry side effects or activate the
			// candidate; preserve the old active version and require reconciliation.
			if c.Renewal != nil && (c.Renewal.Status == "issuing" || c.Renewal.Status == "deploying" || c.Renewal.Status == "recovering") {
				c.Renewal.Status = "recovery_required"
				c.Renewal.Error = "gateway restarted before renewal rollout completed"
				c.LastError = c.Renewal.Error
				recoveryMarked = true
				for _, version := range c.Versions {
					if version != nil && version.ID == c.Renewal.CandidateVersionID {
						version.Status = "recovery_required"
					}
				}
			}
			for _, deployment := range c.Deployments {
				if deployment != nil && deployment.Status == "deploying" {
					deployment.Status = "failed"
					deployment.Stage = "recovery_required"
					deployment.Error = "gateway restarted with deployment outcome indeterminate"
					recoveryMarked = true
				}
			}
			i.certs[c.ID] = c
		}
	}
	for _, a := range snap.Approvals {
		if a != nil && a.ID != "" {
			i.appros[a.ID] = a
		}
	}
	certs, appros := len(i.certs), len(i.appros)
	i.mu.Unlock()

	if recoveryMarked {
		if err := i.PersistNow(); err != nil {
			return fmt.Errorf("persisting restart recovery markers: %w", err)
		}
	}
	if certs > 0 || appros > 0 {
		logger.Info("inventory restored from disk",
			zap.Int("certificates", certs), zap.Int("approvals", appros))
	}
	return nil
}

// persist schedules a debounced snapshot write. The caller must not hold the
// lock, because the snapshot function takes it.
func (i *Inventory) persist() {
	if i.store == nil || !i.store.Enabled() {
		return
	}
	i.store.Save(inventoryCollection, func() any { return i.snapshot() })
}

// snapshot copies the current contents for writing.
func (i *Inventory) snapshot() inventorySnapshot {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.snapshotLocked()
}

// snapshotLocked builds a snapshot while the caller holds i.mu for reading or
// writing. Renewal checkpoints use this form so the state written to disk is
// exactly the state protected by the same critical section.
func (i *Inventory) snapshotLocked() inventorySnapshot {
	snap := inventorySnapshot{
		Certificates: make([]*Certificate, 0, len(i.certs)),
		Approvals:    make([]*Approval, 0, len(i.appros)),
	}
	for _, c := range i.certs {
		snap.Certificates = append(snap.Certificates, cloneCertificateCheckpoint(c))
	}
	for _, a := range i.appros {
		if a == nil {
			continue
		}
		copy := *a
		snap.Approvals = append(snap.Approvals, &copy)
	}
	return snap
}

// persistLocked writes a synchronous checkpoint while the caller holds i.mu.
// Holding the lock across JSON encoding and atomic rename is intentional for
// renewal transitions: no concurrent mutation may slip between the in-memory
// transition and its durable representation.
func (i *Inventory) persistLocked() error {
	if i.store == nil || !i.store.Enabled() {
		return fmt.Errorf("inventory persistence is disabled")
	}
	return i.store.SaveNow(inventoryCollection, i.snapshotLocked())
}

// PersistNow writes the current inventory snapshot synchronously and reports
// failures to callers that must not proceed without a durable checkpoint.
func (i *Inventory) PersistNow() error {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.persistLocked()
}

// FlushState writes the inventory synchronously, for shutdown.
func (i *Inventory) FlushState() {
	if i.store == nil || !i.store.Enabled() {
		return
	}
	if err := i.store.SaveNow(inventoryCollection, i.snapshot()); err != nil && i.logger != nil {
		i.logger.Error("final inventory save failed", zap.Error(err))
	}
}
