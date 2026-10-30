package deploy

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"go.uber.org/zap"
)

const clmOfficialAMI = "ami-0c55b159cbfafe1f0" // Ubuntu 22.04 (placeholder — region-specific)

// EC2Provisioner deploys a Satellite Gateway as an EC2 instance behind an existing ALB.
type EC2Provisioner struct {
	logger *zap.Logger
}

// NewEC2Provisioner creates a new EC2 provisioner.
func NewEC2Provisioner(logger *zap.Logger) *EC2Provisioner {
	return &EC2Provisioner{logger: logger}
}

func (p *EC2Provisioner) Cloud() CloudProvider { return CloudAWS }
func (p *EC2Provisioner) Mode() DeployMode     { return ModeEC2 }

// ValidateCredentials verifies AWS credentials via STS.
func (p *EC2Provisioner) ValidateCredentials(ctx context.Context, creds *CloudCredentials) error {
	cfg, err := p.buildAWSConfig(ctx, creds, "us-east-1")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCredentialsFailed, err)
	}

	stsClient := sts.NewFromConfig(cfg)
	_, err = stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCredentialsFailed, err)
	}
	return nil
}

// ListInfra fetches existing VPCs, subnets, SGs, ALBs, target groups, key pairs, and instance profiles.
func (p *EC2Provisioner) ListInfra(ctx context.Context, creds *CloudCredentials, region string) (*InfraList, error) {
	cfg, err := p.buildAWSConfig(ctx, creds, region)
	if err != nil {
		return nil, err
	}

	ec2Client := ec2.NewFromConfig(cfg)
	elbClient := elbv2.NewFromConfig(cfg)
	iamClient := iam.NewFromConfig(cfg)
	result := &InfraList{}

	// VPCs
	vpcsResp, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
	if err == nil {
		for _, vpc := range vpcsResp.Vpcs {
			name := getEC2TagValue(vpc.Tags, "Name")
			if name == "" {
				name = aws.ToString(vpc.VpcId)
			}
			result.VPCs = append(result.VPCs, InfraItem{
				ID:          aws.ToString(vpc.VpcId),
				Name:        name,
				Description: aws.ToString(vpc.CidrBlock),
			})
		}
	}

	// Subnets
	subnetsResp, err := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{})
	if err == nil {
		for _, subnet := range subnetsResp.Subnets {
			name := getEC2TagValue(subnet.Tags, "Name")
			if name == "" {
				name = aws.ToString(subnet.SubnetId)
			}
			result.Subnets = append(result.Subnets, InfraItem{
				ID:          aws.ToString(subnet.SubnetId),
				Name:        name,
				Description: fmt.Sprintf("%s (%s)", aws.ToString(subnet.CidrBlock), aws.ToString(subnet.AvailabilityZone)),
			})
		}
	}

	// Security Groups
	sgsResp, err := ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{})
	if err == nil {
		for _, sg := range sgsResp.SecurityGroups {
			result.SecurityGroups = append(result.SecurityGroups, InfraItem{
				ID:          aws.ToString(sg.GroupId),
				Name:        aws.ToString(sg.GroupName),
				Description: aws.ToString(sg.Description),
			})
		}
	}

	// Key Pairs
	keysResp, err := ec2Client.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{})
	if err == nil {
		for _, kp := range keysResp.KeyPairs {
			result.KeyPairs = append(result.KeyPairs, InfraItem{
				ID:   aws.ToString(kp.KeyName),
				Name: aws.ToString(kp.KeyName),
			})
		}
	}

	// Load Balancers (ALBs)
	lbResp, err := elbClient.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{})
	if err == nil {
		for _, lb := range lbResp.LoadBalancers {
			if lb.Type == elbv2types.LoadBalancerTypeEnumApplication {
				result.LoadBalancers = append(result.LoadBalancers, InfraItem{
					ID:          aws.ToString(lb.LoadBalancerArn),
					Name:        aws.ToString(lb.LoadBalancerName),
					Description: string(lb.Scheme),
				})
			}
		}
	}

	// Target Groups
	tgResp, err := elbClient.DescribeTargetGroups(ctx, &elbv2.DescribeTargetGroupsInput{})
	if err == nil {
		for _, tg := range tgResp.TargetGroups {
			result.TargetGroups = append(result.TargetGroups, InfraItem{
				ID:          aws.ToString(tg.TargetGroupArn),
				Name:        aws.ToString(tg.TargetGroupName),
				Description: fmt.Sprintf("Port %d, %s", aws.ToInt32(tg.Port), string(tg.Protocol)),
			})
		}
	}

	// IAM Instance Profiles
	profilesResp, err := iamClient.ListInstanceProfiles(ctx, &iam.ListInstanceProfilesInput{})
	if err == nil {
		for _, profile := range profilesResp.InstanceProfiles {
			result.InstanceProfiles = append(result.InstanceProfiles, InfraItem{
				ID:          aws.ToString(profile.InstanceProfileName),
				Name:        aws.ToString(profile.InstanceProfileName),
				Description: aws.ToString(profile.Arn),
			})
		}
	}

	return result, nil
}

