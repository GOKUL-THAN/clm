package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

var (
	ErrJobNotFound       = errors.New("deployment job not found")
	ErrInvalidCloud      = errors.New("unsupported cloud provider")
	ErrInvalidMode       = errors.New("unsupported deployment mode")
	ErrCredentialsFailed = errors.New("credential validation failed")
	ErrDeployInProgress  = errors.New("deployment already in progress")
)

// JobState represents the lifecycle state of a deployment job.
type JobState string

const (
	StatePending    JobState = "pending"
	StateValidating JobState = "validating"
	StateDeploying  JobState = "deploying"
	StateOnline     JobState = "online"
	StateFailed     JobState = "failed"
	StateDestroying JobState = "destroying"
	StateDestroyed  JobState = "destroyed"
)

// CloudProvider identifies the target cloud.
type CloudProvider string

const (
	CloudAWS   CloudProvider = "aws"
	CloudGCP   CloudProvider = "gcp"
	CloudAzure CloudProvider = "azure"
)

// DeployMode identifies how the gateway is deployed.
type DeployMode string

const (
	ModeEC2       DeployMode = "ec2"
	ModeLightsail DeployMode = "lightsail"
	ModeEKS       DeployMode = "eks"
	ModeGCE       DeployMode = "gce"
	ModeGKE       DeployMode = "gke"
	ModeAzureVM   DeployMode = "azure_vm"
	ModeAKS       DeployMode = "aks"
)

// DeployRequest is submitted from the Admin Portal to initiate a gateway deployment.
type DeployRequest struct {
	Name           string        `json:"name" binding:"required"`
	TenantID       uuid.UUID     `json:"tenant_id"`
	CloudAccountID uuid.UUID     `json:"cloud_account_id" binding:"required"`
	Cloud          CloudProvider `json:"cloud" binding:"required"`
	Mode           DeployMode    `json:"mode" binding:"required"`
	Region         string        `json:"region" binding:"required"`

	// Credentials are resolved internally from CloudAccountID and are never
	// accepted from or serialized to the REST API.
	Credentials *CloudCredentials `json:"-"`

	// Existing infrastructure (customer-provided)
	Infra *InfraConfig `json:"infra"`

	// Gateway satellite config
	SatelliteConfig *SatelliteDeployConfig `json:"satellite_config"`
}

// CloudCredentials holds customer-provided cloud auth.
type CloudCredentials struct {
	// AWS
	AWSAccessKeyID     string `json:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `json:"aws_secret_access_key,omitempty"`
	AWSRoleARN         string `json:"aws_role_arn,omitempty"`
	AWSExternalID      string `json:"aws_external_id,omitempty"`

	// GCP
	GCPServiceAccountJSON string `json:"gcp_service_account_json,omitempty"`
	GCPProjectID          string `json:"gcp_project_id,omitempty"`

	// Azure
	AzureTenantID       string `json:"azure_tenant_id,omitempty"`
	AzureClientID       string `json:"azure_client_id,omitempty"`
	AzureClientSecret   string `json:"azure_client_secret,omitempty"`
	AzureSubscriptionID string `json:"azure_subscription_id,omitempty"`
}

// InfraConfig holds customer's existing infrastructure IDs.
type InfraConfig struct {
	// AWS EC2
	VPCID              string `json:"vpc_id,omitempty"`
	SubnetID           string `json:"subnet_id,omitempty"`
	SecurityGroupID    string `json:"security_group_id,omitempty"`
	InstanceType       string `json:"instance_type,omitempty"`
	KeyPair            string `json:"key_pair,omitempty"`
	IAMInstanceProfile string `json:"iam_instance_profile,omitempty"`
	ALBARN             string `json:"alb_arn,omitempty"`
	TargetGroupARN     string `json:"target_group_arn,omitempty"`

	// AWS Lightsail
	LightsailBundle string `json:"lightsail_bundle,omitempty"` // e.g., "medium_2_0"
	LightsailAZ     string `json:"lightsail_az,omitempty"`

	// AWS EKS / GKE / AKS
	ClusterName string `json:"cluster_name,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	ServiceType string `json:"service_type,omitempty"` // ClusterIP, LoadBalancer

	// GCP
	GCPNetwork       string `json:"gcp_network,omitempty"`
	GCPSubnet        string `json:"gcp_subnet,omitempty"`
	GCPInstanceGroup string `json:"gcp_instance_group,omitempty"`
	GCPBackendSvc    string `json:"gcp_backend_service,omitempty"`

	// Azure
	AzureResourceGroup string `json:"azure_resource_group,omitempty"`
	AzureVNet          string `json:"azure_vnet,omitempty"`
	AzureSubnet        string `json:"azure_subnet,omitempty"`
	AzureNSG           string `json:"azure_nsg,omitempty"`
	AzureAppGateway    string `json:"azure_app_gateway,omitempty"`
	AzureBackendPool   string `json:"azure_backend_pool,omitempty"`
}

