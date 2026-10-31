package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/m2p/clm/internal/saas/onboarding"
)

// OnboardingHandler handles corporate customer onboarding endpoints.
type OnboardingHandler struct {
	service *onboarding.Service
}

// NewOnboardingHandler creates a new onboarding handler.
func NewOnboardingHandler(service *onboarding.Service) *OnboardingHandler {
	return &OnboardingHandler{service: service}
}

// Preflight handles POST /api/v1/onboarding/preflight
func (h *OnboardingHandler) Preflight(c *gin.Context) {
	var input onboarding.OnboardingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	report, err := h.service.Preflight(c.Request.Context(), &input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "preflight verification failed: " + err.Error()})
		return
	}

	status := http.StatusOK
	if !report.Valid {
		status = http.StatusUnprocessableEntity
	}

	c.JSON(status, report)
}

// Bootstrap handles POST /api/v1/onboarding/bootstrap
func (h *OnboardingHandler) Bootstrap(c *gin.Context) {
	var input onboarding.OnboardingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	result, err := h.service.Bootstrap(c.Request.Context(), &input)
	if err != nil {
		switch err {
		case onboarding.ErrSlugTaken:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case onboarding.ErrInvalidSlug,
			onboarding.ErrInvalidAWSAccountID,
			onboarding.ErrInvalidVPCID,
			onboarding.ErrInvalidRoleARN,
			onboarding.ErrInvalidExternalID,
			onboarding.ErrInvalidEmail:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "customer onboarding bootstrap failed: " + err.Error()})
		}
		return
	}

	c.Set("audit_tenant_id", result.TenantID)
	c.JSON(http.StatusCreated, result)
}