// Deploy launches an EC2 instance and registers it to the existing ALB target group.
func (p *EC2Provisioner) Deploy(ctx context.Context, req *DeployRequest, job *DeployJob, updateFn func(*DeployJob)) error {
	cfg, err := p.buildAWSConfig(ctx, req.Credentials, req.Region)
	if err != nil {
		return err
	}

	ec2Client := ec2.NewFromConfig(cfg)
	elbClient := elbv2.NewFromConfig(cfg)

	// Define steps
	job.Steps = []JobStep{
		{Name: "Validate credentials", Status: "done"},
		{Name: "Launch EC2 instance", Status: "pending"},
		{Name: "Wait for instance running", Status: "pending"},
		{Name: "Register to target group", Status: "pending"},
		{Name: "Wait for healthy in LB", Status: "pending"},
		{Name: "Register gateway with SaaS", Status: "pending"},
	}
	job.CurrentStep = 1
	updateFn(job)

	// Step 2: Launch instance
	p.startStep(job, 1)
	updateFn(job)

	instanceType := ec2types.InstanceTypeT3Medium
	if req.Infra != nil && req.Infra.InstanceType != "" {
		instanceType = ec2types.InstanceType(req.Infra.InstanceType)
	}

	userData := p.generateBootstrapScript(req)
	runInput := &ec2.RunInstancesInput{
		ImageId:      aws.String(clmOfficialAMI),
		InstanceType: instanceType,
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		UserData:     aws.String(base64.StdEncoding.EncodeToString([]byte(userData))),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags: []ec2types.Tag{
					{Key: aws.String("Name"), Value: aws.String(fmt.Sprintf("clm-gateway-%s", req.Name))},
					{Key: aws.String("managed-by"), Value: aws.String("clm")},
					{Key: aws.String("clm-tenant"), Value: aws.String(req.TenantID.String())},
				},
			},
		},
	}

	if req.Infra != nil {
		if req.Infra.SubnetID != "" {
			runInput.SubnetId = aws.String(req.Infra.SubnetID)
		}
		if req.Infra.SecurityGroupID != "" {
			runInput.SecurityGroupIds = []string{req.Infra.SecurityGroupID}
		}
		if req.Infra.KeyPair != "" {
			runInput.KeyName = aws.String(req.Infra.KeyPair)
		}
		if req.Infra.IAMInstanceProfile != "" {
			runInput.IamInstanceProfile = &ec2types.IamInstanceProfileSpecification{
				Name: aws.String(req.Infra.IAMInstanceProfile),
			}
		}
	}

	runResp, err := ec2Client.RunInstances(ctx, runInput)
	if err != nil {
		p.failStep(job, 1, err)
		return fmt.Errorf("launching EC2 instance: %w", err)
	}

	instanceID := aws.ToString(runResp.Instances[0].InstanceId)
	job.ResourceID = instanceID
	p.completeStep(job, 1)
	updateFn(job)

	// Step 3: Wait for running
	p.startStep(job, 2)
	updateFn(job)

	waiter := ec2.NewInstanceRunningWaiter(ec2Client)
	if err := waiter.Wait(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	}, 5*time.Minute); err != nil {
		p.failStep(job, 2, err)
		return fmt.Errorf("waiting for instance: %w", err)
	}

	// Get private IP
	descResp, _ := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if len(descResp.Reservations) > 0 && len(descResp.Reservations[0].Instances) > 0 {
		inst := descResp.Reservations[0].Instances[0]
		if inst.PublicIpAddress != nil {
			job.PublicIP = aws.ToString(inst.PublicIpAddress)
		} else if inst.PrivateIpAddress != nil {
			job.PublicIP = aws.ToString(inst.PrivateIpAddress) // private IP for internal LB
		}
	}
	p.completeStep(job, 2)
	updateFn(job)

	// Step 4: Register to target group
	p.startStep(job, 3)
	updateFn(job)

	if req.Infra != nil && req.Infra.TargetGroupARN != "" {
		_, err = elbClient.RegisterTargets(ctx, &elbv2.RegisterTargetsInput{
			TargetGroupArn: aws.String(req.Infra.TargetGroupARN),
			Targets: []elbv2types.TargetDescription{
				{Id: aws.String(instanceID), Port: aws.Int32(8080)},
			},
		})
		if err != nil {
			p.failStep(job, 3, err)
			return fmt.Errorf("registering to target group: %w", err)
		}
	}
	p.completeStep(job, 3)
	updateFn(job)

	// Step 5: Wait for healthy
	p.startStep(job, 4)
	updateFn(job)

	if req.Infra != nil && req.Infra.TargetGroupARN != "" {
		if err := p.waitForHealthy(ctx, elbClient, req.Infra.TargetGroupARN, instanceID, 3*time.Minute); err != nil {
			p.logger.Warn("target not yet healthy (may need more time)", zap.Error(err))
		}
	}
	p.completeStep(job, 4)
	updateFn(job)

	// Step 6: Register with SaaS (handled by caller)
	p.startStep(job, 5)
	p.completeStep(job, 5)
	updateFn(job)

	p.logger.Info("EC2 gateway deployed",
		zap.String("instance_id", instanceID),
		zap.String("ip", job.PublicIP),
	)

	return nil
}

