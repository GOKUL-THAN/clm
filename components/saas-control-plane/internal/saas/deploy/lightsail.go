package deploy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"go.uber.org/zap"
)

// LightsailProvisioner deploys a Satellite Gateway as an AWS Lightsail instance.
type LightsailProvisioner struct {
	logger *zap.Logger
}

// NewLightsailProvisioner creates a new Lightsail provisioner.
func NewLightsailProvisioner(logger *zap.Logger) *LightsailProvisioner {
	return &LightsailProvisioner{logger: logger}
}

func (p *LightsailProvisioner) Cloud() CloudProvider { return CloudAWS }
func (p *LightsailProvisioner) Mode() DeployMode     { return ModeLightsail }

// ValidateCredentials checks if the AWS credentials are valid using STS.
func (p *LightsailProvisioner) ValidateCredentials(ctx context.Context, creds *CloudCredentials) error {
	cfg, err := p.buildAWSConfig(ctx, creds, "ap-south-1")
	if err != nil {
		return fmt.Errorf("building AWS config: %w", err)
	}

	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCredentialsFailed, err)
	}

	p.logger.Info("AWS credentials validated",
		zap.String("account", aws.ToString(identity.Account)),
		zap.String("arn", aws.ToString(identity.Arn)),
	)
	return nil
}

