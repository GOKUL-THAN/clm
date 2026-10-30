package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/ca"
	"github.com/m2p/clm/internal/satellite/discovery/scanners"
	"go.uber.org/zap"
)

// handleHealth reports liveness. Unauthenticated on purpose.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "clm-satellite-gateway",
		"id":      s.satelliteID,
		"name":    s.satelliteName,
		"version": s.version,
		"uptime":  time.Since(s.startedAt).Round(time.Second).String(),
	})
}

// handleStatus returns a dashboard summary for the portal.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	stats := s.inventory.CertStats()
	body := map[string]any{
		"id":                s.satelliteID,
		"name":              s.satelliteName,
		"version":           s.version,
		"saas_endpoint":     s.saasEndpoint,
		"uptime":            time.Since(s.startedAt).Round(time.Second).String(),
		"certificates":      stats["total"],
		"expiring_30d":      stats["expiring_30d"],
		"pending_approvals": len(s.inventory.PendingApprovals()),
	}
	if s.agents != nil {
		body["agents"] = s.agents.Stats()
	}
	writeJSON(w, http.StatusOK, body)
}

// handleAgents lists vSatellite agents registered with this gateway.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		writeJSON(w, http.StatusOK, map[string]any{"agents": []any{}, "total": 0})
		return
	}
	agents := s.agents.ListAgents()
	writeJSON(w, http.StatusOK, map[string]any{
		"agents": agents,
		"total":  len(agents),
		"stats":  s.agents.Stats(),
	})
}

// handleListCertificates returns the gateway's certificate inventory.
func (s *Server) handleListCertificates(w http.ResponseWriter, r *http.Request) {
	certs := s.inventory.ListCerts()
	writeJSON(w, http.StatusOK, map[string]any{
		"certificates": certs,
		"total":        len(certs),
		"stats":        s.inventory.CertStats(),
	})
}

