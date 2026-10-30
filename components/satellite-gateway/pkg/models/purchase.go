package models

import (
	"time"

	"github.com/google/uuid"
)

// PurchaseStatus represents the lifecycle of a certificate purchase.
type PurchaseStatus string

const (
	PurchaseQuoted           PurchaseStatus = "quoted"
	PurchasePendingApproval  PurchaseStatus = "pending_approval"
	PurchaseApproved         PurchaseStatus = "approved"
	PurchaseValidating       PurchaseStatus = "validating"
	PurchaseProcessing       PurchaseStatus = "processing"
	PurchaseIssued           PurchaseStatus = "issued"
	PurchaseRejected         PurchaseStatus = "rejected"
	PurchaseCancelled        PurchaseStatus = "cancelled"
	PurchaseFailed           PurchaseStatus = "failed"
)

// CAProvider identifies a certificate authority.
type CAProvider string

const (
	CAGoDaddy      CAProvider = "godaddy"
	CANamecheap    CAProvider = "namecheap"
	CADigiCert     CAProvider = "digicert"
	CASectigo      CAProvider = "sectigo"
	CAGlobalSign   CAProvider = "globalsign"
	CAEntrust      CAProvider = "entrust"
	CALetsEncrypt  CAProvider = "letsencrypt"
	CAZeroSSL      CAProvider = "zerossl"
	CAVaultPKI     CAProvider = "vault_pki"
	CACustom       CAProvider = "custom"
)

// PurchaseOrder represents a certificate purchase from a CA.
type PurchaseOrder struct {
	ID            uuid.UUID      `json:"id" db:"id"`
	TenantID      uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	CertRequestID uuid.UUID      `json:"cert_request_id" db:"cert_request_id"`
	CAProvider    CAProvider     `json:"ca_provider" db:"ca_provider"`
	CAOrderID     string         `json:"ca_order_id" db:"ca_order_id"` // External order ID from CA
	ProductID     string         `json:"product_id" db:"product_id"`
	Domain        string         `json:"domain" db:"domain"`
	SANs          []string       `json:"sans" db:"sans"`
	CertType      CertType       `json:"cert_type" db:"cert_type"`
	Validity      string         `json:"validity" db:"validity"`
	Price         float64        `json:"price" db:"price"`
	Currency      string         `json:"currency" db:"currency"`
	Status        PurchaseStatus `json:"status" db:"status"`
	ApprovalID    *uuid.UUID     `json:"approval_id,omitempty" db:"approval_id"`
	RequestedBy   uuid.UUID      `json:"requested_by" db:"requested_by"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at" db:"updated_at"`
	IssuedAt      *time.Time     `json:"issued_at,omitempty" db:"issued_at"`
}

// Quote represents a price quote from a CA.
type Quote struct {
	CAProvider    CAProvider `json:"ca_provider"`
	ProductName   string     `json:"product_name"`
	Price         float64    `json:"price"`
	Currency      string     `json:"currency"`
	IssuanceTime  string     `json:"issuance_time"`
	Validity      string     `json:"validity"`
	Features      []string   `json:"features"`
}

// QuoteRequest is a request to get pricing from CAs.
type QuoteRequest struct {
	Domain   string   `json:"domain" binding:"required"`
	SANs     []string `json:"sans"`
	CertType CertType `json:"cert_type" binding:"required"`
	Validity string   `json:"validity" binding:"required"`
}

// CAAccountConfig holds credentials for a CA provider.
type CAAccountConfig struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Provider    CAProvider `json:"provider" db:"provider"`
	Name        string     `json:"name" db:"name"`
	Environment string     `json:"environment" db:"environment"`
	CredRef     string     `json:"cred_ref" db:"cred_ref"` // vault path or secret ref
	IsDefault   bool       `json:"is_default" db:"is_default"`
	Budget      float64    `json:"budget" db:"budget"`
	SpentMonth  float64    `json:"spent_month" db:"spent_month"`
	Active      bool       `json:"active" db:"active"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}