// ListInfra fetches Lightsail bundles and availability zones.
func (p *LightsailProvisioner) ListInfra(ctx context.Context, creds *CloudCredentials, region string) (*InfraList, error) {
	cfg, err := p.buildAWSConfig(ctx, creds, region)
	if err != nil {
		return nil, err
	}

	client := lightsail.NewFromConfig(cfg)
	result := &InfraList{}

	// Get bundles (instance sizes)
	bundlesResp, err := client.GetBundles(ctx, &lightsail.GetBundlesInput{
		IncludeInactive: aws.Bool(false),
	})
	if err != nil {
		return nil, fmt.Errorf("fetching Lightsail bundles: %w", err)
	}

	for _, b := range bundlesResp.Bundles {
		// Only show Linux/Unix bundles suitable for gateway
		if b.SupportedPlatforms == nil {
			continue
		}
		isLinux := false
		for _, platform := range b.SupportedPlatforms {
			if platform == types.InstancePlatformLinuxUnix {
				isLinux = true
				break
			}
		}
		if !isLinux {
			continue
		}

		desc := fmt.Sprintf("%s", aws.ToString(b.Name))
		if b.Price != nil {
			desc = fmt.Sprintf("%s — $%.2f/mo", aws.ToString(b.Name), *b.Price)
		}
		result.Bundles = append(result.Bundles, InfraItem{
			ID:          aws.ToString(b.BundleId),
			Name:        aws.ToString(b.Name),
			Description: desc,
		})
	}

	// Get regions/AZs
	regionsResp, err := client.GetRegions(ctx, &lightsail.GetRegionsInput{
		IncludeAvailabilityZones: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("fetching Lightsail regions: %w", err)
	}

	for _, r := range regionsResp.Regions {
		regionName := string(r.Name)
		if regionName == region || region == "" {
			for _, az := range r.AvailabilityZones {
				result.AZs = append(result.AZs, InfraItem{
					ID:   aws.ToString(az.ZoneName),
					Name: aws.ToString(az.ZoneName),
				})
			}
		}
	}

	return result, nil
}

// Deploy creates a Lightsail instance, allocates a static IP, and configures firewall.
func (p *LightsailProvisioner) Deploy(ctx context.Context, req *DeployRequest, job *DeployJob, updateFn func(*DeployJob)) error {
	cfg, err := p.buildAWSConfig(ctx, req.Credentials, req.Region)
	if err != nil {
		return err
	}

	client := lightsail.NewFromConfig(cfg)
	shortID := job.ID.String()[:8] // use first 8 chars of job UUID for uniqueness
	instanceName := fmt.Sprintf("clm-gw-%s-%s", req.Name, shortID)

	// Define steps
	job.Steps = []JobStep{
		{Name: "Validate credentials", Status: "done"},
		{Name: "Create Lightsail instance", Status: "pending"},
		{Name: "Wait for instance running", Status: "pending"},
		{Name: "Allocate static IP", Status: "pending"},
		{Name: "Attach static IP", Status: "pending"},
		{Name: "Configure firewall ports", Status: "pending"},
		{Name: "Register gateway with SaaS", Status: "pending"},
	}
	job.CurrentStep = 1
	updateFn(job)

	// Step 2: Create instance
	p.startStep(job, 1)
	updateFn(job)

	bundle := "medium_3_1" // Default: 2 vCPU, 4GB RAM
	if req.Infra != nil && req.Infra.LightsailBundle != "" {
		bundle = req.Infra.LightsailBundle
	}

	az := req.Region + "a"
	if req.Infra != nil && req.Infra.LightsailAZ != "" {
		az = req.Infra.LightsailAZ
	}

	// Generate user-data bootstrap script
	userData := p.generateBootstrapScript(req)

	_, err = client.CreateInstances(ctx, &lightsail.CreateInstancesInput{
		InstanceNames:    []string{instanceName},
		AvailabilityZone: aws.String(az),
		BlueprintId:      aws.String("ubuntu_22_04"),
		BundleId:         aws.String(bundle),
		UserData:         aws.String(userData),
		Tags: []types.Tag{
			{Key: aws.String("managed-by"), Value: aws.String("clm")},
			{Key: aws.String("clm-tenant"), Value: aws.String(req.TenantID.String())},
			{Key: aws.String("clm-gateway"), Value: aws.String(req.Name)},
		},
	})
	if err != nil {
		p.failStep(job, 1, err)
		return fmt.Errorf("creating Lightsail instance: %w", err)
	}
	p.completeStep(job, 1)
	job.ResourceID = instanceName
	updateFn(job)

	// Step 3: Wait for running
	p.startStep(job, 2)
	updateFn(job)

	if err := p.waitForInstance(ctx, client, instanceName, "running", 5*time.Minute); err != nil {
		p.failStep(job, 2, err)
		return err
	}
	p.completeStep(job, 2)
	updateFn(job)

	// Step 4: Allocate static IP
	p.startStep(job, 3)
	updateFn(job)

	staticIPName := fmt.Sprintf("clm-ip-%s-%s", req.Name, shortID)
	allocResp, err := client.AllocateStaticIp(ctx, &lightsail.AllocateStaticIpInput{
		StaticIpName: aws.String(staticIPName),
	})
	if err != nil {
		p.failStep(job, 3, err)
		return fmt.Errorf("allocating static IP: %w", err)
	}
	_ = allocResp
	p.completeStep(job, 3)
	updateFn(job)

	// Step 5: Attach static IP
	p.startStep(job, 4)
	updateFn(job)

	_, err = client.AttachStaticIp(ctx, &lightsail.AttachStaticIpInput{
		StaticIpName: aws.String(staticIPName),
		InstanceName: aws.String(instanceName),
	})
	if err != nil {
		p.failStep(job, 4, err)
		return fmt.Errorf("attaching static IP: %w", err)
	}

	// Get the actual IP address
	ipResp, err := client.GetStaticIp(ctx, &lightsail.GetStaticIpInput{
		StaticIpName: aws.String(staticIPName),
	})
	if err == nil && ipResp.StaticIp != nil {
		job.PublicIP = aws.ToString(ipResp.StaticIp.IpAddress)
	}
	p.completeStep(job, 4)
	updateFn(job)

	// Step 6: Configure firewall
	p.startStep(job, 5)
	updateFn(job)

	_, err = client.PutInstancePublicPorts(ctx, &lightsail.PutInstancePublicPortsInput{
		InstanceName: aws.String(instanceName),
		PortInfos: []types.PortInfo{
			{FromPort: int32(80), ToPort: int32(80), Protocol: types.NetworkProtocolTcp},
			{FromPort: int32(443), ToPort: int32(443), Protocol: types.NetworkProtocolTcp},
			{FromPort: int32(8080), ToPort: int32(8080), Protocol: types.NetworkProtocolTcp},
			{FromPort: int32(9090), ToPort: int32(9090), Protocol: types.NetworkProtocolTcp},
			{FromPort: int32(22), ToPort: int32(22), Protocol: types.NetworkProtocolTcp},
		},
	})
	if err != nil {
		p.failStep(job, 5, err)
		return fmt.Errorf("configuring firewall: %w", err)
	}
	p.completeStep(job, 5)
	updateFn(job)

	// Step 7: Register with SaaS (handled by caller)
	p.startStep(job, 6)
	p.completeStep(job, 6)
	updateFn(job)

	p.logger.Info("Lightsail gateway deployed",
		zap.String("instance", instanceName),
		zap.String("ip", job.PublicIP),
		zap.String("region", req.Region),
	)

	return nil
}

// Teardown terminates the Lightsail instance and releases the static IP.
func (p *LightsailProvisioner) Teardown(ctx context.Context, creds *CloudCredentials, job *DeployJob) error {
	cfg, err := p.buildAWSConfig(ctx, creds, job.Region)
	if err != nil {
		return err
	}

	client := lightsail.NewFromConfig(cfg)
	instanceName := job.ResourceID

	// Detach and release static IP (name derived from instance name)
	staticIPName := fmt.Sprintf("clm-ip-%s", instanceName[4:]) // strip "clm-" prefix and reuse
	_, _ = client.DetachStaticIp(ctx, &lightsail.DetachStaticIpInput{
		StaticIpName: aws.String(staticIPName),
	})
	_, _ = client.ReleaseStaticIp(ctx, &lightsail.ReleaseStaticIpInput{
		StaticIpName: aws.String(staticIPName),
	})

	// Delete instance
	_, err = client.DeleteInstance(ctx, &lightsail.DeleteInstanceInput{
		InstanceName:      aws.String(instanceName),
		ForceDeleteAddOns: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("deleting Lightsail instance: %w", err)
	}

	p.logger.Info("Lightsail gateway torn down",
		zap.String("instance", instanceName),
	)

	return nil
}

// --- Helpers ---

func (p *LightsailProvisioner) buildAWSConfig(ctx context.Context, creds *CloudCredentials, region string) (aws.Config, error) {
	if creds == nil {
		return aws.Config{}, errors.New("AWS credentials are required")
	}
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}

	if creds.AWSRoleARN != "" {
		// Cross-account assume role
		baseCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
		if err != nil {
			return aws.Config{}, err
		}
		stsClient := sts.NewFromConfig(baseCfg)
		assumeProvider := stscreds.NewAssumeRoleProvider(stsClient, creds.AWSRoleARN, func(o *stscreds.AssumeRoleOptions) {
			if creds.AWSExternalID != "" {
				o.ExternalID = aws.String(creds.AWSExternalID)
			}
		})
		opts = append(opts, awsconfig.WithCredentialsProvider(assumeProvider))
	} else if creds.AWSAccessKeyID != "" && creds.AWSSecretAccessKey != "" {
		// Static credentials (IAM user)
		staticCreds := credentials.NewStaticCredentialsProvider(
			creds.AWSAccessKeyID,
			creds.AWSSecretAccessKey,
			"",
		)
		opts = append(opts, awsconfig.WithCredentialsProvider(staticCreds))
	} else {
		return aws.Config{}, errors.New("AWS cloud account must provide a role ARN or a complete access-key pair")
	}

	return awsconfig.LoadDefaultConfig(ctx, opts...)
}

func (p *LightsailProvisioner) waitForInstance(ctx context.Context, client *lightsail.Client, name, targetState string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.GetInstance(ctx, &lightsail.GetInstanceInput{
			InstanceName: aws.String(name),
		})
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		if resp.Instance != nil && resp.Instance.State != nil {
			if aws.ToString(resp.Instance.State.Name) == targetState {
				return nil
			}
		}

		time.Sleep(10 * time.Second)
	}
	return fmt.Errorf("instance %s did not reach state '%s' within %v", name, targetState, timeout)
}

