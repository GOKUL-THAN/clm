package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	auditservice "github.com/m2p/clm/internal/services/audit"
	"github.com/m2p/clm/pkg/models"
)

type AuditHandler struct{ service *auditservice.Service }

func NewAuditHandler(service *auditservice.Service) *AuditHandler {
	return &AuditHandler{service: service}
}
func (h *AuditHandler) List(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if raw := c.Query("tenant_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
			return
		}
		tenantID = parsed
	}
	if !middleware.RequireTenantAccess(c, tenantID) {
		return
	}
	filter := models.AuditFilter{Action: c.Query("action"), ResourceType: c.Query("resource_type"), Outcome: c.Query("outcome")}
	if raw := c.Query("actor_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid actor_id"})
			return
		}
		filter.ActorID = &id
	}
	if raw := c.Query("from"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from timestamp"})
			return
		}
		filter.From = &value
	}
	if raw := c.Query("to"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to timestamp"})
			return
		}
		filter.To = &value
	}
	filter.Limit, _ = strconv.Atoi(c.Query("limit"))
	filter.Offset, _ = strconv.Atoi(c.Query("offset"))
	events, err := h.service.List(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list audit events"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"audit_events": events, "total": len(events)})
}
