package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/saas/sso"
	"go.uber.org/zap"
)

// SSOHandler handles all SSO authentication endpoints.
type SSOHandler struct {
	ssoService  *sso.Service
	groupMapper *sso.GroupMapper
}

// NewSSOHandler creates a new SSO handler.
func NewSSOHandler(ssoService *sso.Service) *SSOHandler {
	return &SSOHandler{
		ssoService:  ssoService,
		groupMapper: sso.NewGroupMapper(zap.NewNop()),
	}
}

func (h *SSOHandler) ListGroupMappings(c *gin.Context) {
	tenantID, ok := h.mappingTenantID(c)
	if !ok {
		return
	}
	mappings := h.groupMapper.ListMappings(tenantID)
	c.JSON(http.StatusOK, gin.H{"mappings": mappings, "count": len(mappings)})
}

func (h *SSOHandler) CreateGroupMapping(c *gin.Context) {
	tenantID, ok := h.mappingTenantID(c)
	if !ok {
		return
	}

	var mapping sso.GroupRoleMapping
	if err := c.ShouldBindJSON(&mapping); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}
	created, err := h.groupMapper.AddMapping(c.Request.Context(), tenantID, &mapping)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"mapping": created})
}

func (h *SSOHandler) DeleteGroupMapping(c *gin.Context) {
	tenantID, ok := h.mappingTenantID(c)
	if !ok {
		return
	}
	mappingID, err := uuid.Parse(c.Param("mapping_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mapping ID"})
		return
	}
	if err := h.groupMapper.DeleteMapping(tenantID, mappingID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "group mapping deleted"})
}

// ListProviders returns available SSO methods for the login page.
// GET /api/v1/auth/sso/providers?tenant_id=xxx
func (h *SSOHandler) ListProviders(c *gin.Context) {
	tenantIDStr := c.Query("tenant_id")
	if tenantIDStr == "" {
		// Try to resolve from domain query param
		c.JSON(http.StatusOK, gin.H{"providers": []sso.ProviderInfo{}})
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	providers := h.ssoService.ListProviders(tenantID)
	c.JSON(http.StatusOK, gin.H{"providers": providers})
}

// --- SAML Endpoints ---

// SAMLLogin initiates SAML authentication by redirecting to the IdP.
// GET /api/v1/auth/saml/login?tenant_id=xxx
func (h *SSOHandler) SAMLLogin(c *gin.Context) {
	tenantID, err := h.parseTenantID(c)
	if err != nil {
		return
	}

	provider, err := h.ssoService.GetProvider(tenantID, sso.ProviderSAML)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SAML not configured for this tenant"})
		return
	}

	state := generateState(tenantID)
	authURL, err := provider.GetAuthURL(c.Request.Context(), state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate SAML request"})
		return
	}

	c.Redirect(http.StatusFound, authURL.URL)
}

// SAMLCallback processes the SAML Response from the IdP.
// POST /api/v1/auth/saml/callback
func (h *SSOHandler) SAMLCallback(c *gin.Context) {
	relayState := c.PostForm("RelayState")
	tenantID, err := parseTenantFromState(relayState)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid relay state"})
		return
	}

	provider, err := h.ssoService.GetProvider(tenantID, sso.ProviderSAML)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SAML not configured"})
		return
	}

	params := map[string]string{
		"SAMLResponse": c.PostForm("SAMLResponse"),
		"RelayState":   relayState,
	}

	ssoUser, err := provider.HandleCallback(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	token, err := h.ssoService.ProcessSSOLogin(c.Request.Context(), tenantID, ssoUser)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, token)
}

// --- OIDC Endpoints ---

// OIDCLogin initiates generic OIDC authorization code flow.
// GET /api/v1/auth/oidc/login?tenant_id=xxx
func (h *SSOHandler) OIDCLogin(c *gin.Context) {
	tenantID, err := h.parseTenantID(c)
	if err != nil {
		return
	}

	provider, err := h.ssoService.GetProvider(tenantID, sso.ProviderOIDC)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OIDC not configured for this tenant"})
		return
	}

	state := generateState(tenantID)
	authURL, err := provider.GetAuthURL(c.Request.Context(), state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build OIDC auth URL"})
		return
	}

	c.Redirect(http.StatusFound, authURL.URL)
}

// OIDCCallback handles the OIDC authorization code callback.
// GET /api/v1/auth/oidc/callback?code=xxx&state=xxx
func (h *SSOHandler) OIDCCallback(c *gin.Context) {
	state := c.Query("state")
	tenantID, err := parseTenantFromState(state)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state parameter"})
		return
	}

	provider, err := h.ssoService.GetProvider(tenantID, sso.ProviderOIDC)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OIDC not configured"})
		return
	}

	params := map[string]string{
		"code":              c.Query("code"),
		"state":             state,
		"error":             c.Query("error"),
		"error_description": c.Query("error_description"),
	}

	ssoUser, err := provider.HandleCallback(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	token, err := h.ssoService.ProcessSSOLogin(c.Request.Context(), tenantID, ssoUser)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, token)
}

// --- Google Endpoints ---

// GoogleLogin initiates Google OAuth2 login.
// GET /api/v1/auth/google/login?tenant_id=xxx
func (h *SSOHandler) GoogleLogin(c *gin.Context) {
	tenantID, err := h.parseTenantID(c)
	if err != nil {
		return
	}

	provider, err := h.ssoService.GetProvider(tenantID, sso.ProviderGoogle)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Google SSO not configured for this tenant"})
		return
	}

	state := generateState(tenantID)
	authURL, err := provider.GetAuthURL(c.Request.Context(), state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build Google auth URL"})
		return
	}

	c.Redirect(http.StatusFound, authURL.URL)
}