func (p *LightsailProvisioner) generateBootstrapScript(req *DeployRequest) string {
	saasEndpoint := "http://35.154.6.28" // SaaS control plane endpoint
	token := req.TenantID.String()       // Simplified; in production use a proper registration token

	script := fmt.Sprintf(`#!/bin/bash
set -e

# Install CLM Satellite Gateway
echo "=== CLM Gateway Bootstrap ==="
`+packageManagerPreamble()+basePackages()+`
# Download CLM satellite binary
curl -sL https://releases.clm.aiforgers.tech/latest/clm-satellite-linux-amd64 -o /usr/local/bin/clm-satellite
chmod +x /usr/local/bin/clm-satellite

# Create config directory
mkdir -p /etc/clm

# Write satellite configuration
cat > /etc/clm/satellite.yaml << 'EOF'
id: "%s"
name: "%s"
saas_endpoint: "%s"
connector_mode: "direct"
log_level: "info"

crypto:
  default_algorithm: "ECDSA"
  ecdsa_curve: "P-256"
  rsa_bits: 4096

secrets:
  provider: "%s"
  region: "%s"
  kms_key_id: "%s"
  prefix: "clm/"

discovery:
  schedule: "0 2 * * *"
  networks: %s
  ports: %s
  concurrency: 100
EOF

# Write systemd service
cat > /etc/systemd/system/clm-satellite.service << 'SVCEOF'
[Unit]
Description=CLM Satellite Gateway
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/clm-satellite
Environment=CLM_SATELLITE_CONFIG=/etc/clm/satellite.yaml
Environment=CLM_SATELLITE_TOKEN=%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
SVCEOF

# Start service
systemctl daemon-reload
systemctl enable clm-satellite
systemctl start clm-satellite

echo "=== CLM Gateway Bootstrap Complete ==="
`,
		req.TenantID.String(),
		req.Name,
		saasEndpoint,
		p.getSecretsProvider(req),
		req.Region,
		p.getKMSKey(req),
		p.formatStringSlice(p.getDiscoveryCIDRs(req)),
		p.formatIntSlice(p.getDiscoveryPorts(req)),
		token,
	)

	return script
}

