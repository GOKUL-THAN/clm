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
	ID                   uuid.UUID       `json:"id" db:"id"`
	TenantID             uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Name                 string          `json:"name" db:"name"`
	Type                 SatelliteType   `json:"type" db:"type"`
	Status               SatelliteStatus `json:"status" db:"status"`
	ConnectorMode        ConnectorMode   `json:"connector_mode" db:"connector_mode"`
	Version              string          `json:"version" db:"version"`
	Fingerprint          string          `json:"fingerprint" db:"fingerprint"`
	LastHeartbeat        *time.Time      `json:"last_heartbeat,omitempty" db:"last_heartbeat"`
	IPAddress            string          `json:"ip_address,omitempty" db:"ip_address"`
	Region               string          `json:"region,omitempty" db:"region"`
	Cloud                string          `json:"cloud,omitempty" db:"cloud"`
	SecretsBackend       string          `json:"secrets_backend" db:"secrets_backend"`
	AgentCount           int             `json:"agent_count" db:"agent_count"`
	CertCount            int             `json:"cert_count" db:"cert_count"`
	CredentialGeneration int             `json:"credential_generation" db:"credential_generation"`
	CredentialCreatedAt  *time.Time      `json:"credential_created_at,omitempty" db:"credential_created_at"`
	CredentialLastUsedAt *time.Time      `json:"credential_last_used_at,omitempty" db:"credential_last_used_at"`
	CredentialRevokedAt  *time.Time      `json:"credential_revoked_at,omitempty" db:"credential_revoked_at"`
	CreatedAt            time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at" db:"updated_at"`
	CredentialHash       string          `json:"-" db:"credential_hash"`
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
	ID                  uuid.UUID     `json:"id" db:"id"`
	TenantID            uuid.UUID     `json:"tenant_id" db:"tenant_id"`
	SatelliteID         uuid.UUID     `json:"satellite_id" db:"satellite_id"`
	IdempotencyKey      string        `json:"idempotency_key" db:"idempotency_key"`
	Type                string        `json:"type" db:"type"`
	Payload             string        `json:"payload" db:"payload"`
	Status              CommandStatus `json:"status" db:"status"`
	Priority            int           `json:"priority" db:"priority"`
	Result              string        `json:"result,omitempty" db:"result"`
	Attempts            int           `json:"attempts" db:"attempts"`
	ExecutionGeneration int           `json:"execution_generation" db:"execution_generation"`
	MaxAttempts         int           `json:"max_attempts" db:"max_attempts"`
	AvailableAt         time.Time     `json:"available_at" db:"available_at"`
	LeaseOwner          string        `json:"lease_owner,omitempty" db:"lease_owner"`
	LeaseToken          *uuid.UUID    `json:"lease_token,omitempty" db:"lease_token"`
	LeaseUntil          *time.Time    `json:"lease_until,omitempty" db:"lease_until"`
	AcknowledgedAt      *time.Time    `json:"acknowledged_at,omitempty" db:"acknowledged_at"`
	LastError           string        `json:"last_error,omitempty" db:"last_error"`
	TerminalAt          *time.Time    `json:"terminal_at,omitempty" db:"terminal_at"`
	CreatedAt           time.Time     `json:"created_at" db:"created_at"`
	SentAt              *time.Time    `json:"sent_at,omitempty" db:"sent_at"`
	CompletedAt         *time.Time    `json:"completed_at,omitempty" db:"completed_at"`
}

func (c *SatelliteCommand) SetDefaults() {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.IdempotencyKey == "" {
		c.IdempotencyKey = c.ID.String()
	}
	if c.Status == "" {
		c.Status = CommandQueued
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if c.AvailableAt.IsZero() {
		c.AvailableAt = c.CreatedAt
	}
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
