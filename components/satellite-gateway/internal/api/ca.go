package api

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/ca"
	"github.com/m2p/clm/internal/satellite/policy"
	"github.com/m2p/clm/internal/satellite/securefs"
	"go.uber.org/zap"
	"golang.org/x/crypto/acme"
)

// Multi-vendor CA routing.
//
// Issuance was hardwired to one ACME directory, and the two fields that accepted
// a CA choice were discarded without a word: a request for DigiCert produced a
// Let's Encrypt certificate. Now every CA sits behind ca.Provider, ACME included,
// and a request either reaches the CA it named or is refused with the list of
// accounts that do exist.

// acmeAdapter presents the existing ACME manager as a ca.Provider, so ACME is
// routed the same way as every commercial vendor rather than being a special case
// in the handler.
type acmeAdapter struct {
	mgr    *ACMEManager
	logger *zap.Logger
}

// NewACMEProvider wraps the ACME manager as a CA provider.
func NewACMEProvider(mgr *ACMEManager, logger *zap.Logger) ca.Provider {
	return &acmeAdapter{mgr: mgr, logger: logger}
}

func (a *acmeAdapter) Name() string { return "acme" }

func (a *acmeAdapter) Capabilities() ca.Capabilities {
	dcvMethods := []string{}
	notes := "Free and immediate, DV only."
	if a.mgr.SupportsHTTP01() {
		dcvMethods = append(dcvMethods, ca.DCVHTTP)
		notes += " HTTP-01 is enabled for ordinary names."
	}
	wildcard := a.mgr.SupportsDNS01()
	if wildcard {
		dcvMethods = append(dcvMethods, ca.DCVDNS)
		notes += " DNS-01 is enabled for wildcard and DNS-selected multi-SAN issuance."
	} else {
		notes += " Wildcards require a live DNS-01 provider and a policy other than explicit HTTP-only mode."
	}
	return ca.Capabilities{
		Synchronous:      true,
		CertTypes:        []string{"dv"},
		Wildcard:         wildcard,
		MultiSAN:         true,
		DCVMethods:       dcvMethods,
		RequiresCSR:      false,
		Revocation:       true,
		Quoting:          false,
		MaxValidityYears: 1,
		Notes:            notes,
	}
}

func (a *acmeAdapter) Quote(ctx context.Context, req ca.QuoteRequest) ([]ca.Quote, error) {
	// ACME CAs are free; a zero-price quote would clutter a price comparison.
	return nil, nil
}

func (a *acmeAdapter) Issue(ctx context.Context, req ca.IssueRequest) (*ca.IssueResult, error) {
	if ca.NormaliseCertType(req.CertType) != "dv" {
		return nil, fmt.Errorf("ACME issues domain-validated certificates only; " +
			"OV and EV require a commercial CA account")
	}
	if ca.HasWildcard(req.Domains) && !a.mgr.SupportsDNS01() {
		return nil, fmt.Errorf("wildcard names require a live DNS-01 provider")
	}

	certID := req.CertID
	if certID == "" {
		certID = uuid.NewString()
	}

	issued, err := a.mgr.IssueForTenant(ctx, req.TenantID, certID, req.Domains)
	if err != nil {
		return nil, err
	}
	return &ca.IssueResult{
		Status: ca.StatusIssued, Provider: "acme",
		ChainPEM: issued.ChainPEM, KeyPath: issued.KeyPath,
		NotBefore: issued.NotBefore, NotAfter: issued.NotAfter,
		Serial: issued.Serial, Issuer: issued.Issuer,
	}, nil
}

// OrderStatus has nothing to poll: ACME issuance either completed inside Issue or
// failed outright.
func (a *acmeAdapter) OrderStatus(ctx context.Context, orderID string) (*ca.IssueResult, error) {
	return nil, fmt.Errorf("ACME orders complete synchronously, so there is no order to poll")
}

