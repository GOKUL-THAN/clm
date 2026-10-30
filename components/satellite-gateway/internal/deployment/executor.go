package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"go.uber.org/zap"
)

// Executor orchestrates certificate deployments to multiple targets.
type Executor struct {
	deployers map[string]deployers.Deployer
	logger    *zap.Logger
}

// NewExecutor creates a new deployment executor.
func NewExecutor(logger *zap.Logger) *Executor {
	return &Executor{
		deployers: make(map[string]deployers.Deployer),
		logger:    logger,
	}
}

// RegisterDeployer adds a deployer plugin.
func (e *Executor) RegisterDeployer(d deployers.Deployer) {
	key := d.Category() + "-" + d.Name()
	e.deployers[key] = d
	e.logger.Info("registered deployer", zap.String("name", key))
}

// Deploy executes a deployment to a single target. A deployment is successful
// only after verification confirms the target state. If apply succeeds but
// verification does not, the executor attempts rollback and returns a non-nil
// error with the verification and rollback evidence retained on the result.
func (e *Executor) Deploy(ctx context.Context, req *deployers.DeployRequest) (*deployers.DeployResult, error) {
	if req == nil || req.Target == nil {
		return nil, fmt.Errorf("deployment target is required")
	}
	target := req.Target
	key := target.Category + "-" + target.Type

	d, ok := e.deployers[key]
	if !ok {
		return nil, fmt.Errorf("no deployer registered for target type: %s", key)
	}

	// Preflight must happen before Deploy: a cloud API write or file replacement
	// is already a side effect, and a listener ARN is never a dialable endpoint.
	fingerprint, err := deployers.LeafSHA256(req.CertPEM)
	if err != nil {
		return nil, fmt.Errorf("deployment certificate is invalid: %w", err)
	}
	endpoint, endpointRequired, err := endpointOptions(req, fingerprint)
	if err != nil {
		return nil, err
	}

	e.logger.Info("starting deployment",
		zap.String("target", key),
		zap.String("host", target.Host),
	)

	result, err := d.Deploy(ctx, req)
	if err != nil {
		e.logger.Error("deployment failed",
			zap.String("target", key),
			zap.Error(err),
		)
		return result, err
	}
	if result == nil {
		return nil, fmt.Errorf("deployer %s returned no result", key)
	}
	if !result.Success {
		err := fmt.Errorf("deployer %s reported an unsuccessful apply", key)
		if result.Error == "" {
			result.Error = err.Error()
		}
		return result, err
	}

	adapterEvidence, adapterErr := d.Verify(ctx, req, result)
	if adapterEvidence != nil {
		result.Evidence = append(result.Evidence, *adapterEvidence)
	}

	// Always perform the endpoint check when required, even if the adapter check
	// failed. This prevents file/provider API evidence from hiding what is really
	// being served and leaves useful network evidence on the failed attempt.
	var endpointEvidence *deployers.VerificationEvidence
	var endpointErr error
	if endpointRequired {
		endpointEvidence, endpointErr = deployers.VerifyEndpoint(ctx, endpoint)
		if endpointEvidence != nil {
			result.Evidence = append(result.Evidence, *endpointEvidence)
		}
	}

	verified := adapterErr == nil && adapterEvidence != nil && adapterEvidence.Success
	if endpointRequired {
		verified = verified && endpointErr == nil && endpointEvidence != nil && endpointEvidence.Success
	}
	result.Verified = verified
	if verified {
		return result, nil
	}

	verifyErr := errorsForVerification(adapterErr, adapterEvidence, endpointErr, endpointEvidence)
	result.Success = false
	result.VerifyError = verifyErr.Error()
	result.Error = "deployment verification failed: " + verifyErr.Error()
	result.RollbackAttempted = true

	e.logger.Warn("deployment verification failed; attempting rollback",
		zap.String("target", key),
		zap.Error(verifyErr),
	)
	rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancelRollback()
	if rollbackErr := d.Rollback(rollbackCtx, target); rollbackErr != nil {
		result.RollbackError = rollbackErr.Error()
		e.logger.Error("automatic deployment rollback failed",
			zap.String("target", key),
			zap.Error(rollbackErr),
		)
	} else {
		result.RollbackSuccess = true
		if endpointRequired && result.PreviousFingerprint != "" {
			rollbackEndpoint := endpoint
			rollbackEndpoint.ExpectedFingerprint = result.PreviousFingerprint
			rollbackEvidence, rollbackVerifyErr := deployers.VerifyEndpoint(rollbackCtx, rollbackEndpoint)
			result.RollbackEvidence = rollbackEvidence
			if rollbackVerifyErr != nil {
				result.RollbackSuccess = false
				result.RollbackError = "rollback endpoint verification failed: " + rollbackVerifyErr.Error()
			} else {
				e.logger.Info("automatic rollback independently verified on endpoint",
					zap.String("target", key))
			}
		}
		if result.RollbackSuccess {
			e.logger.Info("automatic deployment rollback completed", zap.String("target", key))
		}
	}

	return result, fmt.Errorf("%s", result.Error)
}

