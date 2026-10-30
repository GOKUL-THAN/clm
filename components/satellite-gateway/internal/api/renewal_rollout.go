package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/satellite/ca"
	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"go.uber.org/zap"
)

func sanitizedTarget(target *deployers.DeployTarget) *deployers.DeployTarget {
	if target == nil {
		return nil
	}
	out := *target
	out.Credentials = nil
	return &out
}

type deploymentBindingIdentity struct {
	Category       string `json:"category,omitempty"`
	Type           string `json:"type"`
	Host           string `json:"host,omitempty"`
	Port           int    `json:"port,omitempty"`
	ServingHost    string `json:"serving_host,omitempty"`
	ServingPort    int    `json:"serving_port,omitempty"`
	ServerName     string `json:"server_name,omitempty"`
	ResourceID     string `json:"resource_id,omitempty"`
	Region         string `json:"region,omitempty"`
	VaultName      string `json:"vault_name,omitempty"`
	ApplyTo        string `json:"apply_to,omitempty"`
	ListenerARN    string `json:"listener_arn,omitempty"`
	CertificateARN string `json:"certificate_arn,omitempty"`
	CertPath       string `json:"cert_path,omitempty"`
	KeyPath        string `json:"key_path,omitempty"`
	ConfigFile     string `json:"config_file,omitempty"`
}

func deploymentBindingID(target *deployers.DeployTarget, fallbackType, fallbackHost, fallbackRegion string) string {
	identity := deploymentBindingIdentity{
		Type:   strings.ToLower(strings.TrimSpace(fallbackType)),
		Host:   strings.ToLower(strings.TrimSpace(fallbackHost)),
		Region: strings.ToLower(strings.TrimSpace(fallbackRegion)),
	}
	if target != nil {
		identity = deploymentBindingIdentity{
			Category:    strings.ToLower(strings.TrimSpace(target.Category)),
			Type:        strings.ToLower(strings.TrimSpace(target.Type)),
			Host:        strings.ToLower(strings.TrimSpace(target.Host)),
			Port:        target.Port,
			ServingHost: strings.ToLower(strings.TrimSpace(target.ServingHost)),
			ServingPort: target.ServingPort,
			ServerName:  strings.ToLower(strings.TrimSpace(target.ServerName)),
			ResourceID:  strings.TrimSpace(target.ResourceID),
			Region:      strings.ToLower(strings.TrimSpace(target.Region)),
			VaultName:   strings.ToLower(strings.TrimSpace(target.VaultName)),
			ApplyTo:     strings.ToLower(strings.TrimSpace(target.ApplyTo)),
			ListenerARN: strings.TrimSpace(target.ListenerARN),
			CertPath:    strings.TrimSpace(target.CertPath),
			KeyPath:     strings.TrimSpace(target.KeyPath),
			ConfigFile:  strings.TrimSpace(target.ConfigFile),
		}

		if identity.Type == "aws-acm" {
			listenerARN := identity.ListenerARN
			if listenerARN == "" {
				legacyListener := firstNonEmpty(identity.ResourceID, identity.Host)
				if strings.Contains(legacyListener, ":listener") {
					listenerARN = legacyListener
				}
			}
			certificateARN := strings.TrimSpace(target.CertificateARN)
			if certificateARN == "" && strings.Contains(identity.ResourceID, ":certificate/") {
				certificateARN = identity.ResourceID
			}
			if identity.ApplyTo == "" {
				if listenerARN != "" {
					identity.ApplyTo = "acm+alb"
				} else {
					identity.ApplyTo = "acm"
				}
			}
			identity.ListenerARN = listenerARN
			if listenerARN != "" {
				// Normalize legacy listener_arn spellings in resource_id or host.
				identity.ResourceID = ""
				identity.Host = ""
			}
			if strings.EqualFold(target.VersionPolicy, "replace-in-place") || identity.ApplyTo == "alb" {
				identity.CertificateARN = certificateARN
			}
		}
	}
	encoded, _ := json.Marshal(identity)
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", sum[:])
}

