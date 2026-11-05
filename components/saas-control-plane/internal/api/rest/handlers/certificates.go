package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/internal/store/postgres"
	"github.com/m2p/clm/pkg/models"
)

// CertHandler handles certificate management endpoints.
type CertHandler struct {
	certRepo core.CertificateRepository
	satRepo  core.SatelliteRepository
}

// NewCertHandler creates a new certificate handler.
func NewCertHandler(certRepo core.CertificateRepository, satRepo core.SatelliteRepository) *CertHandler {
	return &CertHandler{certRepo: certRepo, satRepo: satRepo}
}

// CreateCertificate handles POST /api/v1/certificates
func (h *CertHandler) CreateCertificate(c *gin.Context) {
	var req models.CertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	req.TenantID = tenantID
	req.RequestedBy = userID
	if req.KeyAlgorithm == "" {
		req.KeyAlgorithm = models.KeyAlgoECDSAP256
	}

	satelliteID, err := uuid.Parse(req.SatelliteID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid satellite ID"})
		return
	}
	payload, err := json.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize certificate command"})
		return
	}

	now := time.Now().UTC()
	cert := &models.Certificate{
		ID:           uuid.New(),
		TenantID:     tenantID,
		Domain:       req.Domain,
		SANs:         req.SANs,
		Type:         req.Type,
		Status:       models.CertStatusPending,
		KeyAlgorithm: req.KeyAlgorithm,
		Environment:  req.Environment,
		SatelliteID:  req.SatelliteID,
		CreatedBy:    userID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.certRepo.Create(c.Request.Context(), cert); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create certificate"})
		return
	}

	cmd := &models.SatelliteCommand{
		ID:             uuid.New(),
		TenantID:       tenantID,
		SatelliteID:    satelliteID,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		Type:           "create_cert",
		Payload:        string(payload),
		Status:         models.CommandQueued,
		Priority:       1,
	}
	if cmd.IdempotencyKey == "" {
		cmd.IdempotencyKey = "create-cert:" + cert.ID.String()
	}
	if err := h.satRepo.QueueCommandForTenant(c.Request.Context(), tenantID, cmd); err != nil {
		if errors.Is(err, postgres.ErrIdempotencyConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "idempotency key is already bound to a different command"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue certificate creation"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"certificate": cert,
		"message":     "certificate creation queued on satellite",
	})
}

// ListCertificates handles GET /api/v1/certificates
func (h *CertHandler) ListCertificates(c *gin.Context) {
	tenantID, ok := tenantForRequest(c)
	if !ok {
		return
	}

	filter := &core.CertFilter{
		Environment: c.Query("environment"),
		SatelliteID: c.Query("satellite_id"),
		Page:        1,
		PageSize:    50,
	}

	certs, total, err := h.certRepo.List(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list certificates"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"certificates": certs, "total": total})
}

// GetCertificate handles GET /api/v1/certificates/:id
func (h *CertHandler) GetCertificate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid certificate ID"})
		return
	}

	tenantID := middleware.GetTenantID(c)
	cert, err := h.certRepo.GetByIDForTenant(c.Request.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "certificate not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get certificate"})
		return
	}

	c.JSON(http.StatusOK, cert)
}

// RevokeCertificate handles POST /api/v1/certificates/:id/revoke
func (h *CertHandler) RevokeCertificate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid certificate ID"})
		return
	}

	tenantID := middleware.GetTenantID(c)
	cert, err := h.certRepo.GetByIDForTenant(c.Request.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "certificate not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get certificate"})
		return
	}

	cert.Status = models.CertStatusRevoked
	cert.UpdatedAt = time.Now().UTC()
	if err := h.certRepo.Update(c.Request.Context(), cert); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "certificate revoked", "certificate": cert})
}
