package vsatellite

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// API is the agent-facing side of the coordinator: the endpoints a vSatellite
// agent calls to join the fleet, report in, and collect work.
//
// Every route here authenticates. Enrolment requires a single-use token issued
// by an operator through the portal; everything afterwards requires the
// per-agent secret handed out in exchange for that token. This used to be wide
// open — registration accepted any payload, a heartbeat with an unknown agent ID
// silently auto-registered it, and an agent listing was published without
// credentials. Anyone able to reach the port could join the fleet, enumerate it,
// and drain another agent's queued commands by heartbeating with its ID.
type API struct {
	coordinator *Coordinator
	logger      *zap.Logger
}

// NewAPI creates a new vSatellite API handler.
func NewAPI(coord *Coordinator, logger *zap.Logger) *API {
	return &API{coordinator: coord, logger: logger}
}

// RegisterRoutes adds vSatellite routes to an HTTP mux.
//
// Note there is deliberately no unauthenticated fleet listing here. The portal
// API on :8080 exposes the fleet behind a JWT check, which is the right place
// for it.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/vsatellite/enroll", a.handleEnroll)
	mux.HandleFunc("/vsatellite/register", a.handleEnroll) // retained name, same contract
	mux.HandleFunc("/vsatellite/heartbeat", a.handleHeartbeat)
	mux.HandleFunc("/vsatellite/result", a.handleResult)
}

// bearer extracts a bearer token, falling back to the header agents use for the
// agent secret.
func bearer(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return strings.TrimSpace(r.Header.Get("X-Agent-Secret"))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// handleEnroll exchanges an enrolment token for an agent identity and secret.
func (a *API) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var req struct {
		Token   string `json:"token"`
		AgentID string `json:"agent_id"`
		Host    string `json:"host"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// The token may arrive in the body or as a bearer header; agents installed by
	// the generated script use the header.
	token := req.Token
	if token == "" {
		token = bearer(r)
	}
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "an enrolment token is required; create one in the gateway portal " +
				"under vSatellites",
		})
		return
	}

	agent, secret, err := a.coordinator.ConsumeEnrolment(token, req.AgentID, req.Host, req.Version)
	if err != nil {
		a.logger.Warn("vSatellite enrolment rejected",
			zap.String("remote", r.RemoteAddr), zap.Error(err))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "enrolled",
		"agent_id":     agent.ID,
		"agent_secret": secret,
		"name":         agent.Name,
		"cidrs":        agent.CIDRs,
		"capabilities": agent.Capabilities,
		"note":         "send agent_secret as 'X-Agent-Secret' on heartbeat and result calls",
	})
}

func (a *API) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var hb struct {
		AgentID   string `json:"agent_id"`
		CertCount int    `json:"cert_count"`
		Version   string `json:"version"`
	}
	if err := json.Unmarshal(body, &hb); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// No auto-registration. An unknown agent ID with no valid secret is refused
	// rather than created, which is what previously allowed an uninvited host to
	// join and to collect commands queued for a legitimate agent.
	if !a.coordinator.AuthenticateAgent(hb.AgentID, bearer(r)) {
		a.logger.Warn("vSatellite heartbeat rejected",
			zap.String("agent_id", hb.AgentID), zap.String("remote", r.RemoteAddr))
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "unknown agent or invalid agent secret; re-enrol this agent",
		})
		return
	}

	cmds, err := a.coordinator.Heartbeat(hb.AgentID, hb.CertCount, hb.Version)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "commands": cmds})
}

func (a *API) handleResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var result struct {
		AgentID string `json:"agent_id"`
		CmdID   string `json:"cmd_id"`
		Status  string `json:"status"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if !a.coordinator.AuthenticateAgent(result.AgentID, bearer(r)) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "unknown agent or invalid agent secret",
		})
		return
	}

	_ = a.coordinator.ReportResult(result.AgentID, result.CmdID, result.Status, result.Result)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
