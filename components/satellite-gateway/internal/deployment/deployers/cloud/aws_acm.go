package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"go.uber.org/zap"
)

// AWS certificate deployment.
//
// Importing a certificate into ACM and putting it in front of traffic on a load
// balancer are two separate decisions, and rotating a certificate on a listener
// is a third. The previous implementation collapsed all three into one path:
// import into ACM, then — only if target.Host happened to hold a listener ARN —
// call ModifyListener with a single certificate. That was wrong in three ways.
//
//   - target.ResourceID was passed as ImportCertificate's CertificateArn while
//     the target schema documents that field as the listener ARN, so a request
//     built from the schema asked ACM to re-import over a load balancer ARN.
//   - ModifyListener sets only the listener's *default* certificate and detaches
//     the one it replaces. There was no way to keep the outgoing certificate
//     attached, so there was no way back other than deploying again.
//   - The access key, secret and session token that the schema advertises were
//     never read. Every deployment silently used the gateway's own instance
//     role, which is not what an operator who filled those fields in expects.
//
// The three decisions are now explicit and separate: where to apply
// (ApplyTo), what to do on the ACM side (VersionPolicy) and what to do on the
// listener (ListenerPolicy).

// ApplyTo selects which AWS surfaces a deployment touches.
const (
	// ApplyACM imports into ACM and stops. Nothing serves the certificate until
	// something is pointed at it.
	ApplyACM = "acm"
	// ApplyALB attaches an ACM certificate that already exists to a listener,
	// importing nothing. Requires certificate_arn.
	ApplyALB = "alb"
	// ApplyBoth imports into ACM and then attaches the result to a listener.
	ApplyBoth = "acm+alb"
)

// VersionPolicy selects what happens to the ACM certificate itself.
const (
	// VersionNew imports as a brand-new ACM certificate with its own ARN and
	// leaves earlier ones in place. Anything pinned to the old ARN keeps working,
	// which is what makes a staged rollout and a fast rollback possible.
	VersionNew = "new-version"
	// VersionReplace re-imports over an existing ARN. ACM keeps one certificate
	// and everything referencing that ARN picks up the new material with no
	// repointing — and no way back, because the old material is gone.
	VersionReplace = "replace-in-place"
)

// ListenerPolicy selects what happens on the ELBv2 listener.
const (
	// ListenerReplace makes the new certificate the listener default and lets the
	// outgoing default be detached. Ends with only the new certificate in play.
	ListenerReplace = "replace"
	// ListenerKeepPrevious makes the new certificate the default but first moves
	// the outgoing one into the listener's certificate list, so reverting is a
	// single ModifyListener call. This is what Rollback uses.
	ListenerKeepPrevious = "keep-previous"
	// ListenerAddOnly adds to the listener's certificate list and leaves the
	// default alone. For putting an additional name on a multi-domain listener
	// rather than rotating an existing one.
	ListenerAddOnly = "add-only"
)

const (
	tagManaged     = "clm-managed"
	tagDomain      = "clm-domain"
	tagImportedAt  = "clm-imported-at"
	tagFingerprint = "clm-fingerprint"
)

// AWSACMDeployer handles importing certificates to ACM and binding them to AWS resources.
type AWSACMDeployer struct {
	region string
	logger *zap.Logger
}

// NewAWSACMDeployer creates a new AWS ACM deployer.
func NewAWSACMDeployer(region string, logger *zap.Logger) *AWSACMDeployer {
	return &AWSACMDeployer{region: region, logger: logger}
}

func (d *AWSACMDeployer) Name() string     { return "aws-acm" }
func (d *AWSACMDeployer) Category() string { return "cloud" }

// awsPlan is the resolved, validated set of choices for one deployment.
type awsPlan struct {
	applyTo        string
	versionPolicy  string
	listenerPolicy string
	listenerARN    string
	certificateARN string
	retain         int
}

