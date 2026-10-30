package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// AWSSecretsManager implements core.SecretsStore for AWS Secrets Manager.
type AWSSecretsManager struct {
	region   string
	kmsKeyID string
	prefix   string
	logger   *zap.Logger
	// In production: aws-sdk-go-v2 secretsmanager.Client
}

// NewAWSSecretsManager creates a new AWS secrets store.
func NewAWSSecretsManager(region, kmsKeyID, prefix string, logger *zap.Logger) *AWSSecretsManager {
	return &AWSSecretsManager{
		region:   region,
		kmsKeyID: kmsKeyID,
		prefix:   prefix,
		logger:   logger,
	}
}

func (s *AWSSecretsManager) Store(ctx context.Context, req *core.StoreSecretRequest) (*core.SecretRef, error) {
	secretName := s.buildSecretName(req.TenantID, req.Environment, req.Domain)

	// Serialize to JSON
	jsonData, err := json.Marshal(req.Secret)
	if err != nil {
		return nil, fmt.Errorf("marshaling secret: %w", err)
	}

	// In production: call secretsmanager.CreateSecret or PutSecretValue
	// aws_client.CreateSecret(&secretsmanager.CreateSecretInput{
	//   Name:         &secretName,
	//   SecretString: aws.String(string(jsonData)),
	//   KmsKeyId:     &s.kmsKeyID,
	//   Tags:         convertTags(req.Tags),
	// })

	s.logger.Info("stored secret in AWS Secrets Manager",
		zap.String("name", secretName),
		zap.Int("size_bytes", len(jsonData)),
	)

	return &core.SecretRef{
		Backend:  "aws_secrets_manager",
		SecretID: secretName,
		Version:  "AWSCURRENT",
		Region:   s.region,
	}, nil
}

func (s *AWSSecretsManager) Retrieve(ctx context.Context, ref *core.SecretRef) (*models.CertSecret, error) {
	// In production: call secretsmanager.GetSecretValue
	// result, err := aws_client.GetSecretValue(&secretsmanager.GetSecretValueInput{
	//   SecretId:     &ref.SecretID,
	//   VersionStage: aws.String("AWSCURRENT"),
	// })

	s.logger.Info("retrieving secret from AWS", zap.String("id", ref.SecretID))

	// Placeholder — real impl would parse result.SecretString
	return nil, fmt.Errorf("AWS SDK call required: GetSecretValue(%s)", ref.SecretID)
}

func (s *AWSSecretsManager) RetrieveField(ctx context.Context, ref *core.SecretRef, field string) ([]byte, error) {
	secret, err := s.Retrieve(ctx, ref)
	if err != nil {
		return nil, err
	}

	switch field {
	case "private_key":
		return []byte(secret.PrivateKey), nil
	case "certificate":
		return []byte(secret.Certificate), nil
	case "chain":
		return []byte(secret.Chain), nil
	case "fullchain":
		return []byte(secret.FullChain), nil
	case "csr":
		return []byte(secret.CSR), nil
	case "pfx":
		return []byte(secret.PFX), nil
	default:
		return nil, fmt.Errorf("unknown field: %s", field)
	}
}

func (s *AWSSecretsManager) Update(ctx context.Context, ref *core.SecretRef, req *core.StoreSecretRequest) (*core.SecretRef, error) {
	// In production: PutSecretValue (creates new version)
	s.logger.Info("updating secret in AWS", zap.String("id", ref.SecretID))
	return ref, nil
}

func (s *AWSSecretsManager) Delete(ctx context.Context, ref *core.SecretRef) error {
	// In production: DeleteSecret with recovery window
	s.logger.Info("deleting secret from AWS", zap.String("id", ref.SecretID))
	return nil
}

func (s *AWSSecretsManager) Disable(ctx context.Context, ref *core.SecretRef) error {
	// In production: attach deny policy to block all access
	s.logger.Warn("disabling secret access", zap.String("id", ref.SecretID))
	return nil
}

func (s *AWSSecretsManager) List(ctx context.Context, prefix string) ([]*core.SecretRef, error) {
	// In production: ListSecrets with filter
	return nil, nil
}

func (s *AWSSecretsManager) buildSecretName(tenantID, env, domain string) string {
	// Normalize domain for secret name
	normalized := strings.ReplaceAll(domain, ".", "-")
	normalized = strings.ReplaceAll(normalized, "*", "wildcard")
	return fmt.Sprintf("%s%s/%s/%s", s.prefix, tenantID, env, normalized)
}
