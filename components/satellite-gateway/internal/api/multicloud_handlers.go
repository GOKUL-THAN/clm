package api

import (
	"net/http"

	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/deployment/sync"
	"go.uber.org/zap"
)

// handleMultiCloudSyncStatus returns the latest synchronization status and configuration.
func (s *Server) handleMultiCloudSyncStatus(w http.ResponseWriter, r *http.Request) {
	if s.syncController == nil {
		var defaultCfg sync.SyncTargetConfig
		defaultCfg.AWS.Enabled = true
		defaultCfg.AWS.Regions = []string{"ap-south-1", "us-east-1"}
		defaultCfg.AWS.TagManagedBy = "CLM"
		defaultCfg.AWS.ComplianceScope = "PCI-DSS-v4"
		defaultCfg.GCP.Enabled = true
		defaultCfg.GCP.Locations = []string{"global"}
		s.syncController = sync.NewMultiCloudSyncController(defaultCfg, s.logger)
	}

	result, cfg := s.syncController.GetStatus()
	writeJSON(w, http.StatusOK, map[string]any{
		"config":      cfg,
		"last_result": result,
	})
}

// handleMultiCloudSyncRun triggers an on-demand multi-cloud certificate sync.
func (s *Server) handleMultiCloudSyncRun(w http.ResponseWriter, r *http.Request) {
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) && role != "operator" {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot trigger multi-cloud synchronization")
		return
	}

	dryRun := r.URL.Query().Get("dry_run") == "true"

	if s.syncController == nil {
		var defaultCfg sync.SyncTargetConfig
		defaultCfg.AWS.Enabled = true
		defaultCfg.AWS.Regions = []string{"ap-south-1", "us-east-1"}
		defaultCfg.AWS.TagManagedBy = "CLM"
		defaultCfg.AWS.ComplianceScope = "PCI-DSS-v4"
		defaultCfg.GCP.Enabled = true
		defaultCfg.GCP.Locations = []string{"global"}
		s.syncController = sync.NewMultiCloudSyncController(defaultCfg, s.logger)
	}

	// Gather inventory certificates for sync
	allCerts := s.inventory.ListCertificates()
	descriptors := make([]sync.CertificateDescriptor, 0, len(allCerts))
	for _, c := range allCerts {
		descriptors = append(descriptors, sync.CertificateDescriptor{
			ID:       c.ID,
			Domain:   c.Domain,
			SANs:     c.SANs,
			CertPEM:  c.CertPEM,
			NotAfter: c.NotAfter,
		})
	}

	result, err := s.syncController.SyncNow(r.Context(), descriptors, dryRun)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "multi-cloud synchronization failed: "+err.Error())
		return
	}

	s.logger.Info("multi-cloud synchronization executed",
		zap.String("job_id", result.JobID),
		zap.Bool("dry_run", dryRun),
		zap.Int("synced", result.SyncedCerts),
		zap.Int("drifted", result.DriftedCerts),
	)

	s.recordAudit(r, "multicloud.sync", "synchronizer", result.JobID, "all-clouds", audit.ResultSuccess,
		nonEmpty(r.URL.RawQuery, "multi-cloud SSL reconciliation executed"))

	writeJSON(w, http.StatusOK, result)
}

// handleMultiCloudSyncHistory returns past sync executions.
func (s *Server) handleMultiCloudSyncHistory(w http.ResponseWriter, r *http.Request) {
	if s.syncController == nil {
		writeJSON(w, http.StatusOK, map[string]any{"history": []any{}, "total": 0})
		return
	}

	history := s.syncController.GetHistory()
	writeJSON(w, http.StatusOK, map[string]any{
		"history": history,
		"total":   len(history),
	})
}