// resolvePlan fills in defaults and refuses combinations that cannot work.
//
// Defaults are chosen to be the least destructive option that still ends with
// the new certificate serving traffic: a new ACM certificate rather than
// overwriting one, and the outgoing listener certificate kept attached so a
// rollback does not need another deployment.
func resolvePlan(target *deployers.DeployTarget) (awsPlan, error) {
	p := awsPlan{
		applyTo:        strings.ToLower(strings.TrimSpace(target.ApplyTo)),
		versionPolicy:  strings.ToLower(strings.TrimSpace(target.VersionPolicy)),
		listenerPolicy: strings.ToLower(strings.TrimSpace(target.ListenerPolicy)),
		retain:         target.RetainVersions,
	}

	// The listener ARN moved to its own field. ResourceID and Host are still
	// accepted because the previous schema documented ResourceID as the listener
	// ARN and the previous code read it from Host, so requests exist in both shapes.
	p.listenerARN = firstNonEmpty(target.ListenerARN, target.ResourceID, target.Host)
	if !strings.Contains(p.listenerARN, ":listener") {
		// A value that is not a listener ARN is almost certainly an ACM ARN that
		// landed in the wrong field, or a hostname. Either way, binding it would
		// fail deep inside ELBv2 with an unhelpful message.
		if strings.Contains(p.listenerARN, ":certificate/") {
			p.certificateARN = firstNonEmpty(target.CertificateARN, p.listenerARN)
			p.listenerARN = ""
		} else if p.listenerARN != "" && !strings.HasPrefix(p.listenerARN, "arn:") {
			p.listenerARN = ""
		}
	}
	p.certificateARN = firstNonEmpty(target.CertificateARN, p.certificateARN)

	if p.applyTo == "" {
		if p.listenerARN != "" {
			p.applyTo = ApplyBoth
		} else {
			p.applyTo = ApplyACM
		}
	}
	switch p.applyTo {
	case ApplyACM, ApplyALB, ApplyBoth:
	default:
		return p, fmt.Errorf("apply_to must be %q, %q or %q, not %q",
			ApplyACM, ApplyALB, ApplyBoth, p.applyTo)
	}

	if p.versionPolicy == "" {
		p.versionPolicy = VersionNew
	}
	switch p.versionPolicy {
	case VersionNew, VersionReplace:
	default:
		return p, fmt.Errorf("version_policy must be %q or %q, not %q",
			VersionNew, VersionReplace, p.versionPolicy)
	}

	if p.listenerPolicy == "" {
		p.listenerPolicy = ListenerKeepPrevious
	}
	switch p.listenerPolicy {
	case ListenerReplace, ListenerKeepPrevious, ListenerAddOnly:
	default:
		return p, fmt.Errorf("listener_policy must be %q, %q or %q, not %q",
			ListenerReplace, ListenerKeepPrevious, ListenerAddOnly, p.listenerPolicy)
	}

	// Cross-field requirements. These cannot be expressed in the static field
	// schema, so they are checked here rather than failing at the API.
	if p.applyTo != ApplyACM && p.listenerARN == "" {
		return p, fmt.Errorf(
			"apply_to %q needs listener_arn (an arn:aws:elasticloadbalancing:...:listener/... value)",
			p.applyTo)
	}
	if p.applyTo == ApplyALB && p.certificateARN == "" {
		return p, fmt.Errorf(
			"apply_to %q attaches an existing certificate, so certificate_arn is required; "+
				"use %q to import this certificate first", ApplyALB, ApplyBoth)
	}
	if p.versionPolicy == VersionReplace && p.applyTo != ApplyALB && p.certificateARN == "" {
		return p, fmt.Errorf(
			"version_policy %q re-imports over an existing certificate, so certificate_arn "+
				"is required; use %q to create one", VersionReplace, VersionNew)
	}
	if p.retain < 0 {
		p.retain = 0
	}
	return p, nil
}