// handleRequestCertificate records a certificate request as a bookkeeping
// entry, separate from actually ordering one (POST /api/v1/certificates/issue).
//
// This had no role check at all: a viewer token could create request records
// indefinitely. Recording a request is lower-stakes than issuing or deploying,
// so the bar is the same as deploy's — anyone who can act operationally, not a
// read-only viewer.
func (s *Server) handleRequestCertificate(w http.ResponseWriter, r *http.Request) {
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) && role != "operator" {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot request certificates")
		return
	}

	var req struct {
		Domain      string `json:"domain"`
		Type        string `json:"type"`
		Environment string `json:"environment"`
		CAProvider  string `json:"ca_provider"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Domain) == "" {
		writeErr(w, http.StatusBadRequest, "domain is required")
		return
	}
	if req.Type == "" {
		req.Type = "DV"
	}
	if req.Environment == "" {
		req.Environment = "prod"
	}

	cert := s.inventory.AddRequest(req.Domain, req.Type, req.Environment, callerName(r))
	s.logger.Info("certificate requested via gateway portal",
		zap.String("domain", req.Domain), zap.String("by", callerName(r)))
	s.recordAudit(r, "certificate.request", "certificate", cert.ID, req.Domain, audit.ResultSuccess, "")

	writeJSON(w, http.StatusAccepted, map[string]any{
		"certificate": cert,
		"status":      "pending",
		"message": "Request recorded. Automated issuance is not configured on this " +
			"gateway yet, so the certificate must be issued and imported manually.",
	})
}

// handlePendingApprovals lists approvals awaiting a decision.
func (s *Server) handlePendingApprovals(w http.ResponseWriter, r *http.Request) {
	pending := s.inventory.PendingApprovals()
	writeJSON(w, http.StatusOK, map[string]any{
		"approvals": pending,
		"total":     len(pending),
	})
}

// handleApprove records an approval decision.
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	s.decide(w, r, "approved")
}

// handleReject records a rejection.
func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	s.decide(w, r, "rejected")
}

// decide applies an approval decision. Only roles that can authorise change are
// allowed; a viewer must not be able to approve.
func (s *Server) decide(w http.ResponseWriter, r *http.Request, status string) {
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot approve or reject requests")
		return
	}

	var req struct {
		Comment string `json:"comment"`
	}
	_ = decodeBody(r, &req) // comment is optional

	id := r.PathValue("id")
	approval, ok := s.inventory.Decide(id, status, callerName(r), req.Comment)
	if !ok {
		writeErr(w, http.StatusNotFound, "no pending approval with id "+id)
		return
	}

	s.logger.Info("approval decision recorded",
		zap.String("id", id), zap.String("status", status), zap.String("by", callerName(r)))
	s.recordAudit(r, "approval."+status, "approval", approval.ID, approval.Domain, audit.ResultSuccess,
		nonEmpty(req.Comment, approval.RequestType))
	writeJSON(w, http.StatusOK, map[string]any{"approval": approval, "status": status})
}

// canApprove reports whether a role may sign off on a change.
func canApprove(role string) bool {
	switch role {
	case "super_admin", "admin", "tenant_admin", "approver", "cert_manager":
		return true
	}
	return false
}

// handleDiscover runs a real TLS scan across the requested CIDRs and folds the
// results into the certificate inventory.
//
// This had no role check: any authenticated principal, including a read-only
// viewer, could trigger a network sweep of arbitrary CIDRs from the gateway.
// A scan is a real network operation with a real blast radius (it is how this
// gateway probes internal ranges), so it is held to the same bar as deploy and
// certificate requests rather than to read access.
func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		writeErr(w, http.StatusServiceUnavailable, "network scanner is not configured")
		return
	}
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) && role != "operator" {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot run a discovery scan")
		return
	}

	var req struct {
		CIDRs []string `json:"cidrs"`
		Hosts []string `json:"hosts"`
		// Domains are resolved and probed by name with SNI. A bare host probe
		// would report only the default certificate of whatever the name
		// resolves to, which is the wrong answer on any shared address.
		Domains []string `json:"domains"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	targets := make([]string, 0, len(req.CIDRs))
	for _, c := range req.CIDRs {
		if strings.TrimSpace(c) != "" {
			targets = append(targets, strings.TrimSpace(c))
		}
	}
	if len(targets) == 0 && len(req.Hosts) == 0 && len(req.Domains) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one CIDR, host or domain is required")
		return
	}

	// Scans are slow; bound them so the request cannot hang the portal.
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()

	found := 0
	scanErrs := make([]string, 0)

	for _, cidr := range targets {
		certs, err := s.scanner.ScanNetwork(ctx, cidr)
		if err != nil {
			scanErrs = append(scanErrs, cidr+": "+err.Error())
			continue
		}
		found += s.recordDiscovered(certs)
	}
	for _, host := range req.Hosts {
		certs, err := s.scanner.ScanHost(ctx, strings.TrimSpace(host))
		if err != nil {
			scanErrs = append(scanErrs, host+": "+err.Error())
			continue
		}
		found += s.recordDiscovered(certs)
	}

	// Domains go through the DNS scanner so the handshake carries SNI. Record
	// walking and wordlist enumeration are left off here; POST /discover/dns is
	// the endpoint for that.
	for _, domain := range req.Domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if s.dnsScanner == nil {
			scanErrs = append(scanErrs, domain+": DNS scanner is not configured")
			continue
		}
		result, err := s.dnsScanner.ScanDomain(ctx, domain, scanners.DNSScanOptions{})
		if err != nil {
			scanErrs = append(scanErrs, domain+": "+err.Error())
			continue
		}
		found += s.recordDiscovered(result.Certs)
	}

	s.logger.Info("discovery scan finished",
		zap.Strings("cidrs", targets),
		zap.Strings("domains", req.Domains),
		zap.Int("certs_found", found),
		zap.Int("errors", len(scanErrs)))

	scanned := append([]string{}, targets...)
	scanned = append(scanned, req.Hosts...)
	scanned = append(scanned, req.Domains...)

	auditResult := audit.ResultSuccess
	if len(scanErrs) > 0 && found == 0 {
		auditResult = audit.ResultFailure
	}
	s.recordAudit(r, "discovery.network", "discovery", "", strings.Join(scanned, ", "), auditResult,
		fmt.Sprintf("certs_found=%d errors=%d", found, len(scanErrs)))

	body := map[string]any{
		"certs_found": found,
		"scanned":     scanned,
	}
	if len(scanErrs) > 0 {
		body["errors"] = scanErrs
	}
	writeJSON(w, http.StatusOK, body)
}

