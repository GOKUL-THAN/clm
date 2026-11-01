package models

import (
	"time"

	"github.com/google/uuid"
)

// CorporateDepartment represents the cross-functional corporate department owning or operating the contract
type CorporateDepartment string

const (
	DeptHR                  CorporateDepartment = "HR"
	DeptFinance             CorporateDepartment = "Finance"
	DeptProcurement         CorporateDepartment = "Procurement"
	DeptSales               CorporateDepartment = "Sales"
	DeptLegal               CorporateDepartment = "Legal"
	DeptIT                  CorporateDepartment = "IT"
	DeptSecurity            CorporateDepartment = "Security"
	DeptOperations          CorporateDepartment = "Operations"
	DeptManufacturing       CorporateDepartment = "Manufacturing"
	DeptFacilities          CorporateDepartment = "Facilities"
	DeptMarketing           CorporateDepartment = "Marketing"
	DeptCompliance          CorporateDepartment = "Compliance"
	DeptAdministration      CorporateDepartment = "Administration"
	DeptExecutiveManagement CorporateDepartment = "Executive Management"
)

// ContractType represents the legal categorization of the agreement
type ContractType string

const (
	ContractTypeMSA             ContractType = "MSA"
	ContractTypeNDA             ContractType = "NDA"
	ContractTypeSOW             ContractType = "SOW"
	ContractTypeSLA             ContractType = "SLA"
	ContractTypeVendor          ContractType = "Vendor"
	ContractTypeCustomer        ContractType = "Customer"
	ContractTypeLease           ContractType = "Lease"
	ContractTypePartnership     ContractType = "Partnership"
	ContractTypeLicense         ContractType = "License"
	ContractTypeEmployment      ContractType = "Employment"
	ContractTypeProcurement     ContractType = "Procurement"
	ContractTypeOfferLetter     ContractType = "OfferLetter"
	ContractTypeFinancial       ContractType = "Financial"
	ContractTypePurchase        ContractType = "Purchase"
	ContractTypeSales           ContractType = "Sales"
	ContractTypeSaaS            ContractType = "SaaS"
	ContractTypeDPA             ContractType = "DPA"
	ContractTypeQualityTooling  ContractType = "QualityTooling"
	ContractTypeFacilityLease   ContractType = "FacilityLease"
	ContractTypeMarketingAgency ContractType = "MarketingAgency"
	ContractTypeRegulatoryAudit ContractType = "RegulatoryAudit"
	ContractTypeDOA             ContractType = "DOA"
	ContractTypeStrategicJoint  ContractType = "StrategicJoint"
)

// ContractStatus represents the operational lifecycle state
type ContractStatus string

const (
	StatusDraft             ContractStatus = "draft"
	StatusReview            ContractStatus = "review"
	StatusApproval          ContractStatus = "approval"
	StatusNegotiation       ContractStatus = "negotiation"
	StatusReadyForSignature ContractStatus = "ready_for_signature"
	StatusPartiallySigned   ContractStatus = "partially_signed"
	StatusExecuted          ContractStatus = "executed"
	StatusActive            ContractStatus = "active"
	StatusAmended           ContractStatus = "amended"
	StatusRenewing          ContractStatus = "renewing"
	StatusExpired           ContractStatus = "expired"
	StatusTerminated        ContractStatus = "terminated"
)

// ContractStage represents the macro stage in CLM
type ContractStage string

const (
	StageIntake        ContractStage = "intake"
	StageAuthoring     ContractStage = "authoring"
	StageNegotiation   ContractStage = "negotiation"
	StageApproval      ContractStage = "approval"
	StageSignature     ContractStage = "signature"
	StagePostExecution ContractStage = "post_execution"
	StageRenewal       ContractStage = "renewal"
	StageArchival      ContractStage = "archival"
)