// Deploy imports a certificate to ACM and/or binds one to an ELBv2 listener.
func (d *AWSACMDeployer) Deploy(ctx context.Context, req *deployers.DeployRequest) (*deployers.DeployResult, error) {
	start := time.Now()
	target := req.Target

	fail := func(format string, args ...any) (*deployers.DeployResult, error) {
		err := fmt.Errorf(format, args...)
		return &deployers.DeployResult{
			Success: false, Target: target, Error: err.Error(),
			Duration: time.Since(start),
		}, err
	}

	plan, err := resolvePlan(target)
	if err != nil {
		return fail("%s", err.Error())
	}

	cfg, err := d.awsConfig(ctx, target)
	if err != nil {
		return fail("%s", err.Error())
	}
	acmClient := acm.NewFromConfig(cfg)

	var (
		notes               []string
		certARN             = plan.certificateARN
		domain              = leafDomain(req.CertPEM)
		pruned              []string
		previous            string
		previousFingerprint string
	)

	// --- ACM side ---
	if plan.applyTo == ApplyACM || plan.applyTo == ApplyBoth {
		// ACM's Certificate field takes exactly one certificate. Callers routinely
		// pass a fullchain here — the deploy handler sets cert_pem and chain_pem to
		// the same fullchain for a certificate this gateway issued — so the leaf is
		// split out rather than trusting the caller to have done it.
		leaf := leafPEM(req.CertPEM)
		if len(leaf) == 0 {
			return fail("cert_pem contains no CERTIFICATE block")
		}
		in := &acm.ImportCertificateInput{
			Certificate: leaf,
			PrivateKey:  req.KeyPEM,
		}
		// CertificateChain is optional, but ACM rejects it as a validation error
		// when present and empty rather than treating it as absent. A certificate
		// with no intermediates — self-signed, or issued directly by a root — is a
		// legitimate thing to import.
		//
		// The chain must also not repeat the leaf: callers here often pass a
		// fullchain, and ACM refuses a chain whose first certificate is the leaf.
		if chain := chainWithoutLeaf(req.ChainPEM, req.CertPEM); len(chain) > 0 {
			in.CertificateChain = chain
		}
		if plan.versionPolicy == VersionReplace {
			in.CertificateArn = aws.String(plan.certificateARN)
		} else {
			// Tags are only accepted when creating; a re-import over an existing ARN
			// rejects them. They are what makes version management possible later,
			// so they go on at creation time.
			in.Tags = []types.Tag{
				{Key: aws.String(tagManaged), Value: aws.String("true")},
				{Key: aws.String(tagImportedAt), Value: aws.String(time.Now().UTC().Format(time.RFC3339))},
			}
			if domain != "" {
				in.Tags = append(in.Tags, types.Tag{Key: aws.String(tagDomain), Value: aws.String(domain)})
			}
			if fp := leafFingerprint(req.CertPEM); fp != "" {
				in.Tags = append(in.Tags, types.Tag{Key: aws.String(tagFingerprint), Value: aws.String(fp)})
			}
		}

		out, err := acmClient.ImportCertificate(ctx, in)
		if err != nil {
			return fail("ACM import failed: %v", err)
		}
		certARN = aws.ToString(out.CertificateArn)

		if plan.versionPolicy == VersionReplace {
			notes = append(notes, fmt.Sprintf("re-imported in place over %s", shortARN(certARN)))
			// The tags on an existing certificate are not refreshed by a re-import,
			// so record the rotation explicitly. Best-effort: a missing tag must not
			// fail a deployment that has already succeeded.
			d.tagRotation(ctx, acmClient, certARN, domain, req.CertPEM)
		} else {
			notes = append(notes, fmt.Sprintf("imported as new certificate %s", shortARN(certARN)))
		}
		d.logger.Info("certificate imported to ACM",
			zap.String("arn", certARN), zap.String("region", cfg.Region),
			zap.String("version_policy", plan.versionPolicy), zap.String("domain", domain))
	}

	// Verify() only receives the target, so record what was just deployed on it.
	// The executor calls Verify with this same pointer immediately afterwards.
	target.CertificateARN = certARN

	// --- listener side ---
	if plan.applyTo == ApplyALB || plan.applyTo == ApplyBoth {
		elbClient := elbv2.NewFromConfig(cfg)

		current, defaultARN, err := listenerCertificates(ctx, elbClient, plan.listenerARN)
		if err != nil {
			return fail("certificate %s is in ACM but reading listener %s failed: %v",
				shortARN(certARN), shortARN(plan.listenerARN), err)
		}
		previous = defaultARN
		if defaultARN != "" {
			if old, getErr := acmClient.GetCertificate(ctx, &acm.GetCertificateInput{
				CertificateArn: aws.String(defaultARN),
			}); getErr == nil {
				previousFingerprint = leafFingerprint([]byte(aws.ToString(old.Certificate)))
			} else {
				d.logger.Warn("could not capture outgoing listener certificate fingerprint",
					zap.String("certificate_arn", defaultARN), zap.Error(getErr))
			}
		}

		switch plan.listenerPolicy {
		case ListenerAddOnly:
			if containsARN(current, certARN) {
				notes = append(notes, "already attached to the listener")
			} else if err := addListenerCert(ctx, elbClient, plan.listenerARN, certARN); err != nil {
				return fail("certificate %s is in ACM but attaching it to listener %s failed: %v",
					shortARN(certARN), shortARN(plan.listenerARN), err)
			} else {
				notes = append(notes, "added to the listener certificate list; the default is unchanged")
			}

		case ListenerKeepPrevious, ListenerReplace:
			if defaultARN == certARN {
				notes = append(notes, "already the listener default")
				break
			}
			// Keeping the outgoing certificate means attaching it to the list
			// before the default moves, because changing the default detaches the
			// certificate it replaces.
			if plan.listenerPolicy == ListenerKeepPrevious && defaultARN != "" {
				if err := addListenerCert(ctx, elbClient, plan.listenerARN, defaultARN); err != nil {
					return fail("certificate %s is in ACM but preserving the outgoing "+
						"certificate %s on listener %s failed, so the default was left alone: %v",
						shortARN(certARN), shortARN(defaultARN), shortARN(plan.listenerARN), err)
				}
			}
			if _, err := elbClient.ModifyListener(ctx, &elbv2.ModifyListenerInput{
				ListenerArn:  aws.String(plan.listenerARN),
				Certificates: []elbv2types.Certificate{{CertificateArn: aws.String(certARN)}},
			}); err != nil {
				return fail("certificate %s is in ACM but making it the default on listener %s failed: %v",
					shortARN(certARN), shortARN(plan.listenerARN), err)
			}
			if plan.listenerPolicy == ListenerKeepPrevious && defaultARN != "" {
				notes = append(notes, fmt.Sprintf(
					"listener default moved to %s; %s stays attached for rollback",
					shortARN(certARN), shortARN(defaultARN)))
			} else {
				notes = append(notes, fmt.Sprintf("listener default set to %s", shortARN(certARN)))
				if defaultARN != "" {
					notes = append(notes, fmt.Sprintf("%s detached", shortARN(defaultARN)))
				}
			}
		}

		d.logger.Info("certificate applied to ELBv2 listener",
			zap.String("cert_arn", certARN), zap.String("listener_arn", plan.listenerARN),
			zap.String("listener_policy", plan.listenerPolicy),
			zap.String("previous_default", defaultARN))
	}

	// --- retention ---
	// Only meaningful for new-version imports, which are what accumulate.
	if plan.retain > 0 && plan.versionPolicy == VersionNew && domain != "" {
		var perr error
		pruned, perr = d.pruneOldVersions(ctx, acmClient, domain, certARN, plan.retain)
		if perr != nil {
			// Retention is housekeeping. Failing the deployment over it would be
			// wrong: the certificate is live.
			notes = append(notes, "retention pass failed: "+perr.Error())
			d.logger.Warn("ACM retention pass failed",
				zap.String("domain", domain), zap.Error(perr))
		} else if len(pruned) > 0 {
			notes = append(notes, fmt.Sprintf("deleted %d superseded certificate(s) beyond the %d kept",
				len(pruned), plan.retain))
		}
	}

	return &deployers.DeployResult{
		Success:             true,
		Target:              target,
		PreviousCert:        previous,
		PreviousFingerprint: previousFingerprint,
		Message:             strings.Join(notes, "; "),
		Duration:            time.Since(start),
	}, nil
}