// handleDiscoverDNS discovers certificates by name: it collects candidate
// hostnames from DNS and probes each one with SNI, then folds the results into
// the inventory.
//
// This finds what a CIDR sweep cannot — name-based virtual hosts that only
// present their certificate when the right SNI value is sent, and hosts whose
// addresses fall outside any scanned range, such as anything behind a CDN or a
// load balancer.
func (s *Server) handleDiscoverDNS(w http.ResponseWriter, r *http.Request) {
	if s.dnsScanner == nil {
		writeErr(w, http.StatusServiceUnavailable, "DNS scanner is not configured")
		return
	}
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) && role != "operator" {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot run a discovery scan")
		return
	}

	var req struct {
		Domain     string   `json:"domain"`
		Domains    []string `json:"domains"`
		DNSServer  string   `json:"dns_server"`
		Enumerate  bool     `json:"enumerate"`
		FollowSANs bool     `json:"follow_sans"`
		Wordlist   []string `json:"wordlist"`
		ExtraNames []string `json:"extra_names"`
		Ports      []int    `json:"ports"`
		MaxNames   int      `json:"max_names"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	domains := make([]string, 0, len(req.Domains)+1)
	for _, d := range append([]string{req.Domain}, req.Domains...) {
		if d = strings.TrimSpace(d); d != "" {
			domains = append(domains, d)
		}
	}
	if len(domains) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one domain is required")
		return
	}

	// Enumeration multiplies candidate names by ports, so the work is bounded
	// well inside nginx's 300s proxy_read_timeout to keep the portal responsive.
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()

	scanner := s.dnsScanner.WithDNSServer(req.DNSServer)
	opts := scanners.DNSScanOptions{
		Enumerate:  req.Enumerate,
		FollowSANs: req.FollowSANs,
		Wordlist:   req.Wordlist,
		ExtraNames: req.ExtraNames,
		Ports:      req.Ports,
		MaxNames:   req.MaxNames,
	}

	results := make([]*scanners.DNSScanResult, 0, len(domains))
	recorded := 0
	scanErrs := make([]string, 0)

	for _, domain := range domains {
		result, err := scanner.ScanDomain(ctx, domain, opts)
		if err != nil {
			scanErrs = append(scanErrs, domain+": "+err.Error())
			continue
		}
		recorded += s.recordDiscovered(result.Certs)
		results = append(results, result)
	}

	namesTried, namesResolved, namesLimit := 0, 0, 0
	for _, r := range results {
		namesTried += r.NamesTried
		namesResolved += len(r.NamesResolved)
		if r.NamesLimit > namesLimit {
			namesLimit = r.NamesLimit
		}
	}

	s.logger.Info("dns discovery finished",
		zap.Strings("domains", domains),
		zap.String("dns_server", scanner.DNSServer()),
		zap.Bool("enumerate", req.Enumerate),
		zap.Int("names_tried", namesTried),
		zap.Int("names_resolved", namesResolved),
		zap.Int("certs_found", recorded),
		zap.Int("errors", len(scanErrs)))
	s.recordAudit(r, "discovery.dns", "discovery", "", strings.Join(domains, ", "), audit.ResultSuccess,
		fmt.Sprintf("certs_found=%d names_tried=%d", recorded, namesTried))

	body := map[string]any{
		"certs_found":       recorded,
		"names_tried":       namesTried,
		"names_resolved":    namesResolved,
		"names_limit":       namesLimit,
		"max_names_allowed": scanners.MaxNamesLimit,
		"dns_server":        scanner.DNSServer(),
		"results":           results,
	}
	if len(scanErrs) > 0 {
		body["errors"] = scanErrs
	}
	writeJSON(w, http.StatusOK, body)
}

// recordDiscovered converts scan results into inventory entries and returns the
// number of distinct certificates seen. The same certificate is often served on
// several ports, so probe hits are deduplicated by fingerprint to match what
// the inventory actually stores.
func (s *Server) recordDiscovered(certs []*scanners.DiscoveredCert) int {
	unique := make(map[string]struct{}, len(certs))
	for _, d := range certs {
		key := d.Fingerprint
		if key == "" {
			key = d.Subject + "|" + d.Host
		}
		unique[key] = struct{}{}
	}

	for _, d := range certs {
		domain := d.Subject
		// A wildcard subject names no single host, and DNS discovery already
		// knows the exact name that served the certificate, so prefer it.
		if strings.HasPrefix(domain, "*.") && d.Host != "" {
			domain = d.Host
		}
		if domain == "" && len(d.SANs) > 0 {
			domain = d.SANs[0]
		}
		if domain == "" {
			domain = d.Host
		}

		status, days := statusFor(d.NotAfter)
		s.inventory.UpsertDiscovered(&Certificate{
			Domain:       domain,
			Type:         "discovered",
			Status:       status,
			Issuer:       d.Issuer,
			Environment:  "unknown",
			Source:       "discovery",
			Host:         d.Host,
			Port:         d.Port,
			SerialNumber: d.SerialNumber,
			Fingerprint:  d.Fingerprint,
			SANs:         d.SANs,
			SelfSigned:   d.SelfSigned,
			NotBefore:    d.NotBefore,
			NotAfter:     d.NotAfter,
			DaysLeft:     days,
			// Carried through so the weak-key control and the compliance report
			// have something to assess; the scanner collected these and the
			// inventory previously discarded them.
			KeyAlgorithm:    d.KeyAlgorithm,
			KeySize:         d.KeySize,
			DiscoveryMethod: d.DiscoveryMethod,
			CreatedAt:       time.Now(),
		})
	}
	return len(unique)
}

// handleQuote is the superseded purchasing endpoint, kept for callers that
// still point at it.
//
// It used to hard-code "no CA accounts are configured on this gateway" from the
// days when that was true. It is no longer true: accounts are registered at
// runtime and /api/v1/ca/quote returns live pricing for them, so the two
// endpoints contradicted each other. This now delegates to the same registry
// and translates the result into the old response shape, rather than reporting
// an emptiness that does not exist.
//
// Deprecated: use POST /api/v1/ca/quote, which supports multiple domains and
// reports per-account errors.
func (s *Server) handleQuote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain   string   `json:"domain"`
		Domains  []string `json:"domains"`
		CertType string   `json:"cert_type"`
		Validity string   `json:"validity"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	const deprecation = "this endpoint is superseded by POST /api/v1/ca/quote"

	body := map[string]any{
		"domain":     req.Domain,
		"cert_type":  req.CertType,
		"deprecated": deprecation,
	}

	if s.caRegistry == nil || s.caRegistry.Count() == 0 {
		body["quotes"] = []any{}
		body["message"] = "No CA accounts are configured on this gateway, so live pricing " +
			"is unavailable. Register a CA account to enable quotes."
		writeJSON(w, http.StatusOK, body)
		return
	}

	domains := collectDomains(req.Domain, req.Domains)
	if len(domains) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one domain is required")
		return
	}
	body["domains"] = domains

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	quotes, errs := s.caRegistry.QuoteAll(ctx, ca.QuoteRequest{
		Domains:       domains,
		CertType:      ca.NormaliseCertType(req.CertType),
		ValidityYears: legacyValidityYears(req.Validity),
	})

	// The old shape used ca/total_price/issuance_time. Both spellings go out so
	// an existing caller keeps working while a new one can read the real fields.
	legacy := make([]map[string]any, 0, len(quotes))
	for _, q := range quotes {
		legacy = append(legacy, map[string]any{
			"ca":            q.Account,
			"provider":      q.Provider,
			"account":       q.Account,
			"product_name":  q.ProductName,
			"cert_type":     q.CertType,
			"wildcard":      q.Wildcard,
			"total_price":   q.Price,
			"price":         q.Price,
			"currency":      q.Currency,
			"validity":      q.Validity,
			"issuance_time": q.IssuanceTime,
		})
	}
	body["quotes"] = legacy
	body["count"] = len(legacy)
	if len(errs) > 0 {
		body["errors"] = errs
	}
	if len(legacy) == 0 && len(errs) == 0 {
		body["message"] = "none of the configured CA accounts expose pricing through their API"
	}
	writeJSON(w, http.StatusOK, body)
}

// legacyValidityYears reads the old free-text validity field ("1y", "2y", "12m")
// as a whole number of years, defaulting to 1 rather than 0 so a vendor is not
// asked for a zero-year certificate.
func legacyValidityYears(v string) int {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return 1
	}
	digits := strings.TrimRight(v, "ymdo ")
	n, err := strconv.Atoi(strings.TrimSpace(digits))
	if err != nil || n <= 0 {
		return 1
	}
	if strings.HasSuffix(v, "m") && !strings.HasSuffix(v, "ym") {
		// Months: round up to whole years, keeping at least one.
		years := n / 12
		if n%12 != 0 {
			years++
		}
		if years <= 0 {
			years = 1
		}
		return years
	}
	return n
}
