package api

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/satellite/ca"
	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"github.com/m2p/clm/internal/satellite/state"
	"go.uber.org/zap"
)

// CertificateVersion is immutable public metadata plus a local key reference for
// one issuance in a certificate lineage. Private key bytes are never persisted
// in inventory; SecretRef remains inside the gateway execution boundary.
type CertificateVersion struct {
	ID          string    `json:"id"`
	Number      int       `json:"number"`
	Status      string    `json:"status"` // active, candidate, retained, failed, recovery_required
	Serial      string    `json:"serial_number,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Issuer      string    `json:"issuer,omitempty"`
	NotBefore   time.Time `json:"not_before,omitempty"`
	NotAfter    time.Time `json:"not_after,omitempty"`
	ChainPEM    string    `json:"chain_pem,omitempty"`
	SecretRef   string    `json:"secret_ref,omitempty"`
	IssuedAt    time.Time `json:"issued_at"`
	ActivatedAt time.Time `json:"activated_at,omitempty"`
	RetiredAt   time.Time `json:"retired_at,omitempty"`
}

// RenewalAttempt is the durable lineage-level checkpoint. It makes an
// interrupted rollout visible after restart and keeps the previously active
// version authoritative until every required endpoint has been verified.
type RenewalAttempt struct {
	ID                 string    `json:"id"`
	CandidateVersionID string    `json:"candidate_version_id,omitempty"`
	Status             string    `json:"status"` // issuing, deploying, committed, failed, recovery_required
	StartedAt          time.Time `json:"started_at"`
	CompletedAt        time.Time `json:"completed_at,omitempty"`
	Error              string    `json:"error,omitempty"`
}

// Certificate is a certificate tracked by this gateway. Records come from
// network discovery or from an operator request in the portal. Inventory state
// is mirrored to atomic snapshots under state.dir when persistence is enabled.
type Certificate struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id,omitempty"`
	Domain       string    `json:"domain"`
	Type         string    `json:"type"`   // DV, OV, EV, Wildcard, discovered
	Status       string    `json:"status"` // active, expired, expiring, pending
	Issuer       string    `json:"issuer"`
	Environment  string    `json:"environment"`
	Source       string    `json:"source"` // discovery, request
	Host         string    `json:"host,omitempty"`
	Port         int       `json:"port,omitempty"`
	SerialNumber string    `json:"serial_number,omitempty"`
	Fingerprint  string    `json:"fingerprint,omitempty"`
	SANs         []string  `json:"sans,omitempty"`
	SelfSigned   bool      `json:"self_signed,omitempty"`
	NotBefore    time.Time `json:"not_before,omitempty"`
	NotAfter     time.Time `json:"not_after,omitempty"`
	DaysLeft     int       `json:"days_left"`
	CreatedAt    time.Time `json:"created_at"`
	// LastSeenAt is refreshed every time discovery observes the certificate
	// again. A record that has not been seen for a long time usually means the
	// host is gone or stopped serving it, which is the signal for pruning the
	// inventory rather than letting stale entries accumulate forever.
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`

	// KeyAlgorithm and KeySize come from discovery and are what expose a weak
	// key; they were previously collected by the scanner and then discarded.
	KeyAlgorithm string `json:"key_algorithm,omitempty"`
	KeySize      int    `json:"key_size,omitempty"`
	// DiscoveryMethod is "network" or "dns" for discovered certificates.
	DiscoveryMethod string `json:"discovery_method,omitempty"`

	// --- lifecycle ownership, set for certificates the gateway manages ---

	// ManagedBy is "acme" for certificates this gateway issues and renews, empty
	// for observed-only records. Only managed certificates can be auto-renewed:
	// the gateway holds no key for anything it merely discovered.
	ManagedBy string `json:"managed_by,omitempty"`
	// ACMEAccount and CAName identify where a managed certificate came from.
	CAName      string `json:"ca_name,omitempty"`
	RequestedBy string `json:"requested_by,omitempty"`
	// IssuedAt, RenewedAt and RenewalCount track the automated lifecycle.
	IssuedAt     time.Time `json:"issued_at,omitempty"`
	RenewedAt    time.Time `json:"renewed_at,omitempty"`
	RenewalCount int       `json:"renewal_count,omitempty"`
	// LastError records why the most recent issuance or renewal attempt failed,
	// so a silent failure is visible in the inventory rather than only in logs.
	LastError string `json:"last_error,omitempty"`
	// RevokedAt is set when the certificate has been revoked at the CA.
	//
	// Revoked is the flag clients should test. `omitempty` does not omit a zero
	// time.Time, so revoked_at is always present as "0001-01-01T00:00:00Z" for a
	// live certificate — which is truthy in JavaScript and quietly broke every
	// client that checked the timestamp instead of a boolean.
	RevokedAt time.Time `json:"revoked_at,omitempty"`
	Revoked   bool      `json:"revoked"`
	// CAAccount is the named account the certificate was ordered from, and
	// CAOrderID the vendor's own order identifier — needed to poll an
	// asynchronous commercial order and to revoke it later.
	CAAccount string `json:"ca_account,omitempty"`
	CAOrderID string `json:"ca_order_id,omitempty"`
	// Validation holds the outstanding domain control steps for a pending order.
	// Without these a pending order can never progress.
	Validation []ca.ValidationInstruction `json:"validation,omitempty"`
	// SecretRef points at where the private key is held. The key itself is never
	// placed in the inventory or in a snapshot.
	SecretRef string `json:"secret_ref,omitempty"`
	// ChainPEM is the issued certificate chain. Public material, safe to store.
	ChainPEM string `json:"chain_pem,omitempty"`

	// Versions retain immutable issuance metadata and local key references so a
	// renewal can be rolled back without asking the CA to reissue the old cert.
	// ActiveVersionID changes only after all required endpoint checks pass.
	Versions        []*CertificateVersion `json:"versions,omitempty"`
	ActiveVersionID string                `json:"active_version_id,omitempty"`
	Renewal         *RenewalAttempt       `json:"renewal,omitempty"`

	// Deployments tracks where this certificate has been installed by the deploy
	// endpoint. Each entry records the target, the time, the result and whether
	// it is still the current version on that target. Updated on every
	// POST /api/v1/deploy that references this cert_id.
	Deployments []*DeploymentRecord `json:"deployments,omitempty"`
}

// DeploymentStage is durable evidence of one step in a deployment attempt.
type DeploymentStage struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"` // running, completed, failed, skipped
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// DeploymentRecord is one immutable deployment attempt. It is created before
// apply starts and updated synchronously as the attempt is verified or rolled
// back, so a restart does not erase an in-flight or failed operation.
type DeploymentRecord struct {
	ID            string `json:"id"`
	AttemptID     string `json:"attempt_id"`
	CorrelationID string `json:"correlation_id"`
	TargetType    string `json:"target_type"`           // aws-acm, linux, postgresql, gcp-lb, azure-keyvault
	TargetHost    string `json:"target_host,omitempty"` // where: the host, ALB, key vault, etc.
	// BindingID is a stable hash of endpoint-defining target metadata. It excludes
	// resolved credentials and provider-generated version identifiers, allowing a
	// verified replacement to supersede only the exact prior binding.
	BindingID string `json:"binding_id,omitempty"`
	TargetARN string `json:"target_arn,omitempty"` // for AWS: the certificate or listener ARN
	// Target is a sanitized, replayable binding snapshot. It retains only public
	// target metadata and credential_ref; resolved credentials are always nil.
	Target              *deployers.DeployTarget          `json:"target,omitempty"`
	CertificateVersion  int                              `json:"certificate_version,omitempty"`
	RenewalAttemptID    string                           `json:"renewal_attempt_id,omitempty"`
	Region              string                           `json:"region,omitempty"`
	Executor            string                           `json:"executor"` // gateway or a vSatellite agent id
	Status              string                           `json:"status"`   // deploying, deployed, failed, superseded, queued
	Stage               string                           `json:"stage"`
	Stages              []DeploymentStage                `json:"stages,omitempty"`
	Verified            bool                             `json:"verified"`
	VerifyError         string                           `json:"verify_error,omitempty"`
	Evidence            []deployers.VerificationEvidence `json:"evidence,omitempty"`
	RollbackAttempted   bool                             `json:"rollback_attempted"`
	RollbackSuccess     bool                             `json:"rollback_success"`
	RollbackError       string                           `json:"rollback_error,omitempty"`
	RollbackEvidence    *deployers.VerificationEvidence  `json:"rollback_evidence,omitempty"`
	PreviousCert        string                           `json:"previous_cert,omitempty"`
	PreviousFingerprint string                           `json:"previous_fingerprint,omitempty"`
	Message             string                           `json:"message,omitempty"`
	Error               string                           `json:"error,omitempty"`
	StartedAt           time.Time                        `json:"started_at"`
	CompletedAt         time.Time                        `json:"completed_at,omitempty"`
	DeployedAt          time.Time                        `json:"deployed_at"`
	DeployedBy          string                           `json:"deployed_by,omitempty"`
}

// Approval is a change awaiting sign-off before the gateway acts on it.
type Approval struct {
	ID            string    `json:"id"`
	RequestType   string    `json:"request_type"`
	Level         string    `json:"level"`
	EstimatedCost float64   `json:"estimated_cost"`
	Justification string    `json:"justification"`
	Status        string    `json:"status"` // pending, approved, rejected
	RequestedBy   string    `json:"requested_by"`
	DecidedBy     string    `json:"decided_by,omitempty"`
	Comment       string    `json:"comment,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	DecidedAt     time.Time `json:"decided_at,omitempty"`

	// Domain, CertType and Environment link an approval to a specific issuance
	// request. Populated only for RequestType == "certificate_issue", which is
	// what the policy engine's RequireApprovalForCertTypes rule creates and then
	// checks for — before this, an approval was a freeform record nothing in the
	// codebase actually gated on, so approving or rejecting one had no effect on
	// whether an issuance could proceed.
	Domain      string `json:"domain,omitempty"`
	CertType    string `json:"cert_type,omitempty"`
	Environment string `json:"environment,omitempty"`
	CAAccount   string `json:"ca_account,omitempty"`
}

// Inventory is the gateway's certificate and approval store. It is held in
// memory for reads and mirrored to disk by persist.go, so a restart no longer
// loses approvals or certificate history.
type Inventory struct {
	mu     sync.RWMutex
	certs  map[string]*Certificate
	appros map[string]*Approval

	// store is nil or disabled when persistence is not configured; every write
	// path calls persist(), which then does nothing.
	store  *state.Store
	logger *zap.Logger
}

// NewInventory creates an empty inventory.
func NewInventory() *Inventory {
	return &Inventory{
		certs:  make(map[string]*Certificate),
		appros: make(map[string]*Approval),
	}
}

// statusFor derives a lifecycle status from the remaining validity.
func statusFor(notAfter time.Time) (string, int) {
	if notAfter.IsZero() {
		return "pending", 0
	}
	days := int(time.Until(notAfter).Hours() / 24)
	switch {
	case days < 0:
		return "expired", days
	case days <= 30:
		return "expiring", days
	default:
		return "active", days
	}
}

// UpsertDiscovered records a certificate found by a network scan, keyed by
// fingerprint so repeat scans update rather than duplicate.
func (i *Inventory) UpsertDiscovered(c *Certificate) {
	i.mu.Lock()

	c.LastSeenAt = time.Now()

	key := c.Fingerprint
	if key == "" {
		key = c.Domain + "|" + c.Host
	}
	replaced := false
	for _, existing := range i.certs {
		if existing.Fingerprint == key || existing.ID == key {
			// Discovery is observational. It must never replace lifecycle-owned
			// state while renewal or deployment can be in progress.
			if c.Source == "discovery" && existing.ManagedBy != "" {
				existing.LastSeenAt = c.LastSeenAt
				existing.DiscoveryMethod = c.DiscoveryMethod
				existing.KeyAlgorithm = c.KeyAlgorithm
				existing.KeySize = c.KeySize
				existing.SelfSigned = c.SelfSigned
				if c.Host != "" {
					existing.Host = c.Host
				}
				if c.Port != 0 {
					existing.Port = c.Port
				}
				replaced = true
				break
			}

			c.ID = existing.ID
			c.CreatedAt = existing.CreatedAt
			if c.Source == "discovery" && existing.Environment != "" && existing.Environment != "unknown" {
				c.Environment = existing.Environment
			}
			i.certs[existing.ID] = c
			replaced = true
			break
		}
	}
	if !replaced {
		if c.ID == "" {
			c.ID = uuid.NewString()
		}
		if c.CreatedAt.IsZero() {
			c.CreatedAt = time.Now()
		}
		i.certs[c.ID] = c
	}
	i.mu.Unlock()

	i.persist()
}

// Upsert stores a certificate the gateway itself owns, such as one issued by
// ACME, keyed by ID rather than fingerprint.
func (i *Inventory) Upsert(c *Certificate) {
	i.mu.Lock()
	if c.ManagedBy != "" && c.ChainPEM != "" && c.ActiveVersionID == "" {
		if _, err := ensureActiveVersionLocked(c); err != nil {
			c.LastError = "initializing certificate version: " + err.Error()
		}
	}
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	i.certs[c.ID] = c
	i.mu.Unlock()

	i.persist()
}

// Get returns one certificate by ID.
func (i *Inventory) Get(id string) (*Certificate, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	c, ok := i.certs[id]
	return c, ok
}

// Delete removes a certificate from the inventory.
func (i *Inventory) Delete(id string) bool {
	i.mu.Lock()
	_, ok := i.certs[id]
	if ok {
		delete(i.certs, id)
	}
	i.mu.Unlock()

	if ok {
		i.persist()
	}
	return ok
}

// AddRequest records an operator-requested certificate as pending.
func (i *Inventory) AddRequest(domain, certType, environment, requestedBy string) *Certificate {
	i.mu.Lock()
	c := &Certificate{
		ID:          uuid.NewString(),
		Domain:      domain,
		Type:        certType,
		Status:      "pending",
		Environment: environment,
		Source:      "request",
		RequestedBy: requestedBy,
		CreatedAt:   time.Now(),
	}
	i.certs[c.ID] = c
	i.mu.Unlock()

	i.persist()
	return c
}

// ListCerts returns all certificates, soonest to expire first, with statuses
// recomputed so the view is correct without a background job.
func (i *Inventory) ListCerts() []*Certificate {
	i.mu.Lock()
	defer i.mu.Unlock()

	out := make([]*Certificate, 0, len(i.certs))
	for _, c := range i.certs {
		// Revoked and failed are terminal states and must not be recomputed from
		// the expiry date. A revoked certificate is not "active" merely because
		// its not-after date is still in the future, and recomputing it that way
		// made revocation invisible in the list.
		c.Revoked = !c.RevokedAt.IsZero()

		switch {
		case c.Revoked:
			c.Status = "revoked"
			c.DaysLeft = int(time.Until(c.NotAfter).Hours() / 24)
		case c.Status == "failed":
			// Leave as-is: this record is a failed issuance attempt, not a
			// certificate that exists anywhere.
		case c.Source == "discovery" || !c.NotAfter.IsZero():
			c.Status, c.DaysLeft = statusFor(c.NotAfter)
		}
		out = append(out, c)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].NotAfter.IsZero() != out[b].NotAfter.IsZero() {
			return out[b].NotAfter.IsZero()
		}
		return out[a].NotAfter.Before(out[b].NotAfter)
	})
	return out
}

// CertStats summarises the inventory for the dashboard cards.
func (i *Inventory) CertStats() map[string]int {
	certs := i.ListCerts()
	stats := map[string]int{"total": len(certs)}
	for _, c := range certs {
		stats[c.Status]++
		if c.Status == "expiring" || c.Status == "expired" {
			stats["expiring_30d"]++
		}
	}
	return stats
}

// AddApproval records a new pending approval.
func (i *Inventory) AddApproval(a *Approval) *Approval {
	i.mu.Lock()
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	a.Status = "pending"
	a.CreatedAt = time.Now()
	i.appros[a.ID] = a
	i.mu.Unlock()

	i.persist()
	return a
}

// PendingApprovals returns approvals still awaiting a decision, oldest first.
func (i *Inventory) PendingApprovals() []*Approval {
	i.mu.RLock()
	defer i.mu.RUnlock()

	out := make([]*Approval, 0)
	for _, a := range i.appros {
		if a.Status == "pending" {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(x, y int) bool { return out[x].CreatedAt.Before(out[y].CreatedAt) })
	return out
}

// HasApprovedIssuance reports whether an approved (not merely pending) approval
// exists for this exact domain, cert type and environment. This is what lets
// the policy engine's RequireApprovalForCertTypes rule actually gate issuance:
// without a specific match, "an approval exists somewhere" would let one
// approved OV request for example.com authorise an unrelated EV request for
// example.net.
func (i *Inventory) HasApprovedIssuance(domain, certType, environment string) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	for _, a := range i.appros {
		if a.Status != "approved" || a.RequestType != "certificate_issue" {
			continue
		}
		if strings.EqualFold(a.Domain, domain) &&
			strings.EqualFold(a.CertType, certType) &&
			strings.EqualFold(a.Environment, environment) {
			return true
		}
	}
	return false
}

// Decide approves or rejects an approval. It reports false when the id is
// unknown or a decision was already recorded.
func (i *Inventory) Decide(id, status, decidedBy, comment string) (*Approval, bool) {
	i.mu.Lock()
	a, ok := i.appros[id]
	if !ok || a.Status != "pending" {
		i.mu.Unlock()
		return nil, false
	}
	a.Status = status
	a.DecidedBy = decidedBy
	a.Comment = comment
	a.DecidedAt = time.Now()
	i.mu.Unlock()

	// An approval decision is the least recoverable record the gateway holds, so
	// it is written synchronously rather than debounced.
	i.FlushState()
	return a, true
}
