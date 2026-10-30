package approval

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

var (
	ErrRequestNotFound  = errors.New("approval request not found")
	ErrAlreadyResolved  = errors.New("request already resolved")
	ErrNotAuthorized    = errors.New("user not authorized to approve this request")
)

// Engine handles the multi-level approval workflow.
type Engine struct {
	repo   core.ApprovalRepository
	logger *zap.Logger
}

// NewEngine creates a new approval engine.
func NewEngine(repo core.ApprovalRepository, logger *zap.Logger) *Engine {
	return &Engine{repo: repo, logger: logger}
}

// SubmitRequest creates a new approval request, evaluating rules to determine the level.
func (e *Engine) SubmitRequest(ctx context.Context, tenantID uuid.UUID, req *SubmitInput) (*models.ApprovalRequest, error) {
	// Evaluate approval rules to determine level
	level, err := e.evaluateRules(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}

	// If auto-approved, skip the queue
	if level == models.ApprovalAuto {
		approval := &models.ApprovalRequest{
			ID:            uuid.New(),
			TenantID:      tenantID,
			RequestType:   req.RequestType,
			Level:         models.ApprovalAuto,
			Status:        models.ApprovalApproved,
			RequestedBy:   req.RequestedBy,
			Justification: req.Justification,
			Payload:       req.Payload,
			EstimatedCost: req.EstimatedCost,
			SLADeadline:   time.Now(),
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		now := time.Now()
		approval.ResolvedAt = &now
		systemID := uuid.Nil
		approval.ResolvedBy = &systemID
		approval.Comment = "Auto-approved by policy"

		if err := e.repo.Create(ctx, approval); err != nil {
			return nil, err
		}

		e.logger.Info("request auto-approved",
			zap.String("request_id", approval.ID.String()),
			zap.String("type", req.RequestType),
		)
		return approval, nil
	}

	// Calculate SLA deadline based on level
	slaHours := e.slaForLevel(level)
	slaDeadline := time.Now().Add(time.Duration(slaHours) * time.Hour)

	approval := &models.ApprovalRequest{
		ID:            uuid.New(),
		TenantID:      tenantID,
		RequestType:   req.RequestType,
		Level:         level,
		Status:        models.ApprovalPending,
		RequestedBy:   req.RequestedBy,
		Justification: req.Justification,
		Payload:       req.Payload,
		EstimatedCost: req.EstimatedCost,
		SLADeadline:   slaDeadline,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := e.repo.Create(ctx, approval); err != nil {
		return nil, err
	}

	e.logger.Info("approval request created",
		zap.String("request_id", approval.ID.String()),
		zap.String("level", string(level)),
		zap.String("type", req.RequestType),
		zap.Time("sla_deadline", slaDeadline),
	)

	return approval, nil
}

// Approve approves a pending request.
func (e *Engine) Approve(ctx context.Context, requestID uuid.UUID, approverID uuid.UUID, comment string) (*models.ApprovalRequest, error) {
	req, err := e.repo.GetByID(ctx, requestID)
	if err != nil {
		return nil, ErrRequestNotFound
	}

	if req.Status != models.ApprovalPending {
		return nil, ErrAlreadyResolved
	}

	now := time.Now()
	req.Status = models.ApprovalApproved
	req.ResolvedBy = &approverID
	req.ResolvedAt = &now
	req.Comment = comment
	req.UpdatedAt = now

	if err := e.repo.Update(ctx, req); err != nil {
		return nil, err
	}

	e.logger.Info("request approved",
		zap.String("request_id", requestID.String()),
		zap.String("approver", approverID.String()),
	)

	return req, nil
}

// Reject rejects a pending request.
func (e *Engine) Reject(ctx context.Context, requestID uuid.UUID, approverID uuid.UUID, reason string) (*models.ApprovalRequest, error) {
	req, err := e.repo.GetByID(ctx, requestID)
	if err != nil {
		return nil, ErrRequestNotFound
	}

	if req.Status != models.ApprovalPending {
		return nil, ErrAlreadyResolved
	}

	now := time.Now()
	req.Status = models.ApprovalRejected
	req.ResolvedBy = &approverID
	req.ResolvedAt = &now
	req.Comment = reason
	req.UpdatedAt = now

	if err := e.repo.Update(ctx, req); err != nil {
		return nil, err
	}

	e.logger.Info("request rejected",
		zap.String("request_id", requestID.String()),
		zap.String("approver", approverID.String()),
		zap.String("reason", reason),
	)

	return req, nil
}

// ListPending returns all pending approvals for a tenant.
func (e *Engine) ListPending(ctx context.Context, tenantID uuid.UUID) ([]*models.ApprovalRequest, error) {
	return e.repo.ListPending(ctx, tenantID)
}

// GetRequest returns a single approval request.
func (e *Engine) GetRequest(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error) {
	return e.repo.GetByID(ctx, id)
}

// CheckExpired checks for SLA-breached requests and escalates them.
func (e *Engine) CheckExpired(ctx context.Context, tenantID uuid.UUID) error {
	pending, err := e.repo.ListPending(ctx, tenantID)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, req := range pending {
		if now.After(req.SLADeadline) {
			// Escalate: bump level and extend SLA
			newLevel := e.escalateLevel(req.Level)
			newSLA := now.Add(time.Duration(e.slaForLevel(newLevel)) * time.Hour)

			req.Level = newLevel
			req.SLADeadline = newSLA
			req.UpdatedAt = now

			if err := e.repo.Update(ctx, req); err != nil {
				e.logger.Error("failed to escalate request", zap.Error(err))
				continue
			}

			e.logger.Warn("request escalated due to SLA breach",
				zap.String("request_id", req.ID.String()),
				zap.String("new_level", string(newLevel)),
			)
		}
	}
	return nil
}

// evaluateRules determines the approval level based on configured rules.
func (e *Engine) evaluateRules(ctx context.Context, tenantID uuid.UUID, req *SubmitInput) (models.ApprovalLevel, error) {
	rules, err := e.repo.GetRules(ctx, tenantID)
	if err != nil || len(rules) == 0 {
		// Default rules if none configured
		return e.defaultRuleEval(req), nil
	}

	// Evaluate rules in priority order
	for _, rule := range rules {
		if e.matchesConditions(rule, req) {
			return rule.Level, nil
		}
	}

	// Default to L1 if no rules match
	return models.ApprovalL1, nil
}

// defaultRuleEval applies built-in rules when no custom rules are configured.
func (e *Engine) defaultRuleEval(req *SubmitInput) models.ApprovalLevel {
	// Auto-approve: free certs (Let's Encrypt), renewals in non-prod
	if req.EstimatedCost == 0 && (req.RequestType == "cert_renew" || req.Environment == "dev" || req.Environment == "staging") {
		return models.ApprovalAuto
	}

	// L1: standard requests under $100
	if req.EstimatedCost < 100 {
		return models.ApprovalL1
	}

	// L2: wildcard, EV, or $100-$500
	if req.IsWildcard || req.CertType == "EV" || (req.EstimatedCost >= 100 && req.EstimatedCost < 500) {
		return models.ApprovalL2
	}

	// L3: high cost or multi-year
	return models.ApprovalL3
}

func (e *Engine) matchesConditions(rule *models.ApprovalRule, req *SubmitInput) bool {
	cond := rule.Conditions

	if len(cond.CertTypes) > 0 && !contains(cond.CertTypes, req.CertType) {
		return false
	}
	if len(cond.Environments) > 0 && !contains(cond.Environments, req.Environment) {
		return false
	}
	if cond.CostAbove != nil && req.EstimatedCost <= *cond.CostAbove {
		return false
	}
	if cond.CostBelow != nil && req.EstimatedCost >= *cond.CostBelow {
		return false
	}
	if cond.IsWildcard != nil && *cond.IsWildcard != req.IsWildcard {
		return false
	}
	if len(cond.CAProviders) > 0 && !contains(cond.CAProviders, req.CAProvider) {
		return false
	}
	return true
}

func (e *Engine) slaForLevel(level models.ApprovalLevel) int {
	switch level {
	case models.ApprovalAuto:
		return 0
	case models.ApprovalL1:
		return 4
	case models.ApprovalL2:
		return 8
	case models.ApprovalL3:
		return 24
	case models.ApprovalEmergency:
		return 1
	default:
		return 8
	}
}

func (e *Engine) escalateLevel(current models.ApprovalLevel) models.ApprovalLevel {
	switch current {
	case models.ApprovalL1:
		return models.ApprovalL2
	case models.ApprovalL2:
		return models.ApprovalL3
	default:
		return models.ApprovalL3
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// SubmitInput holds the data needed to create an approval request.
type SubmitInput struct {
	RequestType   string    `json:"request_type"`
	RequestedBy   uuid.UUID `json:"requested_by"`
	Justification string    `json:"justification"`
	Payload       string    `json:"payload"` // JSON of the original request
	EstimatedCost float64   `json:"estimated_cost"`
	CertType      string    `json:"cert_type"`
	Environment   string    `json:"environment"`
	IsWildcard    bool      `json:"is_wildcard"`
	CAProvider    string    `json:"ca_provider"`
}

// PayloadToJSON serializes the input for storage.
func PayloadToJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
