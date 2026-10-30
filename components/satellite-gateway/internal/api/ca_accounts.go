package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/ca"
	"github.com/m2p/clm/internal/satellite/credstore"
	"go.uber.org/zap"
)

// Registering CA vendors at runtime.
//
// Accounts could only come from environment variables read once at startup, so
// onboarding a CA meant editing a compose file and restarting the gateway. These
// endpoints let an operator add, test and remove an account while it runs, with
// credentials kept in the credential store and never returned.

// caAccountKind groups credential sets belonging to CA accounts.
const caAccountKind = "ca"

// handleCAVendors describes what can be registered and which fields each vendor
// needs, so the portal renders the right form without knowing any vendor.
// GET /api/v1/ca/vendors
func (s *Server) handleCAVendors(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"vendors": ca.SupportedVendors(),
		"note": "credentials are stored on the gateway and never returned; a secret " +
			"can be left blank on update to keep the stored value",
	})
}

// handleCreateCAAccount creates or conflict-safely updates a CA account.
// POST /api/v1/ca/accounts creates; PUT /api/v1/ca/accounts/{name} updates.
func (s *Server) handleCreateCAAccount(w http.ResponseWriter, r *http.Request) {
	if s.caRegistry == nil {
		writeErr(w, http.StatusServiceUnavailable, "the CA registry is not available")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden,
			"registering a CA account requires the super_admin, admin, tenant_admin or cert_manager role")
		return
	}
	if s.creds == nil || !s.creds.Enabled() {
		writeErr(w, http.StatusServiceUnavailable,
			"credential storage is unavailable: set state.dir so accounts survive a restart")
		return
	}

	var req struct {
		Name        string            `json:"name"`
		Vendor      string            `json:"vendor"`
		Environment string            `json:"environment"`
		Default     bool              `json:"default"`
		Credentials map[string]string `json:"credentials"`
		RemoveKeys  []string          `json:"remove_keys"`
		UpdatedAt   time.Time         `json:"updated_at"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	pathName := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	bodyName := strings.ToLower(strings.TrimSpace(req.Name))
	name := bodyName
	if pathName != "" {
		name = pathName
		if bodyName != "" && bodyName != pathName {
			writeErr(w, http.StatusBadRequest, "body name must match the CA account name in the URL")
			return
		}
	}
	if !validCredentialName(name) {
		writeErr(w, http.StatusBadRequest,
			"name must be 1-63 lowercase letters, numbers, dots, underscores or hyphens and start with a letter or number")
		return
	}
	if req.Credentials == nil {
		req.Credentials = map[string]string{}
	}

	// Serialize namespace and version checks with every credential/endpoint
	// mutation so account updates cannot overwrite deployment Credential Sets.
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()

	existingValues, existingSet, existingErr := s.creds.Reveal(name)
	exists := existingErr == nil
	if r.Method == http.MethodPost && exists {
		writeErr(w, http.StatusConflict,
			"CA account already exists; use PUT with the current credential_updated_at value")
		return
	}
	if r.Method == http.MethodPut && !exists {
		writeErr(w, http.StatusNotFound, "API-managed CA account not found")
		return
	}
	if r.Method == http.MethodPut && req.UpdatedAt.IsZero() {
		writeErr(w, http.StatusBadRequest, "updated_at is required for conflict-safe updates")
		return
	}
	if exists && existingSet.Kind != caAccountKind {
		writeErr(w, http.StatusConflict,
			"that name belongs to a deployment credential set; choose a different CA account name")
		return
	}

	vendor := strings.ToLower(strings.TrimSpace(req.Vendor))
	if vendor == "" && exists {
		vendor = existingSet.Service
	}
	if _, ok := ca.VendorByName(vendor); !ok {
		writeErr(w, http.StatusBadRequest,
			"unsupported vendor "+vendor+"; see GET /api/v1/ca/vendors")
		return
	}
	if exists && !strings.EqualFold(existingSet.Service, vendor) {
		writeErr(w, http.StatusConflict,
			"vendor cannot be changed on an existing CA account; create a new account instead")
		return
	}
	if err := ca.ValidateCredentialKeys(vendor, req.Credentials); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	removeMap := map[string]string{}
	for _, key := range req.RemoveKeys {
		removeMap[strings.TrimSpace(key)] = "remove"
	}
	if err := ca.ValidateCredentialKeys(vendor, removeMap); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	effective := map[string]string{}
	if exists {
		for key, value := range existingValues {
			effective[key] = value
		}
	}
	for _, key := range req.RemoveKeys {
		delete(effective, strings.TrimSpace(key))
	}
	for key, value := range req.Credentials {
		if strings.TrimSpace(value) != "" {
			effective[key] = value
		}
	}
	env := strings.TrimSpace(req.Environment)
	if env == "" {
		env = strings.TrimSpace(effective["environment"])
	}
	if env == "" {
		env = "unspecified"
	}
	effective["environment"] = env

	if err := ca.ValidateCredentials(vendor, effective); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	provider, err := ca.Build(vendor, effective, s.keyDir, s.logger)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not create the account: "+err.Error())
		return
	}

	next := &credstore.Set{
		Name: name, Kind: caAccountKind, Service: vendor,
		Values: effective, CreatedBy: callerName(r),
	}
	var storeErr error
	if r.Method == http.MethodPut {
		storeErr = s.creds.ReplaceIfUnmodified(next, ca.SecretKeys(vendor), req.UpdatedAt)
	} else {
		storeErr = s.creds.Replace(next, ca.SecretKeys(vendor))
	}
	if errors.Is(storeErr, credstore.ErrConcurrentUpdate) {
		writeErr(w, http.StatusConflict,
			"CA credentials changed after this form was loaded; reload and apply the update again")
		return
	}
	if storeErr != nil {
		writeErr(w, http.StatusInternalServerError, storeErr.Error())
		return
	}

	s.caRegistry.Replace(name, env, req.Default, provider)
	operation := "created"
	status := http.StatusCreated
	action := "ca_account.register"
	if exists {
		operation = "updated"
		status = http.StatusOK
		action = "ca_account.update"
	}
	s.logger.Info("ca account stored through the API",
		zap.String("name", name), zap.String("vendor", vendor),
		zap.String("environment", env), zap.String("operation", operation),
		zap.String("by", callerName(r)))
	s.recordAudit(r, action, "ca_account", name, name, audit.ResultSuccess,
		"vendor="+vendor+" environment="+env)

	redacted, _ := s.creds.Redacted(name)
	writeJSON(w, status, map[string]any{
		"name": name, "vendor": provider.Name(), "environment": env,
		"default": s.caRegistry.Default() == name, "operation": operation,
		"credential_updated_at": redacted["updated_at"],
		"capabilities":          provider.Capabilities(),
		"note": "credentials are stored on the gateway and are never returned. " +
			"Use POST /api/v1/ca/accounts/" + name + "/test to confirm they work.",
	})
}

// handleDeleteCAAccount removes an API-managed account and its credentials.
// DELETE /api/v1/ca/accounts/{name}
func (s *Server) handleDeleteCAAccount(w http.ResponseWriter, r *http.Request) {
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "removing a CA account requires an admin or cert_manager role")
		return
	}
	if s.caRegistry == nil || s.creds == nil || !s.creds.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "durable CA account storage is unavailable")
		return
	}

	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()

	account, err := s.caRegistry.Get(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	_, set, err := s.creds.Reveal(name)
	if err != nil || set.Kind != caAccountKind {
		writeErr(w, http.StatusConflict,
			"the account is not backed by an API-managed CA credential set and cannot be removed durably")
		return
	}
	wasDefault := s.caRegistry.Default() == name
	if err := s.caRegistry.Remove(name); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if err := s.creds.Delete(name); err != nil {
		// Restore the runtime registry so the API never reports a removal that
		// will be silently reversed from leftover credentials on restart.
		s.caRegistry.Replace(name, account.Environment, wasDefault, account.Provider_())
		writeErr(w, http.StatusInternalServerError,
			"could not durably remove CA account credentials: "+err.Error())
		return
	}

	s.logger.Info("ca account removed",
		zap.String("name", name), zap.String("by", callerName(r)))
	s.recordAudit(r, "ca_account.remove", "ca_account", name, name, audit.ResultSuccess, "")
	writeJSON(w, http.StatusOK, map[string]any{
		"removed": name,
		"note": "certificates already issued by this account stay in the inventory, but " +
			"they can no longer be renewed or revoked until the account is registered again",
	})
}

// handleTestCAAccount proves the stored credentials actually work.
//
// Registering an account only checks that the fields parse. This makes a real
// call, because credentials that are merely well-formed are the usual cause of a
// first order failing hours later.
// POST /api/v1/ca/accounts/{name}/test
func (s *Server) handleTestCAAccount(w http.ResponseWriter, r *http.Request) {
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "testing a CA account requires an admin or cert_manager role")
		return
	}

	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	account, err := s.caRegistry.Get(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	provider := account.Provider_()
	caps := provider.Capabilities()

	result := map[string]any{
		"account": account.Name, "vendor": provider.Name(),
		"environment": account.Environment,
	}

	switch {
	case caps.Quoting:
		// A pricing call is the cheapest authenticated round trip that proves the
		// credentials: it spends nothing and still fails on bad auth.
		quotes, qerr := provider.Quote(ctx, ca.QuoteRequest{
			Domains: []string{"example.com"}, CertType: "dv", ValidityYears: 1,
		})
		if qerr != nil {
			result["ok"] = false
			result["error"] = qerr.Error()
			result["checked"] = "pricing call"
			writeJSON(w, http.StatusBadGateway, result)
			return
		}
		result["ok"] = true
		result["checked"] = "pricing call"
		result["products_returned"] = len(quotes)

	case provider.Name() == "acme":
		// There is no free read on ACME, so probe the order endpoint: it registers
		// the account and fails on bad configuration without ordering anything.
		if _, oerr := provider.OrderStatus(ctx, "probe"); oerr != nil &&
			!strings.Contains(oerr.Error(), "synchronously") {
			result["ok"] = false
			result["error"] = oerr.Error()
			result["checked"] = "account probe"
			writeJSON(w, http.StatusBadGateway, result)
			return
		}
		result["ok"] = true
		result["checked"] = "configuration only"
		result["note"] = "ACME exposes no free authenticated read, so this confirms the " +
			"directory and account key are usable but does not place an order"

	default:
		// GoDaddy has no free read either; asking for a certificate that cannot
		// exist authenticates without ordering.
		_, oerr := provider.OrderStatus(ctx, "clm-credential-probe")
		if oerr != nil && looksLikeAuthFailure(oerr.Error()) {
			result["ok"] = false
			result["error"] = oerr.Error()
			result["checked"] = "authenticated read"
			writeJSON(w, http.StatusBadGateway, result)
			return
		}
		result["ok"] = true
		result["checked"] = "authenticated read"
		result["note"] = "the CA answered the request, so the credentials are accepted; " +
			"the probe order id is expected not to exist"
		if oerr != nil {
			result["detail"] = oerr.Error()
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// looksLikeAuthFailure separates a rejected credential from a merely unknown
// order, which is the difference between a broken account and a working one.
func looksLikeAuthFailure(msg string) bool {
	m := strings.ToLower(msg)
	for _, needle := range []string{
		"401", "403", "unable_to_authenticate", "unauthorized", "unauthorised",
		"invalid api", "api key is invalid", "access has not been enabled",
		"authentication", "forbidden", "1011150",
	} {
		if strings.Contains(m, needle) {
			return true
		}
	}
	return false
}

// handleSetDefaultCAAccount moves the default account.
// POST /api/v1/ca/accounts/{name}/default
func (s *Server) handleSetDefaultCAAccount(w http.ResponseWriter, r *http.Request) {
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "changing the default CA requires an admin or cert_manager role")
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()
	if err := s.caRegistry.SetDefault(name); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.logger.Info("default ca account changed",
		zap.String("name", name), zap.String("by", callerName(r)))
	s.recordAudit(r, "ca_account.set_default", "ca_account", name, name, audit.ResultSuccess, "")
	writeJSON(w, http.StatusOK, map[string]any{"default": name})
}

// restoreCAAccounts re-registers accounts saved through the API.
//
// Called at startup after the environment-configured accounts, so an account
// registered in the portal survives a restart. A credential set that no longer
// builds is logged and skipped rather than stopping startup: one bad account must
// not take the gateway down.
func (s *Server) RestoreCAAccounts() {
	if s.creds == nil || s.caRegistry == nil {
		return
	}
	restored, failed := 0, 0
	for _, entry := range s.creds.List(caAccountKind) {
		name, _ := entry["name"].(string)
		vendor, _ := entry["service"].(string)
		if name == "" || vendor == "" {
			continue
		}
		values, _, err := s.creds.Reveal(name)
		if err != nil {
			continue
		}
		provider, err := ca.Build(vendor, values, s.keyDir, s.logger)
		if err != nil {
			failed++
			s.logger.Error("a stored CA account could not be restored",
				zap.String("name", name), zap.String("vendor", vendor), zap.Error(err))
			continue
		}
		env := values["environment"]
		if env == "" {
			env = "unspecified"
		}
		s.caRegistry.Replace(name, env, false, provider)
		restored++
	}
	if restored > 0 || failed > 0 {
		s.logger.Info("stored CA accounts restored",
			zap.Int("restored", restored), zap.Int("failed", failed))
	}
}

// --- credential sets ---

const deployCredentialKind = "deploy"

func validCredentialName(name string) bool {
	if len(name) == 0 || len(name) > 63 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			(i > 0 && (c == '-' || c == '_' || c == '.')) {
			continue
		}
		return false
	}
	return true
}

// deployCredentialRules derives every accepted key from the same target schema
// used by deployment validation and the portal. This prevents UI, API and
// deployer credential contracts from drifting independently.
func deployCredentialRules(service string) (allowed, secrets map[string]bool, required []string, ok bool) {
	fields, ok := deployerSchemaFor(service)
	if !ok {
		return nil, nil, nil, false
	}
	allowed = map[string]bool{}
	secrets = map[string]bool{}
	for _, field := range fields {
		if !field.Credential {
			continue
		}
		allowed[field.Key] = true
		if field.Secret {
			secrets[field.Key] = true
		}
		if field.Required {
			required = append(required, field.Key)
		}
	}
	return allowed, secrets, required, true
}

func (s *Server) credentialEndpointReferences(name string) []string {
	if s.endpointStore == nil {
		return nil
	}
	refs := []string{}
	for _, endpoint := range s.endpointStore.List("", "") {
		if strings.EqualFold(strings.TrimSpace(endpoint.CredentialRef), name) {
			refs = append(refs, endpoint.Name)
		}
	}
	return refs
}

func (s *Server) decorateDeployCredential(credential map[string]any) map[string]any {
	if credential == nil || credential["kind"] != deployCredentialKind {
		return credential
	}
	name, _ := credential["name"].(string)
	refs := s.credentialEndpointReferences(name)
	credential["in_use"] = len(refs) > 0
	credential["endpoint_references"] = refs
	return credential
}

// handleListCredentials lists deployment Credential Sets with every value masked.
// GET /api/v1/credentials?kind=deploy
func (s *Server) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	if s.creds == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"credentials": []any{},
			"reason":      "credential storage is unavailable: set state.dir",
		})
		return
	}
	if !s.creds.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{
			"credentials": []any{},
			"reason":      "durable credential storage is unavailable: set state.dir",
		})
		return
	}
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	if kind != "" && kind != deployCredentialKind {
		writeErr(w, http.StatusBadRequest,
			"this endpoint lists deployment Credential Sets only; CA account credentials are never exposed")
		return
	}
	credentials := s.creds.List(deployCredentialKind)
	for _, credential := range credentials {
		s.decorateDeployCredential(credential)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"credentials": credentials,
		"note":        "secret values are never returned; credential snapshots are gateway-local and not encrypted at rest",
	})
}

// handleGetCredentials returns one redacted deployment credential set.
// GET /api/v1/credentials/{name}
func (s *Server) handleGetCredentials(w http.ResponseWriter, r *http.Request) {
	if s.creds == nil || !s.creds.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "credential storage is unavailable")
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	_, set, err := s.creds.Reveal(name)
	if err != nil || set.Kind != deployCredentialKind {
		writeErr(w, http.StatusNotFound, "deployment credential set not found")
		return
	}
	redacted, err := s.creds.Redacted(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"credential": s.decorateDeployCredential(redacted),
		"note":       "secret values are never returned",
	})
}

// handlePutCredentials creates or updates a deployment credential set.
// POST /api/v1/credentials creates; PUT /api/v1/credentials/{name} performs a
// conflict-safe update.
func (s *Server) handlePutCredentials(w http.ResponseWriter, r *http.Request) {
	if s.creds == nil || !s.creds.Enabled() {
		writeErr(w, http.StatusServiceUnavailable,
			"credential storage is unavailable: set state.dir")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "storing credentials requires an admin or cert_manager role")
		return
	}

	var req struct {
		Name       string            `json:"name"`
		Kind       string            `json:"kind"`
		Service    string            `json:"service"`
		Values     map[string]string `json:"values"`
		RemoveKeys []string          `json:"remove_keys"`
		UpdatedAt  time.Time         `json:"updated_at"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	pathName := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	bodyName := strings.ToLower(strings.TrimSpace(req.Name))
	name := bodyName
	if pathName != "" {
		name = pathName
		if bodyName != "" && bodyName != pathName {
			writeErr(w, http.StatusBadRequest, "body name must match the credential name in the URL")
			return
		}
	}
	if name == "" {
		writeErr(w, http.StatusBadRequest, "a name is required")
		return
	}

	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()

	existingValues, existingSet, existingErr := s.creds.Reveal(name)
	exists := existingErr == nil
	if !exists && !validCredentialName(name) {
		writeErr(w, http.StatusBadRequest,
			"name must be 1-63 lowercase letters, numbers, dots, underscores or hyphens and start with a letter or number")
		return
	}
	if r.Method == http.MethodPost && exists {
		writeErr(w, http.StatusConflict,
			"credential set already exists; use PUT with the current updated_at value")
		return
	}
	if r.Method == http.MethodPut && !exists {
		writeErr(w, http.StatusNotFound, "deployment credential set not found")
		return
	}
	if r.Method == http.MethodPut && req.UpdatedAt.IsZero() {
		writeErr(w, http.StatusBadRequest, "updated_at is required for conflict-safe updates")
		return
	}
	if exists && existingSet.Kind != deployCredentialKind {
		writeErr(w, http.StatusConflict,
			"that name belongs to a CA account credential and cannot be changed through the deployment credential API")
		return
	}

	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind == "" {
		kind = deployCredentialKind
	}
	if kind != deployCredentialKind {
		writeErr(w, http.StatusBadRequest, "kind must be deploy; CA credentials are managed through the CA account API")
		return
	}

	service := strings.ToLower(strings.TrimSpace(req.Service))
	if service == "" && exists {
		service = existingSet.Service
	}
	if service == "" {
		writeErr(w, http.StatusBadRequest, "a supported deployment service is required")
		return
	}
	if exists && !strings.EqualFold(existingSet.Service, service) {
		writeErr(w, http.StatusConflict,
			"service cannot be changed on an existing credential set; create a new set instead")
		return
	}

	allowed, secrets, required, known := deployCredentialRules(service)
	if !known {
		writeErr(w, http.StatusBadRequest,
			"unsupported deployment service "+service+"; see GET /api/v1/deploy/targets")
		return
	}

	effective := map[string]string{}
	if exists {
		for key, value := range existingValues {
			effective[key] = value
		}
	}
	for _, key := range req.RemoveKeys {
		key = strings.TrimSpace(key)
		if !allowed[key] {
			if _, legacy := effective[key]; !legacy {
				writeErr(w, http.StatusBadRequest, "remove_keys contains unsupported credential field "+key)
				return
			}
		}
		delete(effective, key)
	}
	for key, value := range req.Values {
		if key != strings.TrimSpace(key) || !allowed[key] {
			writeErr(w, http.StatusBadRequest,
				"unsupported credential field "+key+" for service "+service)
			return
		}
		// Blank values never erase an existing credential accidentally. Clients
		// must use remove_keys, which also makes required-field validation explicit.
		if strings.TrimSpace(value) == "" {
			continue
		}
		effective[key] = value
	}

	if len(effective) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one credential value is required")
		return
	}
	missing := []string{}
	for _, key := range required {
		if strings.TrimSpace(effective[key]) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		writeErr(w, http.StatusBadRequest,
			service+" credential set requires: "+strings.Join(missing, ", "))
		return
	}

	next := &credstore.Set{
		Name: name, Kind: deployCredentialKind, Service: service,
		Values: effective, CreatedBy: callerName(r),
	}
	var storeErr error
	if r.Method == http.MethodPut {
		storeErr = s.creds.ReplaceIfUnmodified(next, secrets, req.UpdatedAt)
	} else {
		storeErr = s.creds.Replace(next, secrets)
	}
	if errors.Is(storeErr, credstore.ErrConcurrentUpdate) {
		writeErr(w, http.StatusConflict,
			"credential set changed after this form was loaded; reload and apply the update again")
		return
	}
	if storeErr != nil {
		writeErr(w, http.StatusInternalServerError, storeErr.Error())
		return
	}

	redacted, _ := s.creds.Redacted(name)
	operation := "created"
	status := http.StatusCreated
	action := "credential.create"
	if exists {
		operation = "updated"
		status = http.StatusOK
		action = "credential.update"
	}
	s.recordAudit(r, action, "credential", name, name, audit.ResultSuccess,
		"kind="+deployCredentialKind+" service="+service)
	writeJSON(w, status, map[string]any{
		"credential": s.decorateDeployCredential(redacted),
		"operation":  operation,
		"note":       "secret values are stored on the gateway and never returned",
	})
}

// handleDeleteCredentials removes an unreferenced deployment credential set.
// DELETE /api/v1/credentials/{name}
func (s *Server) handleDeleteCredentials(w http.ResponseWriter, r *http.Request) {
	if s.creds == nil || !s.creds.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "credential storage is unavailable")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "removing credentials requires an admin or cert_manager role")
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()
	_, set, err := s.creds.Reveal(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if set.Kind != deployCredentialKind {
		writeErr(w, http.StatusConflict,
			"CA account credentials must be removed through the CA account API")
		return
	}
	if refs := s.credentialEndpointReferences(name); len(refs) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":      "credential set is still referenced by saved deployment endpoints",
			"credential": name,
			"references": refs,
		})
		return
	}
	if err := s.creds.Delete(name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit(r, "credential.delete", "credential", name, name,
		audit.ResultSuccess, "")
	writeJSON(w, http.StatusOK, map[string]any{"removed": name})
}