// SatelliteDeployConfig is injected into the gateway at deploy time.
type SatelliteDeployConfig struct {
	SecretsProvider string   `json:"secrets_provider"` // aws, gcp, azure, vault
	SecretsRegion   string   `json:"secrets_region"`
	KMSKeyID        string   `json:"kms_key_id"`
	SecretsPrefix   string   `json:"secrets_prefix"`
	DiscoveryCIDRs  []string `json:"discovery_cidrs"`
	DiscoveryPorts  []int    `json:"discovery_ports"`
	CryptoAlgorithm string   `json:"crypto_algorithm"` // ECDSA, RSA
	ConnectorMode   string   `json:"connector_mode"`   // direct, proxy
}

// DeployJob tracks the state and progress of a deployment.
type DeployJob struct {
	ID             uuid.UUID     `json:"id"`
	TenantID       uuid.UUID     `json:"tenant_id"`
	CloudAccountID uuid.UUID     `json:"cloud_account_id"`
	Name           string        `json:"name"`
	Cloud          CloudProvider `json:"cloud"`
	Mode           DeployMode    `json:"mode"`
	Region         string        `json:"region"`
	State          JobState      `json:"state"`
	Steps          []JobStep     `json:"steps"`
	CurrentStep    int           `json:"current_step"`
	Error          string        `json:"error,omitempty"`
	ResourceID     string        `json:"resource_id,omitempty"` // instance ID, pod name, etc.
	PublicIP       string        `json:"public_ip,omitempty"`
	SatelliteID    *uuid.UUID    `json:"satellite_id,omitempty"`
	StartedAt      time.Time     `json:"started_at"`
	CompletedAt    *time.Time    `json:"completed_at,omitempty"`
}