func (a *acmeAdapter) Revoke(ctx context.Context, req ca.RevokeRequest) error {
	if req.KeyPEM == "" {
		return fmt.Errorf("ACME revocation must be signed with the certificate's own key")
	}
	return a.mgr.RevokeForTenant(ctx, req.TenantID, req.ChainPEM, req.KeyPEM, revocationReason(req.Reason))
}

// --- handlers ---

// handleListCAProviders reports the configured CA accounts and what each can do.
// GET /api/v1/ca/providers
func (s *Server) handleListCAProviders(w http.ResponseWriter, r *http.Request) {
	if s.caRegistry == nil || s.caRegistry.Count() == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"accounts": []any{}, "default": "",
			"reason": "no CA accounts are configured; set acme.enabled, or supply " +
				"GoDaddy or Namecheap credentials in the environment",
		})
		return
	}
	accounts := s.caRegistry.List()
	// Fleet managers receive write-only credential metadata needed for safe
	// edits; values remain masked and other roles see account capabilities only.
	if canManageFleet(callerClaims(r).EffectiveRole()) && s.creds != nil && s.creds.Enabled() {
		for _, account := range accounts {
			name, _ := account["name"].(string)
			redacted, err := s.creds.Redacted(name)
			if err != nil || redacted["kind"] != caAccountKind {
				continue
			}
			account["credential_keys"] = redacted["keys"]
			account["credential_updated_at"] = redacted["updated_at"]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accounts": accounts,
		"default":  s.caRegistry.Default(),
	})
}