// awsConfig builds an AWS config for one target.
//
// Static keys are used when supplied, then an assumed role when a role ARN is
// given, and otherwise the gateway's ambient credentials. The schema has always
// offered these fields; until now nothing read them, so a deployment quietly ran
// as the gateway no matter what was filled in.
func (d *AWSACMDeployer) awsConfig(ctx context.Context, target *deployers.DeployTarget) (aws.Config, error) {
	region := firstNonEmpty(target.Region, d.region)
	if region == "" {
		return aws.Config{}, fmt.Errorf("region is required")
	}
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}

	creds := target.Credentials
	key := strings.TrimSpace(creds["access_key_id"])
	secret := strings.TrimSpace(creds["secret_access_key"])
	sessionToken := strings.TrimSpace(creds["session_token"])
	roleARN := strings.TrimSpace(creds["role_arn"])
	externalID := strings.TrimSpace(creds["external_id"])

	switch {
	case key != "" && secret != "":
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(key, secret, sessionToken)))
	case key != "" || secret != "":
		return aws.Config{}, fmt.Errorf(
			"access_key_id and secret_access_key must be given together; " +
				"leave both empty to use the gateway's own role")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("loading AWS config: %w", err)
	}

	if roleARN != "" {
		// Cross-account deployment: the certificate lives in one account and the
		// load balancer in another.
		stsClient := sts.NewFromConfig(cfg)
		provider := stscreds.NewAssumeRoleProvider(stsClient, roleARN, func(o *stscreds.AssumeRoleOptions) {
			o.RoleSessionName = "clm-gateway-deploy"
			if externalID != "" {
				o.ExternalID = aws.String(externalID)
			}
		})
		cfg.Credentials = aws.NewCredentialsCache(provider)
	}
	return cfg, nil
}

