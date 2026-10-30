package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/saas/users"
	"github.com/m2p/clm/pkg/models"
)

// UserHandler handles user management endpoints.
type UserHandler struct {
	userService *users.Service
}

// NewUserHandler creates a new user handler.
func NewUserHandler(us *users.Service) *UserHandler {
	return &UserHandler{userService: us}
}

type createUserRequest struct {
	Email    string          `json:"email" binding:"required,email"`
	Name     string          `json:"name" binding:"required"`
	Password string          `json:"password" binding:"required,min=8"`
	Role     models.UserRole `json:"role" binding:"required"`
}

// CreateUser handles POST /api/v1/users
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := users.CanAssignRole(contextRole(c), req.Role); err != nil {
		status := http.StatusForbidden
		if errors.Is(err, users.ErrInvalidRole) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}

	user, err := h.userService.CreateUser(c.Request.Context(), tenantID, req.Email, req.Name, req.Password, req.Role)
	if err != nil {
		if err == users.ErrUserExists {
			c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, user)
}

// ListUsers handles GET /api/v1/users
func (h *UserHandler) ListUsers(c *gin.Context) {
	var usersList []*models.User
	var err error
	if c.Query("all_tenants") == "true" {
		if !middleware.IsPlatformAdmin(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "platform admin role required"})
			return
		}
		usersList, err = h.userService.ListUsersAllTenants(c.Request.Context())
	} else {
		tenantID, ok := tenantForRequest(c)
		if !ok {
			return
		}
		usersList, err = h.userService.ListUsers(c.Request.Context(), tenantID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": usersList, "total": len(usersList)})
}

// GetUser handles GET /api/v1/users/:id
func (h *UserHandler) GetUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var user *models.User
	if middleware.IsPlatformAdmin(c) && c.Query("tenant_id") == "" {
		user, err = h.userService.GetUserAllTenants(c.Request.Context(), id)
	} else {
		tenantID, ok := tenantForRequest(c)
		if !ok {
			return
		}
		user, err = h.userService.GetUser(c.Request.Context(), tenantID, id)
	}
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

type updateRoleRequest struct {
	Role models.UserRole `json:"role" binding:"required"`
}

// UpdateRole handles PATCH /api/v1/users/:id/role
func (h *UserHandler) UpdateRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req updateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := users.CanAssignRole(contextRole(c), req.Role); err != nil {
		status := http.StatusForbidden
		if errors.Is(err, users.ErrInvalidRole) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	if err := h.userService.UpdateRole(c.Request.Context(), tenantID, id, req.Role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "role updated"})
}

// DisableUser handles DELETE /api/v1/users/:id
func (h *UserHandler) DisableUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	if err := h.userService.DisableUser(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to disable user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user disabled"})
}

func contextRole(c *gin.Context) string {
	value, _ := c.Get("role")
	role, _ := value.(string)
	return role
}
