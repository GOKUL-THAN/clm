package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/m2p/clm/internal/satellite/discovery/scanners"
	"go.uber.org/zap"
)

// ACMScanner discovers certificates from AWS Certificate Manager.
type ACMScanner struct {
	regions []string
	logger  *zap.Logger
}

// NewACMScanner creates a new ACM certificate scanner.
func NewACMScanner(regions []string, logger *zap.Logger) *ACMScanner {
	if len(regions) == 0 {
		regions = []string{"ap-south-1"}
	}
	return &ACMScanner{regions: regions, logger: logger}
}

// ScanAllRegions scans ACM certificates across all configured regions.
func (s *ACMScanner) ScanAllRegions(ctx context.Context) ([]*ACMCertificate, error) {
	var allCerts []*ACMCertificate

	for _, region := range s.regions {
		certs, err := s.ScanRegion(ctx, region)
		if err != nil {
			s.logger.Warn("ACM scan failed for region", zap.String("region", region), zap.Error(err))
			continue
		}
		allCerts = append(allCerts, certs...)
	}

	s.logger.Info("ACM discovery complete",
		zap.Int("total_certs", len(allCerts)),
		zap.Int("regions_scanned", len(s.regions)),
	)

	return allCerts, nil
}

// ScanRegion scans ACM certificates in a specific region.
func (s *ACMScanner) ScanRegion(ctx context.Context, region string) ([]*ACMCertificate, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	client := acm.NewFromConfig(cfg)

	// List all certificates
	var certs []*ACMCertificate
	var nextToken *string

	for {
		output, err := client.ListCertificates(ctx, &acm.ListCertificatesInput{
			NextToken: nextToken,
			Includes: &types.Filters{
				KeyTypes: []types.KeyAlgorithm{
					types.KeyAlgorithmRsa2048,
					types.KeyAlgorithmRsa4096,
					types.KeyAlgorithmEcPrime256v1,
					types.KeyAlgorithmEcSecp384r1,
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("listing ACM certs in %s: %w", region, err)
		}

		for _, summary := range output.CertificateSummaryList {
			cert, err := s.describeCert(ctx, client, region, aws.ToString(summary.CertificateArn))
			if err != nil {
				s.logger.Warn("failed to describe ACM cert",
					zap.String("arn", aws.ToString(summary.CertificateArn)),
					zap.Error(err),
				)
				continue
			}
			certs = append(certs, cert)
		}

		nextToken = output.NextToken
		if nextToken == nil {
			break
		}
	}

	s.logger.Info("ACM region scan complete",
		zap.String("region", region),
		zap.Int("certs_found", len(certs)),
	)

	return certs, nil
}

func (s *ACMScanner) describeCert(ctx context.Context, client *acm.Client, region, arn string) (*ACMCertificate, error) {
	output, err := client.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
		CertificateArn: aws.String(arn),
	})
	if err != nil {
		return nil, err
	}

	detail := output.Certificate
	fingerprint := sha256.Sum256([]byte(arn))

	var sans []string
	for _, san := range detail.SubjectAlternativeNames {
		sans = append(sans, san)
	}

	var inUseBy []string
	for _, resource := range detail.InUseBy {
		inUseBy = append(inUseBy, resource)
	}

	daysLeft := 0
	if detail.NotAfter != nil {
		daysLeft = int(time.Until(*detail.NotAfter).Hours() / 24)
	}

	keyAlgo := ""
	if detail.KeyAlgorithm != "" {
		keyAlgo = string(detail.KeyAlgorithm)
	}

	return &ACMCertificate{
		ARN:              arn,
		Domain:           aws.ToString(detail.DomainName),
		SANs:             sans,
		Status:           string(detail.Status),
		Type:             string(detail.Type),
		Issuer:           aws.ToString(detail.Issuer),
		KeyAlgorithm:     keyAlgo,
		Serial:           aws.ToString(detail.Serial),
		Fingerprint:      hex.EncodeToString(fingerprint[:]),
		NotBefore:        detail.NotBefore,
		NotAfter:         detail.NotAfter,
		DaysLeft:         daysLeft,
		RenewalEligible:  detail.RenewalEligibility == types.RenewalEligibilityEligible,
		InUseBy:          inUseBy,
		Region:           region,
		Source:           string(detail.Type), // AMAZON_ISSUED or IMPORTED
		ValidationMethod: getValidationMethod(detail),
		CreatedAt:        detail.CreatedAt,
		DiscoveredAt:     time.Now(),
	}, nil
}

func getValidationMethod(cert *types.CertificateDetail) string {
	if len(cert.DomainValidationOptions) > 0 {
		return string(cert.DomainValidationOptions[0].ValidationMethod)
	}
	return "NONE"
}

// ToDiscoveredCert converts an ACMCertificate to the generic DiscoveredCert format.
func (c *ACMCertificate) ToDiscoveredCert() *scanners.DiscoveredCert {
	var notBefore, notAfter time.Time
	if c.NotBefore != nil {
		notBefore = *c.NotBefore
	}
	if c.NotAfter != nil {
		notAfter = *c.NotAfter
	}

	return &scanners.DiscoveredCert{
		Host:         c.ARN,
		Port:         0,
		Subject:      c.Domain,
		Issuer:       c.Issuer,
		SANs:         c.SANs,
		SerialNumber: c.Serial,
		Fingerprint:  c.Fingerprint,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyAlgorithm: c.KeyAlgorithm,
		DaysLeft:     c.DaysLeft,
		Protocol:     "acm",
		ServerName:   c.Domain,
		SelfSigned:   false,
		ChainValid:   c.Status == "ISSUED",
		DiscoveredAt: c.DiscoveredAt,
	}
}

// ACMCertificate represents a certificate discovered in AWS ACM.
type ACMCertificate struct {
	ARN              string     `json:"arn"`
	Domain           string     `json:"domain"`
	SANs             []string   `json:"sans"`
	Status           string     `json:"status"`   // PENDING_VALIDATION, ISSUED, INACTIVE, REVOKED, FAILED, EXPIRED
	Type             string     `json:"type"`     // AMAZON_ISSUED, IMPORTED, PRIVATE
	Issuer           string     `json:"issuer"`
	KeyAlgorithm     string     `json:"key_algorithm"`
	Serial           string     `json:"serial"`
	Fingerprint      string     `json:"fingerprint"`
	NotBefore        *time.Time `json:"not_before"`
	NotAfter         *time.Time `json:"not_after"`
	DaysLeft         int        `json:"days_left"`
	RenewalEligible  bool       `json:"renewal_eligible"`
	InUseBy          []string   `json:"in_use_by"`       // ARNs of resources using this cert
	Region           string     `json:"region"`
	Source           string     `json:"source"`          // AMAZON_ISSUED or IMPORTED
	ValidationMethod string     `json:"validation_method"`
	CreatedAt        *time.Time `json:"created_at"`
	DiscoveredAt     time.Time  `json:"discovered_at"`
}
