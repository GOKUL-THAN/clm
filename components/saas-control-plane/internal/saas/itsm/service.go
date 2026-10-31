// Package itsm provides enterprise IT Service Management, incident triage, and service catalog workflows.
//
// Benchmarked against ServiceNow ITSM and Jira Service Management standards, this controller
// manages SEV-1 to SEV-4 security incidents caused by certificate expirations, chain anomalies,
// or revoked root CAs, calculates blast-radius impact, coordinates emergency remediation,
// and provides a self-service catalog for enterprise certificate requests.
package itsm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	ErrIncidentNotFound    = errors.New("incident not found")
	ErrRequestNotFound     = errors.New("service request not found")
	ErrInvalidSeverity     = errors.New("invalid incident severity level (must be SEV1, SEV2, SEV3, or SEV4)")
	ErrAlreadyResolved     = errors.New("incident is already resolved")
)

type IncidentSeverity string

const (
	SeveritySEV1 IncidentSeverity = "SEV1" // Critical Outage (production domain TLS failure)
	SeveritySEV2 IncidentSeverity = "SEV2" // High Risk (expiry within 72h on critical asset)
	SeveritySEV3 IncidentSeverity = "SEV3" // Medium Risk (expiry within 15 days or weak cipher)
	SeveritySEV4 IncidentSeverity = "SEV4" // Low Risk (minor policy violation or staging cert)
)

type IncidentStatus string

const (
	StatusOpen         IncidentStatus = "open"
	StatusInvestigating IncidentStatus = "investigating"
	StatusMitigating   IncidentStatus = "mitigating"
	StatusResolved     IncidentStatus = "resolved"
	StatusClosed       IncidentStatus = "closed"
)

// Incident represents an operational or security incident involving TLS infrastructure.
type Incident struct {
	ID               uuid.UUID        `json:"id"`
	TenantID         uuid.UUID        `json:"tenant_id"`
	Severity         IncidentSeverity `json:"severity"`
	Title            string           `json:"title"`
	Status           IncidentStatus   `json:"status"`
	AffectedDomain   string           `json:"affected_domain"`
	AffectedCertID   string           `json:"affected_cert_id,omitempty"`
	BlastRadiusScore int              `json:"blast_radius_score"` // 1 - 100 based on traffic & criticality
	RootCause        string           `json:"root_cause,omitempty"`
	RemediationSteps string           `json:"remediation_steps,omitempty"`
	ITSMExternalRef  string           `json:"itsm_external_ref,omitempty"` // ServiceNow / Jira Key (e.g. INC009124)
	AssignedTo       string           `json:"assigned_to,omitempty"`
	OpenedAt         time.Time        `json:"opened_at"`
	ResolvedAt       *time.Time       `json:"resolved_at,omitempty"`
}

