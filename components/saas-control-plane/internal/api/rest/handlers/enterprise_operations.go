package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/saas/integrations"
	"github.com/m2p/clm/internal/saas/itsm"
	"github.com/m2p/clm/internal/saas/organization"
	"github.com/m2p/clm/internal/saas/risk"
	"go.uber.org/zap"
)

// EnterpriseOperationsHandler coordinates ITSM, Risk, Integrations, and Department controllers.
type EnterpriseOperationsHandler struct {
	itsmCtrl  *itsm.ITSMController
	riskCtrl  *risk.RiskController
	integCtrl *integrations.IntegrationController
	deptCtrl  *organization.DepartmentController
	logger    *zap.Logger
}

// NewEnterpriseOperationsHandler creates a new handler.
func NewEnterpriseOperationsHandler(
	itsmCtrl *itsm.ITSMController,
	riskCtrl *risk.RiskController,
	integCtrl *integrations.IntegrationController,
	deptCtrl *organization.DepartmentController,
	logger *zap.Logger,
) *EnterpriseOperationsHandler {
	return &EnterpriseOperationsHandler{
		itsmCtrl:  itsmCtrl,
		riskCtrl:  riskCtrl,
		integCtrl: integCtrl,
		deptCtrl:  deptCtrl,
		logger:    logger,
	}
}

// --- ITSM & Incident Handlers ---

func (h *EnterpriseOperationsHandler) ListIncidents(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	incidents := h.itsmCtrl.ListIncidents(tenantID)
	c.JSON(http.StatusOK, gin.H{"incidents": incidents, "count": len(incidents)})
}

func (h *EnterpriseOperationsHandler) RaiseIncident(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var req itsm.Incident
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	inc, err := h.itsmCtrl.RaiseIncident(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"incident": inc})
}

func (h *EnterpriseOperationsHandler) ResolveIncident(c *gin.Context) {
	idStr := c.Param("id")
	incidentID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid incident ID"})
		return
	}

	var req struct {
		RootCause        string `json:"root_cause"`
		RemediationSteps string `json:"remediation_steps"`
	}
	_ = c.ShouldBindJSON(&req)

	resolved, err := h.itsmCtrl.ResolveIncident(c.Request.Context(), incidentID, req.RootCause, req.RemediationSteps)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"incident": resolved})
}

func (h *EnterpriseOperationsHandler) ListServiceRequests(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	reqs := h.itsmCtrl.ListServiceRequests(tenantID)
	c.JSON(http.StatusOK, gin.H{"requests": reqs, "count": len(reqs)})
}

func (h *EnterpriseOperationsHandler) SubmitServiceRequest(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var req itsm.ServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	created, err := h.itsmCtrl.SubmitServiceRequest(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"request": created})
}

// --- Cryptographic Risk Register Handlers ---

func (h *EnterpriseOperationsHandler) ListRisks(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	risks := h.riskCtrl.ListRisks(tenantID)
	c.JSON(http.StatusOK, gin.H{"risks": risks, "count": len(risks)})
}

func (h *EnterpriseOperationsHandler) LogRisk(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var item risk.CryptographicRiskItem
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	recorded, err := h.riskCtrl.LogRisk(tenantID, &item)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"risk": recorded})
}

func (h *EnterpriseOperationsHandler) GetRiskPosture(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	// Mock/Evaluated certs
	certs := []risk.CertInput{
		{ID: "c1", Domain: "*.m2pfintech.com", KeyAlgorithm: "RSA", KeySize: 2048, DaysLeft: 42, Issuer: "DigiCert", IsWildcard: true},
		{ID: "c2", Domain: "api-uat.m2p.in", KeyAlgorithm: "ECDSA", KeySize: 256, DaysLeft: 12, Issuer: "Let's Encrypt", IsWildcard: false},
		{ID: "c3", Domain: "internal-mgmt.local", KeyAlgorithm: "RSA", KeySize: 4096, DaysLeft: 280, Issuer: "Private CA", IsWildcard: false},
	}

	summary := h.riskCtrl.EvaluateRisk(c.Request.Context(), tenantID, certs)
	c.JSON(http.StatusOK, summary)
}

// --- Enterprise Integrations & Webhook Handlers ---

func (h *EnterpriseOperationsHandler) ListIntegrations(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	subs := h.integCtrl.ListSubscriptions(tenantID)
	c.JSON(http.StatusOK, gin.H{"integrations": subs, "count": len(subs)})
}

func (h *EnterpriseOperationsHandler) RegisterIntegration(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var req integrations.Subscription
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	sub, err := h.integCtrl.RegisterSubscription(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"integration": sub})
}

func (h *EnterpriseOperationsHandler) DeleteIntegration(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	idStr := c.Param("id")
	subID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subscription ID"})
		return
	}

	if err := h.integCtrl.DeleteSubscription(tenantID, subID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "integration deleted"})
}

// --- Departments & Cost Centers Handlers ---

func (h *EnterpriseOperationsHandler) ListDepartments(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	depts := h.deptCtrl.ListDepartments(tenantID)
	c.JSON(http.StatusOK, gin.H{"departments": depts, "count": len(depts)})
}

func (h *EnterpriseOperationsHandler) CreateDepartment(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	var req organization.Department
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	dept, err := h.deptCtrl.CreateDepartment(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"department": dept})
}
