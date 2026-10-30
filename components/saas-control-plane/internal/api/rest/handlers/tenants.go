package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/saas/tenant"
)

// TenantHandler handles tenant management endpoints.
type TenantHandler struct {
	tenantService *tenant.Service
}

// NewTenantHandler creates a new tenant handler.
func NewTenantHandler(ts *tenant.Service) *TenantHandler {
	return &TenantHandler{tenantService: ts}
}

type createTenantRequest struct {
	Name   string `json:"name" binding:"required"`
	Plan   string `json:"plan" binding:"required"`
	Region string `json:"region" binding:"required"`
}

// CreateTenant handles POST /api/v1/tenants
func (h *TenantHandler) CreateTenant(c *gin.Context) {
	var req createTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	t, err := h.tenantService.CreateTenant(c.Request.Context(), req.Name, req.Plan, req.Region)
	if err != nil {
		if err == tenant.ErrTenantExists {
			c.JSON(http.StatusConflict, gin.H{"error": "tenant already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tenant"})
		return
	}

	c.Set("audit_tenant_id", t.ID)
	c.JSON(http.StatusCreated, t)
}

// GetTenant handles GET /api/v1/tenants/:id
func (h *TenantHandler) GetTenant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		// Try "me" shortcut — returns current tenant
		if idStr == "me" {
			id = middleware.GetTenantID(c)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant ID"})
			return
		}
	}

	if !middleware.RequireTenantAccess(c, id) {
		return
	}
	t, err := h.tenantService.GetTenant(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}

	c.JSON(http.StatusOK, t)
}

// ListTenants handles GET /api/v1/tenants (super admin only)
func (h *TenantHandler) ListTenants(c *gin.Context) {
	tenants, err := h.tenantService.ListTenants(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tenants"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tenants": tenants, "total": len(tenants)})
}

// UpdateTenant handles PATCH /api/v1/tenants/:id
func (h *TenantHandler) UpdateTenant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant ID"})
		return
	}

	if !middleware.RequireTenantAccess(c, id) {
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	t, err := h.tenantService.UpdateTenant(c.Request.Context(), id, updates)
	if err != nil {
		if errors.Is(err, tenant.ErrTenantNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, t)
}

// DeleteTenant handles DELETE /api/v1/tenants/:id
//
// By default the tenant must be empty. Pass ?force=true to remove its users,
// gateways, certificates and audit history along with it.
func (h *TenantHandler) DeleteTenant(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant ID"})
		return
	}

	if !middleware.RequireTenantAccess(c, id) {
		return
	}
	// Deleting your own tenant would remove your own account mid-request and
	// lock you out, so it is refused outright.
	if callerTenant := middleware.GetTenantID(c); callerTenant == id {
		c.JSON(http.StatusConflict, gin.H{
			"error": "you cannot delete the tenant your own account belongs to",
		})
		return
	}

	force := c.Query("force") == "true"

	err = h.tenantService.DeleteTenant(c.Request.Context(), id, force)
	if err != nil {
		var deps *tenant.DependentsError
		if errors.As(err, &deps) {
			c.JSON(http.StatusConflict, gin.H{
				"error":      deps.Error(),
				"dependents": deps.Counts,
				"hint": "remove these records first, deactivate the tenant instead, " +
					"or repeat the request with ?force=true to delete them too",
			})
			return
		}
		if errors.Is(err, tenant.ErrTenantNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tenant deleted", "id": id, "forced": force})
}