// Teardown terminates the EC2 instance and deregisters from target group.
func (p *EC2Provisioner) Teardown(ctx context.Context, creds *CloudCredentials, job *DeployJob) error {
	cfg, err := p.buildAWSConfig(ctx, creds, job.Region)
	if err != nil {
		return err
	}

	ec2Client := ec2.NewFromConfig(cfg)

	_, err = ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{job.ResourceID},
	})
	if err != nil {
		return fmt.Errorf("terminating EC2 instance: %w", err)
	}

	p.logger.Info("EC2 gateway terminated", zap.String("instance_id", job.ResourceID))
	return nil
}

// --- Helpers ---

func (p *EC2Provisioner) buildAWSConfig(ctx context.Context, creds *CloudCredentials, region string) (aws.Config, error) {
	if creds == nil {
		return aws.Config{}, errors.New("AWS credentials are required")
	}
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}

	if creds.AWSRoleARN != "" {
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

func (p *EC2Provisioner) waitForHealthy(ctx context.Context, client *elbv2.Client, tgARN, instanceID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.DescribeTargetHealth(ctx, &elbv2.DescribeTargetHealthInput{
			TargetGroupArn: aws.String(tgARN),
			Targets:        []elbv2types.TargetDescription{{Id: aws.String(instanceID)}},
		})
		if err == nil {
			for _, thd := range resp.TargetHealthDescriptions {
				if thd.TargetHealth != nil && thd.TargetHealth.State == elbv2types.TargetHealthStateEnumHealthy {
					return nil
				}
			}
		}
		time.Sleep(10 * time.Second)
	}
	return fmt.Errorf("target %s not healthy within %v", instanceID, timeout)
}

func (p *EC2Provisioner) generateBootstrapScript(req *DeployRequest) string {
	// Same bootstrap as Lightsail but without the static IP steps.
	return fmt.Sprintf(`#!/bin/bash
set -e
`+packageManagerPreamble()+basePackages()+`
curl -sL https://releases.clm.aiforgers.tech/latest/clm-satellite-linux-amd64 -o /usr/local/bin/clm-satellite
chmod +x /usr/local/bin/clm-satellite
mkdir -p /etc/clm
cat > /etc/clm/satellite.yaml << 'EOF'
id: "%s"
name: "%s"
saas_endpoint: "https://clm.aiforgers.tech"
connector_mode: "direct"
log_level: "info"
crypto:
  default_algorithm: "ECDSA"
  ecdsa_curve: "P-256"
secrets:
  provider: "aws"
  region: "%s"
  prefix: "clm/"
discovery:
  schedule: "0 2 * * *"
  networks: ["10.0.0.0/16"]
  ports: [443, 8443, 636, 6443]
  concurrency: 100
EOF
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
systemctl daemon-reload && systemctl enable clm-satellite && systemctl start clm-satellite
`, req.TenantID.String(), req.Name, req.Region, req.TenantID.String())
}

func (p *EC2Provisioner) startStep(job *DeployJob, idx int) {
	now := time.Now()
	job.Steps[idx].Status = "running"
	job.Steps[idx].StartedAt = &now
	job.CurrentStep = idx + 1
}

func (p *EC2Provisioner) completeStep(job *DeployJob, idx int) {
	now := time.Now()
	job.Steps[idx].Status = "done"
	job.Steps[idx].DoneAt = &now
}

func (p *EC2Provisioner) failStep(job *DeployJob, idx int, err error) {
	now := time.Now()
	job.Steps[idx].Status = "failed"
	job.Steps[idx].DoneAt = &now
	job.Steps[idx].Error = err.Error()
}

func getEC2TagValue(tags []ec2types.Tag, key string) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == key {
			return aws.ToString(t.Value)
		}
	}
	return ""
}
