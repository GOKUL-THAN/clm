package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CloudAccount is the redacted API/domain representation. Encrypted credential
// material is deliberately held in a separate persistence-only structure.
type CloudAccount struct {
	ID                uuid.UUID `json:"id" db:"id"`
	TenantID          uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name              string    `json:"name" db:"name"`
	Provider          string    `json:"provider" db:"provider"`
	CredentialVersion int       `json:"credential_version" db:"credential_version"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

// EncryptedCloudCredentials is never serialized by REST responses.
type EncryptedCloudCredentials struct {
	CloudAccount
	EncryptionVersion int    `json:"-" db:"encryption_version"`
	Nonce             []byte `json:"-" db:"credential_nonce"`
	Ciphertext        []byte `json:"-" db:"credential_ciphertext"`
}

// CloudCredentialPayload is provider-neutral JSON supplied only on create or
// rotation. It is encrypted as one authenticated blob and never logged.
type CloudCredentialPayload map[string]json.RawMessage

type CloudAccountCreate struct {
	Name        string                 `json:"name" binding:"required"`
	Provider    string                 `json:"provider" binding:"required"`
	Credentials CloudCredentialPayload `json:"credentials" binding:"required"`
}

type CloudAccountUpdate struct {
	Name string `json:"name" binding:"required"`
}

type CloudAccountValidation struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