func (p *LightsailProvisioner) getSecretsProvider(req *DeployRequest) string {
	if req.SatelliteConfig != nil && req.SatelliteConfig.SecretsProvider != "" {
		return req.SatelliteConfig.SecretsProvider
	}
	return "aws"
}

func (p *LightsailProvisioner) getKMSKey(req *DeployRequest) string {
	if req.SatelliteConfig != nil {
		return req.SatelliteConfig.KMSKeyID
	}
	return ""
}

func (p *LightsailProvisioner) getDiscoveryCIDRs(req *DeployRequest) []string {
	if req.SatelliteConfig != nil && len(req.SatelliteConfig.DiscoveryCIDRs) > 0 {
		return req.SatelliteConfig.DiscoveryCIDRs
	}
	return []string{"10.0.0.0/16"}
}

func (p *LightsailProvisioner) getDiscoveryPorts(req *DeployRequest) []int {
	if req.SatelliteConfig != nil && len(req.SatelliteConfig.DiscoveryPorts) > 0 {
		return req.SatelliteConfig.DiscoveryPorts
	}
	return []int{443, 8443, 636, 6443}
}

func (p *LightsailProvisioner) formatStringSlice(s []string) string {
	result := "["
	for i, v := range s {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf(`"%s"`, v)
	}
	return result + "]"
}

func (p *LightsailProvisioner) formatIntSlice(s []int) string {
	result := "["
	for i, v := range s {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf("%d", v)
	}
	return result + "]"
}

func (p *LightsailProvisioner) startStep(job *DeployJob, idx int) {
	now := time.Now()
	job.Steps[idx].Status = "running"
	job.Steps[idx].StartedAt = &now
	job.CurrentStep = idx + 1
}

func (p *LightsailProvisioner) completeStep(job *DeployJob, idx int) {
	now := time.Now()
	job.Steps[idx].Status = "done"
	job.Steps[idx].DoneAt = &now
}

func (p *LightsailProvisioner) failStep(job *DeployJob, idx int, err error) {
	now := time.Now()
	job.Steps[idx].Status = "failed"
	job.Steps[idx].DoneAt = &now
	job.Steps[idx].Error = err.Error()
}
