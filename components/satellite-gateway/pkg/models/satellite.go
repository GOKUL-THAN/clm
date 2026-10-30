package models

import (
	"time"

	"github.com/google/uuid"
)

type SatelliteType string

const (
	SatelliteLocal   SatelliteType = "local"
	SatelliteVirtual SatelliteType = "vsatellite"
)

type SatelliteStatus string

const (
	SatelliteOnline      SatelliteStatus = "online"
	SatelliteOffline     SatelliteStatus = "offline"
	SatelliteRegistering SatelliteStatus = "registering"
	SatelliteDisabled    SatelliteStatus = "disabled"
)

type ConnectorMode string

const (
	ConnectorDirect      ConnectorMode = "direct"
	ConnectorProxy       ConnectorMode = "proxy"
	ConnectorPrivateLink ConnectorMode = "privatelink"
	ConnectorVPN         ConnectorMode = "vpn"
	ConnectorVSAT        ConnectorMode = "vsat"
	ConnectorOffline     ConnectorMode = "offline"
)

type Satellite struct {
	ID            uuid.UUID       `json:"id"`
	TenantID      uuid.UUID       `json:"tenant_id"`
	Name          string          `json:"name"`
	Type          SatelliteType   `json:"type"`
	Status        SatelliteStatus `json:"status"`
	ConnectorMode ConnectorMode   `json:"connector_mode"`
	Version       string          `json:"version"`
	LastHeartbeat *time.Time      `json:"last_heartbeat,omitempty"`
}

type CommandStatus string

const (
	CommandQueued     CommandStatus = "queued"
	CommandLeased     CommandStatus = "leased"
	CommandRunning    CommandStatus = "running"
	CommandSucceeded  CommandStatus = "succeeded"
	CommandFailed     CommandStatus = "failed"
	CommandDeadLetter CommandStatus = "dead_letter"
	CommandCancelled  CommandStatus = "cancelled"
)

type SatelliteCommand struct {
	ID                  uuid.UUID     `json:"id"`
	TenantID            uuid.UUID     `json:"tenant_id"`
	SatelliteID         uuid.UUID     `json:"satellite_id"`
	IdempotencyKey      string        `json:"idempotency_key"`
	Type                string        `json:"type"`
	Payload             string        `json:"payload"`
	Status              CommandStatus `json:"status"`
	Priority            int           `json:"priority"`
	Result              string        `json:"result,omitempty"`
	Attempts            int           `json:"attempts"`
	ExecutionGeneration int           `json:"execution_generation"`
	MaxAttempts         int           `json:"max_attempts"`
	AvailableAt         time.Time     `json:"available_at"`
	LeaseOwner          string        `json:"lease_owner,omitempty"`
	LeaseToken          *uuid.UUID    `json:"lease_token,omitempty"`
	LeaseUntil          *time.Time    `json:"lease_until,omitempty"`
	AcknowledgedAt      *time.Time    `json:"acknowledged_at,omitempty"`
	LastError           string        `json:"last_error,omitempty"`
	TerminalAt          *time.Time    `json:"terminal_at,omitempty"`
	CreatedAt           time.Time     `json:"created_at"`
}

type SatelliteIdentity struct {
	SatelliteID uuid.UUID
	TenantID    uuid.UUID
}

type SatelliteHeartbeat struct {
	SatelliteID   uuid.UUID `json:"satellite_id"`
	Status        string    `json:"status"`
	Version       string    `json:"version"`
	AgentCount    int       `json:"agent_count"`
	CertCount     int       `json:"cert_count"`
	QueueDepth    int       `json:"queue_depth"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryPercent float64   `json:"memory_percent"`
	DiskPercent   float64   `json:"disk_percent"`
	Timestamp     time.Time `json:"timestamp"`
}