// listenerCertificates returns every certificate on a listener plus the ARN of
// the default one.
func listenerCertificates(ctx context.Context, c *elbv2.Client, listenerARN string) ([]string, string, error) {
	var (
		all       []string
		defaultAR string
		marker    *string
	)
	for {
		out, err := c.DescribeListenerCertificates(ctx, &elbv2.DescribeListenerCertificatesInput{
			ListenerArn: aws.String(listenerARN),
			Marker:      marker,
		})
		if err != nil {
			return nil, "", err
		}
		for _, cert := range out.Certificates {
			arn := aws.ToString(cert.CertificateArn)
			all = append(all, arn)
			if cert.IsDefault != nil && *cert.IsDefault {
				defaultAR = arn
			}
		}
		if out.NextMarker == nil || aws.ToString(out.NextMarker) == "" {
			break
		}
		marker = out.NextMarker
	}
	return all, defaultAR, nil
}

func addListenerCert(ctx context.Context, c *elbv2.Client, listenerARN, certARN string) error {
	_, err := c.AddListenerCertificates(ctx, &elbv2.AddListenerCertificatesInput{
		ListenerArn:  aws.String(listenerARN),
		Certificates: []elbv2types.Certificate{{CertificateArn: aws.String(certARN)}},
	})
	return err
}

// managedCert is one CLM-imported ACM certificate for a domain.
type managedCert struct {
	arn        string
	importedAt time.Time
	inUseBy    int
}

// pruneOldVersions deletes CLM-managed certificates for a domain beyond the most
// recent retain, newest first, skipping the one just deployed and anything AWS
// still reports as in use.
//
// A certificate that something is still pointing at must not be deleted: ACM
// refuses, but more importantly the operator's intent when keeping old versions
// is exactly that those references keep working.
func (d *AWSACMDeployer) pruneOldVersions(ctx context.Context, c *acm.Client,
	domain, keepARN string, retain int) ([]string, error) {

	candidates, err := d.managedCertsFor(ctx, c, domain)
	if err != nil {
		return nil, err
	}

	doomed, skippedInUse := selectForDeletion(candidates, keepARN, retain)
	for _, arn := range skippedInUse {
		d.logger.Info("keeping a superseded ACM certificate because it is still in use",
			zap.String("arn", arn))
	}

	var deleted []string
	for _, arn := range doomed {
		if _, err := c.DeleteCertificate(ctx, &acm.DeleteCertificateInput{
			CertificateArn: aws.String(arn),
		}); err != nil {
			// Report and stop rather than churning through deletions after the
			// first refusal.
			return deleted, fmt.Errorf("deleting %s: %w", shortARN(arn), err)
		}
		deleted = append(deleted, arn)
		d.logger.Info("deleted superseded ACM certificate",
			zap.String("arn", arn), zap.String("domain", domain))
	}
	return deleted, nil
}

// selectForDeletion decides which superseded certificates may go.
//
// Kept separate from the AWS calls because getting this wrong deletes a
// certificate somebody is relying on, and that is not something to find out in
// production. Returns the ARNs to delete, newest-first order preserved, and the
// ones skipped because AWS still reports them as in use.
//
// The certificate just deployed is never a candidate and does not consume a
// retention slot — retain counts how many *superseded* versions to keep.
func selectForDeletion(candidates []managedCert, keepARN string, retain int) (doomed, inUse []string) {
	ordered := make([]managedCert, len(candidates))
	copy(ordered, candidates)
	// Newest first. Ties break on ARN so the outcome does not depend on the order
	// AWS happened to list them in.
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].importedAt.Equal(ordered[j].importedAt) {
			return ordered[i].arn < ordered[j].arn
		}
		return ordered[i].importedAt.After(ordered[j].importedAt)
	})

	kept := 0
	for _, cand := range ordered {
		if cand.arn == keepARN {
			continue
		}
		if kept < retain {
			kept++
			continue
		}
		if cand.inUseBy > 0 {
			inUse = append(inUse, cand.arn)
			continue
		}
		doomed = append(doomed, cand.arn)
	}
	return doomed, inUse
}