func endpointOptions(req *deployers.DeployRequest, fingerprint string) (deployers.EndpointVerifyOptions, bool, error) {
	target := req.Target
	targetType := strings.ToLower(strings.TrimSpace(target.Type))
	explicit := strings.TrimSpace(target.ServingHost) != ""
	required := explicit
	protocol := "tls"

	switch targetType {
	case "linux":
		required = true
	case "postgresql":
		required = true
		protocol = "postgresql"
	case "aws-acm":
		applyTo := strings.ToLower(strings.TrimSpace(target.ApplyTo))
		hasListener := strings.TrimSpace(target.ListenerARN) != "" ||
			strings.Contains(target.ResourceID, ":listener/") ||
			strings.Contains(target.Host, ":listener/")
		if applyTo == "alb" || applyTo == "acm+alb" || (applyTo == "" && hasListener) {
			required = true
			if !explicit {
				return deployers.EndpointVerifyOptions{}, false, fmt.Errorf(
					"AWS listener deployments require target.serving_host; a listener ARN is not a network host")
			}
		}
	case "gcp-lb":
		if strings.TrimSpace(target.ResourceID) != "" {
			required = true
			if !explicit {
				return deployers.EndpointVerifyOptions{}, false, fmt.Errorf(
					"GCP target proxy deployments require target.serving_host for post-deploy endpoint verification")
			}
		}
	}

	if !required {
		return deployers.EndpointVerifyOptions{}, false, nil
	}
	host := strings.TrimSpace(target.ServingHost)
	if host == "" && (targetType == "linux" || targetType == "postgresql") {
		host = strings.TrimSpace(target.Host)
	}
	if host == "" {
		return deployers.EndpointVerifyOptions{}, false, fmt.Errorf(
			"target.serving_host is required for endpoint verification")
	}
	if strings.HasPrefix(host, "arn:") || strings.Contains(host, ":listener/") {
		return deployers.EndpointVerifyOptions{}, false, fmt.Errorf(
			"target.serving_host must be a DNS name or IP address, not an ARN")
	}

	port := target.ServingPort
	if port == 0 {
		port = target.Port
	}
	if port == 0 {
		if protocol == "postgresql" {
			port = 5432
		} else {
			port = 443
		}
	}
	serverName := strings.TrimSpace(target.ServerName)
	if serverName == "" {
		serverName = host
	}

	return deployers.EndpointVerifyOptions{
		Host:                host,
		Port:                port,
		ServerName:          serverName,
		Protocol:            protocol,
		ExpectedFingerprint: fingerprint,
		ExpectedIssuer:      target.ExpectedIssuer,
		TrustPEM:            req.ChainPEM,
	}, true, nil
}

