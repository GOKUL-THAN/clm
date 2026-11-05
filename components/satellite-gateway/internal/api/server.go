package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/ca"
	"github.com/m2p/clm/internal/satellite/credstore"
	"github.com/m2p/clm/internal/satellite/deployment"
	"github.com/m2p/clm/internal/satellite/deployment/migration"
	deploymentSync "github.com/m2p/clm/internal/satellite/deployment/sync"
	"github.com/m2p/clm/internal/satellite/discovery/scanners"
	"github.com/m2p/clm/internal/satellite/policy"
	"github.com/m2p/clm/internal/satellite/securefs"
	"github.com/m2p/clm/internal/satellite/vsatellite"
	"go.uber.org/zap"
)

// Server is the Gateway Portal's REST API.
type Server struct {
	satelliteID   string
	satelliteName string
	saasEndpoint  string
	version       string
	startedAt     time.Time

	auth       *Authenticator
	inventory  *Inventory
	scanner    *scanners.NetworkScanner
	dnsScanner *scanners.DNSScanner
	deployer   *deployment.Executor
	agents     *vsatellite.Coordinator
	// acme, alerter and scheduler are nil when the corresponding capability is
	// not configured; every handler that uses one checks first and says so.
	acme      *ACMEManager
	alerter   *Alerter
	scheduler *Scheduler
	// caRegistry holds every configured CA account, ACME included, so issuance
	// routes by account name instead of being wired to one directory.
	caRegistry *ca.Registry
	// creds holds CA account and deployment-target credentials. Nil when state
	// persistence is off, in which case runtime registration is refused rather
	// than silently forgotten on restart.
	creds *credstore.Store
	// endpointStore holds saved deployment endpoints (ARNs, hosts, vault names)
	// that are separate from credentials. One credential set can serve many
	// endpoints.
	endpointStore *EndpointStore
	// credentialMu serializes credential-reference creation/deletion across the
	// separate credential and endpoint stores, preventing check/use races.
	credentialMu sync.Mutex
	// certificateOperationMu protects the keyed operation locks shared by manual
	// deployment, renewal, and recovery. Provider calls run under only the keyed
	// lock, never Inventory.mu, so unrelated certificates remain concurrent.
	certificateOperationMu    sync.Mutex
	certificateOperationLocks map[string]*sync.Mutex
	// audit is the append-only record of who did what. Nil-safe: every method
	// on a nil *audit.Logger is a no-op, so handlers call it unconditionally
	// rather than checking first.
	audit *audit.Logger
	// policies enforces issuance-time rules (key size, validity, allowed CAs per
	// environment, wildcard, approval requirements). Nil when disabled.
	policies *policy.Store
	// keyDir is where private keys for CA-issued certificates are written.
	keyDir               string
	filePolicy           *securefs.Policy
	syncController       *deploymentSync.MultiCloudSyncController
	complianceController *audit.ComplianceController
	migrationController  *migration.BulkMigrationController
	rotationController   *deploymentSync.RotationController
	logger               *zap.Logger
}

// Options configures the API server.
type Options struct {
	SatelliteID          string
	SatelliteName        string
	SaaSEndpoint         string
	Version              string
	Auth                 *Authenticator
	Scanner              *scanners.NetworkScanner
	DNSScanner           *scanners.DNSScanner
	Deployer             *deployment.Executor
	Agents               *vsatellite.Coordinator
	ACME                 *ACMEManager
	Alerter              *Alerter
	CARegistry           *ca.Registry
	Credentials          *credstore.Store
	EndpointStore        *EndpointStore
	Audit                *audit.Logger
	Policies             *policy.Store
	SyncController       *deploymentSync.MultiCloudSyncController
	ComplianceController *audit.ComplianceController
	KeyDir               string
	FilePolicy           *securefs.Policy
	Logger               *zap.Logger
}

// NewServer builds the portal API.
func NewServer(o Options) *Server {
	return &Server{
		satelliteID:               o.SatelliteID,
		satelliteName:             o.SatelliteName,
		saasEndpoint:              o.SaaSEndpoint,
		version:                   o.Version,
		startedAt:                 time.Now(),
		auth:                      o.Auth,
		inventory:                 NewInventory(),
		scanner:                   o.Scanner,
		dnsScanner:                o.DNSScanner,
		acme:                      o.ACME,
		alerter:                   o.Alerter,
		caRegistry:                o.CARegistry,
		creds:                     o.Credentials,
		endpointStore:             o.EndpointStore,
		certificateOperationLocks: make(map[string]*sync.Mutex),
		audit:                     o.Audit,
		policies:                  o.Policies,
		syncController:            o.SyncController,
		complianceController:      o.ComplianceController,
		keyDir:                    o.KeyDir,
		filePolicy:                o.FilePolicy,
		deployer:                  o.Deployer,
		agents:                    o.Agents,
		logger:                    o.Logger,
	}
}

