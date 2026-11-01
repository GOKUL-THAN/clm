package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/saas/aicontract"
	"github.com/m2p/clm/internal/saas/contract"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// ContractsHandler exposes contract lifecycle management HTTP endpoints
type ContractsHandler struct {
	contractSvc contract.Service
	aiSvc       aicontract.Service
	logger      *zap.Logger
}

func NewContractsHandler(contractSvc contract.Service, aiSvc aicontract.Service, logger *zap.Logger) *ContractsHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ContractsHandler{
		contractSvc: contractSvc,
		aiSvc:       aiSvc,
		logger:      logger,
	}
}

// GET /api/v1/contracts
func (h *ContractsHandler) ListContracts(c *gin.Context) {
	tenantID := h.getTenantID(c)
	filter := contract.Filter{
		Type:         c.Query("type"),
		Status:       c.Query("status"),
		Stage:        c.Query("stage"),
		Search:       c.Query("search"),
		Counterparty: c.Query("counterparty"),
	}

	contracts, total, err := h.contractSvc.ListContracts(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"contracts": contracts,
		"total":     total,
	})
}

// GET /api/v1/contracts/:id
func (h *ContractsHandler) GetContract(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	cnt, err := h.contractSvc.GetContract(c.Request.Context(), tenantID, contractID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"contract": cnt})
}

// POST /api/v1/contracts
func (h *ContractsHandler) CreateContract(c *gin.Context) {
	tenantID := h.getTenantID(c)
	var req models.Contract
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cnt, err := h.contractSvc.CreateContract(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"contract": cnt})
}

// PUT /api/v1/contracts/:id
func (h *ContractsHandler) UpdateContract(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	var req models.Contract
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.ID = contractID

	cnt, err := h.contractSvc.UpdateContract(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"contract": cnt})
}

// POST /api/v1/contracts/:id/transition
func (h *ContractsHandler) TransitionContract(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	var req struct {
		TargetStatus models.ContractStatus `json:"target_status"`
		TargetStage  models.ContractStage  `json:"target_stage"`
		Comment      string                `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cnt, err := h.contractSvc.TransitionContract(c.Request.Context(), tenantID, contractID, req.TargetStatus, req.TargetStage, req.Comment)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"contract": cnt, "message": "transition successful"})
}

// GET /api/v1/contracts/:id/versions
func (h *ContractsHandler) ListVersions(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	versions, err := h.contractSvc.ListVersions(c.Request.Context(), tenantID, contractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// POST /api/v1/contracts/:id/versions
func (h *ContractsHandler) CreateVersion(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	var req struct {
		Title       string `json:"title"`
		Content     string `json:"content_body"`
		Changelog   string `json:"changelog_summary"`
		AuthorEmail string `json:"author_email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	v, err := h.contractSvc.CreateVersion(c.Request.Context(), tenantID, contractID, req.Title, req.Content, req.Changelog, req.AuthorEmail)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"version": v})
}

// GET /api/v1/contracts/:id/negotiations
func (h *ContractsHandler) ListNegotiations(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	redlines, err := h.contractSvc.ListRedlines(c.Request.Context(), tenantID, contractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"redlines": redlines})
}

// POST /api/v1/contracts/:id/negotiations
func (h *ContractsHandler) SubmitNegotiation(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	var req models.ContractRedline
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	redline, err := h.contractSvc.SubmitRedline(c.Request.Context(), tenantID, contractID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"redline": redline})
}

// GET /api/v1/contracts/:id/signatures
func (h *ContractsHandler) ListSignatures(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	sigs, err := h.contractSvc.ListSignatures(c.Request.Context(), tenantID, contractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"signatures": sigs})
}

// POST /api/v1/contracts/:id/signatures
func (h *ContractsHandler) AddSigner(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	var req models.ContractSignature
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sig, err := h.contractSvc.AddSigner(c.Request.Context(), tenantID, contractID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"signature": sig})
}

// POST /api/v1/contracts/signatures/:sig_id/execute
func (h *ContractsHandler) ExecuteSignature(c *gin.Context) {
	tenantID := h.getTenantID(c)
	sigID, err := uuid.Parse(c.Param("sig_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signature id"})
		return
	}

	var req struct {
		AuthMethod string `json:"auth_method"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.AuthMethod == "" {
		req.AuthMethod = "email_otp"
	}

	sig, err := h.contractSvc.ExecuteSignature(c.Request.Context(), tenantID, sigID, req.AuthMethod, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"signature": sig, "message": "signed successfully"})
}

// GET /api/v1/contracts/obligations
func (h *ContractsHandler) ListObligations(c *gin.Context) {
	tenantID := h.getTenantID(c)
	var contractID *uuid.UUID
	if cidStr := c.Query("contract_id"); cidStr != "" {
		if parsed, err := uuid.Parse(cidStr); err == nil {
			contractID = &parsed
		}
	}

	obls, err := h.contractSvc.ListObligations(c.Request.Context(), tenantID, contractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"obligations": obls})
}

// POST /api/v1/contracts/:id/obligations
func (h *ContractsHandler) AddObligation(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	var req models.ContractObligation
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	obl, err := h.contractSvc.AddObligation(c.Request.Context(), tenantID, contractID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"obligation": obl})
}

// POST /api/v1/contracts/obligations/:obl_id/fulfill
func (h *ContractsHandler) FulfillObligation(c *gin.Context) {
	tenantID := h.getTenantID(c)
	oblID, err := uuid.Parse(c.Param("obl_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid obligation id"})
		return
	}

	var req struct {
		VerifiedBy    string `json:"verified_by"`
		EvidenceNotes string `json:"evidence_notes"`
	}
	_ = c.ShouldBindJSON(&req)

	obl, err := h.contractSvc.FulfillObligation(c.Request.Context(), tenantID, oblID, req.VerifiedBy, req.EvidenceNotes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"obligation": obl, "message": "obligation fulfilled"})
}

// POST /api/v1/contracts/:id/ai-analyze
func (h *ContractsHandler) AnalyzeContractAI(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	cnt, err := h.contractSvc.GetContract(c.Request.Context(), tenantID, contractID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	analysis, err := h.aiSvc.AnalyzeContract(c.Request.Context(), tenantID, contractID, cnt.ContentBody)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"analysis": analysis})
}

// GET /api/v1/contracts/:id/ai-analysis
func (h *ContractsHandler) GetAIAnalysis(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	analysis, err := h.aiSvc.GetAnalysis(c.Request.Context(), tenantID, contractID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"analysis": analysis})
}

// POST /api/v1/contracts/:id/ask
func (h *ContractsHandler) AskAI(c *gin.Context) {
	tenantID := h.getTenantID(c)
	contractID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid contract id"})
		return
	}

	var req struct {
		Question string `json:"question"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ans, err := h.aiSvc.AskAssistant(c.Request.Context(), tenantID, contractID, req.Question)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ans)
}