func errorsForVerification(adapterErr error, adapter *deployers.VerificationEvidence,
	endpointErr error, endpoint *deployers.VerificationEvidence) error {
	var failures []string
	if adapterErr != nil {
		failures = append(failures, "adapter verification: "+adapterErr.Error())
	} else if adapter == nil || !adapter.Success {
		failures = append(failures, "adapter verification returned no successful evidence")
	}
	if endpointErr != nil {
		failures = append(failures, "endpoint verification: "+endpointErr.Error())
	} else if endpoint != nil && !endpoint.Success {
		failures = append(failures, "endpoint verification returned unsuccessful evidence")
	}
	if len(failures) == 0 {
		failures = append(failures, "verification did not confirm the deployed target state")
	}
	return fmt.Errorf("%s", strings.Join(failures, "; "))
}

// DeployGroup deploys to multiple targets sequentially or in parallel.
func (e *Executor) DeployGroup(ctx context.Context, req *deployers.DeployRequest, targets []*deployers.DeployTarget, parallel bool) ([]*deployers.DeployResult, error) {
	if parallel {
		return e.deployParallel(ctx, req, targets)
	}
	return e.deploySequential(ctx, req, targets)
}

func (e *Executor) deploySequential(ctx context.Context, req *deployers.DeployRequest, targets []*deployers.DeployTarget) ([]*deployers.DeployResult, error) {
	var results []*deployers.DeployResult

	for _, target := range targets {
		singleReq := &deployers.DeployRequest{
			CertPEM:      req.CertPEM,
			KeyPEM:       req.KeyPEM,
			ChainPEM:     req.ChainPEM,
			FullChainPEM: req.FullChainPEM,
			PFXPEM:       req.PFXPEM,
			Target:       target,
		}

		result, err := e.Deploy(ctx, singleReq)
		if err != nil {
			results = append(results, &deployers.DeployResult{
				Success: false,
				Target:  target,
				Error:   err.Error(),
			})
			// Stop on first failure in sequential mode
			break
		}
		results = append(results, result)
	}

	return results, nil
}

func (e *Executor) deployParallel(ctx context.Context, req *deployers.DeployRequest, targets []*deployers.DeployTarget) ([]*deployers.DeployResult, error) {
	resultCh := make(chan *deployers.DeployResult, len(targets))

	for _, target := range targets {
		go func(t *deployers.DeployTarget) {
			singleReq := &deployers.DeployRequest{
				CertPEM:      req.CertPEM,
				KeyPEM:       req.KeyPEM,
				ChainPEM:     req.ChainPEM,
				FullChainPEM: req.FullChainPEM,
				PFXPEM:       req.PFXPEM,
				Target:       t,
			}

			result, err := e.Deploy(ctx, singleReq)
			if err != nil {
				resultCh <- &deployers.DeployResult{
					Success: false,
					Target:  t,
					Error:   err.Error(),
				}
				return
			}
			resultCh <- result
		}(target)
	}

	var results []*deployers.DeployResult
	for i := 0; i < len(targets); i++ {
		select {
		case r := <-resultCh:
			results = append(results, r)
		case <-time.After(5 * time.Minute):
			results = append(results, &deployers.DeployResult{Success: false, Error: "timeout"})
		}
	}

	return results, nil
}

// Rollback reverts a deployment on the specified target.
func (e *Executor) Rollback(ctx context.Context, target *deployers.DeployTarget) error {
	key := target.Category + "-" + target.Type
	d, ok := e.deployers[key]
	if !ok {
		return fmt.Errorf("no deployer for: %s", key)
	}

	return d.Rollback(ctx, target)
}

// ParseDeployPayload parses a JSON command payload into deploy request.
func ParseDeployPayload(payload string) (*deployers.DeployRequest, error) {
	var req deployers.DeployRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// DeployerNames returns the registered deployer identifiers.
//
// The portal needs this to tell a target type that is merely described from one
// that can actually be dispatched: offering a form for a deployer that was never
// registered produces a failure at submit time with nothing to explain it.
// The returned names are the registration keys, "category-name", which is what
// Deploy looks up.
func (e *Executor) DeployerNames() []string {
	// No locking: deployers are registered during startup, before the API accepts
	// a request, and never afterwards.
	names := make([]string, 0, len(e.deployers))
	for name := range e.deployers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
