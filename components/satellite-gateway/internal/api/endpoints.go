package api

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"github.com/m2p/clm/internal/satellite/state"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Deployment Endpoints registry.
//
// Credentials and endpoints are different things:
//   - Credential = how you authenticate (AWS access key + secret, SSH private
//     key, GCP service account JSON, Azure client secret, WinRM password).
//   - Endpoint = where you deploy a certificate (an ACM ARN, an ALB listener
//     ARN, a CloudFront distribution ID, an API Gateway domain, a Linux host
//     IP + path, an Azure Key Vault name).
//
// One credential set can serve many endpoints. The UI was treating them as one,
// which forced operators to enter the same keys over and over for different
// ARNs in the same AWS account. This registry lets them define endpoints once,
// link each to a stored credential set by name, and then deploy with one click.
// ---------------------------------------------------------------------------

// Endpoint is a saved deployment target.
type Endpoint struct {
	ID            string `json:"id"`
	Name          string `json:"name"`           // human-readable, unique
	Service       string `json:"service"`        // aws-acm, gcp-lb, linux, windows, etc.
	CredentialRef string `json:"credential_ref"` // name of the stored credential set
	Executor      string `json:"executor"`       // gateway or a vSatellite agent id
	// Endpoint-specific fields: what identifies the target. For AWS this is the
	// region + ARN(s); for Linux it's the host + paths; for Azure the vault name
	// + cert name.
	Properties map[string]string `json:"properties"`
	// Labels for organisation / filtering.
	Environment string    `json:"environment,omitempty"` // prod, staging, dev
	Description string    `json:"description,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// EndpointStore persists deployment endpoints.
type EndpointStore struct {
	mu        sync.RWMutex
	endpoints map[string]*Endpoint // keyed by ID
	store     *state.Store
	logger    *zap.Logger
}

const endpointCollection = "endpoints"

// NewEndpointStore creates the store and loads persisted endpoints.
func NewEndpointStore(st *state.Store, logger *zap.Logger) *EndpointStore {
	s := &EndpointStore{endpoints: map[string]*Endpoint{}, store: st, logger: logger}
	if st != nil && st.Enabled() {
		var snap struct {
			Endpoints []*Endpoint `json:"endpoints"`
		}
		if err := st.Load(endpointCollection, &snap); err != nil {
			logger.Error("loading endpoints failed", zap.Error(err))
		} else {
			for _, ep := range snap.Endpoints {
				if ep != nil && ep.ID != "" {
					s.endpoints[ep.ID] = ep
				}
			}
			if len(s.endpoints) > 0 {
				logger.Info("deployment endpoints restored", zap.Int("count", len(s.endpoints)))
			}
		}
	}
	return s
}

// Enabled reports whether endpoint mutations can be durably persisted.
func (s *EndpointStore) Enabled() bool {
	return s != nil && s.store != nil && s.store.Enabled()
}

func cloneEndpoint(endpoint *Endpoint) *Endpoint {
	if endpoint == nil {
		return nil
	}
	out := *endpoint
	out.Properties = make(map[string]string, len(endpoint.Properties))
	for key, value := range endpoint.Properties {
		out.Properties[key] = value
	}
	return &out
}

func (s *EndpointStore) snapshotLocked() map[string]any {
	ids := make([]string, 0, len(s.endpoints))
	for id := range s.endpoints {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Endpoint, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneEndpoint(s.endpoints[id]))
	}
	return map[string]any{"endpoints": out}
}

func (s *EndpointStore) persistNowLocked() error {
	if !s.Enabled() {
		return fmt.Errorf("endpoint persistence is disabled")
	}
	return s.store.SaveNow(endpointCollection, s.snapshotLocked())
}

// ErrEndpointConcurrentUpdate means the caller edited a stale endpoint and must
// reload before retrying rather than overwriting a newer change.
var ErrEndpointConcurrentUpdate = errors.New("endpoint changed since it was loaded")

// Create adds a new endpoint and returns only after durable persistence.
func (s *EndpointStore) Create(endpoint *Endpoint, caller string) error {
	return s.put(endpoint, caller, nil)
}

// ReplaceIfUnmodified replaces an endpoint only when updatedAt still matches
// the stored representation.
func (s *EndpointStore) ReplaceIfUnmodified(endpoint *Endpoint, updatedAt time.Time) error {
	return s.put(endpoint, "", &updatedAt)
}

func (s *EndpointStore) put(endpoint *Endpoint, caller string, expectedUpdatedAt *time.Time) error {
	if endpoint == nil {
		return fmt.Errorf("endpoint is required")
	}
	stored := cloneEndpoint(endpoint)

	s.mu.Lock()
	defer s.mu.Unlock()

	var previous *Endpoint
	if expectedUpdatedAt == nil {
		if stored.ID != "" {
			return fmt.Errorf("new endpoints must not include an id")
		}
		stored.ID = uuid.NewString()
		stored.CreatedAt = time.Now().UTC()
		stored.CreatedBy = caller
	} else {
		if stored.ID == "" {
			return fmt.Errorf("endpoint id is required")
		}
		existing, ok := s.endpoints[stored.ID]
		if !ok {
			return fmt.Errorf("endpoint %q not found", stored.ID)
		}
		if !existing.UpdatedAt.Equal(*expectedUpdatedAt) {
			return ErrEndpointConcurrentUpdate
		}
		previous = existing
		stored.CreatedAt = existing.CreatedAt
		stored.CreatedBy = existing.CreatedBy
	}
	for id, candidate := range s.endpoints {
		if id != stored.ID && strings.EqualFold(candidate.Name, stored.Name) {
			return fmt.Errorf("an endpoint named %q already exists", stored.Name)
		}
	}
	stored.UpdatedAt = time.Now().UTC()
	s.endpoints[stored.ID] = stored
	if err := s.persistNowLocked(); err != nil {
		if previous != nil {
			s.endpoints[stored.ID] = previous
		} else {
			delete(s.endpoints, stored.ID)
		}
		return fmt.Errorf("persisting endpoint %q: %w", stored.Name, err)
	}
	*endpoint = *cloneEndpoint(stored)
	return nil
}

// Delete removes an endpoint by ID and returns only after durable persistence.
func (s *EndpointStore) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, ok := s.endpoints[id]
	if !ok {
		return false, nil
	}
	delete(s.endpoints, id)
	if err := s.persistNowLocked(); err != nil {
		s.endpoints[id] = previous
		return false, fmt.Errorf("persisting endpoint removal %q: %w", id, err)
	}
	return true, nil
}

// List returns independent endpoint copies, optionally filtered.
func (s *EndpointStore) List(service, env string) []*Endpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Endpoint, 0, len(s.endpoints))
	for _, endpoint := range s.endpoints {
		if service != "" && !strings.EqualFold(endpoint.Service, service) {
			continue
		}
		if env != "" && !strings.EqualFold(endpoint.Environment, env) {
			continue
		}
		out = append(out, cloneEndpoint(endpoint))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns an independent endpoint copy by ID.
func (s *EndpointStore) Get(id string) (*Endpoint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	endpoint, ok := s.endpoints[id]
	return cloneEndpoint(endpoint), ok
}

// ByName finds an endpoint by its human-readable name.
func (s *EndpointStore) ByName(name string) (*Endpoint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, endpoint := range s.endpoints {
		if strings.EqualFold(endpoint.Name, name) {
			return cloneEndpoint(endpoint), true
		}
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

// handleListEndpoints returns all registered deployment endpoints.
// GET /api/v1/deploy/endpoints
func (s *Server) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	if s.endpointStore == nil || !s.endpointStore.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"endpoints": []any{}})
		return
	}
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoints": s.endpointStore.List(service, env),
	})
}

// handleSaveEndpoint creates with POST or conflict-safely updates with PUT.
// POST /api/v1/deploy/endpoints
// PUT  /api/v1/deploy/endpoints/{id}
func (s *Server) handleSaveEndpoint(w http.ResponseWriter, r *http.Request) {
	if s.endpointStore == nil || !s.endpointStore.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "endpoint storage is unavailable: set state.dir")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "managing endpoints requires an admin or cert_manager role")
		return
	}

	var req struct {
		Name          string            `json:"name"`
		Service       string            `json:"service"`
		CredentialRef string            `json:"credential_ref"`
		Executor      string            `json:"executor"`
		Properties    map[string]string `json:"properties"`
		Environment   string            `json:"environment"`
		Description   string            `json:"description"`
		UpdatedAt     time.Time         `json:"updated_at"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	updating := r.Method == http.MethodPut
	endpointID := ""
	if updating {
		endpointID = strings.TrimSpace(r.PathValue("id"))
		if endpointID == "" {
			writeErr(w, http.StatusBadRequest, "endpoint id is required")
			return
		}
		if req.UpdatedAt.IsZero() {
			writeErr(w, http.StatusBadRequest, "updated_at is required when updating an endpoint")
			return
		}
	} else if !req.UpdatedAt.IsZero() {
		writeErr(w, http.StatusBadRequest, "updated_at is only valid when updating an endpoint")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	service := strings.ToLower(strings.TrimSpace(req.Service))
	if service == "" {
		writeErr(w, http.StatusBadRequest, "service is required (e.g. aws-acm, linux, gcp-lb)")
		return
	}
	spec, known := deployers.TargetSchemaFor(service)
	if !known {
		writeErr(w, http.StatusBadRequest,
			"unsupported deployment service "+service+"; see GET /api/v1/deploy/targets")
		return
	}
	if req.Properties == nil {
		req.Properties = map[string]string{}
	}
	allowedProperties := map[string]bool{}
	missing := []string{}
	for _, field := range spec.Fields {
		if field.Credential {
			continue
		}
		allowedProperties[field.Key] = true
		if field.Required && strings.TrimSpace(req.Properties[field.Key]) == "" {
			missing = append(missing, field.Key)
		}
	}
	for key := range req.Properties {
		if !allowedProperties[key] {
			writeErr(w, http.StatusBadRequest,
				"unsupported endpoint property "+key+" for service "+service)
			return
		}
	}
	if len(missing) > 0 {
		writeErr(w, http.StatusBadRequest,
			service+" endpoint requires: "+strings.Join(missing, ", "))
		return
	}
	executor := strings.TrimSpace(req.Executor)
	if executor == "" {
		executor = "gateway"
	}
	if !strings.EqualFold(executor, "gateway") {
		writeErr(w, http.StatusBadRequest,
			"only the gateway executor is available until durable vSatellite deployment is enabled")
		return
	}

	// Hold the relationship lock from reference checks through durable endpoint
	// persistence so concurrent credential deletion cannot create a dangling ref.
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()

	if updating {
		if _, exists := s.endpointStore.Get(endpointID); !exists {
			writeErr(w, http.StatusNotFound, "endpoint not found")
			return
		}
	}
	if named, exists := s.endpointStore.ByName(req.Name); exists && named.ID != endpointID {
		writeErr(w, http.StatusConflict, "an endpoint named '"+req.Name+"' already exists")
		return
	}

	// Validate that the reference is a deployment credential for this service.
	if ref := strings.TrimSpace(req.CredentialRef); ref != "" {
		if s.creds == nil || !s.creds.Enabled() {
			writeErr(w, http.StatusServiceUnavailable,
				"credential_ref was given but durable credential storage is unavailable")
			return
		}
		_, set, err := s.creds.Reveal(ref)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "credential_ref '"+ref+"' not found: "+err.Error())
			return
		}
		if set.Kind != deployCredentialKind {
			writeErr(w, http.StatusBadRequest,
				"credential_ref '"+ref+"' is not a deployment credential set")
			return
		}
		if set.Service != "" && !strings.EqualFold(set.Service, service) {
			writeErr(w, http.StatusBadRequest,
				"credential_ref '"+ref+"' is for service '"+set.Service+"', not '"+service+"'")
			return
		}
	}

	ep := &Endpoint{
		ID:            endpointID,
		Name:          strings.TrimSpace(req.Name),
		Service:       service,
		CredentialRef: strings.TrimSpace(req.CredentialRef),
		Executor:      "gateway",
		Properties:    req.Properties,
		Environment:   strings.TrimSpace(req.Environment),
		Description:   strings.TrimSpace(req.Description),
	}
	var storeErr error
	status := http.StatusCreated
	operation := "created"
	if updating {
		storeErr = s.endpointStore.ReplaceIfUnmodified(ep, req.UpdatedAt)
		status = http.StatusOK
		operation = "updated"
	} else {
		storeErr = s.endpointStore.Create(ep, callerName(r))
	}
	if errors.Is(storeErr, ErrEndpointConcurrentUpdate) {
		writeErr(w, http.StatusConflict,
			"endpoint changed after this form was loaded; reload and apply the update again")
		return
	}
	if storeErr != nil {
		writeErr(w, http.StatusInternalServerError, storeErr.Error())
		return
	}
	writeJSON(w, status, map[string]any{
		"saved": true, "operation": operation, "endpoint": ep,
	})
}

// handleDeleteEndpoint removes a deployment endpoint.
// DELETE /api/v1/deploy/endpoints/{id}
func (s *Server) handleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	if s.endpointStore == nil || !s.endpointStore.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "endpoint storage is unavailable")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "removing endpoints requires an admin or cert_manager role")
		return
	}
	id := r.PathValue("id")
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()
	removed, err := s.endpointStore.Delete(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !removed {
		writeErr(w, http.StatusNotFound, "endpoint not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": id})
}
