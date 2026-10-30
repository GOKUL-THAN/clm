package vsatellite

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Enrolment is a single-use invitation for a vSatellite agent to join this
// gateway's fleet.
//
// Registration used to be open: any client that could reach the coordinator port
// could register an arbitrary agent, and a heartbeat carrying an unknown agent ID
// auto-registered it. That let an unauthenticated caller join the fleet and,
// worse, drain another agent's queued commands by heartbeating with its ID. An
// agent now has to present an enrolment token to join, and every subsequent call
// has to present the per-agent secret issued in exchange.
//
// Neither credential is stored in the clear. The map is keyed by the SHA-256 hash
// of the token and TokenHash is what reaches disk, so a snapshot cannot be
// replayed to enrol an agent.
type Enrolment struct {
	// Token is only ever populated on the value returned by CreateEnrolment. The
	// stored record leaves it empty.
	Token        string    `json:"token,omitempty"`
	TokenHash    string    `json:"token_hash"`
	TokenPrefix  string    `json:"token_prefix"`
	Name         string    `json:"name"`
	VPC          string    `json:"vpc,omitempty"`
	Subnet       string    `json:"subnet,omitempty"`
	CIDRs        []string  `json:"cidrs,omitempty"`
	Capabilities []string  `json:"capabilities,omitempty"`
	CreatedBy    string    `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	ConsumedAt   time.Time `json:"consumed_at,omitempty"`
	ConsumedBy   string    `json:"consumed_by,omitempty"`
	Status       string    `json:"status"` // pending, consumed, expired, revoked
	Revoked      bool      `json:"revoked,omitempty"`
}

// DefaultEnrolmentTTL is how long an unused enrolment stays valid.
const DefaultEnrolmentTTL = 24 * time.Hour

// randomToken returns a cryptographically random hex string. Enrolment tokens
// and agent secrets are bearer credentials, so math/rand is not acceptable here.
func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateEnrolment issues a single-use enrolment token. The returned value is the
// only copy of the token; the stored record keeps only its hash.
func (c *Coordinator) CreateEnrolment(name, vpc, subnet string, cidrs, capabilities []string, ttl time.Duration, createdBy string) (*Enrolment, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if ttl <= 0 {
		ttl = DefaultEnrolmentTTL
	}

	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	hash := hashSecret(token)

	stored := &Enrolment{
		TokenHash:    hash,
		TokenPrefix:  token[:8],
		Name:         strings.TrimSpace(name),
		VPC:          vpc,
		Subnet:       subnet,
		CIDRs:        cidrs,
		Capabilities: capabilities,
		CreatedBy:    createdBy,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(ttl),
		Status:       "pending",
	}

	c.mu.Lock()
	if c.enrolments == nil {
		c.enrolments = make(map[string]*Enrolment)
	}
	c.enrolments[hash] = stored
	c.persistLocked()
	c.mu.Unlock()

	c.logger.Info("vSatellite enrolment created",
		zap.String("name", stored.Name),
		zap.String("token_prefix", stored.TokenPrefix),
		zap.String("created_by", createdBy),
		zap.Time("expires_at", stored.ExpiresAt),
	)

	// Return a copy carrying the plaintext token for the response only.
	out := *stored
	out.Token = token
	return &out, nil
}

// ConsumeEnrolment exchanges a valid enrolment token for a registered agent and
// its per-agent secret. The token is single-use.
//
// The returned secret is the only copy; only its hash is retained.
func (c *Coordinator) ConsumeEnrolment(token, agentID, host, version string) (*Agent, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e := c.enrolments[hashSecret(token)]
	if e == nil {
		return nil, "", fmt.Errorf("unknown enrolment token")
	}
	if e.Revoked {
		return nil, "", fmt.Errorf("enrolment token was revoked")
	}
	if !e.ConsumedAt.IsZero() {
		return nil, "", fmt.Errorf("enrolment token was already used")
	}
	if time.Now().After(e.ExpiresAt) {
		return nil, "", fmt.Errorf("enrolment token expired at %s", e.ExpiresAt.Format(time.RFC3339))
	}

	if strings.TrimSpace(agentID) == "" {
		generated, err := randomToken(8)
		if err != nil {
			return nil, "", err
		}
		agentID = "vsat-" + generated
	}
	if _, exists := c.agents[agentID]; exists {
		return nil, "", fmt.Errorf("agent id already registered: %s", agentID)
	}

	secret, err := randomToken(32)
	if err != nil {
		return nil, "", err
	}

	agent := &Agent{
		ID:            agentID,
		Name:          e.Name,
		Host:          host,
		VPC:           e.VPC,
		Subnet:        e.Subnet,
		Status:        AgentOnline,
		Version:       version,
		CIDRs:         e.CIDRs,
		Capabilities:  e.Capabilities,
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
	}
	c.agents[agentID] = agent
	c.commands[agentID] = make([]*Command, 0)

	if c.agentSecrets == nil {
		c.agentSecrets = make(map[string]string)
	}
	c.agentSecrets[agentID] = hashSecret(secret)

	e.ConsumedAt = time.Now()
	e.ConsumedBy = agentID
	e.Status = "consumed"
	c.persistLocked()

	c.logger.Info("vSatellite enrolled",
		zap.String("agent_id", agentID),
		zap.String("name", agent.Name),
		zap.String("host", host),
		zap.String("token_prefix", e.TokenPrefix),
	)

	return agent, secret, nil
}

// EnrolmentUsable reports whether a token is still valid and unused, without
// consuming it. The installer download is gated on this so an invalid token
// cannot even retrieve the script.
func (c *Coordinator) EnrolmentUsable(token string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e := c.enrolments[hashSecret(token)]
	return e != nil && !e.Revoked && e.ConsumedAt.IsZero() && time.Now().Before(e.ExpiresAt)
}

// AuthenticateAgent reports whether the secret belongs to the agent. Unknown
// agents and wrong secrets are indistinguishable to the caller by design.
func (c *Coordinator) AuthenticateAgent(agentID, secret string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return verifySecret(c.agentSecrets[agentID], secret)
}

// RevokeEnrolment cancels an unused enrolment, matched by full token or by the
// prefix shown in the portal.
func (c *Coordinator) RevokeEnrolment(tokenOrPrefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	candidates := []*Enrolment{}
	if e, ok := c.enrolments[hashSecret(tokenOrPrefix)]; ok {
		candidates = append(candidates, e)
	} else {
		for _, e := range c.enrolments {
			if e.TokenPrefix == tokenOrPrefix {
				candidates = append(candidates, e)
			}
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("enrolment not found")
	}

	e := candidates[0]
	if !e.ConsumedAt.IsZero() {
		return fmt.Errorf("enrolment was already used by %s", e.ConsumedBy)
	}
	e.Revoked = true
	e.Status = "revoked"
	c.persistLocked()

	c.logger.Info("vSatellite enrolment revoked", zap.String("token_prefix", e.TokenPrefix))
	return nil
}

// ListEnrolments returns enrolments newest first. Neither the token nor its hash
// is included, so nothing reusable leaves the coordinator after creation.
func (c *Coordinator) ListEnrolments() []*Enrolment {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]*Enrolment, 0, len(c.enrolments))
	for _, e := range c.enrolments {
		copied := *e
		copied.Token = ""
		copied.TokenHash = ""
		if copied.Status == "pending" && time.Now().After(copied.ExpiresAt) {
			copied.Status = "expired"
		}
		out = append(out, &copied)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// Decommission removes an agent and its credential, and drops any queued
// commands so they cannot be collected later.
func (c *Coordinator) Decommission(agentID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.agents[agentID]; !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}
	delete(c.agents, agentID)
	delete(c.commands, agentID)
	delete(c.agentSecrets, agentID)
	c.persistLocked()

	c.logger.Info("vSatellite decommissioned", zap.String("agent_id", agentID))
	return nil
}

// PendingCommands reports how many commands are waiting for an agent, so the
// portal can show queue depth without draining the queue.
func (c *Coordinator) PendingCommands(agentID string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.commands[agentID])
}
