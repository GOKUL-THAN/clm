package deployers

import (
	"context"
	"time"
)

// DeployTarget identifies where to deploy a certificate.
type DeployTarget struct {
	Category string `json:"category"` // cloud, os, data, app
	Type     string `json:"type"`     // aws-alb, linux-nginx, postgresql, etc.
	// Host and Port remain the deployment/control-plane address for backwards
	// compatibility. ServingHost and ServingPort identify the actual TLS traffic
	// endpoint; ServerName is used for SNI and hostname/SAN verification.
	Host           string `json:"host"`
	Port           int    `json:"port"`
	ServingHost    string `json:"serving_host,omitempty"`
	ServerName     string `json:"server_name,omitempty"`
	ServingPort    int    `json:"serving_port,omitempty"`
	ExpectedIssuer string `json:"expected_issuer,omitempty"`
	// Cloud-specific
	ResourceID string `json:"resource_id,omitempty"`
	Region     string `json:"region,omitempty"`
	// VaultName identifies the Azure Key Vault endpoint. It belongs to the target,
	// not the reusable Azure identity, so one credential set can serve many vaults.
	VaultName string `json:"vault_name,omitempty"`

	// AWS: applying a certificate to ACM and putting it in front of traffic on a
	// load balancer are separate decisions, and rotating one on a listener is a
	// third. Overloading ResourceID and Host for both ARNs made two of the three
	// impossible to express, so each has its own field.
	//
	// ApplyTo is acm, alb or acm+alb.
	ApplyTo string `json:"apply_to,omitempty"`
	// VersionPolicy is new-version (import alongside, keep the old ARN usable) or
	// replace-in-place (re-import over an existing ARN).
	VersionPolicy string `json:"version_policy,omitempty"`
	// ListenerPolicy is replace, keep-previous or add-only.
	ListenerPolicy string `json:"listener_policy,omitempty"`
	// ListenerARN is the ELBv2 listener to attach to.
	ListenerARN string `json:"listener_arn,omitempty"`
	// CertificateARN is the ACM certificate to re-import over or to attach. Set
	// by a successful deploy so verification knows what to look for.
	CertificateARN string `json:"certificate_arn,omitempty"`
	// RetainVersions is how many superseded CLM-imported certificates to keep for
	// the same name. 0 keeps everything; anything still in use is never deleted.
	RetainVersions int `json:"retain_versions,omitempty"`
	// OS-specific
	CertPath  string `json:"cert_path,omitempty"`
	KeyPath   string `json:"key_path,omitempty"`
	Service   string `json:"service,omitempty"`
	ReloadCmd string `json:"reload_cmd,omitempty"`
	// Data-specific
	ConfigFile   string `json:"config_file,omitempty"`
	ReloadMethod string `json:"reload_method,omitempty"`

	// Credentials carry the per-service secrets the deployer needs — an AWS key
	// pair, a GCP service account JSON, an Azure client secret, an SSH key. Before
	// this the deploy request carried certificate material and nothing else, so a
	// cloud target had no way to authenticate at all.
	//
	// Never logged and never echoed back in a DeployResult.
	Credentials map[string]string `json:"credentials,omitempty"`
	// CredentialRef names a stored credential set to use instead of inlining
	// values, so a secret does not have to travel in every request.
	CredentialRef string `json:"credential_ref,omitempty"`
}

// DeployRequest contains everything needed to deploy a cert to a target.
type DeployRequest struct {
	CertPEM      []byte        `json:"cert_pem"`
	KeyPEM       []byte        `json:"key_pem"`
	ChainPEM     []byte        `json:"chain_pem"`
	FullChainPEM []byte        `json:"fullchain_pem"`
	PFXPEM       []byte        `json:"pfx_pem"`
	Target       *DeployTarget `json:"target"`
}

// VerificationEvidence is non-secret structured proof of what was checked.
// It deliberately contains no credentials, certificate PEM, or private material.
type VerificationEvidence struct {
	Kind                string    `json:"kind"` // endpoint or adapter
	Method              string    `json:"method"`
	Adapter             string    `json:"adapter,omitempty"`
	Resource            string    `json:"resource,omitempty"`
	DialAddress         string    `json:"dial_address,omitempty"`
	ServerName          string    `json:"server_name,omitempty"`
	Protocol            string    `json:"protocol,omitempty"`
	ExpectedFingerprint string    `json:"expected_fingerprint,omitempty"`
	ObservedFingerprint string    `json:"observed_fingerprint,omitempty"`
	Subject             string    `json:"subject,omitempty"`
	Issuer              string    `json:"issuer,omitempty"`
	NotBefore           time.Time `json:"not_before,omitempty"`
	NotAfter            time.Time `json:"not_after,omitempty"`
	FingerprintMatch    bool      `json:"fingerprint_match,omitempty"`
	ValidityValid       bool      `json:"validity_valid,omitempty"`
	ChainTrusted        bool      `json:"chain_trusted,omitempty"`
	HostnameValid       bool      `json:"hostname_valid,omitempty"`
	IssuerMatch         bool      `json:"issuer_match,omitempty"`
	Success             bool      `json:"success"`
	CheckedAt           time.Time `json:"checked_at"`
	Message             string    `json:"message,omitempty"`
}

// DeployResult contains the outcome of a deployment.
type DeployResult struct {
	Success bool          `json:"success"`
	Target  *DeployTarget `json:"target"`
	Message string        `json:"message"`
	// AdapterResource is a non-secret provider identifier used by Verify (for
	// example a GCP sslCertificate name created during this attempt).
	AdapterResource string `json:"adapter_resource,omitempty"`
	// Evidence contains adapter checks and, for serving targets, the independent
	// network observation that is required to commit the deployment.
	Evidence []VerificationEvidence `json:"evidence,omitempty"`
	// Verified is true only when the post-deploy check confirms the target state.
	// A provider write without this confirmation is not a successful deployment.
	Verified bool `json:"verified"`
	// VerifyError says why verification did not pass.
	VerifyError string `json:"verify_error,omitempty"`
	// Rollback fields preserve the recovery evidence when a verification failure
	// causes the executor to revert the target automatically.
	RollbackAttempted bool   `json:"rollback_attempted"`
	RollbackSuccess   bool   `json:"rollback_success"`
	RollbackError     string `json:"rollback_error,omitempty"`
	// PreviousCert and PreviousFingerprint identify what was replaced. When a
	// rollback succeeds and the fingerprint is available, the executor performs
	// a fresh endpoint verification against it.
	PreviousCert        string                `json:"previous_cert,omitempty"`
	PreviousFingerprint string                `json:"previous_fingerprint,omitempty"`
	RollbackEvidence    *VerificationEvidence `json:"rollback_evidence,omitempty"`
	Duration            time.Duration         `json:"duration"`
	Error               string                `json:"error,omitempty"`
}

// Deployer is the interface for all target-specific deployers.
type Deployer interface {
	// Name returns the deployer identifier.
	Name() string
	// Category returns the deployer category: cloud, os, data, app.
	Category() string
	// Deploy installs the certificate on the target.
	Deploy(ctx context.Context, req *DeployRequest) (*DeployResult, error)
	// Verify checks adapter state and returns non-secret structured evidence. The
	// executor separately verifies any serving endpoint; adapter evidence can
	// never substitute for observing the expected certificate on the wire.
	Verify(ctx context.Context, req *DeployRequest, applied *DeployResult) (*VerificationEvidence, error)
	// Rollback reverts to the previous certificate.
	Rollback(ctx context.Context, target *DeployTarget) error
}