func cleanupUntrackedIssuedKey(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing untracked issued private key: %w", err)
	}
	return nil
}

func cloneVersion(version *CertificateVersion) *CertificateVersion {
	if version == nil {
		return nil
	}
	out := *version
	return &out
}

// cloneCertificateCheckpoint deep-copies mutable certificate state so durable
// snapshots can be encoded after releasing Inventory.mu without racing writers.
func cloneCertificateCheckpoint(cert *Certificate) *Certificate {
	if cert == nil {
		return nil
	}
	out := *cert
	out.SANs = append([]string(nil), cert.SANs...)
	out.Validation = append([]ca.ValidationInstruction(nil), cert.Validation...)
	out.Versions = make([]*CertificateVersion, len(cert.Versions))
	for n, version := range cert.Versions {
		out.Versions[n] = cloneVersion(version)
	}
	if cert.Renewal != nil {
		renewal := *cert.Renewal
		out.Renewal = &renewal
	}
	out.Deployments = make([]*DeploymentRecord, len(cert.Deployments))
	for n, deployment := range cert.Deployments {
		if deployment == nil {
			continue
		}
		copy := *deployment
		copy.Target = sanitizedTarget(deployment.Target)
		copy.Stages = append([]DeploymentStage(nil), deployment.Stages...)
		copy.Evidence = append([]deployers.VerificationEvidence(nil), deployment.Evidence...)
		if deployment.RollbackEvidence != nil {
			evidence := *deployment.RollbackEvidence
			copy.RollbackEvidence = &evidence
		}
		out.Deployments[n] = &copy
	}
	return &out
}

func ensureActiveVersionLocked(cert *Certificate) (*CertificateVersion, error) {
	if cert.ActiveVersionID != "" {
		for _, version := range cert.Versions {
			if version != nil && version.ID == cert.ActiveVersionID {
				return version, nil
			}
		}
		return nil, fmt.Errorf("active certificate version %q is missing", cert.ActiveVersionID)
	}
	if cert.ChainPEM == "" || cert.SecretRef == "" {
		return nil, fmt.Errorf("active certificate material is incomplete")
	}
	fingerprint := cert.Fingerprint
	if fingerprint == "" {
		var err error
		fingerprint, err = deployers.LeafSHA256([]byte(cert.ChainPEM))
		if err != nil {
			return nil, fmt.Errorf("parsing active certificate: %w", err)
		}
	}
	issuedAt := cert.IssuedAt
	if issuedAt.IsZero() {
		issuedAt = cert.CreatedAt
	}
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	version := &CertificateVersion{
		ID: uuid.NewString(), Number: 1, Status: "active",
		Serial: cert.SerialNumber, Fingerprint: fingerprint, Issuer: cert.Issuer,
		NotBefore: cert.NotBefore, NotAfter: cert.NotAfter, ChainPEM: cert.ChainPEM,
		SecretRef: cert.SecretRef, IssuedAt: issuedAt, ActivatedAt: issuedAt,
	}
	cert.Fingerprint = fingerprint
	cert.Versions = append(cert.Versions, version)
	cert.ActiveVersionID = version.ID
	return version, nil
}

