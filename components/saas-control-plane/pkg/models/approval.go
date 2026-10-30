package models

import (
	"time"

	"github.com/google/uuid"
)

// ApprovalStatus represents the state of an approval request.
type ApprovalStatus string

const (
	ApprovalPending   ApprovalStatus = "pending"
	ApprovalApproved  ApprovalStatus = "approved"
	ApprovalRejected  ApprovalStatus = "rejected"
	ApprovalExpired   ApprovalStatus = "expired"
	ApprovalCancelled ApprovalStatus = "cancelled"
)

// ApprovalLevel defines the severity/tier of approval.
type ApprovalLevel string

const (
	ApprovalAuto ApprovalLevel = "auto"
	ApprovalL1   ApprovalLevel = "L1"
	ApprovalL2   ApprovalLevel = "L2"
	ApprovalL3   ApprovalLevel = "L3"
	ApprovalEmergency ApprovalLevel = "emergency"
)

// ApprovalRequest is a request that needs human approval before execution.
type ApprovalRequest struct {
	ID            uuid.UUID      `json:"id" db:"id"`
	TenantID      uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	RequestType   string         `json:"request_type" db:"request_type"` // cert_create, cert_purchase, cert_deploy
	Level         ApprovalLevel  `json:"level" db:"level"`
	Status        ApprovalStatus `json:"status" db:"status"`
	RequestedBy   uuid.UUID      `json:"requested_by" db:"requested_by"`
	Justification string         `json:"justification" db:"justification"`
	Payload       string         `json:"payload" db:"payload"` // JSON of the original request
	EstimatedCost float64        `json:"estimated_cost" db:"estimated_cost"`
	SLADeadline   time.Time      `json:"sla_deadline" db:"sla_deadline"`
	ResolvedBy    *uuid.UUID     `json:"resolved_by,omitempty" db:"resolved_by"`
	ResolvedAt    *time.Time     `json:"resolved_at,omitempty" db:"resolved_at"`
	Comment       string         `json:"comment,omitempty" db:"comment"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at" db:"updated_at"`
}

// ApprovalRule defines when and what level of approval is needed.
type ApprovalRule struct {
	ID          uuid.UUID     `json:"id" db:"id"`
	TenantID    uuid.UUID     `json:"tenant_id" db:"tenant_id"`
	Name        string        `json:"name" db:"name"`
	Level       ApprovalLevel `json:"level" db:"level"`
	Priority    int           `json:"priority" db:"priority"`
	Conditions  RuleConditions `json:"conditions" db:"conditions"`
	ApproverRoles []UserRole  `json:"approver_roles" db:"approver_roles"`
	MinApprovals int          `json:"min_approvals" db:"min_approvals"`
	SLAHours    int           `json:"sla_hours" db:"sla_hours"`
	EscalateTo  ApprovalLevel `json:"escalate_to" db:"escalate_to"`
	Active      bool          `json:"active" db:"active"`
}

// RuleConditions determines when a rule matches.
type RuleConditions struct {
	CertTypes     []string `json:"cert_types,omitempty"`
	Environments  []string `json:"environments,omitempty"`
	CostAbove     *float64 `json:"cost_above,omitempty"`
	CostBelow     *float64 `json:"cost_below,omitempty"`
	IsNewDomain   *bool    `json:"is_new_domain,omitempty"`
	IsWildcard    *bool    `json:"is_wildcard,omitempty"`
	CAProviders   []string `json:"ca_providers,omitempty"`
}

// ApprovalAction represents an approve/reject action by a user.
type ApprovalAction struct {
	RequestID uuid.UUID `json:"request_id" binding:"required"`
	Action    string    `json:"action" binding:"required"` // approve, reject
	Comment   string    `json:"comment"`
}
