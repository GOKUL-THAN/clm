package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

type awsSecretsAPI interface {
	CreateSecret(context.Context, *secretsmanager.CreateSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	GetSecretValue(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	PutSecretValue(context.Context, *secretsmanager.PutSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	DeleteSecret(context.Context, *secretsmanager.DeleteSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
	ListSecrets(context.Context, *secretsmanager.ListSecretsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
}

// AWSSecretsManager implements core.SecretsStore using the AWS SDK and the
// configured customer-managed KMS key.
type AWSSecretsManager struct {
	region   string
	kmsKeyID string
	prefix   string
	client   awsSecretsAPI
	logger   *zap.Logger
}

func NewAWSSecretsManager(region, kmsKeyID, prefix string, logger *zap.Logger) (*AWSSecretsManager, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return nil, fmt.Errorf("AWS Secrets Manager region is required")
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS configuration: %w", err)
	}
	return newAWSSecretsManagerWithClient(region, kmsKeyID, prefix, secretsmanager.NewFromConfig(cfg), logger), nil
}

func newAWSSecretsManagerWithClient(region, kmsKeyID, prefix string, client awsSecretsAPI, logger *zap.Logger) *AWSSecretsManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AWSSecretsManager{
		region: strings.TrimSpace(region), kmsKeyID: strings.TrimSpace(kmsKeyID),
		prefix: strings.Trim(strings.TrimSpace(prefix), "/"),
		client: client, logger: logger,
	}
}

func (s *AWSSecretsManager) Store(ctx context.Context, req *core.StoreSecretRequest) (*core.SecretRef, error) {
	if s.client == nil {
		return nil, fmt.Errorf("AWS Secrets Manager client is not configured")
	}
	if req == nil || req.Secret == nil {
		return nil, fmt.Errorf("secret request and secret payload are required")
	}
	name, err := s.buildSecretName(req.TenantID, req.Environment, req.Domain)
	if err != nil {
		return nil, err
	}
	// #nosec G117 -- AWS Secrets Manager encrypts this payload at rest; the configured KMS key is applied when present.
	data, err := json.Marshal(req.Secret)
	if err != nil {
		return nil, fmt.Errorf("marshaling secret: %w", err)
	}
	input := &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String(string(data)),
		Tags:         awsSecretTags(req.Tags),
	}
	if s.kmsKeyID != "" {
		input.KmsKeyId = aws.String(s.kmsKeyID)
	}
	output, err := s.client.CreateSecret(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("creating AWS secret %q: %w", name, err)
	}
	secretID := aws.ToString(output.ARN)
	if secretID == "" {
		secretID = aws.ToString(output.Name)
	}
	if secretID == "" {
		return nil, fmt.Errorf("AWS created secret %q without returning an identifier", name)
	}
	s.logger.Info("stored secret in AWS Secrets Manager", zap.String("secret_id", secretID))
	return &core.SecretRef{
		Backend: "aws_secrets_manager", SecretID: secretID,
		Version: aws.ToString(output.VersionId), Region: s.region,
	}, nil
}

func (s *AWSSecretsManager) Retrieve(ctx context.Context, ref *core.SecretRef) (*models.CertSecret, error) {
	if err := s.validateRef(ref); err != nil {
		return nil, err
	}
	input := &secretsmanager.GetSecretValueInput{SecretId: aws.String(ref.SecretID)}
	if ref.Version == "AWSCURRENT" {
		input.VersionStage = aws.String(ref.Version)
	} else if ref.Version != "" {
		input.VersionId = aws.String(ref.Version)
	}
	output, err := s.client.GetSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("retrieving AWS secret %q: %w", ref.SecretID, err)
	}
	value := aws.ToString(output.SecretString)
	if value == "" && len(output.SecretBinary) > 0 {
		value = string(output.SecretBinary)
	}
	if value == "" {
		return nil, fmt.Errorf("AWS secret %q has no secret value", ref.SecretID)
	}
	var secret models.CertSecret
	if err := json.Unmarshal([]byte(value), &secret); err != nil {
		return nil, fmt.Errorf("decoding AWS secret %q: %w", ref.SecretID, err)
	}
	return &secret, nil
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
		return nil, fmt.Errorf("unknown secret field %q", field)
	}
}