func (i *Inventory) beginRenewal(certID string) (*CertificateVersion, *RenewalAttempt, error) {
	if i.store == nil || !i.store.Enabled() {
		return nil, nil, fmt.Errorf("durable state is required for renewal redeployment")
	}
	i.mu.Lock()
	cert, ok := i.certs[certID]
	if !ok {
		i.mu.Unlock()
		return nil, nil, fmt.Errorf("certificate %q not found", certID)
	}
	if cert.Revoked || !cert.RevokedAt.IsZero() {
		i.mu.Unlock()
		return nil, nil, fmt.Errorf("certificate %q is revoked and cannot be renewed", certID)
	}
	if cert.ManagedBy != "acme" {
		i.mu.Unlock()
		return nil, nil, fmt.Errorf("certificate %q is not managed by ACME", certID)
	}
	if cert.Renewal != nil {
		switch cert.Renewal.Status {
		case "issuing", "deploying", "recovering":
			i.mu.Unlock()
			return nil, nil, fmt.Errorf("renewal %s is already in progress", cert.Renewal.ID)
		case "recovery_required":
			i.mu.Unlock()
			return nil, nil, fmt.Errorf("renewal %s requires endpoint reconciliation before another renewal", cert.Renewal.ID)
		}
	}
	before := cloneCertificateCheckpoint(cert)
	active, err := ensureActiveVersionLocked(cert)
	if err != nil {
		i.mu.Unlock()
		return nil, nil, err
	}
	attempt := &RenewalAttempt{ID: uuid.NewString(), Status: "issuing", StartedAt: time.Now().UTC()}
	cert.Renewal = attempt
	cert.Status = "renewing"
	cert.LastError = ""
	activeCopy := cloneVersion(active)
	attemptCopy := *attempt
	if err := i.persistLocked(); err != nil {
		i.certs[certID] = before
		i.mu.Unlock()
		return nil, nil, fmt.Errorf("persisting renewal checkpoint: %w", err)
	}
	i.mu.Unlock()
	return activeCopy, &attemptCopy, nil
}

func (i *Inventory) setRenewalCandidate(certID, attemptID string, candidate *CertificateVersion) error {
	if candidate == nil {
		return fmt.Errorf("candidate certificate version is required")
	}
	i.mu.Lock()
	cert, ok := i.certs[certID]
	if !ok || cert.Renewal == nil || cert.Renewal.ID != attemptID {
		i.mu.Unlock()
		return fmt.Errorf("renewal attempt %q is no longer current", attemptID)
	}
	before := cloneCertificateCheckpoint(cert)
	maxVersion := 0
	for _, version := range cert.Versions {
		if version != nil && version.Number > maxVersion {
			maxVersion = version.Number
		}
	}
	candidate.Number = maxVersion + 1
	candidate.Status = "candidate"
	cert.Versions = append(cert.Versions, candidate)
	cert.Renewal.CandidateVersionID = candidate.ID
	cert.Renewal.Status = "deploying"
	if err := i.persistLocked(); err != nil {
		i.certs[certID] = before
		i.mu.Unlock()
		return fmt.Errorf("persisting renewal candidate: %w", err)
	}
	i.mu.Unlock()
	return nil
}

