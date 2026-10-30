package vsatellite

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/m2p/clm/internal/satellite/state"
	"go.uber.org/zap"
)

// AgentStatus represents the current state of a connected vSatellite agent.
type AgentStatus string

const (
	AgentOnline  AgentStatus = "online"
	AgentOffline AgentStatus = "offline"
	AgentBusy    AgentStatus = "busy"
)

// Agent represents a registered vSatellite endpoint agent.
type Agent struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Host          string      `json:"host"`
	VPC           string      `json:"vpc"`
	Subnet        string      `json:"subnet"`
	Status        AgentStatus `json:"status"`
	Version       string      `json:"version"`
	CIDRs         []string    `json:"cidrs"`          // CIDRs this agent is responsible for
	CertCount     int         `json:"cert_count"`     // Certs managed by this agent
	LastHeartbeat time.Time   `json:"last_heartbeat"`
	RegisteredAt  time.Time   `json:"registered_at"`
	Capabilities  []string    `json:"capabilities"`   // deploy_linux, deploy_k8s, scan_network, etc.
}

// Command represents a task dispatched to a vSatellite agent.
type Command struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	Type        string    `json:"type"`     // scan_cidr, deploy_cert, verify_cert, health_check
	Payload     string    `json:"payload"`
	Status      string    `json:"status"`   // queued, sent, completed, failed
	Priority    int       `json:"priority"`
	Result      string    `json:"result"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// Coordinator manages vSatellite agents connected to this gateway.
type Coordinator struct {
	agents   map[string]*Agent
	commands map[string][]*Command // agentID -> pending commands
	// enrolments are single-use join tokens, keyed by token. agentSecrets holds
	// the per-agent credential issued when an enrolment is consumed, keyed by
	// agent ID. Both live in enrolment.go.
	enrolments map[string]*Enrolment
	// agentSecrets holds the SHA-256 hash of each agent's secret, never the
	// secret itself, so a snapshot on disk cannot be replayed as a credential.
	agentSecrets map[string]string
	store        *state.Store
	mu           sync.RWMutex
	logger       *zap.Logger
}

// NewCoordinator creates a new vSatellite coordinator.
func NewCoordinator(logger *zap.Logger) *Coordinator {
	return &Coordinator{
		agents:       make(map[string]*Agent),
		commands:     make(map[string][]*Command),
		enrolments:   make(map[string]*Enrolment),
		agentSecrets: make(map[string]string),
		logger:       logger,
	}
}

// RegisterAgent registers a new vSatellite agent with the gateway.
func (c *Coordinator) RegisterAgent(agent *Agent) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	agent.Status = AgentOnline
	agent.RegisteredAt = time.Now()
	agent.LastHeartbeat = time.Now()

	c.agents[agent.ID] = agent
	c.commands[agent.ID] = make([]*Command, 0)

	c.logger.Info("vSatellite agent registered",
		zap.String("agent_id", agent.ID),
		zap.String("name", agent.Name),
		zap.String("vpc", agent.VPC),
		zap.Strings("cidrs", agent.CIDRs),
	)

	return nil
}

// UnregisterAgent removes a vSatellite agent.
func (c *Coordinator) UnregisterAgent(agentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.agents, agentID)
	delete(c.commands, agentID)

	c.logger.Info("vSatellite agent unregistered", zap.String("agent_id", agentID))
}

// Heartbeat updates agent status and returns pending commands.
func (c *Coordinator) Heartbeat(agentID string, certCount int, version string) ([]*Command, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	agent, ok := c.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("agent not registered: %s", agentID)
	}

	agent.Status = AgentOnline
	agent.LastHeartbeat = time.Now()
	agent.CertCount = certCount
	agent.Version = version

	// Return and clear pending commands
	pending := c.commands[agentID]
	c.commands[agentID] = make([]*Command, 0)
	c.persistLocked()

	// Mark as sent
	for _, cmd := range pending {
		cmd.Status = "sent"
	}

	return pending, nil
}

// DispatchCommand sends a command to a specific agent.
func (c *Coordinator) DispatchCommand(agentID string, cmd *Command) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.agents[agentID]; !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	cmd.Status = "queued"
	cmd.CreatedAt = time.Now()
	c.commands[agentID] = append(c.commands[agentID], cmd)

	c.logger.Info("command dispatched to vSatellite",
		zap.String("agent_id", agentID),
		zap.String("cmd_type", cmd.Type),
	)

	return nil
}

// DispatchToAll sends a command to all online agents.
func (c *Coordinator) DispatchToAll(cmdType, payload string, priority int) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for agentID, agent := range c.agents {
		if agent.Status != AgentOnline {
			continue
		}
		cmd := &Command{
			ID:        fmt.Sprintf("cmd-%d-%s", time.Now().UnixNano(), agentID[:8]),
			AgentID:   agentID,
			Type:      cmdType,
			Payload:   payload,
			Priority:  priority,
			Status:    "queued",
			CreatedAt: time.Now(),
		}
		c.commands[agentID] = append(c.commands[agentID], cmd)
		count++
	}

	c.logger.Info("command dispatched to all agents",
		zap.String("type", cmdType),
		zap.Int("agents", count),
	)

	return count
}

// FindAgentForCIDR finds the agent responsible for a given CIDR.
func (c *Coordinator) FindAgentForCIDR(cidr string) *Agent {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, agent := range c.agents {
		for _, agentCIDR := range agent.CIDRs {
			if agentCIDR == cidr {
				return agent
			}
		}
	}
	return nil
}

// ListAgents returns all registered agents.
func (c *Coordinator) ListAgents() []*Agent {
	c.mu.RLock()
	defer c.mu.RUnlock()

	agents := make([]*Agent, 0, len(c.agents))
	for _, a := range c.agents {
		agents = append(agents, a)
	}
	return agents
}

// GetAgent returns a specific agent by ID.
func (c *Coordinator) GetAgent(id string) (*Agent, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	agent, ok := c.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	return agent, nil
}

// ReportResult records the result of a command execution.
func (c *Coordinator) ReportResult(agentID, cmdID, status, result string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("command result received",
		zap.String("agent_id", agentID),
		zap.String("cmd_id", cmdID),
		zap.String("status", status),
	)

	return nil
}

// MonitorAgents checks for offline agents (no heartbeat within timeout).
func (c *Coordinator) MonitorAgents(ctx context.Context, timeout time.Duration) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			changed := false
			for _, agent := range c.agents {
				if time.Since(agent.LastHeartbeat) > timeout {
					if agent.Status == AgentOnline {
						agent.Status = AgentOffline
						changed = true
						c.logger.Warn("vSatellite agent went offline",
							zap.String("agent_id", agent.ID),
							zap.String("name", agent.Name),
							zap.Duration("since_heartbeat", time.Since(agent.LastHeartbeat)),
						)
					}
				}
			}
			if changed {
				c.persistLocked()
			}
			c.mu.Unlock()
		}
	}
}

// Stats returns coordinator statistics.
func (c *Coordinator) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	online := 0
	totalCerts := 0
	for _, a := range c.agents {
		if a.Status == AgentOnline {
			online++
		}
		totalCerts += a.CertCount
	}

	return map[string]interface{}{
		"total_agents":  len(c.agents),
		"online_agents": online,
		"total_certs":   totalCerts,
	}
}

// MarshalJSON for command payload helper.
func CommandPayload(data interface{}) string {
	b, _ := json.Marshal(data)
	return string(b)
}
