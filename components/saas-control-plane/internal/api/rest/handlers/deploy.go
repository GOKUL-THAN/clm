package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/saas/cloudaccounts"
	"github.com/m2p/clm/internal/saas/deploy"
)

type DeployHandler struct {
	deployService *deploy.Service
	cloudAccounts *cloudaccounts.Service
}

func NewDeployHandler(ds *deploy.Service, cloudAccounts *cloudaccounts.Service) *DeployHandler {
	return &DeployHandler{deployService: ds, cloudAccounts: cloudAccounts}
}

func (h *DeployHandler) Deploy(c *gin.Context) {
	var req deploy.DeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID := middleware.GetTenantID(c)
	if req.TenantID != uuid.Nil {
		tenantID = req.TenantID
	}
	if !middleware.RequireTenantAccess(c, tenantID) {
		return
	}
	req.TenantID = tenantID
	credentials, err := h.resolveCredentials(c, tenantID, req.CloudAccountID, req.Cloud)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cloud account is unavailable or has the wrong provider"})
		return
	}
	req.Credentials = credentials
	job, err := h.deployService.Deploy(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Set("audit_tenant_id", tenantID)
	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID, "state": job.State, "message": "deployment initiated"})
}

func (h *DeployHandler) GetJobStatus(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	jobID, err := uuid.Parse(c.Param("job_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job_id"})
		return
	}
	job, err := h.deployService.GetJobForTenant(tenantID, jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *DeployHandler) GetJobLogs(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	jobID, err := uuid.Parse(c.Param("job_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job_id"})
		return
	}
	steps, err := h.deployService.GetJobLogsForTenant(tenantID, jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment job not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"steps": steps})
}

func (h *DeployHandler) CancelJob(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	jobID, err := uuid.Parse(c.Param("job_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job_id"})
		return
	}
	if err := h.deployService.CancelJobForTenant(tenantID, jobID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deployment cancelled"})
}

// ValidateCredentials is deliberately offline: it confirms the encrypted,
// tenant-owned account can be decrypted but never claims provider verification.
func (h *DeployHandler) ValidateCredentials(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	var req struct {
		CloudAccountID uuid.UUID `json:"cloud_account_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.cloudAccounts.ValidateOffline(c.Request.Context(), tenantID, req.CloudAccountID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "cloud account not found"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *DeployHandler) ListInfra(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	var req struct {
		Cloud          deploy.CloudProvider `json:"cloud" binding:"required"`
		Mode           deploy.DeployMode    `json:"mode" binding:"required"`
		Region         string               `json:"region" binding:"required"`
		CloudAccountID uuid.UUID            `json:"cloud_account_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	credentials, err := h.resolveCredentials(c, tenantID, req.CloudAccountID, req.Cloud)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "cloud account not found or provider mismatch"})
		return
	}
	infra, err := h.deployService.ListInfra(c.Request.Context(), req.Cloud, req.Mode, credentials, req.Region)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "provider infrastructure lookup failed"})
		return
	}
	c.JSON(http.StatusOK, infra)
}

func (h *DeployHandler) Teardown(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment job ID"})
		return
	}
	var req struct {
		CloudAccountID uuid.UUID `json:"cloud_account_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	job, err := h.deployService.GetJobForTenant(tenantID, jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment job not found"})
		return
	}
	if req.CloudAccountID != job.CloudAccountID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cloud_account_id does not match the deployment job"})
		return
	}
	credentials, err := h.resolveCredentials(c, tenantID, job.CloudAccountID, job.Cloud)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "cloud account not found or provider mismatch"})
		return
	}
	if err := h.deployService.TeardownForTenant(c.Request.Context(), tenantID, jobID, credentials); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment job not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "gateway teardown initiated"})
}

func (h *DeployHandler) Upgrade(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	satelliteID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid satellite ID"})
		return
	}
	if err := h.deployService.CheckSatelliteOwnership(c.Request.Context(), tenantID, satelliteID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "satellite not found"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "upgrade initiated — satellite will pull latest version on next heartbeat"})
}

func (h *DeployHandler) ListDeployJobs(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	jobs := h.deployService.ListJobs(tenantID)
	c.JSON(http.StatusOK, gin.H{"jobs": jobs})
}

func (h *DeployHandler) resolveCredentials(c *gin.Context, tenantID, accountID uuid.UUID, provider deploy.CloudProvider) (*deploy.CloudCredentials, error) {
	payload, err := h.cloudAccounts.CredentialsForProviderUse(c.Request.Context(), tenantID, accountID, string(provider))
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var credentials deploy.CloudCredentials
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return nil, err
	}
	if credentials == (deploy.CloudCredentials{}) {
		return nil, errors.New("cloud account contains no recognized credentials")
	}
	return &credentials, nil
}
