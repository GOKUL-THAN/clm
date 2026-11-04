package contract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// Service defines contract lifecycle management capabilities
type Service interface {
	// Contracts
	ListContracts(ctx context.Context, tenantID uuid.UUID, filter Filter) ([]*models.Contract, int, error)
	GetContract(ctx context.Context, tenantID, contractID uuid.UUID) (*models.Contract, error)
	CreateContract(ctx context.Context, tenantID uuid.UUID, c *models.Contract) (*models.Contract, error)
	UpdateContract(ctx context.Context, tenantID uuid.UUID, c *models.Contract) (*models.Contract, error)
	TransitionContract(ctx context.Context, tenantID, contractID uuid.UUID, targetStatus models.ContractStatus, targetStage models.ContractStage, comment string) (*models.Contract, error)
	
	// Versions
	ListVersions(ctx context.Context, tenantID, contractID uuid.UUID) ([]*models.ContractVersion, error)
	CreateVersion(ctx context.Context, tenantID, contractID uuid.UUID, title, content, changelog, authorEmail string) (*models.ContractVersion, error)

	// Intake & Requests
	ListRequests(ctx context.Context, tenantID uuid.UUID) ([]*models.ContractRequest, error)
	SubmitRequest(ctx context.Context, tenantID uuid.UUID, req *models.ContractRequest) (*models.ContractRequest, error)
	TriageRequest(ctx context.Context, tenantID, reqID uuid.UUID, assignedCounsel, status string) (*models.ContractRequest, error)
	ConvertRequestToContract(ctx context.Context, tenantID, reqID uuid.UUID) (*models.Contract, error)

	// Templates & Clauses
	ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]*models.ContractTemplate, error)
	CreateTemplate(ctx context.Context, tenantID uuid.UUID, tpl *models.ContractTemplate) (*models.ContractTemplate, error)
	ListClauses(ctx context.Context, tenantID uuid.UUID, category string) ([]*models.Clause, error)
	CreateClause(ctx context.Context, tenantID uuid.UUID, cl *models.Clause) (*models.Clause, error)
	AssembleContractFromTemplate(ctx context.Context, tenantID, templateID uuid.UUID, variables map[string]string, selectedClauseIDs []uuid.UUID) (string, error)

	// Negotiations & Redlines
	ListRedlines(ctx context.Context, tenantID, contractID uuid.UUID) ([]*models.ContractRedline, error)
	SubmitRedline(ctx context.Context, tenantID, contractID uuid.UUID, redline *models.ContractRedline) (*models.ContractRedline, error)
	ResolveRedline(ctx context.Context, tenantID, redlineID uuid.UUID, accepted bool, reviewComment string) (*models.ContractRedline, error)

	// Signatures
	ListSignatures(ctx context.Context, tenantID, contractID uuid.UUID) ([]*models.ContractSignature, error)
	AddSigner(ctx context.Context, tenantID, contractID uuid.UUID, sig *models.ContractSignature) (*models.ContractSignature, error)
	ExecuteSignature(ctx context.Context, tenantID, sigID uuid.UUID, authMethod, ipAddress string) (*models.ContractSignature, error)

	// Obligations
	ListObligations(ctx context.Context, tenantID uuid.UUID, contractID *uuid.UUID) ([]*models.ContractObligation, error)
	AddObligation(ctx context.Context, tenantID, contractID uuid.UUID, obl *models.ContractObligation) (*models.ContractObligation, error)
	FulfillObligation(ctx context.Context, tenantID, oblID uuid.UUID, verifiedBy, evidenceNotes string) (*models.ContractObligation, error)

	// Analytics & Reporting
	GetAnalytics(ctx context.Context, tenantID uuid.UUID) (*AnalyticsSummary, error)
}

// Filter provides querying and search parameters
type Filter struct {
	Type         string
	Status       string
	Stage        string
	Search       string
	Counterparty string
	Limit        int
	Offset       int
}

