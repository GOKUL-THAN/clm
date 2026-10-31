package api

import (
	"fmt"
	"net/http"

	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/deployment/migration"
	"go.uber.org/zap"
)

// handleMigrationValidate checks a certificate bundle without importing it.
// POST /api/v1/migration/validate
func (s *Server) handleMigrationValidate(w http.ResponseWriter, r *http.Request) {
	if s.migrationController == nil {
		s.migrationController = migration.NewBulkMigrationController(s.logger)
	}

	var req migration.IngestionInput
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	req.DryRun = true

	batch, err := s.migrationController.IngestPayload(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, batch)
}

// handleMigrationImport imports a certificate bundle into inventory and queues cloud sync.
// POST /api/v1/migration/import
func (s *Server) handleMigrationImport(w http.ResponseWriter, r *http.Request) {
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) && role != "operator" {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot execute bulk migration import")
		return
	}

	if s.migrationController == nil {
		s.migrationController = migration.NewBulkMigrationController(s.logger)
	}

	var req migration.IngestionInput
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	req.DryRun = false

	batch, err := s.migrationController.IngestPayload(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Add valid certificates to Inventory
	importedCount := 0
	for _, cert := range batch.Certificates {
		if cert.KeyMatchPassed && cert.DaysRemaining > 0 {
			env := req.Environment
			if env == "" {
				env = "prod"
			}
			s.inventory.AddCertificate(&Certificate{
				ID:           cert.ID,
				Domain:       cert.Domain,
				SANs:         cert.SANs,
				Issuer:       cert.Issuer,
				NotBefore:    cert.NotBefore,
				NotAfter:     cert.NotAfter,
				KeyAlgorithm: cert.KeyAlgorithm,
				KeySize:      cert.KeySize,
				Fingerprint:  cert.Fingerprint,
				CertPEM:      cert.CertPEM,
				Status:       "active",
				Environment:  env,
			})
			importedCount++
		}
	}

	s.recordAudit(r, "migration.bulk_import", "migration", batch.BatchID, batch.Source, audit.ResultSuccess,
		fmt.Sprintf("Imported %d certificates (Total: %d, Source: %s)", importedCount, batch.TotalDiscovered, batch.Source))

	writeJSON(w, http.StatusOK, map[string]any{
		"batch":          batch,
		"imported_count": importedCount,
	})
}

// handleMigrationHistory returns the history of bulk migration operations.
// GET /api/v1/migration/history
func (s *Server) handleMigrationHistory(w http.ResponseWriter, r *http.Request) {
	if s.migrationController == nil {
		s.migrationController = migration.NewBulkMigrationController(s.logger)
	}

	history := s.migrationController.ListHistory()
	writeJSON(w, http.StatusOK, map[string]any{
		"history": history,
		"count":   len(history),
	})
}

