package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/internal/saas/approval"
)

// ApprovalHandler handles approval workflow endpoints.
type ApprovalHandler struct {
	engine *approval.Engine
}

// NewApprovalHandler creates a new approval handler.
func NewApprovalHandler(engine *approval.Engine) *ApprovalHandler {
	return &ApprovalHandler{engine: engine}
}

// ListPending handles GET /api/v1/approvals/pending
func (h *ApprovalHandler) ListPending(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}

	reqs, err := h.engine.ListPending(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list approvals"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"approvals": reqs, "total": len(reqs)})
}

// GetRequest handles GET /api/v1/approvals/:id
func (h *ApprovalHandler) GetRequest(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approval ID"})
		return
	}

	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}

	req, err := h.engine.GetRequest(c.Request.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) || errors.Is(err, approval.ErrRequestNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "approval request not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get approval request"})
		return
	}
	c.JSON(http.StatusOK, req)
}

type approveRejectRequest struct {
	Comment string `json:"comment"`
}

// Approve handles POST /api/v1/approvals/:id/approve
func (h *ApprovalHandler) Approve(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approval ID"})
		return
	}
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}

	var req approveRejectRequest
	_ = c.ShouldBindJSON(&req)

	approverID := middleware.GetUserID(c)

	result, err := h.engine.Approve(c.Request.Context(), tenantID, id, approverID, req.Comment)
	if err != nil {
		switch err {
		case approval.ErrSegregationOfDuties:
			c.JSON(http.StatusForbidden, gin.H{
				"error": "segregation of duties violation: requester cannot approve their own request (ARC-004)",
			})
		case approval.ErrAlreadyResolved:
			c.JSON(http.StatusConflict, gin.H{"error": "request already resolved"})
		case approval.ErrRequestNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "decision recorded", "approval": result})
}

// Escalate handles POST /api/v1/approvals/:id/escalate
func (h *ApprovalHandler) Escalate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approval ID"})
		return
	}
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}

	var req approveRejectRequest
	_ = c.ShouldBindJSON(&req)

	escalaterID := middleware.GetUserID(c)
	result, err := h.engine.Escalate(c.Request.Context(), tenantID, id, escalaterID, req.Comment)
	if err != nil {
		switch err {
		case approval.ErrAlreadyResolved:
			c.JSON(http.StatusConflict, gin.H{"error": "request already resolved"})
		case approval.ErrRequestNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to escalate"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "escalated", "approval": result})
}

type submitApprovalRequest struct {
	RequestType   string  `json:"request_type" binding:"required"`
	Justification string  `json:"justification" binding:"required"`
	Payload       string  `json:"payload"`
	EstimatedCost float64 `json:"estimated_cost"`
	CertType      string  `json:"cert_type"`
	Environment   string  `json:"environment"`
	IsWildcard    bool    `json:"is_wildcard"`
	CAProvider    string  `json:"ca_provider"`
}

// Submit handles POST /api/v1/approvals
func (h *ApprovalHandler) Submit(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	var req submitApprovalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := middleware.GetUserID(c)
	approvalReq, err := h.engine.SubmitRequest(c.Request.Context(), tenantID, &approval.SubmitInput{
		RequestType:   req.RequestType,
		RequestedBy:   userID,
		Justification: req.Justification,
		Payload:       req.Payload,
		EstimatedCost: req.EstimatedCost,
		CertType:      req.CertType,
		Environment:   req.Environment,
		IsWildcard:    req.IsWildcard,
		CAProvider:    req.CAProvider,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to submit approval request"})
		return
	}

	c.JSON(http.StatusCreated, approvalReq)
}

// Reject handles POST /api/v1/approvals/:id/reject
func (h *ApprovalHandler) Reject(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approval ID"})
		return
	}
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}

	var req approveRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
		return
	}

	approverID := middleware.GetUserID(c)

	result, err := h.engine.Reject(c.Request.Context(), tenantID, id, approverID, req.Comment)
	if err != nil {
		switch err {
		case approval.ErrAlreadyResolved:
			c.JSON(http.StatusConflict, gin.H{"error": "request already resolved"})
		case approval.ErrRequestNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rejected", "approval": result})
}