type AnalyticsSummary struct {
	TotalContracts       int            `json:"total_contracts"`
	ActiveContracts      int            `json:"active_contracts"`
	InNegotiation        int            `json:"in_negotiation"`
	PendingSignature     int            `json:"pending_signature"`
	ExpiringWithin90Days int            `json:"expiring_within_90_days"`
	TotalContractValue   float64        `json:"total_contract_value"`
	AverageCycleDays     int            `json:"average_cycle_days"`
	ComplianceRate       float64        `json:"compliance_rate"`
	TypeDistribution     map[string]int `json:"type_distribution"`
	RiskDistribution     map[string]int `json:"risk_distribution"`
}

type serviceImpl struct {
	logger *zap.Logger
	mu     sync.RWMutex

	contracts    map[uuid.UUID]*models.Contract
	versions     map[uuid.UUID][]*models.ContractVersion
	requests     map[uuid.UUID]*models.ContractRequest
	templates    map[uuid.UUID]*models.ContractTemplate
	clauses      map[uuid.UUID]*models.Clause
	redlines     map[uuid.UUID]*models.ContractRedline
	signatures   map[uuid.UUID]*models.ContractSignature
	obligations  map[uuid.UUID]*models.ContractObligation
	suppliers    map[uuid.UUID]*models.ProcurementSupplier
	legalMatters map[uuid.UUID]*models.LegalMatter
}

// NewService instantiates enterprise contract lifecycle service with seeded data
func NewService(logger *zap.Logger) Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &serviceImpl{
		logger:       logger,
		contracts:    make(map[uuid.UUID]*models.Contract),
		versions:     make(map[uuid.UUID][]*models.ContractVersion),
		requests:     make(map[uuid.UUID]*models.ContractRequest),
		templates:    make(map[uuid.UUID]*models.ContractTemplate),
		clauses:      make(map[uuid.UUID]*models.Clause),
		redlines:     make(map[uuid.UUID]*models.ContractRedline),
		signatures:   make(map[uuid.UUID]*models.ContractSignature),
		obligations:  make(map[uuid.UUID]*models.ContractObligation),
		suppliers:    make(map[uuid.UUID]*models.ProcurementSupplier),
		legalMatters: make(map[uuid.UUID]*models.LegalMatter),
	}
	s.seedBaselineData()
	return s
}