func (i *Inventory) commitRenewal(certID, attemptID, candidateID string) error {
	i.mu.Lock()
	cert, ok := i.certs[certID]
	if !ok || cert.Renewal == nil || cert.Renewal.ID != attemptID {
		i.mu.Unlock()
		return fmt.Errorf("renewal attempt %q is no longer current", attemptID)
	}
	before := cloneCertificateCheckpoint(cert)
	var candidate *CertificateVersion
	for _, version := range cert.Versions {
		if version != nil && version.ID == candidateID {
			candidate = version
			break
		}
	}
	if candidate == nil {
		i.mu.Unlock()
		return fmt.Errorf("candidate certificate version %q is missing", candidateID)
	}
	for _, deployment := range cert.Deployments {
		if deployment == nil || deployment.Status != "deployed" || !deployment.Verified {
			continue
		}
		if deployment.RenewalAttemptID != attemptID || deployment.CertificateVersion != candidate.Number {
			i.mu.Unlock()
			return fmt.Errorf("active deployment binding %q is not verified on renewal candidate version %d",
				deployment.BindingID, candidate.Number)
		}
	}

	now := time.Now().UTC()
	for _, version := range cert.Versions {
		if version == nil {
			continue
		}
		if version.ID == candidateID {
			version.Status = "active"
			version.ActivatedAt = now
			continue
		}
		if version.ID == cert.ActiveVersionID && version.Status == "active" {
			version.Status = "retained"
			version.RetiredAt = now
		}
	}
	cert.ActiveVersionID = candidate.ID
	cert.SerialNumber = candidate.Serial
	cert.Fingerprint = candidate.Fingerprint
	cert.Issuer = candidate.Issuer
	cert.CAName = candidate.Issuer
	cert.NotBefore = candidate.NotBefore
	cert.NotAfter = candidate.NotAfter
	cert.ChainPEM = candidate.ChainPEM
	cert.SecretRef = candidate.SecretRef
	cert.Status, cert.DaysLeft = statusFor(candidate.NotAfter)
	cert.RenewedAt = now
	cert.RenewalCount++
	cert.LastError = ""
	cert.Renewal.Status = "committed"
	cert.Renewal.CompletedAt = now
	if err := i.persistLocked(); err != nil {
		commitErr := fmt.Errorf("persisting renewal commit: %w", err)
		// Endpoints may already serve the candidate. Restore the previous active
		// lineage in memory, block further renewal, and durably request an explicit
		// reconciliation rather than pretending the commit succeeded.
		i.certs[certID] = before
		before.Renewal.Status = "recovery_required"
		before.Renewal.Error = commitErr.Error()
		before.Renewal.CompletedAt = now
		before.LastError = commitErr.Error()
		for _, version := range before.Versions {
			if version != nil && version.ID == candidateID {
				version.Status = "recovery_required"
			}
		}
		markerErr := i.persistLocked()
		i.mu.Unlock()
		if markerErr != nil {
			return errors.Join(commitErr, fmt.Errorf("persisting recovery marker: %w", markerErr))
		}
		return commitErr
	}
	i.mu.Unlock()
	return nil
}

func (i *Inventory) failRenewal(certID, attemptID, candidateID, message string, recoveryRequired bool) error {
	i.mu.Lock()
	cert, ok := i.certs[certID]
	if !ok || cert.Renewal == nil || cert.Renewal.ID != attemptID {
		i.mu.Unlock()
		return fmt.Errorf("renewal attempt %q is no longer current", attemptID)
	}
	state := "failed"
	if recoveryRequired {
		state = "recovery_required"
	}
	for _, version := range cert.Versions {
		if version != nil && version.ID == candidateID {
			version.Status = state
		}
	}
	cert.Status, cert.DaysLeft = statusFor(cert.NotAfter)
	cert.LastError = message
	cert.Renewal.Status = state
	cert.Renewal.Error = message
	cert.Renewal.CompletedAt = time.Now().UTC()
	if err := i.persistLocked(); err != nil {
		persistErr := fmt.Errorf("persisting failed renewal: %w", err)
		// A failed status that is not durable must not allow another attempt in
		// this process. Escalate it to recovery_required and make one best-effort
		// durable write; the prior issuing/deploying snapshot also fails safe on
		// restart by being converted to recovery_required during load.
		cert.Renewal.Status = "recovery_required"
		cert.Renewal.Error = errors.Join(errors.New(message), persistErr).Error()
		cert.LastError = cert.Renewal.Error
		for _, version := range cert.Versions {
			if version != nil && version.ID == candidateID {
				version.Status = "recovery_required"
			}
		}
		markerErr := i.persistLocked()
		i.mu.Unlock()
		if markerErr != nil {
			return errors.Join(persistErr, fmt.Errorf("persisting recovery marker: %w", markerErr))
		}
		return persistErr
	}
	i.mu.Unlock()
	return nil
}

