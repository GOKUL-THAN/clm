package cloudaccounts

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
)

func TestCloudAccountEncryptionRedactionAndIsolation(t *testing.T) {
	repo := newMemoryRepo()
	service, err := NewService(repo, "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	tenantA, tenantB := uuid.New(), uuid.New()
	secret := "must-not-leak"
	account, err := service.Create(context.Background(), tenantA, models.CloudAccountCreate{Name: "primary", Provider: "aws", Credentials: models.CloudCredentialPayload{
		"aws_access_key_id":     json.RawMessage(`"AKIAEXAMPLE"`),
		"aws_secret_access_key": json.RawMessage(`"` + secret + `"`),
	}}, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(account)
	if string(encoded) == "" || contains(string(encoded), secret) {
		t.Fatalf("redacted response leaked secret: %s", encoded)
	}
	stored := repo.accounts[account.ID]
	if contains(string(stored.Ciphertext), secret) {
		t.Fatal("ciphertext contains plaintext")
	}
	payload, err := service.CredentialsForUse(context.Background(), tenantA, account.ID)
	if err != nil || string(payload["aws_secret_access_key"]) != `"`+secret+`"` {
		t.Fatalf("decrypted payload=%s err=%v", payload, err)
	}
	if _, err := service.Get(context.Background(), tenantB, account.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("cross-tenant read error=%v", err)
	}
	validation, err := service.ValidateOffline(context.Background(), tenantA, account.ID)
	if err != nil || validation.Status != "unverified" {
		t.Fatalf("validation=%+v err=%v", validation, err)
	}
}
func TestCloudAccountIdempotencyConflict(t *testing.T) {
	repo := newMemoryRepo()
	service, _ := NewService(repo, "0123456789abcdef0123456789abcdef")
	tenantID := uuid.New()
	request := models.CloudAccountCreate{Name: "one", Provider: "gcp", Credentials: validGCPCredentials()}
	first, err := service.Create(context.Background(), tenantID, request, "same")
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Create(context.Background(), tenantID, request, "same")
	if err != nil || again.ID != first.ID {
		t.Fatalf("idempotent create=%+v err=%v", again, err)
	}
	request.Name = "two"
	if _, err := service.Create(context.Background(), tenantID, request, "same"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflict error=%v", err)
	}
}
func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

type memoryRepo struct {
	accounts map[uuid.UUID]*models.EncryptedCloudCredentials
	keys     map[string]struct {
		hash string
		id   uuid.UUID
	}
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{accounts: map[uuid.UUID]*models.EncryptedCloudCredentials{}, keys: map[string]struct {
		hash string
		id   uuid.UUID
	}{}}
}
func (r *memoryRepo) Create(_ context.Context, a *models.EncryptedCloudCredentials) error {
	r.accounts[a.ID] = a
	return nil
}
func (r *memoryRepo) GetByIDForTenant(_ context.Context, tenantID, id uuid.UUID) (*models.EncryptedCloudCredentials, error) {
	a, ok := r.accounts[id]
	if !ok || a.TenantID != tenantID {
		return nil, core.ErrNotFound
	}
	return a, nil
}
func (r *memoryRepo) GetByNameForTenant(ctx context.Context, tenantID uuid.UUID, name string) (*models.EncryptedCloudCredentials, error) {
	for _, a := range r.accounts {
		if a.TenantID == tenantID && a.Name == name {
			return a, nil
		}
	}
	return nil, core.ErrNotFound
}
func (r *memoryRepo) ListByTenant(_ context.Context, tenantID uuid.UUID) ([]*models.CloudAccount, error) {
	var out []*models.CloudAccount
	for _, a := range r.accounts {
		if a.TenantID == tenantID {
			x := a.CloudAccount
			out = append(out, &x)
		}
	}
	return out, nil
}
func (r *memoryRepo) RenameForTenant(ctx context.Context, tenantID, id uuid.UUID, name string) error {
	a, err := r.GetByIDForTenant(ctx, tenantID, id)
	if err != nil {
		return err
	}
	a.Name = name
	return nil
}
func (r *memoryRepo) RotateForTenant(ctx context.Context, tenantID, id uuid.UUID, version int, nonce, ciphertext []byte) error {
	a, err := r.GetByIDForTenant(ctx, tenantID, id)
	if err != nil {
		return err
	}
	a.EncryptionVersion = version
	a.Nonce = nonce
	a.Ciphertext = ciphertext
	a.CredentialVersion++
	return nil
}
func (r *memoryRepo) DeleteForTenant(ctx context.Context, tenantID, id uuid.UUID) error {
	if _, err := r.GetByIDForTenant(ctx, tenantID, id); err != nil {
		return err
	}
	delete(r.accounts, id)
	return nil
}
func (r *memoryRepo) CreateIdempotent(_ context.Context, account *models.EncryptedCloudCredentials, key, requestHash string) (*models.EncryptedCloudCredentials, string, error) {
	mapKey := account.TenantID.String() + key
	if value, ok := r.keys[mapKey]; ok {
		return r.accounts[value.id], value.hash, nil
	}
	r.accounts[account.ID] = account
	r.keys[mapKey] = struct {
		hash string
		id   uuid.UUID
	}{requestHash, account.ID}
	return nil, "", nil
}

func TestCloudAccountProviderBindingAndEncryptionVersion(t *testing.T) {
	repo := newMemoryRepo()
	service, err := NewService(repo, "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	tenantID := uuid.New()
	account, err := service.Create(context.Background(), tenantID, models.CloudAccountCreate{
		Name: "aws-primary", Provider: "aws",
		Credentials: models.CloudCredentialPayload{"aws_role_arn": json.RawMessage(`"arn:aws:iam::123456789012:role/CLMDeploy"`)},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CredentialsForProviderUse(context.Background(), tenantID, account.ID, "gcp"); !errors.Is(err, ErrProviderMismatch) {
		t.Fatalf("provider mismatch error = %v", err)
	}
	repo.accounts[account.ID].EncryptionVersion++
	if _, err := service.CredentialsForUse(context.Background(), tenantID, account.ID); err == nil {
		t.Fatal("unsupported encryption version was accepted")
	}
}

func TestCloudAccountRejectsMalformedProviderCredentials(t *testing.T) {
	service, err := NewService(newMemoryRepo(), "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	tenantID := uuid.New()
	tests := []struct {
		name     string
		provider string
		payload  models.CloudCredentialPayload
	}{
		{name: "AWS partial key pair", provider: "aws", payload: models.CloudCredentialPayload{"aws_access_key_id": json.RawMessage(`"AKIAEXAMPLE"`)}},
		{name: "AWS mixed credential modes", provider: "aws", payload: models.CloudCredentialPayload{"aws_role_arn": json.RawMessage(`"arn:aws:iam::123456789012:role/Deploy"`), "aws_access_key_id": json.RawMessage(`"AKIAEXAMPLE"`), "aws_secret_access_key": json.RawMessage(`"secret"`)}},
		{name: "AWS foreign provider field", provider: "aws", payload: models.CloudCredentialPayload{"gcp_project_id": json.RawMessage(`"project"`)}},
		{name: "GCP invalid service account", provider: "gcp", payload: models.CloudCredentialPayload{"gcp_service_account_json": json.RawMessage(`"{}"`), "gcp_project_id": json.RawMessage(`"project"`)}},
		{name: "Azure missing secret", provider: "azure", payload: models.CloudCredentialPayload{"azure_tenant_id": json.RawMessage(`"tenant"`), "azure_client_id": json.RawMessage(`"client"`), "azure_subscription_id": json.RawMessage(`"subscription"`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Create(context.Background(), tenantID, models.CloudAccountCreate{Name: tt.name, Provider: tt.provider, Credentials: tt.payload}, "")
			if !errors.Is(err, ErrInvalidAccount) {
				t.Fatalf("Create() error = %v, want ErrInvalidAccount", err)
			}
		})
	}
}

func TestCloudAccountIdempotentConflictDoesNotCreateOrphan(t *testing.T) {
	repo := newMemoryRepo()
	service, err := NewService(repo, "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	tenantID := uuid.New()
	first := models.CloudAccountCreate{Name: "first", Provider: "gcp", Credentials: validGCPCredentials()}
	if _, err := service.Create(context.Background(), tenantID, first, "request-key"); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Name = "second"
	if _, err := service.Create(context.Background(), tenantID, second, "request-key"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("Create() error = %v, want conflict", err)
	}
	if len(repo.accounts) != 1 {
		t.Fatalf("idempotency conflict left %d accounts, want 1", len(repo.accounts))
	}
}

func validGCPCredentials() models.CloudCredentialPayload {
	serviceAccount := `{"type":"service_account","project_id":"project","client_email":"svc@example.invalid","private_key":"private"}`
	encoded, _ := json.Marshal(serviceAccount)
	return models.CloudCredentialPayload{
		"gcp_service_account_json": encoded,
		"gcp_project_id":           json.RawMessage(`"project"`),
	}
}