// handleCAQuote gathers pricing from every account that supports it.
//
// This fans out a real network call to every configured CA vendor with no rate
// limit and previously no role check, so any authenticated principal could
// trigger unlimited outbound calls against the gateway's own vendor accounts.
// Pricing is not sensitive, but the outbound call itself is a resource a viewer
// should not be able to spend at will.
// POST /api/v1/ca/quote
func (s *Server) handleCAQuote(w http.ResponseWriter, r *http.Request) {
	if s.caRegistry == nil || s.caRegistry.Count() == 0 {
		writeErr(w, http.StatusServiceUnavailable, "no CA accounts are configured")
		return
	}
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) && role != "operator" {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot request CA pricing")
		return
	}

	var req struct {
		Domains       []string `json:"domains"`
		Domain        string   `json:"domain"`
		CertType      string   `json:"cert_type"`
		ValidityYears int      `json:"validity_years"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	domains := collectDomains(req.Domain, req.Domains)
	if len(domains) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one domain is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	quotes, errs := s.caRegistry.QuoteAll(ctx, ca.QuoteRequest{
		Domains: domains, CertType: ca.NormaliseCertType(req.CertType),
		ValidityYears: req.ValidityYears,
	})

	body := map[string]any{
		"domains": domains,
		"quotes":  quotes,
		"count":   len(quotes),
	}
	if len(errs) > 0 {
		body["errors"] = errs
	}
	if len(quotes) == 0 && len(errs) == 0 {
		body["message"] = "none of the configured CA accounts expose pricing through " +
			"their API, so no quotes are available"
	}
	writeJSON(w, http.StatusOK, body)
}

// collectDomains merges the singular and plural forms and normalises them.
func collectDomains(single string, many []string) []string {
	out := make([]string, 0, len(many)+1)
	seen := map[string]struct{}{}
	for _, d := range append([]string{single}, many...) {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if _, dup := seen[d]; dup {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// handleCAIssue orders a certificate from a named CA account.
// POST /api/v1/certificates/issue
func (s *Server) handleCAIssue(w http.ResponseWriter, r *http.Request) {
	if s.caRegistry == nil || s.caRegistry.Count() == 0 {
		writeErr(w, http.StatusServiceUnavailable,
			"no CA accounts are configured on this gateway")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden,
			"issuing a certificate requires the super_admin, admin, tenant_admin or cert_manager role")
		return
	}

	var req struct {
		Domains []string `json:"domains"`
		Domain  string   `json:"domain"`
		// CAAccount names the account; CAProvider is accepted as an alias because
		// that is the field name the older API and the shared models use.
		CAAccount     string           `json:"ca_account"`
		CAProvider    string           `json:"ca_provider"`
		CA            string           `json:"ca"`
		CertType      string           `json:"cert_type"`
		Type          string           `json:"type"`
		ValidityYears int              `json:"validity_years"`
		Environment   string           `json:"environment"`
		ContactEmail  string           `json:"contact_email"`
		DCVMethod     string           `json:"dcv_method"`
		KeyType       string           `json:"key_type"`
		Organization  *ca.Organization `json:"organization"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	domains := collectDomains(req.Domain, req.Domains)
	if len(domains) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one domain is required")
		return
	}

	// Resolve the CA first. An unknown name is refused with the list of what is
	// configured, rather than quietly using a different CA than the one asked for.
	wanted := firstNonEmpty(req.CAAccount, req.CAProvider, req.CA)
	account, err := s.caRegistry.Get(wanted)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	provider := account.Provider_()
	caps := provider.Capabilities()

	certType := ca.NormaliseCertType(firstNonEmpty(req.CertType, req.Type, "dv"))
	if !containsStr(caps.CertTypes, certType) {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf(
			"account %q issues %s certificates; it cannot issue %s",
			account.Name, strings.Join(caps.CertTypes, "/"), certType))
		return
	}
	if ca.HasWildcard(domains) && !caps.Wildcard {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf(
			"account %q cannot issue wildcard certificates", account.Name))
		return
	}
	if len(domains) > 1 && !caps.MultiSAN {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf(
			"account %q issues single-name certificates only", account.Name))
		return
	}
	// OV and EV are vetted against a real organisation, so the details are
	// mandatory. Checking here rather than letting the provider reject it keeps
	// this a 400: it is bad input, not a failure at the CA, and returning 502
	// would suggest the vendor was at fault.
	if certType != "dv" && req.Organization == nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf(
			"%s certificates are vetted against an organisation, so an "+
				"\"organization\" object is required (name, address, city, country, phone "+
				"and a contact)", certType))
		return
	}
	if req.Organization != nil && strings.TrimSpace(req.Organization.Name) == "" {
		writeErr(w, http.StatusBadRequest, "organization.name is required")
		return
	}

	// Certificate policy: enforced before any CA is contacted, so a request a
	// policy would refuse never reaches a commercial order (and never spends
	// money) and never reaches ACME's rate limits.
	environment := nonEmpty(req.Environment, "unknown")
	if s.policies != nil && s.policies.Count() > 0 {
		keyType := nonEmpty(req.KeyType, "ecdsa")
		keyBits := 256 // ECDSA P-256, the default GenerateKeyAndCSR produces
		if strings.EqualFold(keyType, "rsa") {
			keyBits = 2048 // the only RSA size GenerateKeyAndCSR produces today
		}
		approved := s.inventory.HasApprovedIssuance(domains[0], certType, environment)

		violations := s.policies.Evaluate(policy.Request{
			Domains: domains, CertType: certType, Environment: environment,
			CAAccount: account.Name, ValidityYears: req.ValidityYears,
			KeyType: keyType, KeyBits: keyBits, ApprovalDecided: approved,
		})

		// A violation whose only failing rule is "an approval is required" is not
		// refused outright — it creates the pending approval that would satisfy
		// it, so the request becomes actionable instead of being a dead end the
		// requester has to go create separately. Any other violation is refused.
		var blocking []policy.Violation
		needsApproval := false
		for _, v := range violations {
			if v.Rule == "require_approval" {
				needsApproval = true
				continue
			}
			blocking = append(blocking, v)
		}

		if len(blocking) > 0 {
			msgs := make([]string, 0, len(blocking))
			for _, v := range blocking {
				msgs = append(msgs, v.Message)
			}
			s.recordAudit(r, "certificate.issue", "certificate", "", domains[0], audit.ResultDenied,
				"blocked by policy: "+strings.Join(msgs, "; "))
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":      "this request is blocked by certificate policy",
				"violations": blocking,
			})
			return
		}

		if needsApproval && !approved {
			pending := s.inventory.AddApproval(&Approval{
				RequestType: "certificate_issue",
				Level:       "L1",
				Domain:      domains[0],
				CertType:    certType,
				Environment: environment,
				CAAccount:   account.Name,
				Justification: fmt.Sprintf(
					"policy requires approval before issuing a %s certificate for %s in %s",
					strings.ToUpper(certType), domains[0], environment),
				RequestedBy: callerName(r),
			})
			s.recordAudit(r, "certificate.issue", "certificate", pending.ID, domains[0], audit.ResultDenied,
				"policy requires approval; a pending approval was created instead of issuing")
			writeJSON(w, http.StatusAccepted, map[string]any{
				"status":      "awaiting_approval",
				"approval_id": pending.ID,
				"message": fmt.Sprintf(
					"certificate policy requires approval for %s certificates in %s. A pending "+
						"approval has been recorded (id %s); once it is approved, resubmit this "+
						"request to issue.", strings.ToUpper(certType), environment, pending.ID),
			})
			return
		}
	}

	certID := uuid.NewString()
	tenantID := strings.TrimSpace(callerClaims(r).TenantID)
	issueReq := ca.IssueRequest{
		Domains: domains, CertType: certType,
		ValidityYears: req.ValidityYears, ContactEmail: req.ContactEmail,
		Organization: req.Organization, DCVMethod: req.DCVMethod, CertID: certID,
		TenantID: tenantID,
	}

	// Commercial CAs sign a CSR we supply, so generate the key pair here. The key
	// is written only once the order is accepted, so a rejected order leaves
	// nothing behind.
	var pendingKeyPEM string
	if caps.RequiresCSR {
		keyPEM, csrPEM, gerr := ca.GenerateKeyAndCSR(domains, req.KeyType)
		if gerr != nil {
			writeErr(w, http.StatusInternalServerError, gerr.Error())
			return
		}
		issueReq.CSRPEM = csrPEM
		pendingKeyPEM = keyPEM
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()

	result, err := provider.Issue(ctx, issueReq)
	if err != nil {
		s.inventory.Upsert(&Certificate{
			ID: certID, Domain: domains[0], SANs: domains,
			TenantID: tenantID,
			Type:     strings.ToUpper(certType), Status: "failed", Source: "ca",
			ManagedBy: account.Provider, CAName: account.Provider, CAAccount: account.Name,
			Environment: environment, RequestedBy: callerName(r),
			LastError: err.Error(), CreatedAt: time.Now(),
		})
		s.logger.Error("ca issuance failed",
			zap.String("account", account.Name), zap.Strings("domains", domains), zap.Error(err))
		s.recordAudit(r, "certificate.issue", "certificate", certID, domains[0], audit.ResultFailure, err.Error())
		writeErr(w, http.StatusBadGateway, "issuance failed: "+err.Error())
		return
	}

	secretRef, err := s.storeIssuedKey(certID, result, pendingKeyPEM, tenantID)
	if err != nil {
		s.logger.Error("storing the private key failed",
			zap.String("cert_id", certID), zap.Error(err))
		s.recordAudit(r, "certificate.issue", "certificate", certID, domains[0], audit.ResultFailure,
			"issued at the CA but storing the private key failed: "+err.Error())
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	cert := &Certificate{
		ID: certID, Domain: domains[0], SANs: domains,
		TenantID: tenantID,
		Type:     strings.ToUpper(certType), Source: "ca",
		ManagedBy: account.Provider, CAName: nonEmpty(result.Issuer, account.Provider),
		CAAccount: account.Name, CAOrderID: result.OrderID,
		Environment: environment, RequestedBy: callerName(r),
		SecretRef: secretRef, CreatedAt: time.Now(),
	}

	switch result.Status {
	case ca.StatusIssued:
		cert.Status, cert.DaysLeft = statusFor(result.NotAfter)
		cert.NotBefore, cert.NotAfter = result.NotBefore, result.NotAfter
		cert.SerialNumber, cert.Issuer = result.Serial, result.Issuer
		cert.ChainPEM = result.ChainPEM
		cert.IssuedAt = time.Now()
	default:
		// A pending commercial order is a normal outcome, not a failure. It is
		// recorded so the scheduler can poll it and so the validation steps are
		// visible to whoever has to perform them.
		cert.Status = "pending_ca"
		cert.Issuer = account.Provider
		cert.Validation = result.Validation
		cert.LastError = ""
	}
	s.inventory.Upsert(cert)

	status := http.StatusCreated
	body := map[string]any{
		"id": cert.ID, "domains": domains,
		"ca_account": account.Name, "ca_provider": account.Provider,
		"order_status": result.Status, "order_id": result.OrderID,
		"cert_type": certType,
	}
	if result.Status == ca.StatusIssued {
		body["issuer"] = cert.Issuer
		body["not_after"] = cert.NotAfter
		body["days_left"] = cert.DaysLeft
		body["note"] = "the private key is stored on the gateway and is never returned by the API"
	} else {
		status = http.StatusAccepted
		body["validation"] = result.Validation
		body["message"] = nonEmpty(result.Message,
			"the CA has accepted the order and is waiting on domain control validation")
		body["next"] = "complete the validation steps, then the order is polled " +
			"automatically; GET /api/v1/ca/orders shows progress"
	}

	s.logger.Info("ca order placed",
		zap.String("account", account.Name), zap.String("provider", account.Provider),
		zap.Strings("domains", domains), zap.String("status", result.Status),
		zap.String("order_id", result.OrderID), zap.String("by", callerName(r)))
	s.recordAudit(r, "certificate.issue", "certificate", certID, domains[0], audit.ResultSuccess,
		fmt.Sprintf("account=%s cert_type=%s order_status=%s", account.Name, certType, result.Status))

	writeJSON(w, status, body)
}

// readKeyFile loads a stored private key for a deployment. This is the only
// place a key is read back off disk, so the deploy path has one auditable route
// to key material rather than several.
func (s *Server) readKeyFile(path, tenantID string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("no key path recorded for this certificate")
	}
	if s.filePolicy == nil {
		return "", fmt.Errorf("secure file storage policy is not configured")
	}
	data, err := s.filePolicy.ReadFile(path, tenantID, securefs.PrivateKey)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// storeIssuedKey puts the private key where the gateway keeps them and returns
// its path. Providers either hand back a key to store or have stored one already.
func (s *Server) storeIssuedKey(certID string, result *ca.IssueResult, pendingKeyPEM, tenantID string) (string, error) {
	if result.KeyPath != "" {
		return result.KeyPath, nil
	}

	keyPEM := result.PrivateKeyPEM
	if keyPEM == "" {
		keyPEM = pendingKeyPEM
	}
	if keyPEM == "" {
		return "", nil
	}
	if s.keyDir == "" {
		return "", fmt.Errorf("there is nowhere to store the private key: set state.dir")
	}
	if s.filePolicy == nil {
		return "", fmt.Errorf("secure file storage policy is not configured")
	}
	path, err := s.filePolicy.DerivedPath(filepath.Dir(s.keyDir),
		filepath.Join("keys", certID+".key"), tenantID)
	if err != nil {
		return "", fmt.Errorf("resolving the private key path: %w", err)
	}
	if err := s.filePolicy.MkdirAll(filepath.Dir(path), tenantID, 0o700); err != nil {
		return "", fmt.Errorf("creating the key directory: %w", err)
	}
	if err := s.filePolicy.WriteFile(path, tenantID, securefs.PrivateKey, []byte(keyPEM), false); err != nil {
		return "", fmt.Errorf("writing the private key: %w", err)
	}
	return path, nil
}

// handleListCAOrders lists orders still waiting on the CA.
// GET /api/v1/ca/orders
func (s *Server) handleListCAOrders(w http.ResponseWriter, r *http.Request) {
	orders := []map[string]any{}
	for _, c := range s.inventory.ListCerts() {
		if c.Status != "pending_ca" || c.CAOrderID == "" {
			continue
		}
		orders = append(orders, map[string]any{
			"cert_id": c.ID, "domain": c.Domain, "sans": c.SANs,
			"ca_account": c.CAAccount, "ca_provider": c.ManagedBy,
			"order_id": c.CAOrderID, "cert_type": c.Type,
			"placed_at": c.CreatedAt, "validation": c.Validation,
			"last_error":  c.LastError,
			"waiting_for": time.Since(c.CreatedAt).Round(time.Minute).String(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orders": orders, "count": len(orders),
		"note": "OV and EV orders involve human vetting at the CA and can take days",
	})
}

// handleRefreshCAOrder re-reads one order from the CA now.
// POST /api/v1/ca/orders/{id}/refresh
func (s *Server) handleRefreshCAOrder(w http.ResponseWriter, r *http.Request) {
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "refreshing a CA order requires an admin or cert_manager role")
		return
	}

	cert, ok := s.inventory.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "certificate not found")
		return
	}
	if cert.CAOrderID == "" {
		writeErr(w, http.StatusBadRequest, "this certificate has no CA order to refresh")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	changed, err := s.refreshCAOrder(ctx, cert)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": cert.ID, "status": cert.Status, "changed": changed,
		"order_id": cert.CAOrderID, "validation": cert.Validation,
		"not_after": cert.NotAfter, "message": cert.LastError,
	})
}