// Contract represents an enterprise legal contract
type Contract struct {
	ID                   uuid.UUID              `json:"id" db:"id"`
	TenantID             uuid.UUID              `json:"tenant_id" db:"tenant_id"`
	ContractNumber       string                 `json:"contract_number" db:"contract_number"`
	Title                string                 `json:"title" db:"title"`
	Type                 ContractType           `json:"type" db:"type"`
	Status               ContractStatus         `json:"status" db:"status"`
	Stage                ContractStage          `json:"stage" db:"stage"`
	CounterpartyName     string                 `json:"counterparty_name" db:"counterparty_name"`
	CounterpartyContact  string                 `json:"counterparty_contact,omitempty" db:"counterparty_contact"`
	CounterpartyEmail    string                 `json:"counterparty_email,omitempty" db:"counterparty_email"`
	EntityID             string                 `json:"entity_id" db:"entity_id"`
	BusinessUnit         string                 `json:"business_unit" db:"business_unit"`
	Department           string                 `json:"department" db:"department"`
	OwnerUserID          string                 `json:"owner_user_id,omitempty" db:"owner_user_id"`
	OwnerEmail           string                 `json:"owner_email,omitempty" db:"owner_email"`
	EffectiveDate        *time.Time             `json:"effective_date,omitempty" db:"effective_date"`
	ExpirationDate       *time.Time             `json:"expiration_date,omitempty" db:"expiration_date"`
	ExecutionDate        *time.Time             `json:"execution_date,omitempty" db:"execution_date"`
	RenewalType          string                 `json:"renewal_type" db:"renewal_type"` // manual, auto_renewal, evergreen
	RenewalNoticeDays    int                    `json:"renewal_notice_days" db:"renewal_notice_days"`
	NoticePeriodDays     int                    `json:"notice_period_days" db:"notice_period_days"`
	TotalValue           float64                `json:"total_value" db:"total_value"`
	Currency             string                 `json:"currency" db:"currency"`
	PaymentTerms         string                 `json:"payment_terms" db:"payment_terms"`
	GoverningLaw         string                 `json:"governing_law" db:"governing_law"`
	Jurisdiction         string                 `json:"jurisdiction" db:"jurisdiction"`
	RiskScore            int                    `json:"risk_score" db:"risk_score"`
	ComplianceStatus     string                 `json:"compliance_status" db:"compliance_status"`
	CurrentVersion       int                    `json:"current_version" db:"current_version"`
	ContentBody          string                 `json:"content_body" db:"content_body"`
	FolderPath           string                 `json:"folder_path" db:"folder_path"`
	ParentContractID     *uuid.UUID             `json:"parent_contract_id,omitempty" db:"parent_contract_id"`
	MasterAgreementID    *uuid.UUID             `json:"master_agreement_id,omitempty" db:"master_agreement_id"`
	Tags                 []string               `json:"tags,omitempty" db:"tags"`
	Metadata             map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	CreatedAt            time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at" db:"updated_at"`
}

// ContractVersion represents an immutable historical revision
type ContractVersion struct {
	ID               uuid.UUID `json:"id" db:"id"`
	ContractID       uuid.UUID `json:"contract_id" db:"contract_id"`
	VersionNumber    int       `json:"version_number" db:"version_number"`
	Title            string    `json:"title" db:"title"`
	ContentBody      string    `json:"content_body" db:"content_body"`
	ChangelogSummary string    `json:"changelog_summary" db:"changelog_summary"`
	AuthorUserID     string    `json:"author_user_id,omitempty" db:"author_user_id"`
	AuthorEmail      string    `json:"author_email,omitempty" db:"author_email"`
	DocumentHash     string    `json:"document_hash" db:"document_hash"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

// ContractRequest represents an intake or business request
type ContractRequest struct {
	ID                    uuid.UUID              `json:"id" db:"id"`
	TenantID              uuid.UUID              `json:"tenant_id" db:"tenant_id"`
	RequestNumber         string                 `json:"request_number" db:"request_number"`
	Title                 string                 `json:"title" db:"title"`
	ContractType          string                 `json:"contract_type" db:"contract_type"`
	RequestOrigin         string                 `json:"request_origin" db:"request_origin"`
	CounterpartyName      string                 `json:"counterparty_name" db:"counterparty_name"`
	CounterpartyEmail     string                 `json:"counterparty_email,omitempty" db:"counterparty_email"`
	EstimatedValue        float64                `json:"estimated_value" db:"estimated_value"`
	Currency              string                 `json:"currency" db:"currency"`
	Urgency               string                 `json:"urgency" db:"urgency"`
	BusinessJustification string                 `json:"business_justification" db:"business_justification"`
	Department            string                 `json:"department" db:"department"`
	RequesterEmail        string                 `json:"requester_email" db:"requester_email"`
	AssignedCounselEmail  string                 `json:"assigned_counsel_email,omitempty" db:"assigned_counsel_email"`
	Status                string                 `json:"status" db:"status"`
	ConvertedContractID   *uuid.UUID             `json:"converted_contract_id,omitempty" db:"converted_contract_id"`
	SLADeadline           *time.Time             `json:"sla_deadline,omitempty" db:"sla_deadline"`
	FormData              map[string]interface{} `json:"form_data,omitempty" db:"form_data"`
	CreatedAt             time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at" db:"updated_at"`
}

