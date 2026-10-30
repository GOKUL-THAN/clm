package api

import (
	"net/http"

	"github.com/m2p/clm/internal/satellite/policy"
)

// Certificate policy engine: enforceable rules checked before an order is
// placed, not only scored afterwards.
//
// See internal/policy/policy.go for why this is a separate mechanism from the
// existing compliance report. handleCAIssue calls Evaluate before it contacts
// any CA; a violation is a 400 naming every rule that failed, so a commercial
// order is never placed for a request a policy would have rejected anyway.

// handleListPolicies returns every configured policy.
// GET /api/v1/policies
func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	if s.policies == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"policies": []any{},
			"reason":   "the policy engine is unavailable: set state.dir so policies survive a restart",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"policies": s.policies.List()})
}

// handlePutPolicy creates or updates a policy.
// POST /api/v1/policies
func (s *Server) handlePutPolicy(w http.ResponseWriter, r *http.Request) {
	if s.policies == nil {
		writeErr(w, http.StatusServiceUnavailable,
			"the policy engine is unavailable: set state.dir so policies survive a restart")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden,
			"changing certificate policy requires an admin or cert_manager role")
		return
	}

	var p policy.Policy
	if err := decodeBody(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	saved, err := s.policies.Put(&p, callerName(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	action := "policy.create"
	if p.ID != "" {
		action = "policy.update"
	}
	s.recordAudit(r, action, "policy", saved.ID, saved.Name, "success",
		"environment="+saved.Environment)
	writeJSON(w, http.StatusOK, map[string]any{"policy": saved})
}

// handleDeletePolicy removes a policy.
// DELETE /api/v1/policies/{id}
func (s *Server) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	if s.policies == nil {
		writeErr(w, http.StatusServiceUnavailable, "the policy engine is unavailable")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden,
			"changing certificate policy requires an admin or cert_manager role")
		return
	}

	id := r.PathValue("id")
	if err := s.policies.Delete(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.recordAudit(r, "policy.delete", "policy", id, "", "success", "")
	writeJSON(w, http.StatusOK, map[string]any{"removed": id})
}

// handleEvaluatePolicy dry-runs a hypothetical issuance request against every
// configured policy without ordering anything, so a form can show what would
// be refused before an operator fills in the rest and submits for real.
// POST /api/v1/policies/evaluate
func (s *Server) handleEvaluatePolicy(w http.ResponseWriter, r *http.Request) {
	if s.policies == nil {
		writeJSON(w, http.StatusOK, map[string]any{"violations": []any{}, "allowed": true,
			"note": "the policy engine is unavailable, so nothing is enforced"})
		return
	}

	var req struct {
		Domains       []string `json:"domains"`
		Domain        string   `json:"domain"`
		CertType      string   `json:"cert_type"`
		Environment   string   `json:"environment"`
		CAAccount     string   `json:"ca_account"`
		ValidityYears int      `json:"validity_years"`
		KeyType       string   `json:"key_type"`
		KeyBits       int      `json:"key_bits"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	domains := collectDomains(req.Domain, req.Domains)
	violations := s.policies.Evaluate(policy.Request{
		Domains: domains, CertType: req.CertType, Environment: req.Environment,
		CAAccount: req.CAAccount, ValidityYears: req.ValidityYears,
		KeyType: req.KeyType, KeyBits: req.KeyBits,
		// A dry run cannot know whether an approval will exist by the time a real
		// request is submitted, so it is evaluated as not yet decided — the
		// stricter assumption, which is the useful one to preview.
		ApprovalDecided: false,
	})
	if violations == nil {
		violations = []policy.Violation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"violations": violations,
		"allowed":    len(violations) == 0,
	})
}
