package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"go.uber.org/zap"
)

// Deployment: per-service credentials, a stored-credential reference, and a
// choice of which vSatellite carries out the work.
//
// The old endpoint took certificate material and a free-form target, and nothing
// else. That left three gaps: a cloud target had no way to authenticate, the
// caller had to already know which fields a deployer reads, and everything ran on
// the gateway itself even when the target sat in a segment the gateway cannot
// reach.

// deployerSchemaFor exposes a target type's fields to the credential handler.
func deployerSchemaFor(targetType string) ([]deployers.TargetField, bool) {
	spec, ok := deployers.TargetSchemaFor(targetType)
	if !ok {
		return nil, false
	}
	return spec.Fields, true
}

// handleDeployTargets describes every deployable target type and the executors
// available, so the portal can build the form and offer the right agents.
// GET /api/v1/deploy/targets
func (s *Server) handleDeployTargets(w http.ResponseWriter, r *http.Request) {
	registered := map[string]bool{}
	if s.deployer != nil {
		for _, name := range s.deployer.DeployerNames() {
			registered[name] = true
		}
	}

	specs := deployers.TargetSchemas()
	out := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		out = append(out, map[string]any{
			"type": spec.Type, "category": spec.Category, "title": spec.Title,
			"summary": spec.Summary, "fields": spec.Fields,
			"rollback": spec.Rollback, "remote_capable": spec.RemoteCapable,
			"notes": spec.Notes,
			// A target type whose deployer is not registered would fail at dispatch,
			// so the portal is told up front. The executor keys the registry as
			// "category-name", which is what has to be compared.
			"available": registered[spec.Category+"-"+spec.Type],
		})
	}

	// Release 1 exposes only the gateway executor. vSatellite deployment remains
	// disabled until commands are authenticated, leased, acknowledged, and
	// executed with key material resolved inside the remote segment.
	executors := []map[string]any{{
		"id": "gateway", "name": s.satelliteName + " (this gateway)",
		"status": "online", "kind": "gateway",
		"detail": "runs immediately; commits only after verification succeeds",
	}}

	writeJSON(w, http.StatusOK, map[string]any{
		"targets":   out,
		"executors": executors,
		"note":      "Release 1 uses the gateway executor only. Certificate material and target credentials are resolved locally by cert_id and credential_ref.",
	})
}

