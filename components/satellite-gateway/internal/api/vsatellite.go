package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	vsatellite "github.com/m2p/clm/internal/satellite/vsatellite"
	"go.uber.org/zap"
)

// This file is the vSatellite deployment engine: the operator-facing half of
// agent rollout. The coordinator accepts agents; this decides which agents are
// allowed to exist, and produces the command that puts one on a host.
//
// Deployment is pull-based on purpose. The gateway does not SSH into targets: it
// issues a single-use enrolment token and serves an install script the target
// fetches itself. That works through NAT and outbound-only firewalls, which is
// the normal shape of the segments a vSatellite is meant to reach, and it means
// the gateway never has to hold SSH credentials for the estate.

// canManageFleet reports whether a role may enrol or decommission agents.
// Deploying an agent grants a foothold inside a network segment, so it is held
// to the same bar as approving a change rather than to read access.
func canManageFleet(role string) bool {
	switch role {
	case "super_admin", "admin", "tenant_admin", "cert_manager":
		return true
	}
	return false
}

// handleListVSatellites returns the fleet with queue depth per agent.
// GET /api/v1/vsatellites
func (s *Server) handleListVSatellites(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		writeErr(w, http.StatusServiceUnavailable, "vSatellite coordinator is not configured")
		return
	}

	agents := s.agents.ListAgents()
	out := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		out = append(out, map[string]any{
			"id":              a.ID,
			"name":            a.Name,
			"host":            a.Host,
			"vpc":             a.VPC,
			"subnet":          a.Subnet,
			"status":          a.Status,
			"version":         a.Version,
			"cidrs":           a.CIDRs,
			"capabilities":    a.Capabilities,
			"cert_count":      a.CertCount,
			"last_heartbeat":  a.LastHeartbeat,
			"registered_at":   a.RegisteredAt,
			"queued_commands": s.agents.PendingCommands(a.ID),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agents":      out,
		"total":       len(out),
		"stats":       s.agents.Stats(),
		"enrolments":  s.agents.ListEnrolments(),
		"coordinator": map[string]any{"port": 9090, "path": "/vsatellite"},
	})
}