func activeDeploymentTargets(cert *Certificate) ([]*deployers.DeployTarget, error) {
	if cert == nil {
		return nil, fmt.Errorf("certificate is required")
	}

	// Historical records may carry binding IDs produced by an older projection.
	// Recompute physical identity and keep the last appended verified record for
	// each binding, because append order is the durable operation order. This
	// ensures credential/policy rotations replay the newest configuration.
	latest := make(map[string]*DeploymentRecord)
	order := make([]string, 0)
	for _, deployment := range cert.Deployments {
		if deployment == nil || deployment.Status != "deployed" || !deployment.Verified {
			continue
		}
		key := deploymentBindingID(deployment.Target, deployment.TargetType, deployment.TargetHost, deployment.Region)
		if _, exists := latest[key]; !exists {
			order = append(order, key)
		}
		latest[key] = deployment
	}

	targets := make([]*deployers.DeployTarget, 0, len(order))
	var missing []string
	for _, key := range order {
		deployment := latest[key]
		if deployment.Target == nil {
			missing = append(missing, nonEmpty(deployment.TargetHost, deployment.ID))
			continue
		}
		targets = append(targets, sanitizedTarget(deployment.Target))
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("active deployment bindings lack replayable target metadata: %s", strings.Join(missing, ", "))
	}
	return targets, nil
}

func (s *Server) resolveRenewalCredentials(target *deployers.DeployTarget) error {
	ref := strings.TrimSpace(target.CredentialRef)
	if ref == "" {
		return nil
	}
	if s.creds == nil || !s.creds.Enabled() {
		return fmt.Errorf("credential_ref %q cannot be resolved because credential storage is unavailable", ref)
	}
	credentials, set, err := s.creds.Reveal(ref)
	if err != nil {
		return err
	}
	if set.Kind != deployCredentialKind {
		return fmt.Errorf("credential set %q is kind %q, not a deployment credential set", ref, set.Kind)
	}
	if set.Service != "" && !strings.EqualFold(set.Service, target.Type) {
		return fmt.Errorf("credential set %q is for service %q, not %q", ref, set.Service, target.Type)
	}
	target.Credentials = credentials
	return nil
}