// ServiceRequest represents a formal catalog request from a development team for new TLS assets.
type ServiceRequest struct {
	ID             uuid.UUID `json:"id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	RequesterEmail string    `json:"requester_email"`
	Department     string    `json:"department"`
	DomainName     string    `json:"domain_name"`
	SANs           []string  `json:"sans,omitempty"`
	Environment    string    `json:"environment"` // prod, staging, dev
	CertType       string    `json:"cert_type"`   // dv, ov, ev, private
	ValidityMonths int       `json:"validity_months"`
	Status         string    `json:"status"` // submitted, pending_approval, in_fulfillment, completed, rejected
	ITSMTicketRef  string    `json:"itsm_ticket_ref,omitempty"` // e.g. REQ004512
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ITSMController orchestrates incident triage and service requests.
type ITSMController struct {
	mu        sync.RWMutex
	incidents map[uuid.UUID]*Incident
	requests  map[uuid.UUID]*ServiceRequest
	logger    *zap.Logger
}

// NewITSMController creates a new enterprise ITSM controller.
func NewITSMController(logger *zap.Logger) *ITSMController {
	return &ITSMController{
		incidents: make(map[uuid.UUID]*Incident),
		requests:  make(map[uuid.UUID]*ServiceRequest),
		logger:    logger,
	}
}

// RaiseIncident records a new TLS incident and computes its blast radius.
func (c *ITSMController) RaiseIncident(ctx context.Context, tenantID uuid.UUID, inc *Incident) (*Incident, error) {
	if inc.Severity != SeveritySEV1 && inc.Severity != SeveritySEV2 && inc.Severity != SeveritySEV3 && inc.Severity != SeveritySEV4 {
		return nil, ErrInvalidSeverity
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	inc.ID = uuid.New()
	inc.TenantID = tenantID
	inc.Status = StatusOpen
	inc.OpenedAt = now

	// Calculate default blast radius score if not set
	if inc.BlastRadiusScore <= 0 {
		switch inc.Severity {
		case SeveritySEV1:
			inc.BlastRadiusScore = 95
		case SeveritySEV2:
			inc.BlastRadiusScore = 70
		case SeveritySEV3:
			inc.BlastRadiusScore = 40
		case SeveritySEV4:
			inc.BlastRadiusScore = 15
		}
	}

	if inc.ITSMExternalRef == "" {
		inc.ITSMExternalRef = fmt.Sprintf("INC-%d", now.Unix()%1000000)
	}

	c.incidents[inc.ID] = inc

	c.logger.Warn("ITSM incident raised",
		zap.String("incident_id", inc.ID.String()),
		zap.String("severity", string(inc.Severity)),
		zap.String("domain", inc.AffectedDomain),
		zap.Int("blast_radius", inc.BlastRadiusScore),
	)

	return inc, nil
}

// ResolveIncident marks an incident as resolved with root cause and remediation evidence.
func (c *ITSMController) ResolveIncident(ctx context.Context, incidentID uuid.UUID, rootCause, remediation string) (*Incident, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	inc, ok := c.incidents[incidentID]
	if !ok {
		return nil, ErrIncidentNotFound
	}

	if inc.Status == StatusResolved || inc.Status == StatusClosed {
		return nil, ErrAlreadyResolved
	}

	now := time.Now()
	inc.Status = StatusResolved
	inc.ResolvedAt = &now
	inc.RootCause = rootCause
	inc.RemediationSteps = remediation

	c.logger.Info("ITSM incident resolved",
		zap.String("incident_id", incidentID.String()),
		zap.String("domain", inc.AffectedDomain),
	)

	return inc, nil
}

// ListIncidents returns all active or historical incidents for a tenant.
func (c *ITSMController) ListIncidents(tenantID uuid.UUID) []*Incident {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var list []*Incident
	for _, inc := range c.incidents {
		if inc.TenantID == tenantID {
			list = append(list, inc)
		}
	}
	return list
}

// SubmitServiceRequest logs a new self-service certificate request.
func (c *ITSMController) SubmitServiceRequest(ctx context.Context, tenantID uuid.UUID, req *ServiceRequest) (*ServiceRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	req.ID = uuid.New()
	req.TenantID = tenantID
	req.Status = "submitted"
	req.CreatedAt = now
	req.UpdatedAt = now

	if req.ITSMTicketRef == "" {
		req.ITSMTicketRef = fmt.Sprintf("REQ-%d", now.Unix()%1000000)
	}

	c.requests[req.ID] = req

	c.logger.Info("ITSM service request submitted",
		zap.String("request_id", req.ID.String()),
		zap.String("domain", req.DomainName),
		zap.String("requester", req.RequesterEmail),
	)

	return req, nil
}

// ListServiceRequests returns catalog requests for a tenant.
func (c *ITSMController) ListServiceRequests(tenantID uuid.UUID) []*ServiceRequest {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var list []*ServiceRequest
	for _, r := range c.requests {
		if r.TenantID == tenantID {
			list = append(list, r)
		}
	}
	return list
}