// ContractTemplate represents a reusable legal template
type ContractTemplate struct {
	ID             uuid.UUID              `json:"id" db:"id"`
	TenantID       uuid.UUID              `json:"tenant_id" db:"tenant_id"`
	Name           string                 `json:"name" db:"name"`
	Category       string                 `json:"category" db:"category"`
	Description    string                 `json:"description" db:"description"`
	ContentBody    string                 `json:"content_body" db:"content_body"`
	Version        int                    `json:"version" db:"version"`
	Status         string                 `json:"status" db:"status"`
	Variables      []string               `json:"variables" db:"variables"`
	GoverningRules map[string]interface{} `json:"governing_rules,omitempty" db:"governing_rules"`
	CreatedBy      string                 `json:"created_by" db:"created_by"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at" db:"updated_at"`
}

// Clause represents a standardized pre-approved legal clause
type Clause struct {
	ID                     uuid.UUID `json:"id" db:"id"`
	TenantID               uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Title                  string    `json:"title" db:"title"`
	Category               string    `json:"category" db:"category"`
	StandardText           string    `json:"standard_text" db:"standard_text"`
	FallbackText           string    `json:"fallback_text" db:"fallback_text"`
	RiskLevel              string    `json:"risk_level" db:"risk_level"`
	IsMandatory            bool      `json:"is_mandatory" db:"is_mandatory"`
	PlaybookGuidance       string    `json:"playbook_guidance,omitempty" db:"playbook_guidance"`
	JurisdictionTags       []string  `json:"jurisdiction_tags" db:"jurisdiction_tags"`
	ApprovalRoleRequired   string    `json:"approval_role_required" db:"approval_role_required"`
	CreatedAt              time.Time `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time `json:"updated_at" db:"updated_at"`
}

// ContractRedline represents an external or internal proposed clause change
type ContractRedline struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	ContractID     uuid.UUID  `json:"contract_id" db:"contract_id"`
	NegotiationID  *uuid.UUID `json:"negotiation_id,omitempty" db:"negotiation_id"`
	AuthorName     string     `json:"author_name" db:"author_name"`
	AuthorType     string     `json:"author_type" db:"author_type"` // internal, counterparty
	ChangeType     string     `json:"change_type" db:"change_type"` // addition, deletion, modification
	ClauseTitle    string     `json:"clause_title" db:"clause_title"`
	OriginalText   string     `json:"original_text" db:"original_text"`
	ProposedText   string     `json:"proposed_text" db:"proposed_text"`
	Status         string     `json:"status" db:"status"` // proposed, accepted, rejected, superseded
	ReviewComments string     `json:"review_comments,omitempty" db:"review_comments"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
}

// ContractSignature represents an e-sign or PKI digital signature record
type ContractSignature struct {
	ID                   uuid.UUID  `json:"id" db:"id"`
	ContractID           uuid.UUID  `json:"contract_id" db:"contract_id"`
	SignerName           string     `json:"signer_name" db:"signer_name"`
	SignerEmail          string     `json:"signer_email" db:"signer_email"`
	SignerRole           string     `json:"signer_role" db:"signer_role"`
	SigningOrder         int        `json:"signing_order" db:"signing_order"`
	Status               string     `json:"status" db:"status"` // pending, sent, viewed, signed, declined
	AuthMethod           string     `json:"auth_method" db:"auth_method"` // email_otp, pki_certificate, hardware_token
	IPAddress            string     `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent            string     `json:"user_agent,omitempty" db:"user_agent"`
	DigitalSignatureHash string     `json:"digital_signature_hash,omitempty" db:"digital_signature_hash"`
	CertificateThumbprint string   `json:"certificate_thumbprint,omitempty" db:"certificate_thumbprint"`
	SignedAt             *time.Time `json:"signed_at,omitempty" db:"signed_at"`
	EvidenceBundleURI    string     `json:"evidence_bundle_uri,omitempty" db:"evidence_bundle_uri"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
}