func (s *Server) deployRenewalVersion(ctx context.Context, certID string, version *CertificateVersion,
	renewalID string, target *deployers.DeployTarget, actor string) (*DeploymentRecord, error) {
	if version == nil || target == nil {
		return nil, fmt.Errorf("certificate version and deployment target are required")
	}
	workTarget := sanitizedTarget(target)
	if err := s.resolveRenewalCredentials(workTarget); err != nil {
		return nil, fmt.Errorf("resolving target credentials: %w", err)
	}
	keyPEM, err := readKeyFile(version.SecretRef)
	if err != nil {
		workTarget.Credentials = nil
		return nil, fmt.Errorf("reading version %d private key: %w", version.Number, err)
	}

	started := time.Now().UTC()
	attemptID := uuid.NewString()
	attempt := &DeploymentRecord{
		ID: attemptID, AttemptID: attemptID, CorrelationID: renewalID,
		TargetType: workTarget.Type, TargetHost: firstDeployHost(workTarget),
		BindingID: deploymentBindingID(workTarget, workTarget.Type, firstDeployHost(workTarget), workTarget.Region),
		Target:    sanitizedTarget(workTarget), Region: workTarget.Region,
		CertificateVersion: version.Number, RenewalAttemptID: renewalID,
		Executor: "gateway", Status: "deploying", Stage: "apply",
		Stages: []DeploymentStage{
			{Name: "accepted", Status: "completed", StartedAt: started, CompletedAt: started},
			{Name: "apply", Status: "running", StartedAt: started},
		},
		StartedAt: started, DeployedAt: started, DeployedBy: actor,
	}
	if !s.inventory.RecordDeployment(certID, attempt) {
		workTarget.Credentials = nil
		return nil, fmt.Errorf("recording renewal deployment attempt")
	}
	if err := s.inventory.PersistNow(); err != nil {
		workTarget.Credentials = nil
		return nil, fmt.Errorf("persisting renewal deployment attempt: %w", err)
	}

	result, deployErr := s.deployer.Deploy(ctx, &deployers.DeployRequest{
		CertPEM: []byte(version.ChainPEM), KeyPEM: []byte(keyPEM),
		ChainPEM: []byte(version.ChainPEM), FullChainPEM: []byte(version.ChainPEM),
		Target: workTarget,
	})
	if deployErr == nil && (result == nil || !result.Success || !result.Verified) {
		deployErr = fmt.Errorf("deployment did not produce a verified successful result")
	}
	if result != nil && result.Target != nil {
		result.Target.Credentials = nil
	}
	workTarget.Credentials = nil

	completed := time.Now().UTC()
	final := *attempt
	final.CompletedAt = completed
	final.DeployedAt = completed
	final.Target = sanitizedTarget(workTarget)
	final.TargetARN = firstDeployARN(workTarget, result)
	final.Message = resultMessage(result)
	final.Stages = []DeploymentStage{
		{Name: "accepted", Status: "completed", StartedAt: started, CompletedAt: started},
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
		final.Error = result.Error
	}
	if deployErr != nil {
		final.Error = deployErr.Error()
	}
	applyStatus := "completed"
	if result == nil || (!result.Verified && result.VerifyError == "" && !result.RollbackAttempted) {
		applyStatus = "failed"
	}
	final.Stages = append(final.Stages, DeploymentStage{Name: "apply", Status: applyStatus, StartedAt: started, CompletedAt: completed, Error: final.Error})
	if result != nil && (result.Verified || result.VerifyError != "" || result.RollbackAttempted) {
		verifyStatus := "failed"
		if result.Verified {
			verifyStatus = "completed"
		}
		final.Stages = append(final.Stages, DeploymentStage{Name: "verify", Status: verifyStatus, StartedAt: completed, CompletedAt: completed, Error: result.VerifyError})
	}
	if final.RollbackAttempted {
		rollbackStatus := "failed"
		if final.RollbackSuccess {
			rollbackStatus = "completed"
		}
		final.Stages = append(final.Stages, DeploymentStage{Name: "rollback", Status: rollbackStatus, StartedAt: completed, CompletedAt: completed, Error: final.RollbackError})
	}
	committed := deployErr == nil && result != nil && result.Success && result.Verified
	if committed {
		final.Status = "deployed"
		final.Stage = "committed"
		final.Stages = append(final.Stages, DeploymentStage{Name: "commit", Status: "completed", StartedAt: completed, CompletedAt: completed})
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
	if !s.inventory.UpdateDeployment(certID, attemptID, &final) {
		return &final, fmt.Errorf("updating renewal deployment attempt")
	}
	if err := s.inventory.PersistNow(); err != nil {
		return &final, fmt.Errorf("persisting renewal deployment outcome: %w", err)
	}
	if !committed {
		return &final, fmt.Errorf("deployment to %s failed: %s", firstDeployHost(workTarget), nonEmpty(final.Error, "verification failed"))
	}
	return &final, nil
}

func (s *Server) rolloutRenewal(ctx context.Context, cert *Certificate, active, candidate *CertificateVersion, attempt *RenewalAttempt) error {
	targets, err := activeDeploymentTargets(cert)
	if err != nil {
		return err
	}
	commit := func() error {
		if commitErr := s.inventory.commitRenewal(cert.ID, attempt.ID, candidate.ID); commitErr != nil {
			if markerErr := s.inventory.failRenewal(cert.ID, attempt.ID, candidate.ID, commitErr.Error(), true); markerErr != nil {
				return errors.Join(commitErr, markerErr)
			}
			return commitErr
		}
		return nil
	}
	if len(targets) == 0 {
		return commit()
	}

	succeeded := make([]*deployers.DeployTarget, 0, len(targets))
	for _, target := range targets {
		if _, err := s.deployRenewalVersion(ctx, cert.ID, candidate, attempt.ID, target, "renewal-scheduler"); err != nil {
			// Reapply the prior version to the failing target first because an
			// adapter error or result-persistence failure can happen after external
			// mutation. Then compensate every earlier verified candidate target in
			// reverse order.
			recoveryTargets := make([]*deployers.DeployTarget, 0, len(succeeded)+1)
			recoveryTargets = append(recoveryTargets, sanitizedTarget(target))
			for n := len(succeeded) - 1; n >= 0; n-- {
				recoveryTargets = append(recoveryTargets, succeeded[n])
			}

			var recoveryErrors []error
			for _, recoveryTarget := range recoveryTargets {
				recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
				_, recoveryErr := s.deployRenewalVersion(recoveryCtx, cert.ID, active, attempt.ID, recoveryTarget, "renewal-recovery")
				cancel()
				if recoveryErr != nil {
					recoveryErrors = append(recoveryErrors, recoveryErr)
				}
			}
			recoveryRequired := len(recoveryErrors) > 0
			failure := err
			if recoveryRequired {
				failure = errors.Join(append([]error{err}, recoveryErrors...)...)
			}
			if persistErr := s.inventory.failRenewal(cert.ID, attempt.ID, candidate.ID, failure.Error(), recoveryRequired); persistErr != nil {
				failure = errors.Join(failure, persistErr)
			}
			return failure
		}
		succeeded = append(succeeded, sanitizedTarget(target))
	}
	return commit()
}

func (i *Inventory) beginRecovery(certID string) (*CertificateVersion, *RenewalAttempt, []*deployers.DeployTarget, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	cert, ok := i.certs[certID]
	if !ok {
		return nil, nil, nil, fmt.Errorf("certificate %q not found", certID)
	}
	if cert.Renewal == nil || cert.Renewal.Status != "recovery_required" {
		return nil, nil, nil, fmt.Errorf("certificate %q has no renewal requiring recovery", certID)
	}
	var active *CertificateVersion
	for _, version := range cert.Versions {
		if version != nil && version.ID == cert.ActiveVersionID {
			active = cloneVersion(version)
			break
		}
	}
	if active == nil {
		return nil, nil, nil, fmt.Errorf("active certificate version %q is missing", cert.ActiveVersionID)
	}
	targets, err := activeDeploymentTargets(cert)
	if err != nil {
		return nil, nil, nil, err
	}

	before := cloneCertificateCheckpoint(cert)
	cert.Renewal.Status = "recovering"
	cert.Renewal.Error = ""
	cert.Renewal.CompletedAt = time.Time{}
	if err := i.persistLocked(); err != nil {
		i.certs[certID] = before
		return nil, nil, nil, fmt.Errorf("persisting recovery claim: %w", err)
	}
	attempt := *cert.Renewal
	return active, &attempt, targets, nil
}

func (i *Inventory) finishRecovery(certID, attemptID, candidateID, message string, recoveryRequired bool) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	cert, ok := i.certs[certID]
	if !ok || cert.Renewal == nil || cert.Renewal.ID != attemptID || cert.Renewal.Status != "recovering" {
		return fmt.Errorf("renewal recovery %q is no longer exclusively claimed", attemptID)
	}
	state := "failed"
	if recoveryRequired {
		state = "recovery_required"
	}
	cert.Renewal.Status = state
	cert.Renewal.Error = message
	cert.Renewal.CompletedAt = time.Now().UTC()
	cert.LastError = message
	cert.Status, cert.DaysLeft = statusFor(cert.NotAfter)
	for _, version := range cert.Versions {
		if version != nil && version.ID == candidateID {
			version.Status = state
		}
	}
	if err := i.persistLocked(); err != nil {
		cert.Renewal.Status = "recovery_required"
		cert.Renewal.Error = errors.Join(errors.New(message), err).Error()
		cert.LastError = cert.Renewal.Error
		return fmt.Errorf("persisting renewal recovery result: %w", err)
	}
	return nil
}

