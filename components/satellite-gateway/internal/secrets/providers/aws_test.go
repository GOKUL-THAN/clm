package providers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

type fakeSecretsManager struct {
	createInput *secretsmanager.CreateSecretInput
	getInput    *secretsmanager.GetSecretValueInput
	value       string
	err         error
}

func (f *fakeSecretsManager) CreateSecret(_ context.Context, in *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	f.createInput = in
	if f.err != nil {
		return nil, f.err
	}
	return &secretsmanager.CreateSecretOutput{
		ARN:       aws.String("arn:aws:secretsmanager:us-east-1:123456789012:secret:clm/acme/prod/example-com-AbCd"),
		VersionId: aws.String("version-1"),
	}, nil
}

func (f *fakeSecretsManager) GetSecretValue(_ context.Context, in *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	f.getInput = in
	if f.err != nil {
		return nil, f.err
	}
	return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(f.value)}, nil
}

func (*fakeSecretsManager) PutSecretValue(context.Context, *secretsmanager.PutSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	return &secretsmanager.PutSecretValueOutput{VersionId: aws.String("version-2")}, nil
}

func (*fakeSecretsManager) DeleteSecret(context.Context, *secretsmanager.DeleteSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	return &secretsmanager.DeleteSecretOutput{}, nil
}

func (*fakeSecretsManager) ListSecrets(context.Context, *secretsmanager.ListSecretsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	return &secretsmanager.ListSecretsOutput{}, nil
}

func TestAWSSecretsManagerStoresWithConfiguredKMSKeyAndRetrieves(t *testing.T) {
	payload := &models.CertSecret{PrivateKey: "private-pem", Certificate: "certificate-pem"}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeSecretsManager{value: string(encoded)}
	store := newAWSSecretsManagerWithClient("us-east-1", "alias/clm-secrets", "clm", client, zap.NewNop())

	ref, err := store.Store(context.Background(), &core.StoreSecretRequest{
		TenantID: "tenant-1", Environment: "prod", Domain: "example.com",
		Secret: payload, Tags: map[string]string{"owner": "security"},
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if got := aws.ToString(client.createInput.KmsKeyId); got != "alias/clm-secrets" {
		t.Fatalf("CreateSecret KmsKeyId = %q", got)
	}
	if aws.ToString(client.createInput.Name) != "clm/tenant-1/prod/example-com" {
		t.Fatalf("CreateSecret name = %q", aws.ToString(client.createInput.Name))
	}
	if ref.Version != "version-1" || ref.Region != "us-east-1" {
		t.Fatalf("Store() reference = %#v", ref)
	}

	got, err := store.Retrieve(context.Background(), ref)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if got.PrivateKey != payload.PrivateKey || got.Certificate != payload.Certificate {
		t.Fatalf("Retrieve() payload = %#v", got)
	}
	if aws.ToString(client.getInput.VersionId) != ref.Version {
		t.Fatalf("GetSecretValue VersionId = %q", aws.ToString(client.getInput.VersionId))
	}
}

func TestAWSSecretsManagerDoesNotReportFailedStoreAsSuccess(t *testing.T) {
	client := &fakeSecretsManager{err: errors.New("access denied")}
	store := newAWSSecretsManagerWithClient("us-east-1", "", "", client, zap.NewNop())
	_, err := store.Store(context.Background(), &core.StoreSecretRequest{
		TenantID: "tenant-1", Environment: "prod", Domain: "example.com",
		Secret: &models.CertSecret{PrivateKey: "private-pem"},
	})
	if err == nil {
		t.Fatal("Store() unexpectedly reported success after AWS denied the operation")
	}
}
