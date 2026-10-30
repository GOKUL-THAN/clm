package vsatellite

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

	"github.com/m2p/clm/internal/satellite/state"
	"go.uber.org/zap"
)

// Persistence for the vSatellite fleet.
//
// Agents and their credentials were held only in memory, so a gateway restart
// emptied the fleet and every agent had to be re-enrolled by hand — an operator
// visit to each host, for a restart.
//
// Agent secrets are stored as SHA-256 hashes, never in the clear. The secret is a
// high-entropy random value rather than a user password, so a single unsalted
// SHA-256 is appropriate here: there is no dictionary to attack and no need for a
// slow KDF. The plaintext exists only in the enrolment response.

// fleetSnapshot is the on-disk shape. Only credential hashes are written.
type fleetSnapshot struct {
	Agents       []*Agent          `json:"agents"`
	Enrolments   []*Enrolment      `json:"enrolments"`
	SecretHashes map[string]string `json:"secret_hashes"`
}

const fleetCollection = "fleet"

// hashSecret returns the stored form of an agent secret.
func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// AttachStore gives the coordinator durable storage and restores any fleet.
func (c *Coordinator) AttachStore(st *state.Store) {
	c.mu.Lock()
	c.store = st
	c.mu.Unlock()

	if !st.Enabled() {
		return
	}

	var snap fleetSnapshot
	if err := st.Load(fleetCollection, &snap); err != nil {
		c.logger.Error("loading fleet snapshot failed", zap.Error(err))
		return
	}

	c.mu.Lock()
	for _, a := range snap.Agents {
		if a == nil || a.ID == "" {
			continue
		}
		// A restored agent has not reported in since the restart, so it is
		// offline until it heartbeats. Claiming it is online would misreport the
		// fleet for up to the monitor interval.
		a.Status = AgentOffline
		c.agents[a.ID] = a
		if _, ok := c.commands[a.ID]; !ok {
			c.commands[a.ID] = make([]*Command, 0)
		}
	}
	for _, e := range snap.Enrolments {
		if e != nil && e.TokenHash != "" {
			c.enrolments[e.TokenHash] = e
		}
	}
	for id, h := range snap.SecretHashes {
		c.agentSecrets[id] = h
	}
	agents, enrolments := len(c.agents), len(c.enrolments)
	c.mu.Unlock()

	if agents > 0 || enrolments > 0 {
		c.logger.Info("fleet restored from disk",
			zap.Int("agents", agents), zap.Int("enrolments", enrolments),
			zap.String("note", "agents are marked offline until they heartbeat"))
	}
}

// persistLocked schedules a snapshot write. The caller must hold the lock.
func (c *Coordinator) persistLocked() {
	if c.store == nil || !c.store.Enabled() {
		return
	}
	c.store.Save(fleetCollection, func() any { return c.snapshot() })
}

// snapshot copies the fleet for writing.
func (c *Coordinator) snapshot() fleetSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	snap := fleetSnapshot{
		Agents:       make([]*Agent, 0, len(c.agents)),
		Enrolments:   make([]*Enrolment, 0, len(c.enrolments)),
		SecretHashes: make(map[string]string, len(c.agentSecrets)),
	}
	for _, a := range c.agents {
		snap.Agents = append(snap.Agents, a)
	}
	for _, e := range c.enrolments {
		snap.Enrolments = append(snap.Enrolments, e)
	}
	for id, h := range c.agentSecrets {
		snap.SecretHashes[id] = h
	}
	return snap
}

// FlushState writes the fleet synchronously, for shutdown.
func (c *Coordinator) FlushState() {
	if c.store == nil || !c.store.Enabled() {
		return
	}
	if err := c.store.SaveNow(fleetCollection, c.snapshot()); err != nil {
		c.logger.Error("final fleet save failed", zap.Error(err))
	}
}

// verifySecret compares a presented secret against the stored hash in constant
// time.
func verifySecret(storedHash, presented string) bool {
	if storedHash == "" || presented == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(storedHash), []byte(hashSecret(presented))) == 1
}