// GoogleCallback handles the Google OAuth2 callback.
// GET /api/v1/auth/google/callback?code=xxx&state=xxx
func (h *SSOHandler) GoogleCallback(c *gin.Context) {
	state := c.Query("state")
	tenantID, err := parseTenantFromState(state)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state parameter"})
		return
	}

	provider, err := h.ssoService.GetProvider(tenantID, sso.ProviderGoogle)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Google SSO not configured"})
		return
	}

	params := map[string]string{
		"code":  c.Query("code"),
		"state": state,
		"error": c.Query("error"),
	}

	ssoUser, err := provider.HandleCallback(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	token, err := h.ssoService.ProcessSSOLogin(c.Request.Context(), tenantID, ssoUser)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, token)
}

// --- Microsoft Endpoints ---

// MicrosoftLogin initiates Microsoft OAuth2 login.
// GET /api/v1/auth/microsoft/login?tenant_id=xxx
func (h *SSOHandler) MicrosoftLogin(c *gin.Context) {
	tenantID, err := h.parseTenantID(c)
	if err != nil {
		return
	}

	provider, err := h.ssoService.GetProvider(tenantID, sso.ProviderMicrosoft)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Microsoft SSO not configured for this tenant"})
		return
	}

	state := generateState(tenantID)
	authURL, err := provider.GetAuthURL(c.Request.Context(), state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build Microsoft auth URL"})
		return
	}

	c.Redirect(http.StatusFound, authURL.URL)
}

// MicrosoftCallback handles the Microsoft OAuth2 callback.
// GET /api/v1/auth/microsoft/callback?code=xxx&state=xxx
func (h *SSOHandler) MicrosoftCallback(c *gin.Context) {
	state := c.Query("state")
	tenantID, err := parseTenantFromState(state)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state parameter"})
		return
	}

	provider, err := h.ssoService.GetProvider(tenantID, sso.ProviderMicrosoft)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Microsoft SSO not configured"})
		return
	}

	params := map[string]string{
		"code":              c.Query("code"),
		"state":             state,
		"error":             c.Query("error"),
		"error_description": c.Query("error_description"),
	}

	ssoUser, err := provider.HandleCallback(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	token, err := h.ssoService.ProcessSSOLogin(c.Request.Context(), tenantID, ssoUser)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, token)
}

// --- SSO Config Management ---

// ConfigureSSO sets up SSO for a tenant.
// POST /api/v1/tenants/:id/sso
func (h *SSOHandler) ConfigureSSO(c *gin.Context) {
	tenantIDStr := c.Param("id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant ID"})
		return
	}

	if !middleware.RequireTenantAccess(c, tenantID) {
		return
	}
	var cfg sso.SSOConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.ssoService.RegisterTenantSSO(tenantID, &cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "SSO configured successfully"})
}

// GetSSO returns SSO configuration for a tenant.
// GET /api/v1/tenants/:id/sso
func (h *SSOHandler) GetSSO(c *gin.Context) {
	tenantIDStr := c.Param("id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant ID"})
		return
	}
	if !middleware.RequireTenantAccess(c, tenantID) {
		return
	}

	cfg, err := h.ssoService.GetTenantSSO(tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SSO not configured for this tenant"})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// DeleteSSO disables SSO for a tenant.
// DELETE /api/v1/tenants/:id/sso
func (h *SSOHandler) DeleteSSO(c *gin.Context) {
	tenantIDStr := c.Param("id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant ID"})
		return
	}
	if !middleware.RequireTenantAccess(c, tenantID) {
		return
	}

	h.ssoService.RemoveTenantSSO(tenantID)
	c.JSON(http.StatusOK, gin.H{"message": "SSO disabled"})
}

// --- Helpers ---

func (h *SSOHandler) mappingTenantID(c *gin.Context) (uuid.UUID, bool) {
	tenantID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant ID"})
		return uuid.Nil, false
	}
	if !middleware.RequireTenantAccess(c, tenantID) {
		return uuid.Nil, false
	}
	return tenantID, true
}

func (h *SSOHandler) parseTenantID(c *gin.Context) (uuid.UUID, error) {
	tenantIDStr := c.Query("tenant_id")
	if tenantIDStr == "" {
		// Check if this is a social signup request (no tenant yet)
		mode := c.Query("mode")
		if mode == "signup" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "social sign-up not yet configured",
				"message": "OAuth sign-up requires platform-level OAuth credentials to be configured by the administrator. Please use email registration for now.",
			})
			return uuid.Nil, sso.ErrProviderNotConfigured
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id query parameter required"})
		return uuid.Nil, sso.ErrProviderNotConfigured
	}
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return uuid.Nil, err
	}
	return tenantID, nil
}

// generateState creates an opaque state parameter encoding tenant ID + random nonce.
func generateState(tenantID uuid.UUID) string {
	nonce := make([]byte, 8)
	rand.Read(nonce)
	return tenantID.String() + ":" + hex.EncodeToString(nonce)
}

// parseTenantFromState extracts tenant ID from the state parameter.
func parseTenantFromState(state string) (uuid.UUID, error) {
	// State format: "<tenant-uuid>:<random-hex>"
	if len(state) < 36 {
		return uuid.Nil, sso.ErrInvalidToken
	}
	return uuid.Parse(state[:36])
}
