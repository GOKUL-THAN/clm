package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/internal/store/postgres"
	"github.com/m2p/clm/pkg/config"
	"github.com/m2p/clm/pkg/models"
	"github.com/m2p/clm/pkg/satelliteauth"
)

type SatelliteHandler struct {
	satRepo core.SatelliteRepository
	config  *config.ServerConfig
}

func NewSatelliteHandler(satRepo core.SatelliteRepository, cfg *config.ServerConfig) *SatelliteHandler {
	return &SatelliteHandler{satRepo: satRepo, config: cfg}
}

type registerSatelliteRequest struct {
	Name           string               `json:"name" binding:"required"`
	Type           models.SatelliteType `json:"type" binding:"required"`
	ConnectorMode  models.ConnectorMode `json:"connector_mode" binding:"required"`
	Cloud          string               `json:"cloud"`
	Region         string               `json:"region"`
	SecretsBackend string               `json:"secrets_backend" binding:"required"`
}

func (h *SatelliteHandler) RegisterSatellite(c *gin.Context) {
	var req registerSatelliteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}
	plaintext, encoded, err := satelliteauth.Generate()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to provision satellite identity"})
		return
	}
	now := time.Now().UTC()
	sat := &models.Satellite{ID: uuid.New(), TenantID: tenantID, Name: req.Name, Type: req.Type, Status: models.SatelliteRegistering,
		ConnectorMode: req.ConnectorMode, Cloud: req.Cloud, Region: req.Region, SecretsBackend: req.SecretsBackend,
		CredentialHash: encoded, CredentialGeneration: 1, CredentialCreatedAt: &now, CreatedAt: now, UpdatedAt: now}
	if err := h.satRepo.Create(c.Request.Context(), sat); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register satellite"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"satellite": sat, "credential": plaintext, "message": "credential is shown once; store it in the satellite secret store"})
}