// managedCertsFor lists the CLM-managed ACM certificates covering a domain.
func (d *AWSACMDeployer) managedCertsFor(ctx context.Context, c *acm.Client, domain string) ([]managedCert, error) {
	var (
		out      []managedCert
		nextTok  *string
		examined int
	)
	const maxExamined = 500

	for {
		page, err := c.ListCertificates(ctx, &acm.ListCertificatesInput{NextToken: nextTok})
		if err != nil {
			return nil, fmt.Errorf("listing ACM certificates: %w", err)
		}
		for _, sum := range page.CertificateSummaryList {
			if examined >= maxExamined {
				break
			}
			// Filter on the domain before spending a tag call per certificate.
			if !strings.EqualFold(aws.ToString(sum.DomainName), domain) {
				continue
			}
			examined++
			arn := aws.ToString(sum.CertificateArn)

			tags, err := c.ListTagsForCertificate(ctx, &acm.ListTagsForCertificateInput{
				CertificateArn: aws.String(arn),
			})
			if err != nil {
				d.logger.Warn("could not read ACM certificate tags; skipping it for retention",
					zap.String("arn", arn), zap.Error(err))
				continue
			}
			managed := false
			var importedAt time.Time
			for _, t := range tags.Tags {
				switch aws.ToString(t.Key) {
				case tagManaged:
					managed = strings.EqualFold(aws.ToString(t.Value), "true")
				case tagImportedAt:
					if ts, perr := time.Parse(time.RFC3339, aws.ToString(t.Value)); perr == nil {
						importedAt = ts
					}
				}
			}
			if !managed {
				// Never delete a certificate this gateway did not import.
				continue
			}

			desc, err := c.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
				CertificateArn: aws.String(arn),
			})
			if err != nil {
				d.logger.Warn("could not describe ACM certificate; skipping it for retention",
					zap.String("arn", arn), zap.Error(err))
				continue
			}
			inUse := 0
			if desc.Certificate != nil {
				inUse = len(desc.Certificate.InUseBy)
				if importedAt.IsZero() && desc.Certificate.ImportedAt != nil {
					importedAt = *desc.Certificate.ImportedAt
				}
			}
			out = append(out, managedCert{arn: arn, importedAt: importedAt, inUseBy: inUse})
		}
		if page.NextToken == nil || examined >= maxExamined {
			break
		}
		nextTok = page.NextToken
	}
	return out, nil
}

// tagRotation records a re-import on an existing certificate, best effort.
func (d *AWSACMDeployer) tagRotation(ctx context.Context, c *acm.Client, arn, domain string, certPEM []byte) {
	tags := []types.Tag{
		{Key: aws.String(tagManaged), Value: aws.String("true")},
		{Key: aws.String(tagImportedAt), Value: aws.String(time.Now().UTC().Format(time.RFC3339))},
	}
	if domain != "" {
		tags = append(tags, types.Tag{Key: aws.String(tagDomain), Value: aws.String(domain)})
	}
	if fp := leafFingerprint(certPEM); fp != "" {
		tags = append(tags, types.Tag{Key: aws.String(tagFingerprint), Value: aws.String(fp)})
	}
	if _, err := c.AddTagsToCertificate(ctx, &acm.AddTagsToCertificateInput{
		CertificateArn: aws.String(arn), Tags: tags,
	}); err != nil {
		d.logger.Warn("could not tag the re-imported ACM certificate",
			zap.String("arn", arn), zap.Error(err))
	}
}

// Verify confirms provider state and returns adapter-specific evidence. The
// executor separately observes the traffic endpoint whenever a listener is used.
func (d *AWSACMDeployer) Verify(ctx context.Context, req *deployers.DeployRequest, applied *deployers.DeployResult) (*deployers.VerificationEvidence, error) {
	target := req.Target
	evidence := &deployers.VerificationEvidence{
		Kind: "adapter", Method: "aws-acm-api", Adapter: d.Name(), CheckedAt: time.Now().UTC(),
	}
	fail := func(err error) (*deployers.VerificationEvidence, error) {
		evidence.Message = err.Error()
		return evidence, err
	}

	plan, err := resolvePlan(target)
	if err != nil {
		return fail(err)
	}
	certARN := firstNonEmpty(target.CertificateARN, plan.certificateARN)
	evidence.Resource = certARN
	if certARN == "" {
		return fail(fmt.Errorf("no ACM certificate ARN to verify"))
	}

	cfg, err := d.awsConfig(ctx, target)
	if err != nil {
		return fail(err)
	}

	desc, err := acm.NewFromConfig(cfg).DescribeCertificate(ctx, &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certARN),
	})
	if err != nil {
		return fail(err)
	}
	if desc.Certificate == nil || desc.Certificate.Status != types.CertificateStatusIssued {
		status := "unknown"
		if desc.Certificate != nil {
			status = string(desc.Certificate.Status)
		}
		return fail(fmt.Errorf("ACM reports certificate %s as %s, not ISSUED",
			shortARN(certARN), status))
	}

	if plan.applyTo == ApplyACM || plan.listenerARN == "" {
		evidence.Success = true
		evidence.Message = "ACM reports the imported certificate as ISSUED; no listener was requested"
		return evidence, nil
	}

	evidence.Method = "aws-acm-elbv2-api"
	evidence.Resource = plan.listenerARN
	attached, defaultARN, err := listenerCertificates(ctx, elbv2.NewFromConfig(cfg), plan.listenerARN)
	if err != nil {
		return fail(err)
	}
	if !containsARN(attached, certARN) {
		return fail(fmt.Errorf("certificate %s is issued but not attached to listener %s",
			shortARN(certARN), shortARN(plan.listenerARN)))
	}
	if plan.listenerPolicy != ListenerAddOnly && defaultARN != certARN {
		return fail(fmt.Errorf(
			"certificate %s is attached to listener %s but the default is still %s",
			shortARN(certARN), shortARN(plan.listenerARN), shortARN(defaultARN)))
	}
	evidence.Success = true
	evidence.Message = "ACM certificate is issued and attached to the requested ELBv2 listener; serving state checked separately"
	return evidence, nil
}