func (s *AWSSecretsManager) Update(ctx context.Context, ref *core.SecretRef, req *core.StoreSecretRequest) (*core.SecretRef, error) {
	if err := s.validateRef(ref); err != nil {
		return nil, err
	}
	if req == nil || req.Secret == nil {
		return nil, fmt.Errorf("secret request and secret payload are required")
	}
	// #nosec G117 -- AWS Secrets Manager encrypts this payload at rest; the existing secret retains its KMS configuration.
	data, err := json.Marshal(req.Secret)
	if err != nil {
		return nil, fmt.Errorf("marshaling secret: %w", err)
	}
	output, err := s.client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId: aws.String(ref.SecretID), SecretString: aws.String(string(data)),
	})
	if err != nil {
		return nil, fmt.Errorf("updating AWS secret %q: %w", ref.SecretID, err)
	}
	updated := *ref
	updated.Version = aws.ToString(output.VersionId)
	return &updated, nil
}

func (s *AWSSecretsManager) Delete(ctx context.Context, ref *core.SecretRef) error {
	if err := s.validateRef(ref); err != nil {
		return err
	}
	if _, err := s.client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId: aws.String(ref.SecretID), RecoveryWindowInDays: aws.Int64(7),
	}); err != nil {
		return fmt.Errorf("scheduling deletion of AWS secret %q: %w", ref.SecretID, err)
	}
	return nil
}

func (s *AWSSecretsManager) Disable(context.Context, *core.SecretRef) error {
	return fmt.Errorf("AWS Secrets Manager does not provide a safe disable operation; apply an explicit resource policy")
}

func (s *AWSSecretsManager) List(ctx context.Context, prefix string) ([]*core.SecretRef, error) {
	if s.client == nil {
		return nil, fmt.Errorf("AWS Secrets Manager client is not configured")
	}
	var out []*core.SecretRef
	var nextToken *string
	for {
		page, err := s.client.ListSecrets(ctx, &secretsmanager.ListSecretsInput{
			MaxResults: aws.Int32(100), NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("listing AWS secrets: %w", err)
		}
		for _, item := range page.SecretList {
			name := aws.ToString(item.Name)
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			secretID := aws.ToString(item.ARN)
			if secretID == "" {
				secretID = name
			}
			out = append(out, &core.SecretRef{
				Backend: "aws_secrets_manager", SecretID: secretID,
				Version: "AWSCURRENT", Region: s.region,
			})
		}
		nextToken = page.NextToken
		if nextToken == nil || *nextToken == "" {
			break
		}
	}
	return out, nil
}

func (s *AWSSecretsManager) validateRef(ref *core.SecretRef) error {
	if ref == nil || strings.TrimSpace(ref.SecretID) == "" {
		return fmt.Errorf("AWS secret reference is required")
	}
	if ref.Backend != "" && ref.Backend != "aws_secrets_manager" {
		return fmt.Errorf("secret reference backend %q is not AWS Secrets Manager", ref.Backend)
	}
	if ref.Region != "" && ref.Region != s.region {
		return fmt.Errorf("secret reference region %q does not match configured region %q", ref.Region, s.region)
	}
	if s.client == nil {
		return fmt.Errorf("AWS Secrets Manager client is not configured")
	}
	return nil
}

func (s *AWSSecretsManager) buildSecretName(tenantID, env, domain string) (string, error) {
	tenantID = secretNameSegment(tenantID)
	env = secretNameSegment(env)
	domain = strings.ReplaceAll(strings.TrimSpace(domain), "*", "wildcard")
	domain = secretNameSegment(strings.ReplaceAll(domain, ".", "-"))
	if tenantID == "" || env == "" || domain == "" {
		return "", fmt.Errorf("tenant, environment, and domain are required for an AWS secret name")
	}
	parts := []string{tenantID, env, domain}
	name := strings.Join(parts, "/")
	if s.prefix != "" {
		name = s.prefix + "/" + name
	}
	if len(name) > 512 {
		return "", fmt.Errorf("AWS secret name exceeds 512 characters")
	}
	return name, nil
}

func secretNameSegment(value string) string {
	var out strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			out.WriteRune(r)
		default:
			out.WriteByte('-')
		}
	}
	return strings.Trim(out.String(), "-._")
}

func awsSecretTags(tags map[string]string) []smtypes.Tag {
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]smtypes.Tag, 0, len(keys))
	for _, key := range keys {
		out = append(out, smtypes.Tag{Key: aws.String(key), Value: aws.String(tags[key])})
	}
	return out
}