// handleEnrolVSatellite issues a single-use enrolment token and returns a
// ready-to-run install command.
// POST /api/v1/vsatellites/enroll
func (s *Server) handleEnrolVSatellite(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		writeErr(w, http.StatusServiceUnavailable, "vSatellite coordinator is not configured")
		return
	}
	claims := callerClaims(r)
	if !canManageFleet(claims.EffectiveRole()) {
		writeErr(w, http.StatusForbidden,
			"enrolling a vSatellite places an agent inside a network segment and "+
				"requires the super_admin, admin, tenant_admin or cert_manager role")
		return
	}

	var req struct {
		Name         string   `json:"name"`
		VPC          string   `json:"vpc"`
		Subnet       string   `json:"subnet"`
		CIDRs        []string `json:"cidrs"`
		Capabilities []string `json:"capabilities"`
		TTLHours     int      `json:"ttl_hours"`
		GatewayURL   string   `json:"gateway_url"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	base := strings.TrimSpace(req.GatewayURL)
	if base == "" {
		base = s.portalBaseURL(r)
	}
	var err error
	base, err = validatedGatewayURL(base)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "gateway_url must be an absolute HTTP(S) URL without credentials, query or fragment")
		return
	}
	if len(req.Capabilities) == 0 {
		req.Capabilities = []string{"scan_network", "deploy_linux"}
	}

	ttl := time.Duration(req.TTLHours) * time.Hour
	enrolment, err := s.agents.CreateEnrolment(req.Name, req.VPC, req.Subnet,
		req.CIDRs, req.Capabilities, ttl, callerName(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	installURL := fmt.Sprintf("%s/api/v1/vsatellites/install.sh?token=%s", base, url.QueryEscape(enrolment.Token))

	s.logger.Info("vSatellite enrolment issued",
		zap.String("name", req.Name),
		zap.String("by", callerName(r)),
		zap.String("token_prefix", enrolment.TokenPrefix))

	writeJSON(w, http.StatusCreated, map[string]any{
		"name":         enrolment.Name,
		"token":        enrolment.Token,
		"token_prefix": enrolment.TokenPrefix,
		"expires_at":   enrolment.ExpiresAt,
		"install_url":  installURL,
		"install_command": fmt.Sprintf("curl -fsSL -- %s | sudo bash", shellSingleQuote(installURL)),
		"note": "The token is single use and is shown only now. Run the command on " +
			"the target host; it detects the distribution, installs the agent and " +
			"registers it with this gateway.",
	})
}

// handleVSatelliteInstallScript serves the agent installer.
//
// Authenticated by the enrolment token in the query string rather than by a
// portal JWT, because the caller is the target host running curl, which has no
// portal session. The token is single use and short lived, and it is verified
// here so an invalid token cannot even read the script.
// GET /api/v1/vsatellites/install.sh?token=...
func (s *Server) handleVSatelliteInstallScript(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		writeErr(w, http.StatusServiceUnavailable, "vSatellite coordinator is not configured")
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "an enrolment token is required")
		return
	}
	if !s.agents.EnrolmentUsable(token) {
		// Deliberately terse: this endpoint is reachable without a portal session.
		writeErr(w, http.StatusUnauthorized, "enrolment token is invalid, used or expired")
		return
	}

	gatewayURL, err := validatedGatewayURL(s.portalBaseURL(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "request host must resolve to an absolute HTTP(S) gateway URL")
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(vsatelliteInstallScript(gatewayURL, token, s.satelliteName)))
}

// handleDispatchVSatelliteCommand queues work for one agent.
// POST /api/v1/vsatellites/{id}/command
func (s *Server) handleDispatchVSatelliteCommand(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		writeErr(w, http.StatusServiceUnavailable, "vSatellite coordinator is not configured")
		return
	}
	role := callerClaims(r).EffectiveRole()
	if !canManageFleet(role) && role != "operator" {
		writeErr(w, http.StatusForbidden, "dispatching agent commands requires an operator role or above")
		return
	}

	id := r.PathValue("id")
	var req struct {
		Type     string `json:"type"`
		Payload  any    `json:"payload"`
		Priority int    `json:"priority"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	switch req.Type {
	case "scan_cidr", "health_check":
	case "deploy_cert":
		writeErr(w, http.StatusBadRequest,
			"deploy_cert is disabled until tenant authorization and authenticated deployment transport are available")
		return
	default:
		writeErr(w, http.StatusBadRequest, "type must be one of scan_cidr or health_check")
		return
	}

	cmd := &vsatellite.Command{
		ID:       fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
		AgentID:  id,
		Type:     req.Type,
		Payload:  vsatellite.CommandPayload(req.Payload),
		Priority: req.Priority,
	}
	if err := s.agents.DispatchCommand(id, cmd); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"command_id": cmd.ID,
		"agent_id":   id,
		"type":       cmd.Type,
		"status":     "queued",
		"note":       "the agent collects queued commands on its next heartbeat",
	})
}

// handleDecommissionVSatellite removes an agent, its credential and its queue.
// DELETE /api/v1/vsatellites/{id}
func (s *Server) handleDecommissionVSatellite(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		writeErr(w, http.StatusServiceUnavailable, "vSatellite coordinator is not configured")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "decommissioning a vSatellite requires an admin role")
		return
	}

	id := r.PathValue("id")
	if err := s.agents.Decommission(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.logger.Info("vSatellite decommissioned via portal",
		zap.String("agent_id", id), zap.String("by", callerName(r)))
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "agent removed; its secret is revoked and queued commands were dropped",
	})
}

// handleRevokeEnrolment cancels an unused enrolment.
// DELETE /api/v1/vsatellites/enrollments/{token}
func (s *Server) handleRevokeEnrolment(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		writeErr(w, http.StatusServiceUnavailable, "vSatellite coordinator is not configured")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "revoking an enrolment requires an admin role")
		return
	}
	if err := s.agents.RevokeEnrolment(r.PathValue("token")); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "enrolment revoked"})
}

// portalBaseURL reconstructs the URL the caller reached this gateway on, so the
// generated install command points somewhere the target host can actually get
// to. nginx terminates in front of the API, so the forwarded headers it sets are
// preferred over the local listener address.
func (s *Server) portalBaseURL(r *http.Request) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func validatedGatewayURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	for _, r := range raw {
		if r <= 0x20 || r == 0x7f {
			return "", fmt.Errorf("gateway URL contains a control character or space")
		}
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse gateway URL: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("gateway URL must be an absolute HTTP(S) URL without credentials, query or fragment")
	}
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
