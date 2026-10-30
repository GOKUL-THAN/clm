package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/m2p/clm/internal/saas/tenant"
	"github.com/m2p/clm/internal/saas/users"
	"github.com/m2p/clm/pkg/models"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	userService   *users.Service
	tenantService *tenant.Service
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(us *users.Service, ts *tenant.Service) *AuthHandler {
	return &AuthHandler{userService: us, tenantService: ts}
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := h.userService.Login(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case users.ErrInvalidCredentials:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		case users.ErrUserDisabled:
			c.JSON(http.StatusForbidden, gin.H{"error": "account disabled"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, token)
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		Email        string `json:"email" binding:"required,email"`
		Name         string `json:"name" binding:"required"`
		Password     string `json:"password" binding:"required,min=8"`
		Organization string `json:"organization" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create tenant for this organization
	newTenant, err := h.tenantService.CreateTenant(c.Request.Context(), req.Organization, "free", "ap-south-1")
	if err != nil {
		if err == tenant.ErrTenantExists {
			c.JSON(http.StatusConflict, gin.H{"error": "an organization with this name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create organization"})
		return
	}

	// Create admin user for the tenant
	user, err := h.userService.CreateUser(c.Request.Context(), newTenant.ID, req.Email, req.Name, req.Password, models.RoleTenantAdmin)
	if err != nil {
		if err == users.ErrUserExists {
			c.JSON(http.StatusConflict, gin.H{"error": "an account with this email already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "registration failed"})
		return
	}

	// Auto-login after registration
	token, err := h.userService.Login(c.Request.Context(), &models.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		c.JSON(http.StatusCreated, gin.H{
			"message": "account created successfully",
			"user_id": user.ID,
			"email":   user.Email,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":       "account created successfully",
		"user_id":       user.ID,
		"email":         user.Email,
		"access_token":  token.AccessToken,
		"refresh_token": token.RefreshToken,
		"expires_in":    token.ExpiresIn,
		"token_type":    token.TokenType,
	})
}