// handleDeployCertificate deploys a gateway-managed certificate to a target.
// Private key material and target credentials are never accepted inline: callers
// reference gateway-local certificate and credential records instead.
// POST /api/v1/deploy
func (s *Server) handleDeployCertificate(w http.ResponseWriter, r *http.Request) {
	correlationID := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
	if correlationID == "" || len(correlationID) > 128 {
		correlationID = uuid.NewString()
	}
	r.Header.Set("X-Correlation-ID", correlationID)
	w.Header().Set("X-Correlation-ID", correlationID)

	if s.deployer == nil {
		writeErr(w, http.StatusServiceUnavailable, "the deployment executor is not configured")
		return
	}
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) && role != "operator" {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot deploy certificates")
		return
	}

	var req struct {
		CertID string                  `json:"cert_id"`
		Target *deployers.DeployTarget `json:"target"`
		// Retained only to reject legacy callers explicitly; key material must be
		// resolved from cert_id inside this gateway.
		CertPEM  string `json:"cert_pem"`
		KeyPEM   string `json:"key_pem"`
		ChainPEM string `json:"chain_pem"`
		Executor string `json:"executor"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Target == nil || strings.TrimSpace(req.Target.Type) == "" {
		writeErr(w, http.StatusBadRequest,
			"target.type is required; see GET /api/v1/deploy/targets")
		return
	}
	if req.CertPEM != "" || req.KeyPEM != "" || req.ChainPEM != "" {
		writeErr(w, http.StatusBadRequest,
			"inline certificate/private-key material is disabled; use cert_id of a certificate managed by this gateway")
		return
	}
	req.CertID = strings.TrimSpace(req.CertID)
	if req.CertID == "" {
		writeErr(w, http.StatusBadRequest,
			"cert_id is required; deployment key material must remain local to the gateway")
		return
	}
	unlockCertificate := s.lockCertificateOperation(req.CertID)
	defer unlockCertificate()
	if len(req.Target.Credentials) > 0 {
		writeErr(w, http.StatusBadRequest,
			"inline target credentials are disabled; store them first and use target.credential_ref")
		return
	}

	executor := strings.TrimSpace(req.Executor)
	if executor == "" {
		executor = "gateway"
	}
	if !strings.EqualFold(executor, "gateway") {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"status": "rejected", "executor": executor,
			"correlation_id": correlationID,
			"error":          "vSatellite certificate deployment is disabled until authenticated durable command transport is available; use the gateway executor for a reachable target",
		})
		return
	}

	spec, known := deployers.TargetSchemaFor(req.Target.Type)
	if !known {
		types := []string{}
		for _, sp := range deployers.TargetSchemas() {
			types = append(types, sp.Type)
		}
		writeErr(w, http.StatusBadRequest, fmt.Sprintf(
			"unknown target type %q; supported types are: %s",
			req.Target.Type, strings.Join(types, ", ")))
		return
	}
	if req.Target.Category != "" && !strings.EqualFold(req.Target.Category, spec.Category) {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf(
			"target category %q does not match canonical category %q for %q",
			req.Target.Category, spec.Category, req.Target.Type))
		return
	}
	req.Target.Category = spec.Category

	// Resolve credentials only inside the gateway. The response strips the
	// resolved map before serialization.
	if ref := strings.TrimSpace(req.Target.CredentialRef); ref != "" {
		if s.creds == nil || !s.creds.Enabled() {
			writeErr(w, http.StatusServiceUnavailable,
				"credential_ref was given but credential storage is unavailable: set state.dir")
			return
		}
		stored, set, err := s.creds.Reveal(ref)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if set.Kind != deployCredentialKind {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf(
				"credential set %q is kind %q, not a deployment credential set",
				ref, set.Kind))
			return
		}
		if set.Service != "" && !strings.EqualFold(set.Service, req.Target.Type) {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf(
				"credential set %q is for service %q, not %q",
				ref, set.Service, req.Target.Type))
			return
		}
		req.Target.Credentials = stored
	}

	cert, ok := s.inventory.Get(req.CertID)
	if !ok {
		writeErr(w, http.StatusNotFound, "certificate not found")
		return
	}
	if cert.ChainPEM == "" || cert.SecretRef == "" {
		writeErr(w, http.StatusBadRequest,
			"that certificate has no gateway-local key material and cannot be deployed")
		return
	}
	keyPEM, err := readKeyFile(cert.SecretRef)
	if err != nil {
		writeErr(w, http.StatusInternalServerError,
			"reading the stored private key failed: "+err.Error())
		return
	}

	if missing := deployers.MissingFields(req.Target.Type, req.Target); len(missing) > 0 {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf(
			"%s requires: %s", req.Target.Type, strings.Join(missing, ", ")))
		return
	}

	started := time.Now().UTC()
	attemptID := uuid.NewString()
	attempt := &DeploymentRecord{
		ID:            attemptID,
		AttemptID:     attemptID,
		CorrelationID: correlationID,
		TargetType:    req.Target.Type,
		TargetHost:    firstDeployHost(req.Target),
		BindingID:     deploymentBindingID(req.Target, req.Target.Type, firstDeployHost(req.Target), req.Target.Region),
		Target:        sanitizedTarget(req.Target),
		Region:        req.Target.Region,
		Executor:      "gateway",
		Status:        "deploying",
		Stage:         "apply",
		Stages: []DeploymentStage{
			{Name: "accepted", Status: "completed", StartedAt: started, CompletedAt: started},
			{Name: "apply", Status: "running", StartedAt: started},
		},
		StartedAt:  started,
		DeployedAt: started,
		DeployedBy: callerName(r),
	}
	if !s.inventory.RecordDeployment(req.CertID, attempt) {
		writeErr(w, http.StatusInternalServerError, "could not persist the deployment attempt")
		return
	}

	s.deployLocally(w, r, req.CertID, attempt, req.Target,
		cert.ChainPEM, keyPEM, cert.ChainPEM)
}

// deployLocally runs the deployment on this gateway.
func (s *Server) deployLocally(w http.ResponseWriter, r *http.Request,
	certID string, attempt *DeploymentRecord, target *deployers.DeployTarget,
	certPEM, keyPEM, chainPEM string) {

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	result, deployErr := s.deployer.Deploy(ctx, &deployers.DeployRequest{
		CertPEM:      []byte(certPEM),
		KeyPEM:       []byte(keyPEM),
		ChainPEM:     []byte(chainPEM),
		FullChainPEM: []byte(chainPEM),
		Target:       target,
	})

	// Defend the API boundary as well as the executor boundary: no adapter result
	// may become committed unless both success and verification are true.
	if deployErr == nil && (result == nil || !result.Success || !result.Verified) {
		deployErr = fmt.Errorf("deployment did not produce a verified successful result")
		if result != nil && result.Error == "" {
			result.Error = deployErr.Error()
		}
	}

	// The result echoes the target back, which carries resolved credentials.
	// Strip them before any response, log, audit event, or durable record.
	if result != nil && result.Target != nil {
		result.Target.Credentials = nil
	}
	target.Credentials = nil

	completed := time.Now().UTC()
	final := *attempt
	final.Target = sanitizedTarget(target)
	final.CompletedAt = completed
	final.DeployedAt = completed
	final.TargetARN = firstDeployARN(target, result)
	final.Message = resultMessage(result)
	final.Stages = []DeploymentStage{
		{Name: "accepted", Status: "completed", StartedAt: attempt.StartedAt, CompletedAt: attempt.StartedAt},
	}

	applySucceeded := result != nil && (result.Verified || result.VerifyError != "" || result.RollbackAttempted)
	if applySucceeded {
		final.Stages = append(final.Stages, DeploymentStage{
			Name: "apply", Status: "completed", StartedAt: attempt.StartedAt, CompletedAt: completed,
		})
	} else {
		applyError := "deployment apply failed"
		if deployErr != nil {
			applyError = deployErr.Error()
		} else if result != nil && result.Error != "" {
			applyError = result.Error
		}
		final.Stages = append(final.Stages, DeploymentStage{
			Name: "apply", Status: "failed", StartedAt: attempt.StartedAt,
			CompletedAt: completed, Error: applyError,
		})
	}

	if result != nil {
		final.Verified = result.Verified
		final.VerifyError = result.VerifyError
		final.Evidence = append([]deployers.VerificationEvidence(nil), result.Evidence...)
		final.RollbackAttempted = result.RollbackAttempted
		final.RollbackSuccess = result.RollbackSuccess
		final.RollbackError = result.RollbackError
		final.RollbackEvidence = result.RollbackEvidence
		final.PreviousCert = result.PreviousCert
		final.PreviousFingerprint = result.PreviousFingerprint
		if result.Error != "" {
			final.Error = result.Error
		}
	}
	if deployErr != nil {
		final.Error = deployErr.Error()
	}

	if applySucceeded {
		verifyStage := DeploymentStage{Name: "verify", StartedAt: completed, CompletedAt: completed}
		if result != nil && result.Verified {
			verifyStage.Status = "completed"
		} else {
			verifyStage.Status = "failed"
			verifyStage.Error = final.VerifyError
			if verifyStage.Error == "" {
				verifyStage.Error = "verification did not confirm the target state"
			}
		}
		final.Stages = append(final.Stages, verifyStage)
	}
	if final.RollbackAttempted {
		rollbackStage := DeploymentStage{Name: "rollback", StartedAt: completed, CompletedAt: completed}
		if final.RollbackSuccess {
			rollbackStage.Status = "completed"
		} else {
			rollbackStage.Status = "failed"
			rollbackStage.Error = final.RollbackError
		}
		final.Stages = append(final.Stages, rollbackStage)
	}

	committed := deployErr == nil && result != nil && result.Success && result.Verified
	if committed {
		final.Status = "deployed"
		final.Stage = "committed"
		final.Stages = append(final.Stages, DeploymentStage{
			Name: "commit", Status: "completed", StartedAt: completed, CompletedAt: completed,
		})
	} else {
		final.Status = "failed"
		switch {
		case final.RollbackSuccess:
			final.Stage = "rolled_back"
		case final.RollbackAttempted:
			final.Stage = "rollback_failed"
		default:
			final.Stage = "failed"
		}
	}

	if !s.inventory.UpdateDeployment(certID, attempt.ID, &final) {
		s.logger.Error("deployment attempt state update failed",
			zap.String("attempt_id", attempt.ID), zap.String("correlation_id", attempt.CorrelationID))
	}

	if !committed {
		failure := final.Error
		if failure == "" {
			failure = "deployment failed verification"
		}
		s.logger.Error("gateway deployment failed",
			zap.String("type", target.Type), zap.String("host", target.Host),
			zap.String("attempt_id", attempt.ID), zap.String("correlation_id", attempt.CorrelationID),
			zap.String("stage", final.Stage), zap.String("error", failure))
		s.recordAudit(r, "deployment.deploy", "deployment", attempt.ID, target.Type,
			audit.ResultFailure, fmt.Sprintf("stage=%s rollback_attempted=%t rollback_success=%t error=%s",
				final.Stage, final.RollbackAttempted, final.RollbackSuccess, failure))
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"status": "failed", "executor": "gateway",
			"correlation_id": attempt.CorrelationID, "attempt_id": attempt.ID,
			"error": failure, "result": result, "deployment": &final,
		})
		return
	}

	s.logger.Info("deployment verified and committed",
		zap.String("executor", "gateway"), zap.String("type", target.Type),
		zap.String("attempt_id", attempt.ID), zap.String("correlation_id", attempt.CorrelationID),
		zap.String("by", callerName(r)))
	s.recordAudit(r, "deployment.deploy", "deployment", attempt.ID, target.Type,
		audit.ResultSuccess, "verified and committed to "+firstDeployHost(target))
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "completed", "executor": "gateway",
		"correlation_id": attempt.CorrelationID, "attempt_id": attempt.ID,
		"result": result, "deployment": &final,
	})
}

// firstDeployHost picks the most recognisable identifier for where a cert was
// pushed. This is the value in the Deployed Endpoints view's "where" column.
func firstDeployHost(t *deployers.DeployTarget) string {
	if t == nil {
		return ""
	}
	if t.ServingHost != "" {
		return t.ServingHost
	}
	if t.Host != "" {
		return t.Host
	}
	if t.ListenerARN != "" {
		return t.ListenerARN
	}
	if t.ResourceID != "" {
		return t.ResourceID
	}
	return ""
}

// firstDeployARN picks the cloud ARN that identifies this deployment for verify.
func firstDeployARN(t *deployers.DeployTarget, r *deployers.DeployResult) string {
	if t == nil {
		return ""
	}
	if t.CertificateARN != "" {
		return t.CertificateARN
	}
	if r != nil && r.Target != nil && r.Target.CertificateARN != "" {
		return r.Target.CertificateARN
	}
	return ""
}

func resultMessage(r *deployers.DeployResult) string {
	if r == nil {
		return ""
	}
	return r.Message
}