func (s *serviceImpl) seedBaselineData() {
	defaultTenant := uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	now := time.Now().UTC()
	exp1 := now.Add(180 * 24 * time.Hour)
	exp2 := now.Add(365 * 24 * time.Hour)
	exp3 := now.Add(730 * 24 * time.Hour)

	// Contract 1: MSA
	cnt1ID := uuid.MustParse("c0000001-0000-0000-0000-000000000001")
	s.contracts[cnt1ID] = &models.Contract{
		ID:                  cnt1ID,
		TenantID:            defaultTenant,
		ContractNumber:      "CNT-2026-00892",
		Title:               "Enterprise Master Services Agreement (MSA)",
		Type:                models.ContractTypeMSA,
		Status:              models.StatusActive,
		Stage:               models.StagePostExecution,
		CounterpartyName:    "Apex Global FinTech Ltd",
		CounterpartyContact: "Sarah Jenkins, VP Operations",
		CounterpartyEmail:   "sjenkins@apexfin.com",
		EntityID:            "LEGAL-ENT-US-01",
		BusinessUnit:        "Financial Infrastructure",
		Department:          "Core Banking",
		EffectiveDate:       &now,
		ExpirationDate:      &exp1,
		RenewalType:         "auto_renewal",
		RenewalNoticeDays:   60,
		NoticePeriodDays:    30,
		TotalValue:          350000.00,
		Currency:            "USD",
		PaymentTerms:        "Net 30",
		GoverningLaw:        "State of New York",
		Jurisdiction:        "United States",
		RiskScore:           22,
		ComplianceStatus:    "compliant",
		CurrentVersion:      3,
		ContentBody:         "Master Services Agreement executed between Enterprise Global and Apex FinTech for high-availability cloud payment rails.",
		FolderPath:          "/Financial/MSAs",
		CreatedAt:           now.Add(-90 * 24 * time.Hour),
		UpdatedAt:           now,
	}

	// Contract 2: Vendor
	cnt2ID := uuid.MustParse("c0000002-0000-0000-0000-000000000002")
	s.contracts[cnt2ID] = &models.Contract{
		ID:                  cnt2ID,
		TenantID:            defaultTenant,
		ContractNumber:      "CNT-2026-00914",
		Title:               "Cloud Infrastructure Hosting Agreement",
		Type:                models.ContractTypeVendor,
		Status:              models.StatusNegotiation,
		Stage:               models.StageNegotiation,
		CounterpartyName:    "HyperScale Cloud Systems Inc.",
		CounterpartyContact: "Marcus Vance",
		CounterpartyEmail:   "mvance@hyperscalecloud.io",
		EntityID:            "LEGAL-ENT-US-01",
		BusinessUnit:        "Platform Operations",
		Department:          "Cloud Engineering",
		EffectiveDate:       &now,
		ExpirationDate:      &exp2,
		RenewalType:         "manual",
		RenewalNoticeDays:   30,
		NoticePeriodDays:    30,
		TotalValue:          125000.00,
		Currency:            "USD",
		PaymentTerms:        "Quarterly Advance",
		GoverningLaw:        "State of California",
		Jurisdiction:        "United States",
		RiskScore:           48,
		ComplianceStatus:    "under_review",
		CurrentVersion:      2,
		ContentBody:         "Dedicated private VPC hosting agreement with 99.995% SLA and cryptographic HSM compliance.",
		FolderPath:          "/Vendors/Infrastructure",
		CreatedAt:           now.Add(-10 * 24 * time.Hour),
		UpdatedAt:           now,
	}

	// Contract 3: NDA
	cnt3ID := uuid.MustParse("c0000003-0000-0000-0000-000000000003")
	s.contracts[cnt3ID] = &models.Contract{
		ID:                  cnt3ID,
		TenantID:            defaultTenant,
		ContractNumber:      "CNT-2026-00955",
		Title:               "Mutual Non-Disclosure Agreement (MNDA)",
		Type:                models.ContractTypeNDA,
		Status:              models.StatusReadyForSignature,
		Stage:               models.StageSignature,
		CounterpartyName:    "Quantex AI Research Group",
		CounterpartyContact: "Dr. Elena Rostova",
		CounterpartyEmail:   "elena@quantex-ai.org",
		EntityID:            "LEGAL-ENT-US-01",
		BusinessUnit:        "R&D",
		Department:          "Applied AI",
		EffectiveDate:       &now,
		ExpirationDate:      &exp3,
		RenewalType:         "evergreen",
		RenewalNoticeDays:   30,
		NoticePeriodDays:    15,
		TotalValue:          0.00,
		Currency:            "USD",
		PaymentTerms:        "N/A",
		GoverningLaw:        "State of Delaware",
		Jurisdiction:        "United States",
		RiskScore:           15,
		ComplianceStatus:    "compliant",
		CurrentVersion:      1,
		ContentBody:         "Mutual non-disclosure and intellectual property protection covering cryptographic primitives.",
		FolderPath:          "/Legal/NDAs",
		CreatedAt:           now.Add(-2 * 24 * time.Hour),
		UpdatedAt:           now,
	}

	// Templates
	tpl1ID := uuid.MustParse("f0000001-0000-0000-0000-000000000001")
	s.templates[tpl1ID] = &models.ContractTemplate{
		ID:          tpl1ID,
		TenantID:    defaultTenant,
		Name:        "Enterprise Master Services Agreement (MSA) Template",
		Category:    "MSA",
		Description: "Standard bilateral MSA with mutual indemnity, 99.99% uptime, and PCI-DSS compliance.",
		ContentBody: "This MASTER SERVICES AGREEMENT is made by and between {{PartyA}} and {{PartyB}} on {{EffectiveDate}} for total consideration of {{TotalValue}}.\n\nSection 1: Scope of Services.\nSection 2: Commercial Terms.\nSection 3: Confidentiality.\nSection 4: Limitation of Liability.\nSection 5: Term and Termination.",
		Version:     1,
		Status:      "published",
		Variables:   []string{"PartyA", "PartyB", "EffectiveDate", "TotalValue"},
		CreatedBy:   "legal-ops@enterprise.internal",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Clauses
	cls1ID := uuid.MustParse("b0000001-0000-0000-0000-000000000001")
	s.clauses[cls1ID] = &models.Clause{
		ID:                   cls1ID,
		TenantID:             defaultTenant,
		Title:                "Mutual Confidentiality (3-Year Standard)",
		Category:             "Confidentiality",
		StandardText:         "Each party agrees to preserve in strict confidence all proprietary technical, financial, and business information received from the disclosing party for a period of three (3) years.",
		FallbackText:         "Each party shall exercise reasonable commercial care to safeguard confidential information for two (2) years.",
		RiskLevel:            "low",
		IsMandatory:          true,
		PlaybookGuidance:     "Do not accept indefinite confidentiality without General Counsel written approval.",
		JurisdictionTags:     []string{"global", "delaware", "ny"},
		ApprovalRoleRequired: "legal_counsel",
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	cls2ID := uuid.MustParse("b0000002-0000-0000-0000-000000000002")
	s.clauses[cls2ID] = &models.Clause{
		ID:                   cls2ID,
		TenantID:             defaultTenant,
		Title:                "Limitation of Liability (12-Month Cap)",
		Category:             "Liability",
		StandardText:         "Except for indemnification obligations or willful breach, neither party's aggregate cumulative liability shall exceed fees paid or payable in the twelve (12) months preceding the incident.",
		FallbackText:         "Neither party's aggregate cumulative liability shall exceed 1.5x total contract value.",
		RiskLevel:            "elevated",
		IsMandatory:          true,
		PlaybookGuidance:     "Escalate any uncapped liability to Chief Legal Officer immediately.",
		JurisdictionTags:     []string{"global"},
		ApprovalRoleRequired: "vp_legal",
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	// Obligations
	obl1ID := uuid.MustParse("d0000001-0000-0000-0000-000000000001")
	s.obligations[obl1ID] = &models.ContractObligation{
		ID:                   obl1ID,
		ContractID:           cnt1ID,
		Title:                "Annual SOC 2 Type II Audit Report Delivery",
		Category:             "Regulatory",
		PartyResponsible:     "counterparty",
		OwnerEmail:           "compliance@apexfin.com",
		DueDate:              now.Add(45 * 24 * time.Hour),
		Recurrence:           "annual",
		Status:               "in_progress",
		PenaltyDetails:       "5% fee credit per 15-day delay beyond audit period.",
		ServiceCreditPercent: 5.0,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	// Signatures
	sig1ID := uuid.MustParse("e0000001-0000-0000-0000-000000000001")
	s.signatures[sig1ID] = &models.ContractSignature{
		ID:                   sig1ID,
		ContractID:           cnt3ID,
		SignerName:           "Internal Legal Counsel",
		SignerEmail:          "legal@enterprise.internal",
		SignerRole:           "General Counsel",
		SigningOrder:         1,
		Status:               "signed",
		AuthMethod:           "email_otp",
		IPAddress:            "10.125.4.68",
		DigitalSignatureHash: "9f83a45c6b8d2e1f4a7c0b9e8d7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e2d1c0b9a",
		SignedAt:             &now,
		CreatedAt:            now,
	}
}

// Contracts Implementation
func (s *serviceImpl) ListContracts(ctx context.Context, tenantID uuid.UUID, filter Filter) ([]*models.Contract, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*models.Contract
	searchLower := strings.ToLower(filter.Search)

	for _, c := range s.contracts {
		if c.TenantID != tenantID && tenantID != uuid.Nil {
			// allow fallback multi-tenant read if tenantID matches or is default
			if tenantID.String() != "a0000000-0000-0000-0000-000000000001" && c.TenantID.String() != "a0000000-0000-0000-0000-000000000001" {
				continue
			}
		}

		if filter.Type != "" && !strings.EqualFold(string(c.Type), filter.Type) {
			continue
		}
		if filter.Status != "" && !strings.EqualFold(string(c.Status), filter.Status) {
			continue
		}
		if filter.Stage != "" && !strings.EqualFold(string(c.Stage), filter.Stage) {
			continue
		}
		if filter.Counterparty != "" && !strings.Contains(strings.ToLower(c.CounterpartyName), strings.ToLower(filter.Counterparty)) {
			continue
		}
		if searchLower != "" {
			matches := strings.Contains(strings.ToLower(c.Title), searchLower) ||
				strings.Contains(strings.ToLower(c.ContractNumber), searchLower) ||
				strings.Contains(strings.ToLower(c.CounterpartyName), searchLower)
			if !matches {
				continue
			}
		}

		results = append(results, c)
	}

	return results, len(results), nil
}

func (s *serviceImpl) GetContract(ctx context.Context, tenantID, contractID uuid.UUID) (*models.Contract, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.contracts[contractID]
	if !ok {
		return nil, errors.New("contract not found")
	}
	return c, nil
}

func (s *serviceImpl) CreateContract(ctx context.Context, tenantID uuid.UUID, c *models.Contract) (*models.Contract, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.TenantID == uuid.Nil {
		c.TenantID = tenantID
	}
	if c.ContractNumber == "" {
		c.ContractNumber = fmt.Sprintf("CNT-%d-%05d", time.Now().Year(), len(s.contracts)+100)
	}
	if c.Status == "" {
		c.Status = models.StatusDraft
	}
	if c.Stage == "" {
		c.Stage = models.StageAuthoring
	}
	c.CurrentVersion = 1
	c.CreatedAt = time.Now().UTC()
	c.UpdatedAt = c.CreatedAt

	s.contracts[c.ID] = c

	// Seed initial version
	v := &models.ContractVersion{
		ID:               uuid.New(),
		ContractID:       c.ID,
		VersionNumber:    1,
		Title:            c.Title,
		ContentBody:      c.ContentBody,
		ChangelogSummary: "Initial contract authoring baseline",
		AuthorEmail:      c.OwnerEmail,
		DocumentHash:     s.computeHash(c.ContentBody),
		CreatedAt:        c.CreatedAt,
	}
	s.versions[c.ID] = append(s.versions[c.ID], v)

	return c, nil
}

func (s *serviceImpl) UpdateContract(ctx context.Context, tenantID uuid.UUID, c *models.Contract) (*models.Contract, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.contracts[c.ID]
	if !ok {
		return nil, errors.New("contract not found")
	}

	c.TenantID = existing.TenantID
	c.CreatedAt = existing.CreatedAt
	c.UpdatedAt = time.Now().UTC()
	s.contracts[c.ID] = c

	return c, nil
}

func (s *serviceImpl) TransitionContract(ctx context.Context, tenantID, contractID uuid.UUID, targetStatus models.ContractStatus, targetStage models.ContractStage, comment string) (*models.Contract, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.contracts[contractID]
	if !ok {
		return nil, errors.New("contract not found")
	}

	c.Status = targetStatus
	if targetStage != "" {
		c.Stage = targetStage
	}
	c.UpdatedAt = time.Now().UTC()

	s.logger.Info("contract transitioned",
		zap.String("contract_id", contractID.String()),
		zap.String("new_status", string(targetStatus)),
		zap.String("comment", comment))

	return c, nil
}

// Versions Implementation
func (s *serviceImpl) ListVersions(ctx context.Context, tenantID, contractID uuid.UUID) ([]*models.ContractVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.versions[contractID], nil
}

func (s *serviceImpl) CreateVersion(ctx context.Context, tenantID, contractID uuid.UUID, title, content, changelog, authorEmail string) (*models.ContractVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.contracts[contractID]
	if !ok {
		return nil, errors.New("contract not found")
	}

	nextVersion := c.CurrentVersion + 1
	c.CurrentVersion = nextVersion
	c.ContentBody = content
	c.UpdatedAt = time.Now().UTC()

	v := &models.ContractVersion{
		ID:               uuid.New(),
		ContractID:       contractID,
		VersionNumber:    nextVersion,
		Title:            title,
		ContentBody:      content,
		ChangelogSummary: changelog,
		AuthorEmail:      authorEmail,
		DocumentHash:     s.computeHash(content),
		CreatedAt:        c.UpdatedAt,
	}
	s.versions[contractID] = append(s.versions[contractID], v)

	return v, nil
}

// Intake & Requests
func (s *serviceImpl) ListRequests(ctx context.Context, tenantID uuid.UUID) ([]*models.ContractRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*models.ContractRequest
	for _, r := range s.requests {
		list = append(list, r)
	}
	return list, nil
}

func (s *serviceImpl) SubmitRequest(ctx context.Context, tenantID uuid.UUID, req *models.ContractRequest) (*models.ContractRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.ID == uuid.Nil {
		req.ID = uuid.New()
	}
	req.TenantID = tenantID
	req.RequestNumber = fmt.Sprintf("REQ-%d-%05d", time.Now().Year(), len(s.requests)+1)
	req.Status = "submitted"
	req.CreatedAt = time.Now().UTC()
	req.UpdatedAt = req.CreatedAt

	s.requests[req.ID] = req
	return req, nil
}

func (s *serviceImpl) TriageRequest(ctx context.Context, tenantID, reqID uuid.UUID, assignedCounsel, status string) (*models.ContractRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.requests[reqID]
	if !ok {
		return nil, errors.New("contract request not found")
	}
	if assignedCounsel != "" {
		req.AssignedCounselEmail = assignedCounsel
	}
	if status != "" {
		req.Status = status
	}
	req.UpdatedAt = time.Now().UTC()
	return req, nil
}

func (s *serviceImpl) ConvertRequestToContract(ctx context.Context, tenantID, reqID uuid.UUID) (*models.Contract, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.requests[reqID]
	if !ok {
		return nil, errors.New("contract request not found")
	}

	cnt := &models.Contract{
		ID:               uuid.New(),
		TenantID:         tenantID,
		ContractNumber:   fmt.Sprintf("CNT-%d-%05d", time.Now().Year(), len(s.contracts)+100),
		Title:            req.Title,
		Type:             models.ContractType(req.ContractType),
		Status:           models.StatusDraft,
		Stage:            models.StageAuthoring,
		CounterpartyName: req.CounterpartyName,
		TotalValue:       req.EstimatedValue,
		Currency:         req.Currency,
		Department:       req.Department,
		OwnerEmail:       req.RequesterEmail,
		CurrentVersion:   1,
		ContentBody:      fmt.Sprintf("Contract generated from Request %s: %s\n\nJustification: %s", req.RequestNumber, req.Title, req.BusinessJustification),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}

	s.contracts[cnt.ID] = cnt
	req.Status = "converted"
	req.ConvertedContractID = &cnt.ID
	req.UpdatedAt = time.Now().UTC()

	return cnt, nil
}

// Templates & Clauses
func (s *serviceImpl) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]*models.ContractTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*models.ContractTemplate
	for _, t := range s.templates {
		list = append(list, t)
	}
	return list, nil
}

func (s *serviceImpl) CreateTemplate(ctx context.Context, tenantID uuid.UUID, tpl *models.ContractTemplate) (*models.ContractTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tpl.ID == uuid.Nil {
		tpl.ID = uuid.New()
	}
	tpl.TenantID = tenantID
	tpl.CreatedAt = time.Now().UTC()
	tpl.UpdatedAt = tpl.CreatedAt
	s.templates[tpl.ID] = tpl
	return tpl, nil
}

func (s *serviceImpl) ListClauses(ctx context.Context, tenantID uuid.UUID, category string) ([]*models.Clause, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*models.Clause
	for _, cl := range s.clauses {
		if category != "" && !strings.EqualFold(cl.Category, category) {
			continue
		}
		list = append(list, cl)
	}
	return list, nil
}

func (s *serviceImpl) CreateClause(ctx context.Context, tenantID uuid.UUID, cl *models.Clause) (*models.Clause, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cl.ID == uuid.Nil {
		cl.ID = uuid.New()
	}
	cl.TenantID = tenantID
	cl.CreatedAt = time.Now().UTC()
	cl.UpdatedAt = cl.CreatedAt
	s.clauses[cl.ID] = cl
	return cl, nil
}

func (s *serviceImpl) AssembleContractFromTemplate(ctx context.Context, tenantID, templateID uuid.UUID, variables map[string]string, selectedClauseIDs []uuid.UUID) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tpl, ok := s.templates[templateID]
	if !ok {
		return "", errors.New("template not found")
	}

	assembled := tpl.ContentBody
	for k, v := range variables {
		assembled = strings.ReplaceAll(assembled, "{{"+k+"}}", v)
	}

	if len(selectedClauseIDs) > 0 {
		assembled += "\n\n--- ADDITIONAL PRE-APPROVED CLAUSES ---\n"
		for _, cid := range selectedClauseIDs {
			if cl, found := s.clauses[cid]; found {
				assembled += fmt.Sprintf("\n[%s]\n%s\n", cl.Title, cl.StandardText)
			}
		}
	}

	return assembled, nil
}

// Negotiations & Redlines
func (s *serviceImpl) ListRedlines(ctx context.Context, tenantID, contractID uuid.UUID) ([]*models.ContractRedline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*models.ContractRedline
	for _, r := range s.redlines {
		if r.ContractID == contractID {
			list = append(list, r)
		}
	}
	return list, nil
}

func (s *serviceImpl) SubmitRedline(ctx context.Context, tenantID, contractID uuid.UUID, redline *models.ContractRedline) (*models.ContractRedline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if redline.ID == uuid.Nil {
		redline.ID = uuid.New()
	}
	redline.ContractID = contractID
	redline.Status = "proposed"
	redline.CreatedAt = time.Now().UTC()
	s.redlines[redline.ID] = redline
	return redline, nil
}

func (s *serviceImpl) ResolveRedline(ctx context.Context, tenantID, redlineID uuid.UUID, accepted bool, reviewComment string) (*models.ContractRedline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	redline, ok := s.redlines[redlineID]
	if !ok {
		return nil, errors.New("redline not found")
	}

	if accepted {
		redline.Status = "accepted"
	} else {
		redline.Status = "rejected"
	}
	redline.ReviewComments = reviewComment
	now := time.Now().UTC()
	redline.ResolvedAt = &now
	return redline, nil
}

// Signatures
func (s *serviceImpl) ListSignatures(ctx context.Context, tenantID, contractID uuid.UUID) ([]*models.ContractSignature, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*models.ContractSignature
	for _, sig := range s.signatures {
		if sig.ContractID == contractID {
			list = append(list, sig)
		}
	}
	return list, nil
}

func (s *serviceImpl) AddSigner(ctx context.Context, tenantID, contractID uuid.UUID, sig *models.ContractSignature) (*models.ContractSignature, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sig.ID == uuid.Nil {
		sig.ID = uuid.New()
	}
	sig.ContractID = contractID
	sig.Status = "pending"
	sig.CreatedAt = time.Now().UTC()
	s.signatures[sig.ID] = sig
	return sig, nil
}

func (s *serviceImpl) ExecuteSignature(ctx context.Context, tenantID, sigID uuid.UUID, authMethod, ipAddress string) (*models.ContractSignature, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sig, ok := s.signatures[sigID]
	if !ok {
		return nil, errors.New("signature entry not found")
	}

	if sig.Status == "signed" {
		return nil, errors.New("CLM-CRYPTO-ERR-001: target signature has already been executed (duplicate signing prohibited)")
	}

	now := time.Now().UTC()
	sig.Status = "signed"
	sig.AuthMethod = authMethod
	sig.IPAddress = ipAddress
	sig.SignedAt = &now
	sig.DigitalSignatureHash = s.computeHash(fmt.Sprintf("%s:%s:%s", sig.SignerEmail, now.String(), ipAddress))

	return sig, nil
}

// Obligations
func (s *serviceImpl) ListObligations(ctx context.Context, tenantID uuid.UUID, contractID *uuid.UUID) ([]*models.ContractObligation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*models.ContractObligation
	for _, o := range s.obligations {
		if contractID != nil && o.ContractID != *contractID {
			continue
		}
		list = append(list, o)
	}
	return list, nil
}

func (s *serviceImpl) AddObligation(ctx context.Context, tenantID, contractID uuid.UUID, obl *models.ContractObligation) (*models.ContractObligation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if obl.ID == uuid.Nil {
		obl.ID = uuid.New()
	}
	obl.ContractID = contractID
	obl.Status = "in_progress"
	obl.CreatedAt = time.Now().UTC()
	obl.UpdatedAt = obl.CreatedAt
	s.obligations[obl.ID] = obl
	return obl, nil
}

func (s *serviceImpl) FulfillObligation(ctx context.Context, tenantID, oblID uuid.UUID, verifiedBy, evidenceNotes string) (*models.ContractObligation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	obl, ok := s.obligations[oblID]
	if !ok {
		return nil, errors.New("obligation not found")
	}

	now := time.Now().UTC()
	obl.Status = "fulfilled"
	obl.CompletedAt = &now
	obl.VerifiedBy = verifiedBy
	obl.EvidenceNotes = evidenceNotes
	obl.UpdatedAt = now

	return obl, nil
}

// Analytics
func (s *serviceImpl) GetAnalytics(ctx context.Context, tenantID uuid.UUID) (*AnalyticsSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := &AnalyticsSummary{
		TotalContracts:   len(s.contracts),
		TypeDistribution: make(map[string]int),
		RiskDistribution: make(map[string]int),
		AverageCycleDays: 14,
		ComplianceRate:   96.5,
	}

	now := time.Now().UTC()
	in90 := now.Add(90 * 24 * time.Hour)

	for _, c := range s.contracts {
		summary.TotalContractValue += c.TotalValue
		summary.TypeDistribution[string(c.Type)]++

		if c.Status == models.StatusActive {
			summary.ActiveContracts++
		}
		if c.Status == models.StatusNegotiation {
			summary.InNegotiation++
		}
		if c.Status == models.StatusReadyForSignature {
			summary.PendingSignature++
		}

		if c.ExpirationDate != nil && c.ExpirationDate.After(now) && c.ExpirationDate.Before(in90) {
			summary.ExpiringWithin90Days++
		}

		if c.RiskScore >= 70 {
			summary.RiskDistribution["high"]++
		} else if c.RiskScore >= 40 {
			summary.RiskDistribution["medium"]++
		} else {
			summary.RiskDistribution["low"]++
		}
	}

	return summary, nil
}

func (s *serviceImpl) computeHash(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}
