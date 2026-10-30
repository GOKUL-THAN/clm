package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AuditEvent is an immutable control-plane security record. Metadata must be a
// small allow-listed object and must never contain credentials, tokens, or raw
// request bodies.
type AuditEvent struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	TenantID     uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	RequestID    string          `json:"request_id" db:"request_id"`
	ActorID      uuid.UUID       `json:"actor_id" db:"actor_id"`
	ActorRole    string          `json:"actor_role" db:"actor_role"`
	Action       string          `json:"action" db:"action"`
	ResourceType string          `json:"resource_type" db:"resource_type"`
	ResourceID   string          `json:"resource_id,omitempty" db:"resource_id"`
	Outcome      string          `json:"outcome" db:"outcome"`
	SourceIP     string          `json:"source_ip" db:"source_ip"`
	Metadata     json.RawMessage `json:"metadata,omitempty" db:"metadata"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

type AuditFilter struct {
	Action       string
	ResourceType string
	ActorID      *uuid.UUID
	Outcome      string
	From         *time.Time
	To           *time.Time
	Limit        int
	Offset       int
}
