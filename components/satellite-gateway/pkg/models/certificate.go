package models

import (
	"time"

	"github.com/google/uuid"
)

// CertStatus represents the lifecycle state of a certificate.
type CertStatus string

const (
	CertStatusPending   CertStatus = "pending"
	CertStatusActive    CertStatus = "active"
	CertStatusRenewing  CertStatus = "renewing"
	CertStatusRevoked   CertStatus = "revoked"
	CertStatusExpired   CertStatus = "expired"
	CertStatusFailed    CertStatus = "failed"
	CertStatusSuspended CertStatus = "suspended"
)

// CertType represents the type of certificate.
type CertType string

const (
	CertTypeDV          CertType = "DV"
	CertTypeOV          CertType = "OV"
	CertTypeEV          CertType = "EV"
	CertTypeWildcard    CertType = "wildcard"
	CertTypeSAN         CertType = "SAN"
	CertTypeMTLS        CertType = "mTLS"
	CertTypeSelfSigned  CertType = "self_signed"
	CertTypeCodeSigning CertType = "code_signing"
	CertTypeSSH         CertType = "ssh"
)

// KeyAlgorithm represents the key algorithm type.
type KeyAlgorithm string

const (
	KeyAlgoRSA2048   KeyAlgorithm = "RSA-2048"
	KeyAlgoRSA4096   KeyAlgorithm = "RSA-4096"
	KeyAlgoECDSAP256 KeyAlgorithm = "ECDSA-P256"
	KeyAlgoECDSAP384 KeyAlgorithm = "ECDSA-P384"
	KeyAlgoEd25519   KeyAlgorithm = "Ed25519"
)

// Certificate represents a managed certificate in the system.
type Certificate struct {
	ID              uuid.UUID    `json:"id" db:"id"`
	TenantID        uuid.UUID    `json:"tenant_id" db:"tenant_id"`
	Domain          string       `json:"domain" db:"domain"`
	SANs            []string     `json:"sans" db:"sans"`
	Type            CertType     `json:"type" db:"type"`
	Status          CertStatus   `json:"status" db:"status"`
	KeyAlgorithm    KeyAlgorithm `json:"key_algorithm" db:"key_algorithm"`
	Issuer          string       `json:"issuer" db:"issuer"`
	SerialNumber    string       `json:"serial_number" db:"serial_number"`
	Fingerprint     string       `json:"fingerprint" db:"fingerprint"`
	NotBefore       time.Time    `json:"not_before" db:"not_before"`
	NotAfter        time.Time    `json:"not_after" db:"not_after"`
	Environment     string       `json:"environment" db:"environment"`
	SatelliteID     string       `json:"satellite_id" db:"satellite_id"`
	SecretRef       string       `json:"secret_ref" db:"secret_ref"`
	Tags            []string     `json:"tags" db:"tags"`
	CreatedBy       uuid.UUID    `json:"created_by" db:"created_by"`
	CreatedAt       time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at" db:"updated_at"`
	RenewalCount    int          `json:"renewal_count" db:"renewal_count"`
	LastDeployedAt  *time.Time   `json:"last_deployed_at,omitempty" db:"last_deployed_at"`
	ApprovalID      *uuid.UUID   `json:"approval_id,omitempty" db:"approval_id"`
	PurchaseOrderID *uuid.UUID   `json:"purchase_order_id,omitempty" db:"purchase_order_id"`
}

// CertRequest is used to create a new certificate.
type CertRequest struct {
	TenantID     uuid.UUID    `json:"tenant_id" binding:"required"`
	Domain       string       `json:"domain" binding:"required"`
	SANs         []string     `json:"sans"`
	Type         CertType     `json:"type" binding:"required"`
	KeyAlgorithm KeyAlgorithm `json:"key_algorithm"`
	Validity     string       `json:"validity"` // e.g., "1y", "90d"
	Environment  string       `json:"environment" binding:"required"`
	CAProvider   string       `json:"ca_provider"` // e.g., "letsencrypt", "digicert"
	SatelliteID  string       `json:"satellite_id" binding:"required"`
	RequestedBy  uuid.UUID    `json:"requested_by"`
	Justification string     `json:"justification"`
}

// CertSecret represents all cert artifacts stored as a single secret (base64-encoded).
type CertSecret struct {
	PrivateKey  string       `json:"private_key"`  // base64(PEM)
	CSR         string       `json:"csr"`          // base64(PEM)
	Certificate string       `json:"certificate"`  // base64(PEM)
	Chain       string       `json:"chain"`        // base64(PEM)
	FullChain   string       `json:"fullchain"`    // base64(cert+chain PEM)
	PFX         string       `json:"pfx"`          // base64(PKCS12 binary)
	Metadata    CertMetadata `json:"metadata"`
}

// CertMetadata holds metadata stored inside the secret JSON.
type CertMetadata struct {
	CertID        string    `json:"cert_id"`
	Domain        string    `json:"domain"`
	SANs          []string  `json:"sans"`
	Issuer        string    `json:"issuer"`
	Type          string    `json:"type"`
	KeyAlgorithm  string    `json:"key_algorithm"`
	SerialNumber  string    `json:"serial_number"`
	Fingerprint   string    `json:"fingerprint_sha256"`
	NotBefore     time.Time `json:"not_before"`
	NotAfter      time.Time `json:"not_after"`
	Environment   string    `json:"environment"`
	TenantID      string    `json:"tenant_id"`
	Team          string    `json:"team"`
	Satellite     string    `json:"satellite"`
	CreatedAt     time.Time `json:"created_at"`
	Version       int       `json:"version"`
	Status        string    `json:"status"`
}