func (s *Server) lockCertificateOperation(certID string) func() {
	s.certificateOperationMu.Lock()
	lock := s.certificateOperationLocks[certID]
	if lock == nil {
		lock = &sync.Mutex{}
		s.certificateOperationLocks[certID] = lock
	}
	s.certificateOperationMu.Unlock()

	lock.Lock()
	return lock.Unlock
}

// SetScheduler attaches the background job runner.
//
// The scheduler needs the server to do its work, and the server exposes the
// scheduler's status, so the link is made after both exist rather than in the
// constructor.
func (s *Server) SetScheduler(sc *Scheduler) { s.scheduler = sc }

// Inventory exposes the store so main can flush it on shutdown.
func (s *Server) Inventory() *Inventory { return s.inventory }

// Audit exposes the logger so main can close it cleanly on shutdown.
func (s *Server) Audit() *audit.Logger { return s.audit }

// Handler builds the mux. /health is unauthenticated so nginx and uptime checks
// can use it; everything under /api/v1 requires a valid bearer token.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	protected := map[string]http.HandlerFunc{
		"GET /api/v1/status":        s.handleStatus,
		"GET /api/v1/agents":        s.handleAgents,
		"GET /api/v1/certificates":  s.handleListCertificates,
		"POST /api/v1/certificates": s.handleRequestCertificate,

		// Centralized inventory: facets, filtering, per-record detail and removal.
		"GET /api/v1/inventory":                          s.handleInventory,
		"POST /api/v1/inventory/prune":                   s.handlePruneInventory,
		"GET /api/v1/inventory/deployments":              s.handleListDeployments,
		"POST /api/v1/inventory/deployments/{id}/status": s.handleUpdateDeploymentStatus,
		"GET /api/v1/certificates/{id}":                  s.handleGetCertificate,
		"DELETE /api/v1/certificates/{id}":               s.handleDeleteCertificate,
		"GET /api/v1/approvals/pending":                  s.handlePendingApprovals,
		"POST /api/v1/approvals/{id}/approve":            s.handleApprove,
		"POST /api/v1/approvals/{id}/reject":             s.handleReject,
		"POST /api/v1/discover":                          s.handleDiscover,
		"POST /api/v1/discover/dns":                      s.handleDiscoverDNS,
		"POST /api/v1/discover/acm":                      s.handleDiscoverACM,
		"POST /api/v1/deploy":                            s.handleDeployCertificate,
		"GET /api/v1/deploy/targets":                     s.handleDeployTargets,
		"GET /api/v1/deploy/endpoints":                   s.handleListEndpoints,
		"POST /api/v1/deploy/endpoints":                  s.handleSaveEndpoint,
		"PUT /api/v1/deploy/endpoints/{id}":              s.handleSaveEndpoint,
		"DELETE /api/v1/deploy/endpoints/{id}":           s.handleDeleteEndpoint,

		// CA vendor registration and credential storage.
		"GET /api/v1/ca/vendors":                  s.handleCAVendors,
		"POST /api/v1/ca/accounts":                s.handleCreateCAAccount,
		"PUT /api/v1/ca/accounts/{name}":          s.handleCreateCAAccount,
		"DELETE /api/v1/ca/accounts/{name}":       s.handleDeleteCAAccount,
		"POST /api/v1/ca/accounts/{name}/test":    s.handleTestCAAccount,
		"POST /api/v1/ca/accounts/{name}/default": s.handleSetDefaultCAAccount,
		"GET /api/v1/credentials":                 s.handleListCredentials,
		"POST /api/v1/credentials":                s.handlePutCredentials,
		"GET /api/v1/credentials/{name}":          s.handleGetCredentials,
		"PUT /api/v1/credentials/{name}":          s.handlePutCredentials,
		"DELETE /api/v1/credentials/{name}":       s.handleDeleteCredentials,
		"POST /api/v1/purchases/quote":            s.handleQuote,

		// Certificate lifecycle assessment.
		"GET /api/v1/lifecycle": s.handleLifecycle,

		// Automated issuance, renewal and revocation.
		"GET /api/v1/acme/status":                      s.handleACMEStatus,
		"POST /api/v1/certificates/issue":              s.handleCAIssue,
		"POST /api/v1/certificates/{id}/renew":         s.handleRenewCertificate,
		"POST /api/v1/certificates/{id}/renew/recover": s.handleRecoverRenewal,
		"POST /api/v1/certificates/{id}/revoke":        s.handleRevokeCertificate,

		// Multi-vendor CA accounts, pricing and asynchronous orders.
		"GET /api/v1/ca/providers":            s.handleListCAProviders,
		"POST /api/v1/ca/quote":               s.handleCAQuote,
		"GET /api/v1/ca/orders":               s.handleListCAOrders,
		"POST /api/v1/ca/orders/{id}/refresh": s.handleRefreshCAOrder,

		// Proactive alerting.
		"GET /api/v1/alerts":       s.handleListAlerts,
		"POST /api/v1/alerts/test": s.handleTestAlert,

		// Background processes.
		"GET /api/v1/scheduler":            s.handleSchedulerStatus,
		"POST /api/v1/scheduler/run/{job}": s.handleRunJob,

		// Reporting and compliance.
		"GET /api/v1/reports/compliance":               s.handleComplianceReport,
		"GET /api/v1/reports/corporate-compliance":     s.handleCorporateComplianceReport,
		"GET /api/v1/reports/corporate-compliance/csv": s.handleCorporateComplianceCSV,
		"GET /api/v1/reports/audit-bundle":             s.handleAuditBundle,
		"GET /api/v1/reports/export":                   s.handleExport,

		// Audit trail: who did what, when.
		"GET /api/v1/audit":        s.handleAuditQuery,
		"GET /api/v1/audit/export": s.handleAuditExport,

		// Certificate policy engine: enforceable issuance rules.
		"GET /api/v1/policies":           s.handleListPolicies,
		"POST /api/v1/policies":          s.handlePutPolicy,
		"DELETE /api/v1/policies/{id}":   s.handleDeletePolicy,
		"POST /api/v1/policies/evaluate": s.handleEvaluatePolicy,

		// vSatellite deployment engine.
		"GET /api/v1/vsatellites":                        s.handleListVSatellites,
		"POST /api/v1/vsatellites/enroll":                s.handleEnrolVSatellite,
		"POST /api/v1/vsatellites/{id}/command":          s.handleDispatchVSatelliteCommand,
		"DELETE /api/v1/vsatellites/{id}":                s.handleDecommissionVSatellite,
		"DELETE /api/v1/vsatellites/enrollments/{token}": s.handleRevokeEnrolment,

		// Corporate multi-cloud synchronization engine (AWS ACM & GCP Cert Manager)
		"GET /api/v1/sync/multicloud/status":  s.handleMultiCloudSyncStatus,
		"POST /api/v1/sync/multicloud/run":    s.handleMultiCloudSyncRun,
		"GET /api/v1/sync/multicloud/history": s.handleMultiCloudSyncHistory,

		// Bulk certificate migration & secret ingestion (corporate onboarding)
		"POST /api/v1/migration/validate": s.handleMigrationValidate,
		"POST /api/v1/migration/import":   s.handleMigrationImport,
		"GET /api/v1/migration/history":   s.handleMigrationHistory,

		// Automated zero-downtime certificate rotation & rollback
		"GET /api/v1/rotation/status":        s.handleRotationStatus,
		"POST /api/v1/rotation/execute/{id}": s.handleRotationExecute,
	}
	for pattern, h := range protected {
		mux.Handle(pattern, s.requireAuth(h))
	}

	// The installer is fetched by the target host with curl, which has no portal
	// session, so it authenticates on the single-use enrolment token in the query
	// string instead of a JWT. The handler verifies the token before emitting
	// anything.
	mux.HandleFunc("GET /api/v1/vsatellites/install.sh", s.handleVSatelliteInstallScript)

	// The ACME HTTP-01 responder must be unauthenticated: the caller is the CA's
	// validation server, which holds no credential for this gateway. It only
	// returns a key authorization for a token currently being proved, and answers
	// 404 otherwise, so there is nothing to disclose.
	mux.HandleFunc("GET /.well-known/acme-challenge/{token}", s.handleACMEChallenge)

	return s.withCommon(mux)
}

// withCommon adds request logging and permissive CORS. The portal is served
// same-origin by nginx, so CORS only matters for local development.
func (s *Server) withCommon(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Debug("gateway api request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Duration("took", time.Since(start)))
	})
}

// requireAuth rejects requests without a valid bearer token.
func (s *Server) requireAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r)
		if raw == "" {
			writeErr(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := s.auth.Parse(raw)
		if err != nil {
			s.logger.Warn("rejected gateway api token", zap.Error(err))
			writeErr(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims)))
	})
}

// callerClaims returns the authenticated caller, or an empty Claims value.
func callerClaims(r *http.Request) *Claims {
	if c, ok := r.Context().Value(claimsKey{}).(*Claims); ok && c != nil {
		return c
	}
	return &Claims{}
}

// callerName identifies the caller for audit fields.
func callerName(r *http.Request) string {
	c := callerClaims(r)
	if c.Email != "" {
		return c.Email
	}
	if c.PreferredUser != "" {
		return c.PreferredUser
	}
	return "unknown"
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeBody(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}