// JobStep represents one step in the deployment process.
type JobStep struct {
	Name      string     `json:"name"`
	Status    string     `json:"status"` // pending, running, done, failed
	StartedAt *time.Time `json:"started_at,omitempty"`
	DoneAt    *time.Time `json:"done_at,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// Provisioner is the interface for cloud-specific deployment logic.
type Provisioner interface {
	// Cloud returns the cloud provider this provisioner handles.
	Cloud() CloudProvider
	// Mode returns the deployment mode.
	Mode() DeployMode
	// ValidateCredentials checks if the provided credentials are valid.
	ValidateCredentials(ctx context.Context, creds *CloudCredentials) error
	// ListInfra fetches existing infrastructure from customer cloud (for UI dropdowns).
	ListInfra(ctx context.Context, creds *CloudCredentials, region string) (*InfraList, error)
	// Deploy provisions the gateway and returns the resource ID.
	Deploy(ctx context.Context, req *DeployRequest, job *DeployJob, updateFn func(*DeployJob)) error
	// Teardown removes the deployed resources.
	Teardown(ctx context.Context, creds *CloudCredentials, job *DeployJob) error
}

// InfraList is returned by ListInfra to populate Admin Portal dropdowns.
type InfraList struct {
	// AWS EC2
	VPCs             []InfraItem `json:"vpcs,omitempty"`
	Subnets          []InfraItem `json:"subnets,omitempty"`
	SecurityGroups   []InfraItem `json:"security_groups,omitempty"`
	KeyPairs         []InfraItem `json:"key_pairs,omitempty"`
	InstanceProfiles []InfraItem `json:"instance_profiles,omitempty"`
	LoadBalancers    []InfraItem `json:"load_balancers,omitempty"`
	TargetGroups     []InfraItem `json:"target_groups,omitempty"`
	Clusters         []InfraItem `json:"clusters,omitempty"`

	// Lightsail
	Bundles []InfraItem `json:"bundles,omitempty"`
	AZs     []InfraItem `json:"availability_zones,omitempty"`
}

// InfraItem is a single item in an infrastructure dropdown.
type InfraItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Service orchestrates gateway deployments.
type Service struct {
	provisioners map[string]Provisioner // key: "cloud:mode"
	satRepo      core.SatelliteRepository
	jobs         map[uuid.UUID]*DeployJob
	mu           sync.RWMutex
	logger       *zap.Logger
}

// NewService creates a new deployment service.
func NewService(satRepo core.SatelliteRepository, logger *zap.Logger) *Service {
	return &Service{
		provisioners: make(map[string]Provisioner),
		satRepo:      satRepo,
		jobs:         make(map[uuid.UUID]*DeployJob),
		logger:       logger,
	}
}

// RegisterProvisioner adds a provisioner for a specific cloud+mode combination.
func (s *Service) RegisterProvisioner(p Provisioner) {
	key := fmt.Sprintf("%s:%s", p.Cloud(), p.Mode())
	s.provisioners[key] = p
	s.logger.Info("registered deploy provisioner", zap.String("key", key))
}

// ValidateCredentials tests customer credentials against the target cloud.
func (s *Service) ValidateCredentials(ctx context.Context, cloud CloudProvider, mode DeployMode, creds *CloudCredentials) error {
	p, err := s.getProvisioner(cloud, mode)
	if err != nil {
		return err
	}
	return p.ValidateCredentials(ctx, creds)
}

// ListInfra fetches customer's existing infrastructure for the UI.
func (s *Service) ListInfra(ctx context.Context, cloud CloudProvider, mode DeployMode, creds *CloudCredentials, region string) (*InfraList, error) {
	p, err := s.getProvisioner(cloud, mode)
	if err != nil {
		return nil, err
	}
	return p.ListInfra(ctx, creds, region)
}

// Deploy starts an asynchronous gateway deployment job.
func (s *Service) Deploy(ctx context.Context, req *DeployRequest) (*DeployJob, error) {
	if req == nil || req.TenantID == uuid.Nil || req.CloudAccountID == uuid.Nil || req.Credentials == nil {
		return nil, errors.New("tenant_id, cloud_account_id, and resolved credentials are required")
	}
	p, err := s.getProvisioner(req.Cloud, req.Mode)
	if err != nil {
		return nil, err
	}

	job := &DeployJob{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		CloudAccountID: req.CloudAccountID,
		Name:           req.Name,
		Cloud:          req.Cloud,
		Mode:           req.Mode,
		Region:         req.Region,
		State:          StatePending,
		StartedAt:      time.Now(),
	}

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	// The API request may end as soon as the job is accepted, so detach its
	// cancellation while retaining request values and bounding the job lifetime.
	deployCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Minute)

	// Run deployment asynchronously
	go func() {
		defer cancel()
		job.State = StateValidating

		if err := p.ValidateCredentials(deployCtx, req.Credentials); err != nil {
			s.failJob(job, "credential validation", err)
			return
		}

		job.State = StateDeploying
		if err := p.Deploy(deployCtx, req, job, s.updateJob); err != nil {
			s.failJob(job, "deployment", err)
			return
		}

		// Register as satellite in CLM
		sat := &models.Satellite{
			ID:             uuid.New(),
			TenantID:       req.TenantID,
			Name:           req.Name,
			Type:           models.SatelliteType(req.Mode),
			Status:         "registering",
			ConnectorMode:  "direct",
			Cloud:          string(req.Cloud),
			Region:         req.Region,
			SecretsBackend: "aws",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		if req.SatelliteConfig != nil {
			sat.SecretsBackend = req.SatelliteConfig.SecretsProvider
		}

		if err := s.satRepo.Create(deployCtx, sat); err != nil {
			s.logger.Error("failed to register satellite", zap.Error(err))
		}

		job.SatelliteID = &sat.ID
		job.State = StateOnline
		now := time.Now()
		job.CompletedAt = &now

		s.logger.Info("gateway deployment completed",
			zap.String("job_id", job.ID.String()),
			zap.String("name", req.Name),
			zap.String("resource_id", job.ResourceID),
			zap.String("public_ip", job.PublicIP),
		)
	}()

	return job, nil
}

// GetJob returns the current state of a deployment job.
func (s *Service) GetJob(jobID uuid.UUID) (*DeployJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return nil, ErrJobNotFound
	}
	return job, nil
}

// GetJobLogs returns step-by-step logs for a deployment job.
func (s *Service) GetJobLogs(jobID uuid.UUID) ([]JobStep, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return nil, ErrJobNotFound
	}
	return job.Steps, nil
}

// CancelJob cancels an in-progress deployment.
func (s *Service) CancelJob(jobID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return ErrJobNotFound
	}
	if job.State == StateOnline || job.State == StateDestroyed {
		return fmt.Errorf("cannot cancel job in state: %s", job.State)
	}
	job.State = StateFailed
	job.Error = "cancelled by user"
	return nil
}

// Teardown removes a deployed gateway.
func (s *Service) Teardown(ctx context.Context, jobID uuid.UUID, creds *CloudCredentials) error {
	s.mu.Lock()
	job, ok := s.jobs[jobID]
	if !ok {
		s.mu.Unlock()
		return ErrJobNotFound
	}
	job.State = StateDestroying
	s.mu.Unlock()

	p, err := s.getProvisioner(job.Cloud, job.Mode)
	if err != nil {
		return err
	}

	if err := p.Teardown(ctx, creds, job); err != nil {
		job.State = StateFailed
		job.Error = err.Error()
		return err
	}

	job.State = StateDestroyed
	now := time.Now()
	job.CompletedAt = &now
	return nil
}

// ListJobs returns all deployment jobs for a tenant.
func (s *Service) ListJobs(tenantID uuid.UUID) []*DeployJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*DeployJob
	for _, job := range s.jobs {
		if job.TenantID == tenantID {
			result = append(result, job)
		}
	}
	return result
}

func (s *Service) getProvisioner(cloud CloudProvider, mode DeployMode) (Provisioner, error) {
	key := fmt.Sprintf("%s:%s", cloud, mode)
	p, ok := s.provisioners[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidMode, key)
	}
	return p, nil
}

func (s *Service) failJob(job *DeployJob, step string, err error) {
	job.State = StateFailed
	job.Error = fmt.Sprintf("%s: %v", step, err)
	s.logger.Error("deployment failed",
		zap.String("job_id", job.ID.String()),
		zap.String("step", step),
		zap.Error(err),
	)
}

func (s *Service) updateJob(job *DeployJob) {
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
}

// MarshalJobToJSON is a helper to serialize a job for API response.
func MarshalJobToJSON(job *DeployJob) ([]byte, error) {
	return json.Marshal(job)
}

// Explicit tenant-scoped deployment job operations prevent ID enumeration from
// crossing tenant boundaries.
func (s *Service) GetJobForTenant(tenantID, jobID uuid.UUID) (*DeployJob, error) {
	job, err := s.GetJob(jobID)
	if err != nil || job.TenantID != tenantID {
		return nil, ErrJobNotFound
	}
	return job, nil
}
func (s *Service) GetJobLogsForTenant(tenantID, jobID uuid.UUID) ([]JobStep, error) {
	job, err := s.GetJobForTenant(tenantID, jobID)
	if err != nil {
		return nil, err
	}
	return job.Steps, nil
}
func (s *Service) CancelJobForTenant(tenantID, jobID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok || job.TenantID != tenantID {
		return ErrJobNotFound
	}
	if job.State == StateOnline || job.State == StateDestroyed {
		return fmt.Errorf("cannot cancel job in state: %s", job.State)
	}
	job.State = StateFailed
	job.Error = "cancelled by user"
	return nil
}
func (s *Service) TeardownForTenant(ctx context.Context, tenantID, jobID uuid.UUID, creds *CloudCredentials) error {
	if _, err := s.GetJobForTenant(tenantID, jobID); err != nil {
		return err
	}
	return s.Teardown(ctx, jobID, creds)
}

func (s *Service) CheckSatelliteOwnership(ctx context.Context, tenantID, satelliteID uuid.UUID) error {
	_, err := s.satRepo.GetByIDForTenant(ctx, tenantID, satelliteID)
	return err
}
