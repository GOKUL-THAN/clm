package core

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
)

// ErrNotFound is returned by repositories when no row matches the lookup. It
// lets service code tell a genuine miss apart from a database failure instead of
// reporting every error as "not found".
var ErrNotFound = errors.New("not found")

// CertificateRepository defines data access for certificates.
type CertificateRepository interface {
	Create(ctx context.Context, cert *models.Certificate) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Certificate, error)
	GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.Certificate, error)
	GetByDomain(ctx context.Context, tenantID uuid.UUID, domain string) ([]*models.Certificate, error)
	List(ctx context.Context, tenantID uuid.UUID, filter *CertFilter) ([]*models.Certificate, int, error)
	Update(ctx context.Context, cert *models.Certificate) error
	Delete(ctx context.Context, id uuid.UUID) error
	CountByTenant(ctx context.Context, tenantID uuid.UUID) (int, error)
}

// CertFilter allows filtering certificate queries.
type CertFilter struct {
	Status      []models.CertStatus
	Environment string
	SatelliteID string
	ExpiringIn  int // days
	Page        int
	PageSize    int
}

// TenantRepository defines data access for tenants.
type TenantRepository interface {
	Create(ctx context.Context, tenant *models.Tenant) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*models.Tenant, error)
	List(ctx context.Context) ([]*models.Tenant, error)
	Update(ctx context.Context, tenant *models.Tenant) error
	// CountDependents reports rows referencing the tenant, keyed by table.
	CountDependents(ctx context.Context, id uuid.UUID) (map[string]int, error)
	// Delete removes a tenant that has no dependent rows.
	Delete(ctx context.Context, id uuid.UUID) error
	// DeleteCascade removes a tenant and everything referencing it.
	DeleteCascade(ctx context.Context, id uuid.UUID) error
}

// UserRepository defines data access for users.
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) // platform/internal only
	GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*models.User, error)
	ListAll(ctx context.Context) ([]*models.User, error)
	Update(ctx context.Context, user *models.User) error // platform/internal only
	UpdateForTenant(ctx context.Context, tenantID uuid.UUID, user *models.User) error
	Delete(ctx context.Context, id uuid.UUID) error // platform/internal only
	DeleteForTenant(ctx context.Context, tenantID, id uuid.UUID) error
}

// ApprovalRepository defines data access for approval requests.
type ApprovalRepository interface {
	Create(ctx context.Context, req *models.ApprovalRequest) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.ApprovalRequest, error)
	ListPending(ctx context.Context, tenantID uuid.UUID) ([]*models.ApprovalRequest, error)
	Update(ctx context.Context, req *models.ApprovalRequest) error
	GetRules(ctx context.Context, tenantID uuid.UUID) ([]*models.ApprovalRule, error)
}

// SatelliteRepository defines tenant-bound satellite identity and durable command access.
type SatelliteRepository interface {
	Create(ctx context.Context, sat *models.Satellite) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Satellite, error)
	GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.Satellite, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*models.Satellite, error)
	Update(ctx context.Context, sat *models.Satellite) error
	UpdateForTenant(ctx context.Context, tenantID uuid.UUID, sat *models.Satellite) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteForTenant(ctx context.Context, tenantID, id uuid.UUID) error
	ListAll(ctx context.Context) ([]*models.Satellite, error)

	SetCredentialForTenant(ctx context.Context, tenantID, satelliteID uuid.UUID, encodedHash string) error
	RevokeCredentialForTenant(ctx context.Context, tenantID, satelliteID uuid.UUID) error
	AuthenticateSatellite(ctx context.Context, satelliteID uuid.UUID, plaintext string) (*models.SatelliteIdentity, error)

	QueueCommand(ctx context.Context, cmd *models.SatelliteCommand) error
	QueueCommandForTenant(ctx context.Context, tenantID uuid.UUID, cmd *models.SatelliteCommand) error
	QueueCommandIdempotent(ctx context.Context, tenantID uuid.UUID, cmd *models.SatelliteCommand) (*models.SatelliteCommand, bool, error)
	ClaimCommands(ctx context.Context, satelliteID uuid.UUID, leaseOwner string, limit int, leaseDuration time.Duration) ([]*models.SatelliteCommand, error)
	AcknowledgeCommand(ctx context.Context, satelliteID, commandID, leaseToken uuid.UUID, leaseDuration time.Duration) error
	RenewCommandLease(ctx context.Context, satelliteID, commandID, leaseToken uuid.UUID, leaseDuration time.Duration) error
	CompleteCommand(ctx context.Context, satelliteID, commandID, leaseToken uuid.UUID, status models.CommandStatus, result, lastError string, retryable bool, backoff time.Duration) (*models.SatelliteCommand, error)
	CancelCommandForTenant(ctx context.Context, tenantID, satelliteID, commandID uuid.UUID) error
	RetryCommandForTenant(ctx context.Context, tenantID, satelliteID, commandID uuid.UUID) error

	// Compatibility methods retained for internal v1 producers; gateway routes no longer use read-and-clear semantics.
	GetPendingCommands(ctx context.Context, satelliteID uuid.UUID) ([]*models.SatelliteCommand, error)
	GetPendingCommandsForTenant(ctx context.Context, tenantID, satelliteID uuid.UUID) ([]*models.SatelliteCommand, error)
	UpdateCommand(ctx context.Context, cmd *models.SatelliteCommand) error
	UpdateCommandForSatellite(ctx context.Context, satelliteID uuid.UUID, cmd *models.SatelliteCommand) error
}