// ContractObligation represents a post-signature milestone or deliverable
type ContractObligation struct {
	ID                  uuid.UUID  `json:"id" db:"id"`
	ContractID          uuid.UUID  `json:"contract_id" db:"contract_id"`
	Title               string     `json:"title" db:"title"`
	Category            string     `json:"category" db:"category"` // Operational, Regulatory, Financial, Warranty
	PartyResponsible    string     `json:"party_responsible" db:"party_responsible"` // internal, counterparty, both
	OwnerEmail          string     `json:"owner_email,omitempty" db:"owner_email"`
	DueDate             time.Time  `json:"due_date" db:"due_date"`
	Recurrence          string     `json:"recurrence" db:"recurrence"` // one_time, monthly, quarterly, annual
	Status              string     `json:"status" db:"status"` // in_progress, fulfilled, overdue, waived, breached
	PenaltyDetails      string     `json:"penalty_details,omitempty" db:"penalty_details"`
	ServiceCreditPercent float64   `json:"service_credit_percent" db:"service_credit_percent"`
	EvidenceNotes       string     `json:"evidence_notes,omitempty" db:"evidence_notes"`
	EvidenceDocumentURI string     `json:"evidence_document_uri,omitempty" db:"evidence_document_uri"`
	CompletedAt         *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	VerifiedBy          string     `json:"verified_by,omitempty" db:"verified_by"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

// AIContractAnalysis represents machine-assisted legal risk and clause extraction
type AIContractAnalysis struct {
	ID                   uuid.UUID                `json:"id" db:"id"`
	ContractID           uuid.UUID                `json:"contract_id" db:"contract_id"`
	AnalysisType         string                   `json:"analysis_type" db:"analysis_type"`
	ModelName            string                   `json:"model_name" db:"model_name"`
	ConfidenceScore      float64                  `json:"confidence_score" db:"confidence_score"`
	ExecutiveSummary     string                   `json:"executive_summary" db:"executive_summary"`
	ExtractedClauses     []ExtractedClause        `json:"extracted_clauses" db:"extracted_clauses"`
	ExtractedObligations []ExtractedObligation    `json:"extracted_obligations" db:"extracted_obligations"`
	DetectedRisks        []DetectedRisk           `json:"detected_risks" db:"detected_risks"`
	HumanInLoopVerified  bool                     `json:"human_in_loop_verified" db:"human_in_loop_verified"`
	VerifiedByUser       string                   `json:"verified_by_user,omitempty" db:"verified_by_user"`
	CreatedAt            time.Time                `json:"created_at" db:"created_at"`
}

type ExtractedClause struct {
	ClauseName string `json:"clause_name"`
	Risk       string `json:"risk"`
	Excerpt    string `json:"excerpt"`
}

type ExtractedObligation struct {
	Title       string `json:"title"`
	Recurrence  string `json:"recurrence"`
	Responsible string `json:"responsible"`
}

type DetectedRisk struct {
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// ProcurementSupplier represents vendor/supplier lifecycle
type ProcurementSupplier struct {
	ID                   uuid.UUID  `json:"id" db:"id"`
	TenantID             uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	SupplierName         string     `json:"supplier_name" db:"supplier_name"`
	SupplierCode         string     `json:"supplier_code" db:"supplier_code"`
	Category             string     `json:"category" db:"category"`
	ContactName          string     `json:"contact_name" db:"contact_name"`
	ContactEmail         string     `json:"contact_email" db:"contact_email"`
	OnboardingStatus     string     `json:"onboarding_status" db:"onboarding_status"`
	RiskTier             string     `json:"risk_tier" db:"risk_tier"`
	PerformanceScore     int        `json:"performance_score" db:"performance_score"`
	DiversityStatus      string     `json:"diversity_status" db:"diversity_status"`
	InsuranceCertExpires *time.Time `json:"insurance_cert_expires,omitempty" db:"insurance_cert_expires"`
	AnnualSpend          float64    `json:"annual_spend" db:"annual_spend"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
}