// Rollback puts the previously attached certificate back in front of traffic.
//
// This used to call DeleteCertificate, which is not a rollback: it destroyed a
// certificate and left the listener pointing at the one being rolled back. For
// imported material that is unrecoverable, because the private key cannot be
// re-imported from AWS.
//
// A real revert needs somewhere to revert to, which is what
// listener_policy=keep-previous is for. When the outgoing certificate was kept
// attached, this promotes the newest other CLM-managed certificate on the
// listener back to default.
func (d *AWSACMDeployer) Rollback(ctx context.Context, target *deployers.DeployTarget) error {
	plan, err := resolvePlan(target)
	if err != nil {
		return err
	}
	if plan.listenerARN == "" {
		return fmt.Errorf(
			"nothing to roll back: this target has no listener, and an ACM re-import " +
				"cannot be undone because the previous private key is not recoverable from AWS")
	}

	cfg, err := d.awsConfig(ctx, target)
	if err != nil {
		return err
	}
	elbClient := elbv2.NewFromConfig(cfg)

	attached, defaultARN, err := listenerCertificates(ctx, elbClient, plan.listenerARN)
	if err != nil {
		return err
	}

	acmClient := acm.NewFromConfig(cfg)
	var (
		best     string
		bestTime time.Time
	)
	for _, arn := range attached {
		if arn == defaultARN {
			continue
		}
		desc, derr := acmClient.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
			CertificateArn: aws.String(arn),
		})
		if derr != nil || desc.Certificate == nil {
			continue
		}
		if desc.Certificate.Status != types.CertificateStatusIssued {
			continue
		}
		var when time.Time
		if desc.Certificate.ImportedAt != nil {
			when = *desc.Certificate.ImportedAt
		} else if desc.Certificate.IssuedAt != nil {
			when = *desc.Certificate.IssuedAt
		}
		if best == "" || when.After(bestTime) {
			best, bestTime = arn, when
		}
	}
	if best == "" {
		return fmt.Errorf(
			"listener %s has no other issued certificate to fall back to; "+
				"deploy with listener_policy=%q to keep the outgoing certificate attached",
			shortARN(plan.listenerARN), ListenerKeepPrevious)
	}

	if _, err := elbClient.ModifyListener(ctx, &elbv2.ModifyListenerInput{
		ListenerArn:  aws.String(plan.listenerARN),
		Certificates: []elbv2types.Certificate{{CertificateArn: aws.String(best)}},
	}); err != nil {
		return fmt.Errorf("restoring %s as the default on listener %s: %w",
			shortARN(best), shortARN(plan.listenerARN), err)
	}
	d.logger.Info("rolled back ELBv2 listener default certificate",
		zap.String("listener_arn", plan.listenerARN),
		zap.String("from", defaultARN), zap.String("to", best))
	return nil
}