// refreshCAOrder polls one pending order and completes the record when the CA has
// issued. Shared by the endpoint and the scheduled sweep.
func (s *Server) refreshCAOrder(ctx context.Context, cert *Certificate) (bool, error) {
	account, err := s.caRegistry.Get(nonEmpty(cert.CAAccount, cert.ManagedBy))
	if err != nil {
		return false, fmt.Errorf("the CA account for this order is no longer configured: %w", err)
	}

	result, err := account.Provider_().OrderStatus(ctx, cert.CAOrderID)
	if err != nil {
		cert.LastError = err.Error()
		s.inventory.Upsert(cert)
		return false, err
	}

	switch result.Status {
	case ca.StatusIssued:
		cert.Status, cert.DaysLeft = statusFor(result.NotAfter)
		cert.NotBefore, cert.NotAfter = result.NotBefore, result.NotAfter
		cert.SerialNumber = result.Serial
		cert.Issuer = nonEmpty(result.Issuer, cert.Issuer)
		cert.CAName = cert.Issuer
		cert.ChainPEM = result.ChainPEM
		cert.IssuedAt = time.Now()
		cert.Validation = nil
		cert.LastError = ""
		s.inventory.Upsert(cert)
		s.logger.Info("ca order completed",
			zap.String("domain", cert.Domain), zap.String("account", account.Name),
			zap.Time("not_after", cert.NotAfter))
		return true, nil

	case ca.StatusFailed, ca.StatusRevoked:
		cert.Status = "failed"
		cert.LastError = nonEmpty(result.Message, "the CA reported the order as "+result.Status)
		s.inventory.Upsert(cert)
		return true, nil

	default:
		// Still pending. Refresh the validation steps, which can change as the CA
		// validates each name.
		if len(result.Validation) > 0 {
			cert.Validation = result.Validation
		}
		cert.LastError = result.Message
		s.inventory.Upsert(cert)
		return false, nil
	}
}

// runCAOrderPoll polls every pending order. Called by the scheduler.
func (s *Server) runCAOrderPoll(ctx context.Context) (checked, completed int, errs []string) {
	if s.caRegistry == nil {
		return 0, 0, nil
	}
	for _, cert := range s.inventory.ListCerts() {
		if cert.Status != "pending_ca" || cert.CAOrderID == "" {
			continue
		}
		checked++
		pollCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		done, err := s.refreshCAOrder(pollCtx, cert)
		cancel()
		if err != nil {
			errs = append(errs, cert.Domain+": "+err.Error())
			continue
		}
		if done {
			completed++
		}
	}
	return checked, completed, errs
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// ensure the acme package stays referenced for revocationReason's signature.
var _ = acme.CRLReasonUnspecified
