package cloudaccounts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	clmcrypto "github.com/m2p/clm/pkg/crypto"
	"github.com/m2p/clm/pkg/models"
)

var (
	ErrInvalidAccount      = errors.New("invalid cloud account")
	ErrIdempotencyConflict = errors.New("idempotency key was already used for a different request")
	ErrProviderMismatch    = errors.New("cloud account provider does not match the requested provider")
)

type Service struct {
	repo   core.CloudAccountRepository
	cipher *clmcrypto.AESGCM
}

func NewService(repo core.CloudAccountRepository, masterKey string) (*Service, error) {
	cipher, err := clmcrypto.NewAESGCM([]byte(masterKey))
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, cipher: cipher}, nil
}

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, request models.CloudAccountCreate, idempotencyKey string) (*models.CloudAccount, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Provider = strings.ToLower(strings.TrimSpace(request.Provider))
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if tenantID == uuid.Nil || request.Name == "" || !validProvider(request.Provider) || len(request.Credentials) == 0 || len(idempotencyKey) > 255 {
		return nil, ErrInvalidAccount
	}
	if err := validateCredentialPayload(request.Provider, request.Credentials); err != nil {
		return nil, err
	}

	canonical, err := json.Marshal(request)
	if err != nil {
		return nil, ErrInvalidAccount
	}
	sum := sha256.Sum256(canonical)
	requestHash := hex.EncodeToString(sum[:])

	id := uuid.New()
	plaintext, err := json.Marshal(request.Credentials)
	if err != nil {
		return nil, ErrInvalidAccount
	}
	nonce, ciphertext, err := s.cipher.Encrypt(plaintext, aad(tenantID, id))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	stored := &models.EncryptedCloudCredentials{
		CloudAccount: models.CloudAccount{
			ID:                id,
			TenantID:          tenantID,
			Name:              request.Name,
			Provider:          request.Provider,
			CredentialVersion: 1,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		EncryptionVersion: clmcrypto.EncryptionVersion,
		Nonce:             nonce,
		Ciphertext:        ciphertext,
	}
	if idempotencyKey == "" {
		if err := s.repo.Create(ctx, stored); err != nil {
			return nil, err
		}
		return &stored.CloudAccount, nil
	}
	existing, existingHash, err := s.repo.CreateIdempotent(ctx, stored, idempotencyKey, requestHash)
	if err != nil {
		return nil, fmt.Errorf("create idempotent cloud account: %w", err)
	}
	if existing != nil {
		if existingHash != requestHash {
			return nil, ErrIdempotencyConflict
		}
		return &existing.CloudAccount, nil
	}
	return &stored.CloudAccount, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id uuid.UUID) (*models.CloudAccount, error) {
	if tenantID == uuid.Nil || id == uuid.Nil {
		return nil, ErrInvalidAccount
	}
	stored, err := s.repo.GetByIDForTenant(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return &stored.CloudAccount, nil
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]*models.CloudAccount, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidAccount
	}
	return s.repo.ListByTenant(ctx, tenantID)
}

func (s *Service) Rename(ctx context.Context, tenantID, id uuid.UUID, name string) error {
	name = strings.TrimSpace(name)
	if tenantID == uuid.Nil || id == uuid.Nil || name == "" {
		return ErrInvalidAccount
	}
	return s.repo.RenameForTenant(ctx, tenantID, id, name)
}

func (s *Service) Rotate(ctx context.Context, tenantID, id uuid.UUID, credentials models.CloudCredentialPayload) error {
	if tenantID == uuid.Nil || id == uuid.Nil || len(credentials) == 0 {
		return ErrInvalidAccount
	}
	stored, err := s.repo.GetByIDForTenant(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if err := validateCredentialPayload(stored.Provider, credentials); err != nil {
		return err
	}
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return ErrInvalidAccount
	}
	nonce, ciphertext, err := s.cipher.Encrypt(plaintext, aad(tenantID, id))
	if err != nil {
		return err
	}
	return s.repo.RotateForTenant(ctx, tenantID, id, clmcrypto.EncryptionVersion, nonce, ciphertext)
}

func (s *Service) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	if tenantID == uuid.Nil || id == uuid.Nil {
		return ErrInvalidAccount
	}
	return s.repo.DeleteForTenant(ctx, tenantID, id)
}

