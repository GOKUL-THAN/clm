package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/deployment/sync"
	"go.uber.org/zap"
)

// handleRotationStatus returns current auto-rotation posture and queue.
// GET /api/v1/rotation/status
func (s *Server) handleRotationStatus(w http.ResponseWriter, r *http.Request) {
	if s.rotationController == nil {
		s.rotationController = sync.NewRotationController(sync.RotationPolicyConfig{
			RotationWindowDays: 30,
			WarningWindowDays:  60,
			AutoRollbackOnFail: true,
		}, s.logger)
	}

	certs := s.inventory.ListCertificates()
	candidates := make([]sync.RotationCandidate, 0, len(certs))
	for _, c := range certs {
		daysLeft := int(c.NotAfter.Sub(s.startedAt).Hours() / 24)
		candidates = append(candidates, sync.RotationCandidate{
			ID:           c.ID,
			Domain:       c.Domain,
			Environment:  c.Environment,
			DaysLeft:     daysLeft,
			SerialNumber: c.Fingerprint,
			AutoRenew:    true,
		})
	}

	_, summary := s.rotationController.EvaluateEstate(candidates)
	writeJSON(w, http.StatusOK, summary)
}

// handleRotationExecute triggers rotation for an eligible certificate.
// POST /api/v1/rotation/execute/{id}
func (s *Server) handleRotationExecute(w http.ResponseWriter, r *http.Request) {
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) && role != "operator" {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot trigger certificate rotation")
		return
	}

	certID := r.PathValue("id")
	cert, ok := s.inventory.GetCertificate(certID)
	if !ok {
		writeErr(w, http.StatusNotFound, "certificate "+certID+" not found")
		return
	}

	if s.rotationController == nil {
		s.rotationController = sync.NewRotationController(sync.RotationPolicyConfig{
			RotationWindowDays: 30,
			WarningWindowDays:  60,
			AutoRollbackOnFail: true,
		}, s.logger)
	}

	daysLeft := int(cert.NotAfter.Sub(s.startedAt).Hours() / 24)
	cand := sync.RotationCandidate{
		ID:           cert.ID,
		Domain:       cert.Domain,
		Environment:  cert.Environment,
		DaysLeft:     daysLeft,
		SerialNumber: cert.Fingerprint,
		AutoRenew:    true,
	}

	rec, err := s.rotationController.ExecuteRotation(
		context.Background(),
		cand,
		func() error {
			// Mock simulated zero-downtime deployment staging
			return nil
		},
		func() bool {
			// Simulated TLS probe verification
			return true
		},
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.recordAudit(r, "certificate.auto_rotation", "certificate", cert.ID, cert.Domain, audit.ResultSuccess,
		fmt.Sprintf("Rotated certificate for %s (Serial: %s)", cert.Domain, rec.NewSerial))

	writeJSON(w, http.StatusOK, rec)
}