// DelegationOfAuthorityRule defines multi-tier approval thresholds and role requirements
type DelegationOfAuthorityRule struct {
	ID                        uuid.UUID `json:"id" db:"id"`
	TenantID                  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Department                string    `json:"department" db:"department"`
	MinAmount                 float64   `json:"min_amount" db:"min_amount"`
	MaxAmount                 float64   `json:"max_amount" db:"max_amount"`
	TierLevel                 int       `json:"tier_level" db:"tier_level"`
	RequiredRole              string    `json:"required_role" db:"required_role"`
	RequiresLegalReview       bool      `json:"requires_legal_review" db:"requires_legal_review"`
	RequiresFinanceReview     bool      `json:"requires_finance_review" db:"requires_finance_review"`
	RequiresSecurityReview    bool      `json:"requires_security_review" db:"requires_security_review"`
	EnforceSeparationOfDuties bool      `json:"enforce_separation_of_duties" db:"enforce_separation_of_duties"`
	CreatedAt                 time.Time `json:"created_at" db:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at" db:"updated_at"`
}

// ContractApprovalRecord tracks individual approval decisions in the multi-tier workflow
type ContractApprovalRecord struct {
	ID                    uuid.UUID  `json:"id" db:"id"`
	TenantID              uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ContractID            uuid.UUID  `json:"contract_id" db:"contract_id"`
	TierLevel             int        `json:"tier_level" db:"tier_level"`
	ApproverRole          string     `json:"approver_role" db:"approver_role"`
	ApproverUserID        string     `json:"approver_user_id,omitempty" db:"approver_user_id"`
	ApproverEmail         string     `json:"approver_email" db:"approver_email"`
	Status                string     `json:"status" db:"status"` // pending, approved, rejected, delegated
	ApprovalComments      string     `json:"approval_comments,omitempty" db:"approval_comments"`
	DelegatedToEmail      string     `json:"delegated_to_email,omitempty" db:"delegated_to_email"`
	DecidedAt             *time.Time `json:"decided_at,omitempty" db:"decided_at"`
	IPAddress             string     `json:"ip_address,omitempty" db:"ip_address"`
	DigitalSignatureToken string     `json:"digital_signature_token,omitempty" db:"digital_signature_token"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
}

// ThreeWayMatchRecord represents automated reconciliation between Purchase Order, Goods Receipt, and Supplier Invoice
type ThreeWayMatchRecord struct {
	ID                        uuid.UUID  `json:"id" db:"id"`
	TenantID                  uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ContractID                uuid.UUID  `json:"contract_id" db:"contract_id"`
	POID                      uuid.UUID  `json:"po_id" db:"po_id"`
	ReceiptID                 *uuid.UUID `json:"receipt_id,omitempty" db:"receipt_id"`
	InvoiceID                 uuid.UUID  `json:"invoice_id" db:"invoice_id"`
	MatchResult               string     `json:"match_result" db:"match_result"` // perfect_match, price_variance, quantity_variance, unauthorized_overage
	VarianceAmount            float64    `json:"variance_amount" db:"variance_amount"`
	ToleranceThresholdPercent float64    `json:"tolerance_threshold_percent" db:"tolerance_threshold_percent"`
	ApprovedForDisbursement   bool       `json:"approved_for_disbursement" db:"approved_for_disbursement"`
	DisbursementApproverEmail string     `json:"disbursement_approver_email,omitempty" db:"disbursement_approver_email"`
	VerifiedAt                time.Time  `json:"verified_at" db:"verified_at"`
}

// LegalMatter represents a legal department matter, dispute, or litigation
type LegalMatter struct {
	ID                   uuid.UUID  `json:"id" db:"id"`
	TenantID             uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	MatterNumber         string     `json:"matter_number" db:"matter_number"`
	Title                string     `json:"title" db:"title"`
	CaseType             string     `json:"case_type" db:"case_type"`
	Priority             string     `json:"priority" db:"priority"`
	LeadCounsel          string     `json:"lead_counsel" db:"lead_counsel"`
	OutsideCounselFirm   string     `json:"outside_counsel_firm,omitempty" db:"outside_counsel_firm"`
	LegalBudget          float64    `json:"legal_budget" db:"legal_budget"`
	LegalSpend           float64    `json:"legal_spend" db:"legal_spend"`
	LitigationHoldActive bool       `json:"litigation_hold_active" db:"litigation_hold_active"`
	Status               string     `json:"status" db:"status"`
	RelatedContractID    *uuid.UUID `json:"related_contract_id,omitempty" db:"related_contract_id"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
}