// recoverRenewal deterministically returns every currently active endpoint
// binding to the last committed version. It is deliberately rollback-only: an
// operator recovery action never guesses whether an indeterminate candidate was
// fully deployed or silently promotes it after a restart.
func (s *Server) recoverRenewal(ctx context.Context, certID, actor string) error {
	unlockCertificate := s.lockCertificateOperation(certID)
	defer unlockCertificate()

	active, attempt, targets, err := s.inventory.beginRecovery(certID)
	if err != nil {
		return err
	}
	if len(targets) > 0 && s.deployer == nil {
		failure := fmt.Errorf("deployment executor is not configured")
		if finishErr := s.inventory.finishRecovery(certID, attempt.ID, attempt.CandidateVersionID, failure.Error(), true); finishErr != nil {
			return errors.Join(failure, finishErr)
		}
		return failure
	}

	var recoveryErrors []error
	for _, target := range targets {
		if _, deployErr := s.deployRenewalVersion(ctx, certID, active, attempt.ID, target, actor); deployErr != nil {
			recoveryErrors = append(recoveryErrors, deployErr)
		}
	}
	if len(recoveryErrors) > 0 {
		failure := errors.Join(recoveryErrors...)
		if persistErr := s.inventory.finishRecovery(certID, attempt.ID, attempt.CandidateVersionID,
			failure.Error(), true); persistErr != nil {
			failure = errors.Join(failure, persistErr)
		}
		return failure
	}

	message := "operator recovery redeployed and verified the last committed certificate version"
	if err := s.inventory.finishRecovery(certID, attempt.ID, attempt.CandidateVersionID, message, false); err != nil {
		return err
	}
	return nil
}

