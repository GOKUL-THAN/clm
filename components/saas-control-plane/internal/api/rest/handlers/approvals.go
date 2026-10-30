package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
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
	tenantID := middleware.GetTenantID(c)

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

	req, err := h.engine.GetRequest(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval request not found"})
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

	var req approveRejectRequest
	_ = c.ShouldBindJSON(&req)

	approverID := middleware.GetUserID(c)

	result, err := h.engine.Approve(c.Request.Context(), id, approverID, req.Comment)
	if err != nil {
		switch err {
		case approval.ErrAlreadyResolved:
			c.JSON(http.StatusConflict, gin.H{"error": "request already resolved"})
		case approval.ErrRequestNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "approved", "approval": result})
}

// Reject handles POST /api/v1/approvals/:id/reject
func (h *ApprovalHandler) Reject(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approval ID"})
		return
	}

	var req approveRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
		return
	}

	approverID := middleware.GetUserID(c)

	result, err := h.engine.Reject(c.Request.Context(), id, approverID, req.Comment)
	if err != nil {
		switch err {
		case approval.ErrAlreadyResolved:
			c.JSON(http.StatusConflict, gin.H{"error": "request already resolved"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rejected", "approval": result})
}
