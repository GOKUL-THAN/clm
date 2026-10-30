package dns01

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

type route53API interface {
	ListResourceRecordSets(context.Context, *route53.ListResourceRecordSetsInput, ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
	ChangeResourceRecordSets(context.Context, *route53.ChangeResourceRecordSetsInput, ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
}

// Route53Provider manages only the exact TXT value owned by one ACME
// authorization. It reads the current RRset before every change so concurrent
// apex/wildcard validations do not erase each other's values.
type Route53Provider struct {
	zoneID string
	client route53API
	// Route 53 has no compare-and-swap for RRsets. Serialize this provider's
	// read/merge/write sequence so concurrent ACME authorizations in one gateway
	// cannot overwrite each other's TXT values.
	mu sync.Mutex
}

func newRoute53Provider(cfg Config) (*Route53Provider, error) {
	zoneID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cfg.Zone), "/hostedzone/"))
	if zoneID == "" || strings.ContainsAny(zoneID, "\r\n\t /\\") {
		return nil, fmt.Errorf("Route 53 DNS-01 requires a hosted zone ID in zone")
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{}
	credentialRef := strings.TrimSpace(cfg.CredentialRef)
	switch {
	case credentialRef == "", credentialRef == "workload-identity://default":
		// Default SDK resolution supports EC2/ECS/EKS workload identities and
		// environment/shared configuration without moving secret bytes into CLM.
	case strings.HasPrefix(credentialRef, "aws-profile://"):
		profile := strings.TrimSpace(strings.TrimPrefix(credentialRef, "aws-profile://"))
		if profile == "" || strings.ContainsAny(profile, "\r\n\t /\\") {
			return nil, fmt.Errorf("Route 53 aws-profile credential reference is invalid")
		}
		loadOptions = append(loadOptions, awsconfig.WithSharedConfigProfile(profile))
	default:
		return nil, fmt.Errorf("Route 53 credential_ref must be empty, workload-identity://default, or aws-profile://<name>; raw credentials are forbidden")
	}

	loadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	awsCfg, err := awsconfig.LoadDefaultConfig(loadCtx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("loading Route 53 workload identity: %w", err)
	}
	return &Route53Provider{zoneID: zoneID, client: route53.NewFromConfig(awsCfg)}, nil
}

func (p *Route53Provider) Name() string { return ProviderRoute53 }

func (p *Route53Provider) Present(ctx context.Context, record Record) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	existing, err := p.current(ctx, record.FQDN)
	if err != nil {
		return err
	}
	values := append([]string(nil), existing.values...)
	if !containsTXT(values, record.Value) {
		values = append(values, record.Value)
	}
	ttl := int64(record.TTL)
	if existing.found && existing.ttl > 0 {
		ttl = existing.ttl
	}
	return p.change(ctx, types.ChangeActionUpsert, record.FQDN, ttl, values)
}

func (p *Route53Provider) Cleanup(ctx context.Context, record Record) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	existing, err := p.current(ctx, record.FQDN)
	if err != nil {
		return err
	}
	if !existing.found || !containsTXT(existing.values, record.Value) {
		return nil
	}
	remaining := make([]string, 0, len(existing.values)-1)
	for _, value := range existing.values {
		if value != record.Value {
			remaining = append(remaining, value)
		}
	}
	if len(remaining) == 0 {
		return p.change(ctx, types.ChangeActionDelete, record.FQDN, existing.ttl, existing.values)
	}
	return p.change(ctx, types.ChangeActionUpsert, record.FQDN, existing.ttl, remaining)
}

type route53RecordSet struct {
	found  bool
	ttl    int64
	values []string
}

func (p *Route53Provider) current(ctx context.Context, fqdn string) (route53RecordSet, error) {
	out, err := p.client.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(p.zoneID), StartRecordName: aws.String(fqdn),
		StartRecordType: types.RRTypeTxt, MaxItems: aws.Int32(1),
	})
	if err != nil {
		return route53RecordSet{}, fmt.Errorf("listing Route 53 TXT record %s: %w", fqdn, err)
	}
	if len(out.ResourceRecordSets) == 0 {
		return route53RecordSet{}, nil
	}
	set := out.ResourceRecordSets[0]
	if set.Type != types.RRTypeTxt || !strings.EqualFold(canonicalRecordName(aws.ToString(set.Name)), canonicalRecordName(fqdn)) {
		return route53RecordSet{}, nil
	}
	values := make([]string, 0, len(set.ResourceRecords))
	for _, resource := range set.ResourceRecords {
		value := aws.ToString(resource.Value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		values = append(values, value)
	}
	return route53RecordSet{found: true, ttl: aws.ToInt64(set.TTL), values: values}, nil
}

func (p *Route53Provider) change(ctx context.Context, action types.ChangeAction, fqdn string, ttl int64, values []string) error {
	if ttl <= 0 {
		ttl = 60
	}
	resources := make([]types.ResourceRecord, 0, len(values))
	for _, value := range values {
		quoted := strconv.Quote(value)
		resources = append(resources, types.ResourceRecord{Value: aws.String(quoted)})
	}
	_, err := p.client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(p.zoneID),
		ChangeBatch: &types.ChangeBatch{
			Comment: aws.String("CLM ACME DNS-01 challenge"),
			Changes: []types.Change{{
				Action: action,
				ResourceRecordSet: &types.ResourceRecordSet{
					Name: aws.String(fqdn), Type: types.RRTypeTxt, TTL: aws.Int64(ttl),
					ResourceRecords: resources,
				},
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("changing Route 53 TXT record %s: %w", fqdn, err)
	}
	return nil
}

func canonicalRecordName(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), ".")) + "."
}

func containsTXT(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