// GET /api/v1/contracts/analytics
func (h *ContractsHandler) GetAnalytics(c *gin.Context) {
	tenantID := h.getTenantID(c)
	summary, err := h.contractSvc.GetAnalytics(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// Intake & Requests
// GET /api/v1/contracts/requests
func (h *ContractsHandler) ListRequests(c *gin.Context) {
	tenantID := h.getTenantID(c)
	requests, err := h.contractSvc.ListRequests(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"requests": requests})
}

// POST /api/v1/contracts/requests
func (h *ContractsHandler) SubmitRequest(c *gin.Context) {
	tenantID := h.getTenantID(c)
	var req models.ContractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.contractSvc.SubmitRequest(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"request": created})
}

// PATCH /api/v1/contracts/requests/:id/triage
func (h *ContractsHandler) TriageRequest(c *gin.Context) {
	tenantID := h.getTenantID(c)
	reqID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id"})
		return
	}

	var body struct {
		AssignedCounsel string `json:"assigned_counsel"`
		Status          string `json:"status"`
	}
	_ = c.ShouldBindJSON(&body)

	req, err := h.contractSvc.TriageRequest(c.Request.Context(), tenantID, reqID, body.AssignedCounsel, body.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"request": req})
}

// POST /api/v1/contracts/requests/:id/convert
func (h *ContractsHandler) ConvertRequest(c *gin.Context) {
	tenantID := h.getTenantID(c)
	reqID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id"})
		return
	}

	cnt, err := h.contractSvc.ConvertRequestToContract(c.Request.Context(), tenantID, reqID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"contract": cnt, "message": "converted to contract successfully"})
}

// Templates & Clauses
// GET /api/v1/contracts/templates
func (h *ContractsHandler) ListTemplates(c *gin.Context) {
	tenantID := h.getTenantID(c)
	templates, err := h.contractSvc.ListTemplates(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"templates": templates})
}

// POST /api/v1/contracts/templates
func (h *ContractsHandler) CreateTemplate(c *gin.Context) {
	tenantID := h.getTenantID(c)
	var req models.ContractTemplate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tpl, err := h.contractSvc.CreateTemplate(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"template": tpl})
}

// GET /api/v1/contracts/clauses
func (h *ContractsHandler) ListClauses(c *gin.Context) {
	tenantID := h.getTenantID(c)
	category := c.Query("category")
	clauses, err := h.contractSvc.ListClauses(c.Request.Context(), tenantID, category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"clauses": clauses})
}

// POST /api/v1/contracts/clauses
func (h *ContractsHandler) CreateClause(c *gin.Context) {
	tenantID := h.getTenantID(c)
	var req models.Clause
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cl, err := h.contractSvc.CreateClause(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"clause": cl})
}

// POST /api/v1/contracts/templates/assemble
func (h *ContractsHandler) AssembleContract(c *gin.Context) {
	tenantID := h.getTenantID(c)
	var req struct {
		TemplateID        uuid.UUID         `json:"template_id"`
		Variables         map[string]string `json:"variables"`
		SelectedClauseIDs []uuid.UUID       `json:"selected_clause_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	content, err := h.contractSvc.AssembleContractFromTemplate(c.Request.Context(), tenantID, req.TemplateID, req.Variables, req.SelectedClauseIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"assembled_document": content})
}

func (h *ContractsHandler) getTenantID(c *gin.Context) uuid.UUID {
	callerID := middleware.GetTenantID(c)
	if tidStr := c.Query("tenant_id"); tidStr != "" {
		parsed, err := uuid.Parse(tidStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id format"})
			return uuid.Nil
		}
		if !middleware.RequireTenantAccess(c, parsed) {
			// RequireTenantAccess aborts with 403 Forbidden
			return uuid.Nil
		}
		return parsed
	}
	if callerID != uuid.Nil {
		return callerID
	}
	// Strict Zero-Trust: Never fall back to a hardcoded tenant identity
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated tenant context"})
	return uuid.Nil
}