func (s *Service) CredentialsForUse(ctx context.Context, tenantID, id uuid.UUID) (models.CloudCredentialPayload, error) {
	stored, err := s.repo.GetByIDForTenant(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	payload, err := s.decrypt(stored)
	if err != nil {
		return nil, err
	}
	if err := validateCredentialPayload(stored.Provider, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Service) CredentialsForProviderUse(ctx context.Context, tenantID, id uuid.UUID, provider string) (models.CloudCredentialPayload, error) {
	stored, err := s.repo.GetByIDForTenant(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if stored.Provider != strings.ToLower(strings.TrimSpace(provider)) {
		return nil, ErrProviderMismatch
	}
	payload, err := s.decrypt(stored)
	if err != nil {
		return nil, err
	}
	if err := validateCredentialPayload(stored.Provider, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Service) decrypt(stored *models.EncryptedCloudCredentials) (models.CloudCredentialPayload, error) {
	if stored.EncryptionVersion != clmcrypto.EncryptionVersion {
		return nil, fmt.Errorf("unsupported credential encryption version %d", stored.EncryptionVersion)
	}
	plaintext, err := s.cipher.Decrypt(stored.Nonce, stored.Ciphertext, aad(stored.TenantID, stored.ID))
	if err != nil {
		return nil, err
	}
	var payload models.CloudCredentialPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil || len(payload) == 0 {
		return nil, errors.New("invalid encrypted credential payload")
	}
	return payload, nil
}

func (s *Service) ValidateOffline(ctx context.Context, tenantID, id uuid.UUID) (models.CloudAccountValidation, error) {
	if _, err := s.CredentialsForUse(ctx, tenantID, id); err != nil {
		return models.CloudAccountValidation{}, err
	}
	return models.CloudAccountValidation{
		Status:  "unverified",
		Message: "credential encryption and structure were verified offline; no live provider call was made",
	}, nil
}

func validateCredentialPayload(provider string, payload models.CloudCredentialPayload) error {
	allowed := map[string]struct{}{}
	required := []string{}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "aws":
		allowed = stringSet("aws_access_key_id", "aws_secret_access_key", "aws_role_arn", "aws_external_id")
		accessKey, accessOK := credentialString(payload, "aws_access_key_id")
		secretKey, secretOK := credentialString(payload, "aws_secret_access_key")
		roleARN, roleOK := credentialString(payload, "aws_role_arn")
		_, externalOK := credentialString(payload, "aws_external_id")
		static := accessOK || secretOK
		if roleOK == static || (static && (!accessOK || !secretOK)) || (!roleOK && externalOK) ||
			(roleOK && (!strings.HasPrefix(roleARN, "arn:aws:iam::") || !strings.Contains(roleARN, ":role/"))) ||
			(accessOK && (accessKey == "" || secretKey == "")) {
			return ErrInvalidAccount
		}
	case "gcp":
		allowed = stringSet("gcp_service_account_json", "gcp_project_id")
		required = []string{"gcp_service_account_json", "gcp_project_id"}
		serviceAccount, ok := credentialString(payload, "gcp_service_account_json")
		if !ok || !validGCPServiceAccount(serviceAccount) {
			return ErrInvalidAccount
		}
	case "azure":
		allowed = stringSet("azure_tenant_id", "azure_client_id", "azure_client_secret", "azure_subscription_id")
		required = []string{"azure_tenant_id", "azure_client_id", "azure_client_secret", "azure_subscription_id"}
	default:
		return ErrInvalidAccount
	}
	for key := range payload {
		if _, ok := allowed[key]; !ok {
			return ErrInvalidAccount
		}
		if _, ok := credentialString(payload, key); !ok {
			return ErrInvalidAccount
		}
	}
	for _, key := range required {
		if _, ok := credentialString(payload, key); !ok {
			return ErrInvalidAccount
		}
	}
	return nil
}

func credentialString(payload models.CloudCredentialPayload, key string) (string, bool) {
	raw, ok := payload[key]
	if !ok {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func stringSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func validGCPServiceAccount(raw string) bool {
	var document struct {
		Type        string `json:"type"`
		ProjectID   string `json:"project_id"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	return json.Unmarshal([]byte(raw), &document) == nil && document.Type == "service_account" &&
		strings.TrimSpace(document.ProjectID) != "" && strings.TrimSpace(document.ClientEmail) != "" &&
		strings.TrimSpace(document.PrivateKey) != ""
}

func validProvider(provider string) bool {
	switch strings.ToLower(provider) {
	case "aws", "gcp", "azure":
		return true
	default:
		return false
	}
}

func aad(tenantID, accountID uuid.UUID) []byte {
	return []byte(tenantID.String() + "/" + accountID.String())
}
