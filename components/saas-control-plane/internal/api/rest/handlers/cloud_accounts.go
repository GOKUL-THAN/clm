package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/internal/saas/cloudaccounts"
	"github.com/m2p/clm/pkg/models"
)

type CloudAccountHandler struct{ service *cloudaccounts.Service }

func NewCloudAccountHandler(service *cloudaccounts.Service) *CloudAccountHandler {
	return &CloudAccountHandler{service: service}
}
func tenantForRequest(c *gin.Context) (uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(c)
	if raw := c.Query("tenant_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
			return uuid.Nil, false
		}
		tenantID = parsed
	}
	if !middleware.RequireTenantAccess(c, tenantID) {
		return uuid.Nil, false
	}
	return tenantID, true
}
func (h *CloudAccountHandler) Create(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	var request models.CloudAccountCreate
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	account, err := h.service.Create(c.Request.Context(), tenantID, request, c.GetHeader("Idempotency-Key"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, cloudaccounts.ErrInvalidAccount) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, cloudaccounts.ErrIdempotencyConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.Set("audit_tenant_id", tenantID)
	c.JSON(http.StatusCreated, account)
}
func (h *CloudAccountHandler) List(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	accounts, err := h.service.List(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list cloud accounts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cloud_accounts": accounts, "total": len(accounts)})
}
func (h *CloudAccountHandler) Get(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cloud account ID"})
		return
	}
	account, err := h.service.Get(c.Request.Context(), tenantID, id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, core.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": "cloud account not found"})
		return
	}
	c.JSON(http.StatusOK, account)
}
func (h *CloudAccountHandler) Rename(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cloud account ID"})
		return
	}
	var request models.CloudAccountUpdate
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.Rename(c.Request.Context(), tenantID, id, request.Name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "cloud account not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "cloud account renamed"})
}
func (h *CloudAccountHandler) Rotate(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cloud account ID"})
		return
	}
	var request struct {
		Credentials models.CloudCredentialPayload `json:"credentials" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.Rotate(c.Request.Context(), tenantID, id, request.Credentials); err != nil {
		status := http.StatusNotFound
		message := "cloud account not found"
		if errors.Is(err, cloudaccounts.ErrInvalidAccount) {
			status = http.StatusBadRequest
			message = err.Error()
		}
		c.JSON(status, gin.H{"error": message})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "credentials rotated"})
}
func (h *CloudAccountHandler) Delete(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cloud account ID"})
		return
	}
	if err := h.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "cloud account not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "cloud account deleted"})
}
func (h *CloudAccountHandler) Validate(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cloud account ID"})
		return
	}
	result, err := h.service.ValidateOffline(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "cloud account not found"})
		return
	}
	c.JSON(http.StatusOK, result)
}