// --- helpers ---

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func containsARN(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// shortARN keeps log lines and messages readable without losing the identifier.
func shortARN(arn string) string {
	if arn == "" {
		return "(none)"
	}
	if i := strings.LastIndex(arn, "/"); i >= 0 && i < len(arn)-1 {
		return arn[i+1:]
	}
	return arn
}

// leafDomain reads the certificate's primary name, used to group versions.
func leafDomain(certPEM []byte) string {
	leaf := parseLeaf(certPEM)
	if leaf == nil {
		return ""
	}
	if leaf.Subject.CommonName != "" {
		return strings.ToLower(leaf.Subject.CommonName)
	}
	if len(leaf.DNSNames) > 0 {
		return strings.ToLower(leaf.DNSNames[0])
	}
	return ""
}

func leafFingerprint(certPEM []byte) string {
	leaf := parseLeaf(certPEM)
	if leaf == nil {
		return ""
	}
	sum := sha256.Sum256(leaf.Raw)
	return hex.EncodeToString(sum[:])
}

// parseLeaf decodes the first CERTIFICATE block. The block type is checked
// because a fullchain, a CSR and a key all decode as PEM, and handing a CSR to
// ParseCertificate produces a confusing error rather than a clear one.
func parseLeaf(certPEM []byte) *x509.Certificate {
	for _, der := range certificateBlocks(certPEM) {
		leaf, err := x509.ParseCertificate(der)
		if err != nil {
			return nil
		}
		return leaf
	}
	return nil
}

// certificateBlocks returns the DER bytes of every CERTIFICATE block, in order,
// skipping keys, CSRs and anything else that happens to be in the same PEM.
func certificateBlocks(data []byte) [][]byte {
	var out [][]byte
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return out
		}
		if block.Type == "CERTIFICATE" {
			out = append(out, block.Bytes)
		}
	}
}

// leafPEM re-encodes just the first certificate.
func leafPEM(certPEM []byte) []byte {
	blocks := certificateBlocks(certPEM)
	if len(blocks) == 0 {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: blocks[0]})
}

// chainWithoutLeaf returns the intermediates, dropping any copy of the leaf.
//
// ACM rejects a chain that starts with the certificate being imported, and a
// fullchain is the usual thing to have on hand, so the leaf is filtered out
// instead of being passed through.
func chainWithoutLeaf(chainPEM, certPEM []byte) []byte {
	leafBlocks := certificateBlocks(certPEM)
	if len(leafBlocks) == 0 {
		return nil
	}
	leafDER := leafBlocks[0]

	var out []byte
	for _, der := range certificateBlocks(chainPEM) {
		if bytes.Equal(der, leafDER) {
			continue
		}
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	return out
}

// RequestACMCert requests a new public certificate from ACM (Amazon-issued, DNS validation).
func RequestACMCert(ctx context.Context, region, domain string, sans []string, logger *zap.Logger) (string, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("loading AWS config: %w", err)
	}

	acmClient := acm.NewFromConfig(cfg)

	input := &acm.RequestCertificateInput{
		DomainName:       aws.String(domain),
		ValidationMethod: types.ValidationMethodDns,
		Tags: []types.Tag{
			{Key: aws.String(tagManaged), Value: aws.String("true")},
			{Key: aws.String("clm-requested-at"), Value: aws.String(time.Now().Format(time.RFC3339))},
		},
	}

	if len(sans) > 0 {
		input.SubjectAlternativeNames = sans
	}

	output, err := acmClient.RequestCertificate(ctx, input)
	if err != nil {
		return "", fmt.Errorf("requesting ACM cert: %w", err)
	}

	arn := aws.ToString(output.CertificateArn)
	logger.Info("ACM certificate requested",
		zap.String("arn", arn),
		zap.String("domain", domain),
		zap.String("region", region),
		zap.String("validation", "DNS"),
	)

	return arn, nil
}

// GetACMValidationRecords returns the DNS CNAME records needed to validate an ACM certificate.
func GetACMValidationRecords(ctx context.Context, region, arn string) ([]ValidationRecord, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}

	acmClient := acm.NewFromConfig(cfg)
	output, err := acmClient.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
		CertificateArn: aws.String(arn),
	})
	if err != nil {
		return nil, err
	}

	var records []ValidationRecord
	for _, dv := range output.Certificate.DomainValidationOptions {
		if dv.ResourceRecord != nil {
			records = append(records, ValidationRecord{
				Domain:      aws.ToString(dv.DomainName),
				RecordName:  aws.ToString(dv.ResourceRecord.Name),
				RecordType:  string(dv.ResourceRecord.Type),
				RecordValue: aws.ToString(dv.ResourceRecord.Value),
				Status:      string(dv.ValidationStatus),
			})
		}
	}
	return records, nil
}

// ValidationRecord holds DNS validation details for an ACM certificate.
type ValidationRecord struct {
	Domain      string `json:"domain"`
	RecordName  string `json:"record_name"`
	RecordType  string `json:"record_type"`
	RecordValue string `json:"record_value"`
	Status      string `json:"status"`
}