func (h *SatelliteHandler) ListSatellites(c *gin.Context) {
	var sats []*models.Satellite
	var err error
	if c.Query("all_tenants") == "true" {
		if !middleware.IsPlatformAdmin(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "platform admin role required"})
			return
		}
		sats, err = h.satRepo.ListAll(c.Request.Context())
	} else {
		tenantID, ok := tenantForRequest(c)
		if !ok {
			return
		}
		sats, err = h.satRepo.ListByTenant(c.Request.Context(), tenantID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list satellites"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"satellites": sats, "total": len(sats)})
}
func (h *SatelliteHandler) GetSatellite(c *gin.Context) {
	tenantID, id, ok := h.tenantSatellite(c)
	if !ok {
		return
	}
	sat, err := h.satRepo.GetByIDForTenant(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "satellite not found"})
		return
	}
	c.JSON(http.StatusOK, sat)
}
func (h *SatelliteHandler) UpdateSatellite(c *gin.Context) {
	tenantID, id, ok := h.tenantSatellite(c)
	if !ok {
		return
	}
	sat, err := h.satRepo.GetByIDForTenant(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "satellite not found"})
		return
	}
	var req struct {
		Name   string `json:"name"`
		Cloud  string `json:"cloud"`
		Region string `json:"region"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name != "" {
		sat.Name = req.Name
	}
	if req.Cloud != "" {
		sat.Cloud = req.Cloud
	}
	if req.Region != "" {
		sat.Region = req.Region
	}
	sat.UpdatedAt = time.Now().UTC()
	if err := h.satRepo.UpdateForTenant(c.Request.Context(), tenantID, sat); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update satellite"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "gateway updated", "satellite": sat})
}
func (h *SatelliteHandler) DeleteSatellite(c *gin.Context) {
	tenantID, id, ok := h.tenantSatellite(c)
	if !ok {
		return
	}
	if err := h.satRepo.DeleteForTenant(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "satellite not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "gateway deleted"})
}

func (h *SatelliteHandler) RotateCredential(c *gin.Context) {
	tenantID, id, ok := h.tenantSatellite(c)
	if !ok {
		return
	}
	if _, err := h.satRepo.GetByIDForTenant(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "satellite not found"})
		return
	}
	plaintext, encoded, err := satelliteauth.Generate()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate credential"})
		return
	}
	if err := h.satRepo.SetCredentialForTenant(c.Request.Context(), tenantID, id, encoded); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate credential"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"credential": plaintext, "message": "credential rotated and shown once"})
}
func (h *SatelliteHandler) RevokeCredential(c *gin.Context) {
	tenantID, id, ok := h.tenantSatellite(c)
	if !ok {
		return
	}
	if err := h.satRepo.RevokeCredentialForTenant(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "satellite not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "satellite credential revoked"})
}

func (h *SatelliteHandler) Heartbeat(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid satellite ID"})
		return
	}
	var hb models.SatelliteHeartbeat
	if err := c.ShouldBindJSON(&hb); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if hb.SatelliteID != uuid.Nil && hb.SatelliteID != id {
		c.JSON(http.StatusForbidden, gin.H{"error": "heartbeat satellite identity mismatch"})
		return
	}
	sat, err := h.satRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "satellite not found"})
		return
	}
	now := time.Now().UTC()
	sat.Status = models.SatelliteOnline
	sat.LastHeartbeat = &now
	sat.AgentCount = hb.AgentCount
	sat.CertCount = hb.CertCount
	sat.Version = hb.Version
	sat.UpdatedAt = now
	if err := h.satRepo.Update(c.Request.Context(), sat); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update heartbeat"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
func (h *SatelliteHandler) PullCommands(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid satellite ID"})
		return
	}
	c.Set("satellite_id", id)
	c.Set("command_action", "claim")
	owner := strings.TrimSpace(c.Query("lease_owner"))
	if owner == "" {
		owner = id.String()
	}
	cmds, err := h.satRepo.ClaimCommands(c.Request.Context(), id, owner, h.config.Gateway.MaxPullCommands, h.config.Gateway.LeaseDuration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to claim commands"})
		return
	}
	c.Set("command_count", len(cmds))
	c.JSON(http.StatusOK, gin.H{"commands": cmds})
}
func (h *SatelliteHandler) AcknowledgeCommand(c *gin.Context) { h.leaseAction(c, false) }
func (h *SatelliteHandler) RenewCommandLease(c *gin.Context)  { h.leaseAction(c, true) }
func (h *SatelliteHandler) leaseAction(c *gin.Context, renew bool) {
	sid, cid, token, ok := parseLeaseRequest(c)
	if !ok {
		return
	}
	c.Set("satellite_id", sid)
	c.Set("command_id", cid)
	var err error
	if renew {
		c.Set("command_action", "renew_lease")
		err = h.satRepo.RenewCommandLease(c.Request.Context(), sid, cid, token, h.config.Gateway.LeaseDuration)
	} else {
		c.Set("command_action", "acknowledge")
		err = h.satRepo.AcknowledgeCommand(c.Request.Context(), sid, cid, token, h.config.Gateway.LeaseDuration)
	}
	if errors.Is(err, postgres.ErrInvalidLease) {
		c.JSON(http.StatusConflict, gin.H{"error": "invalid or expired command lease"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update command lease"})
		return
	}
	c.Set("command_status", models.CommandRunning)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
func parseLeaseRequest(c *gin.Context) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	sid, e1 := uuid.Parse(c.Param("id"))
	cid, e2 := uuid.Parse(c.Param("cmd_id"))
	var req struct {
		LeaseToken string `json:"lease_token" binding:"required"`
	}
	e3 := c.ShouldBindJSON(&req)
	token, e4 := uuid.Parse(req.LeaseToken)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid satellite, command, and lease token are required"})
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return sid, cid, token, true
}

func (h *SatelliteHandler) ReportCommandResult(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.config.Gateway.MaxResultBytes)
	sid, e1 := uuid.Parse(c.Param("id"))
	cid, e2 := uuid.Parse(c.Param("cmd_id"))
	var req struct {
		LeaseToken string `json:"lease_token" binding:"required"`
		Status     string `json:"status" binding:"required"`
		Result     string `json:"result"`
		Error      string `json:"error"`
		Retryable  *bool  `json:"retryable"`
	}
	e3 := c.ShouldBindJSON(&req)
	token, e4 := uuid.Parse(req.LeaseToken)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		if errors.Is(e3, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "result body is required"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command result"})
		}
		return
	}
	c.Set("satellite_id", sid)
	c.Set("command_id", cid)
	c.Set("command_action", "record_result")
	status := models.CommandStatus(req.Status)
	if status == "completed" {
		status = models.CommandSucceeded
	}
	if status != models.CommandSucceeded && status != models.CommandFailed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be succeeded or failed"})
		return
	}
	retryable := status == models.CommandFailed
	if req.Retryable != nil {
		retryable = *req.Retryable
	}
	cmd, err := h.satRepo.CompleteCommand(c.Request.Context(), sid, cid, token, status, req.Result, req.Error, retryable, h.config.Gateway.RetryBase)
	if errors.Is(err, postgres.ErrInvalidLease) || errors.Is(err, postgres.ErrInvalidTransition) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, core.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "command not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record command result"})
		return
	}
	c.Set("command_status", cmd.Status)
	c.JSON(http.StatusOK, gin.H{"message": "result recorded", "command": cmd})
}

func (h *SatelliteHandler) CancelCommand(c *gin.Context) {
	tenantID, satelliteID, ok := h.tenantSatellite(c)
	if !ok {
		return
	}
	cmdID, err := uuid.Parse(c.Param("cmd_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command ID"})
		return
	}
	c.Set("tenant_id", tenantID)
	c.Set("satellite_id", satelliteID)
	c.Set("command_id", cmdID)
	c.Set("command_action", "cancel")
	if err := h.satRepo.CancelCommandForTenant(c.Request.Context(), tenantID, satelliteID, cmdID); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "command cannot be cancelled"})
		return
	}
	c.Set("command_status", models.CommandCancelled)
	c.JSON(http.StatusOK, gin.H{"message": "command cancelled"})
}
func (h *SatelliteHandler) RetryCommand(c *gin.Context) {
	tenantID, satelliteID, ok := h.tenantSatellite(c)
	if !ok {
		return
	}
	cmdID, err := uuid.Parse(c.Param("cmd_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command ID"})
		return
	}
	c.Set("tenant_id", tenantID)
	c.Set("satellite_id", satelliteID)
	c.Set("command_id", cmdID)
	c.Set("command_action", "retry")
	if err := h.satRepo.RetryCommandForTenant(c.Request.Context(), tenantID, satelliteID, cmdID); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "command cannot be retried"})
		return
	}
	c.Set("command_status", models.CommandQueued)
	c.JSON(http.StatusOK, gin.H{"message": "command queued for retry"})
}
func (h *SatelliteHandler) tenantSatellite(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid satellite ID"})
		return uuid.Nil, uuid.Nil, false
	}
	tenantID, ok := tenantForRequest(c)
	return tenantID, id, ok
}