func (s *Server) issueAndRolloutRenewal(ctx context.Context, cert *Certificate) error {
	unlockCertificate := s.lockCertificateOperation(cert.ID)
	defer unlockCertificate()

	active, attempt, err := s.inventory.beginRenewal(cert.ID)
	if err != nil {
		return err
	}
	domains := cert.SANs
	if len(domains) == 0 {
		domains = []string{cert.Domain}
	}
	issued, err := s.acme.Issue(ctx, cert.ID, domains)
	if err != nil {
		if failErr := s.inventory.failRenewal(cert.ID, attempt.ID, "", err.Error(), false); failErr != nil {
			return errors.Join(err, failErr)
		}
		return err
	}
	fingerprint, err := deployers.LeafSHA256([]byte(issued.ChainPEM))
	if err != nil {
		failure := fmt.Errorf("fingerprinting renewal certificate: %w", err)
		if cleanupErr := cleanupUntrackedIssuedKey(issued.KeyPath); cleanupErr != nil {
			failure = errors.Join(failure, cleanupErr)
		}
		if failErr := s.inventory.failRenewal(cert.ID, attempt.ID, "", failure.Error(), false); failErr != nil {
			return errors.Join(failure, failErr)
		}
		return failure
	}
	candidate := &CertificateVersion{
		ID: uuid.NewString(), Status: "candidate", Serial: issued.Serial,
		Fingerprint: fingerprint, Issuer: issued.Issuer,
		NotBefore: issued.NotBefore, NotAfter: issued.NotAfter,
		ChainPEM: issued.ChainPEM, SecretRef: issued.KeyPath, IssuedAt: time.Now().UTC(),
	}
	if err := s.inventory.setRenewalCandidate(cert.ID, attempt.ID, candidate); err != nil {
		failure := err
		if cleanupErr := cleanupUntrackedIssuedKey(issued.KeyPath); cleanupErr != nil {
			failure = errors.Join(failure, cleanupErr)
		}
		if failErr := s.inventory.failRenewal(cert.ID, attempt.ID, candidate.ID, failure.Error(), false); failErr != nil {
			return errors.Join(failure, failErr)
		}
		return failure
	}
	if err := s.rolloutRenewal(ctx, cert, active, candidate, attempt); err != nil {
		s.logger.Error("renewal rollout failed", zap.String("cert_id", cert.ID), zap.String("renewal_id", attempt.ID), zap.Error(err))
		return err
	}
	s.logger.Info("renewal issued, redeployed, verified, and committed",
		zap.String("cert_id", cert.ID), zap.String("renewal_id", attempt.ID),
		zap.Int("version", candidate.Number), zap.String("fingerprint", candidate.Fingerprint))
	return nil
}