// AuditRepository only supports inserts and tenant-scoped reads. There are no
// update/delete methods, and the database independently rejects those writes.
type AuditRepository interface {
	Append(ctx context.Context, event *models.AuditEvent) error
	ListByTenant(ctx context.Context, tenantID uuid.UUID, filter models.AuditFilter) ([]*models.AuditEvent, error)
}

// CloudAccountRepository keeps encrypted material internal and exposes only
// explicit tenant-scoped operations.
type CloudAccountRepository interface {
	Create(ctx context.Context, account *models.EncryptedCloudCredentials) error
	GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.EncryptedCloudCredentials, error)
	GetByNameForTenant(ctx context.Context, tenantID uuid.UUID, name string) (*models.EncryptedCloudCredentials, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*models.CloudAccount, error)
	RenameForTenant(ctx context.Context, tenantID, id uuid.UUID, name string) error
	RotateForTenant(ctx context.Context, tenantID, id uuid.UUID, encryptionVersion int, nonce, ciphertext []byte) error
	DeleteForTenant(ctx context.Context, tenantID, id uuid.UUID) error
	// CreateIdempotent atomically inserts the account and idempotency result. On
	// key conflict it rolls back the attempted account and returns the existing
	// account plus the hash originally bound to the key.
	CreateIdempotent(ctx context.Context, account *models.EncryptedCloudCredentials, key, requestHash string) (existing *models.EncryptedCloudCredentials, existingRequestHash string, err error)
}

// PurchaseRepository defines data access for purchase orders.
type PurchaseRepository interface {
	Create(ctx context.Context, order *models.PurchaseOrder) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.PurchaseOrder, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID, status *models.PurchaseStatus) ([]*models.PurchaseOrder, error)
	Update(ctx context.Context, order *models.PurchaseOrder) error
	GetCAAccounts(ctx context.Context, tenantID uuid.UUID) ([]*models.CAAccountConfig, error)
}

// CAProviderPlugin abstracts interactions with a Certificate Authority.
type CAProviderPlugin interface {
	Name() string
	GetQuote(ctx context.Context, req *models.QuoteRequest) (*models.Quote, error)
	SubmitOrder(ctx context.Context, order *models.PurchaseOrder, csr []byte) (externalOrderID string, err error)
	GetOrderStatus(ctx context.Context, externalOrderID string) (string, error)
	DownloadCertificate(ctx context.Context, externalOrderID string) (cert []byte, chain []byte, err error)
	Revoke(ctx context.Context, serialNumber string, reason string) error
}

// SecretsStore abstracts cloud-specific secret storage (single secret per cert).
type SecretsStore interface {
	Store(ctx context.Context, req *StoreSecretRequest) (*SecretRef, error)
	Retrieve(ctx context.Context, ref *SecretRef) (*models.CertSecret, error)
	RetrieveField(ctx context.Context, ref *SecretRef, field string) ([]byte, error)
	Update(ctx context.Context, ref *SecretRef, req *StoreSecretRequest) (*SecretRef, error)
	Delete(ctx context.Context, ref *SecretRef) error
	Disable(ctx context.Context, ref *SecretRef) error
	List(ctx context.Context, prefix string) ([]*SecretRef, error)
}

// StoreSecretRequest contains all cert artifacts to store.
type StoreSecretRequest struct {
	CertID      string
	TenantID    string
	Domain      string
	Environment string
	Secret      *models.CertSecret
	Tags        map[string]string
}

// SecretRef points to a stored secret.
type SecretRef struct {
	Backend  string `json:"backend"`
	SecretID string `json:"secret_id"`
	Version  string `json:"version"`
	Region   string `json:"region"`
}

// CryptoEngine performs all crypto operations on the satellite.
type CryptoEngine interface {
	GenerateKey(ctx context.Context, algo models.KeyAlgorithm) (privateKeyPEM []byte, err error)
	GenerateCSR(ctx context.Context, privateKeyPEM []byte, req *CSRRequest) (csrPEM []byte, err error)
	AssembleChain(ctx context.Context, certPEM []byte, intermediates [][]byte) (chainPEM []byte, fullChainPEM []byte, err error)
	CreatePFX(ctx context.Context, certPEM, keyPEM, chainPEM []byte, password string) (pfxBytes []byte, err error)
	VerifyKeyMatch(ctx context.Context, certPEM, keyPEM []byte) (bool, error)
	ParseCertificate(ctx context.Context, certPEM []byte) (*CertInfo, error)
}

// CSRRequest holds parameters for CSR generation.
type CSRRequest struct {
	CommonName   string
	Organization string
	Country      string
	SANs         []string
}

// CertInfo is parsed certificate metadata.
type CertInfo struct {
	Subject      string
	Issuer       string
	SerialNumber string
	Fingerprint  string
	NotBefore    string
	NotAfter     string
	SANs         []string
	KeyAlgorithm string
}
