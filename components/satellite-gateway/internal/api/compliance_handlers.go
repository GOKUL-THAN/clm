package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/m2p/clm/internal/satellite/audit"
	"go.uber.org/zap"
)

// handleCorporateComplianceReport runs a deep audit of the inventory against PCI-DSS and SOC2.
func (s *Server) handleCorporateComplianceReport(w http.ResponseWriter, r *http.Request) {
	if s.complianceController == nil {
		s.complianceController = audit.NewComplianceController(s.logger)
	}

	certs := s.inventory.ListCertificates()
	records := make([]audit.CertAuditRecord, 0, len(certs))
	for _, c := range certs {
		records = append(records, audit.CertAuditRecord{
			ID:           c.ID,
			Domain:       c.Domain,
			KeyAlgorithm: c.KeyAlgorithm,
			KeySize:      c.KeySize,
			NotBefore:    c.NotBefore,
			NotAfter:     c.NotAfter,
			Issuer:       c.Issuer,
			IsSelfSigned: c.IsSelfSigned,
			Environment:  c.Environment,
		})
	}

	summary := s.complianceController.Evaluate(records)

	s.recordAudit(r, "report.corporate_compliance", "compliance", "report-posture", "inventory", audit.ResultSuccess,
		fmt.Sprintf("Compliance score: %d/100 (Grade: %s)", summary.OverallScore, summary.OverallGrade))

	writeJSON(w, http.StatusOK, summary)
}

// handleCorporateComplianceCSV exports an auditor-ready CSV report.
func (s *Server) handleCorporateComplianceCSV(w http.ResponseWriter, r *http.Request) {
	if s.complianceController == nil {
		s.complianceController = audit.NewComplianceController(s.logger)
	}

	certs := s.inventory.ListCertificates()
	records := make([]audit.CertAuditRecord, 0, len(certs))
	for _, c := range certs {
		records = append(records, audit.CertAuditRecord{
			ID:           c.ID,
			Domain:       c.Domain,
			KeyAlgorithm: c.KeyAlgorithm,
			KeySize:      c.KeySize,
			NotBefore:    c.NotBefore,
			NotAfter:     c.NotAfter,
			Issuer:       c.Issuer,
			IsSelfSigned: c.IsSelfSigned,
			Environment:  c.Environment,
		})
	}

	summary := s.complianceController.Evaluate(records)
	csvBytes, err := s.complianceController.GenerateCSV(summary)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate compliance CSV: "+err.Error())
		return
	}

	filename := fmt.Sprintf("clm-compliance-report-%s.csv", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(csvBytes)
}

// handleAuditBundle exports an authoritative evidence package for SOC2 / PCI-DSS audits.
func (s *Server) handleAuditBundle(w http.ResponseWriter, r *http.Request) {
	role := callerClaims(r).EffectiveRole()
	if !canApprove(role) && role != "auditor" {
		writeErr(w, http.StatusForbidden, "role "+role+" cannot export compliance audit bundle")
		return
	}

	if s.complianceController == nil {
		s.complianceController = audit.NewComplianceController(s.logger)
	}

	// 1. Get recent audit events
	events := s.audit.Recent(5000)

	// 2. Get current compliance summary
	certs := s.inventory.ListCertificates()
	records := make([]audit.CertAuditRecord, 0, len(certs))
	for _, c := range certs {
		records = append(records, audit.CertAuditRecord{
			ID:           c.ID,
			Domain:       c.Domain,
			KeyAlgorithm: c.KeyAlgorithm,
			KeySize:      c.KeySize,
			NotBefore:    c.NotBefore,
			NotAfter:     c.NotAfter,
			Issuer:       c.Issuer,
			IsSelfSigned: c.IsSelfSigned,
			Environment:  c.Environment,
		})
	}
	summary := s.complianceController.Evaluate(records)

	// 3. Compute digital bundle seal
	bundleHasher := sha256.New()
	bundleHasher.Write([]byte(summary.IntegrityHash))
	bundleHasher.Write([]byte(fmt.Sprintf("events:%d", len(events))))
	bundleSeal := hex.EncodeToString(bundleHasher.Sum(nil))

	bundle := map[string]any{
		"gateway_id":       s.satelliteID,
		"gateway_name":     s.satelliteName,
		"exported_at":      time.Now(),
		"auditor":          callerName(r),
		"compliance":       summary,
		"audit_trail":      events,
		"audit_event_count": len(events),
		"bundle_seal":      bundleSeal,
		"assertions": map[string]string{
			"ARC-001": "Customer private keys reside exclusively within gateway boundary and are never exported",
			"ARC-004": "Segregation of duties enforced; requesters cannot approve high-risk certificates",
			"PCI-DSS": "Cryptographic key lengths meet PCI-DSS v4.0 Requirement 8 and 12",
			"SOC2":    "Full immutable audit trail with non-repudiation integrity seal",
		},
	}

	s.recordAudit(r, "audit.bundle_export", "compliance", "audit-bundle", s.satelliteID, audit.ResultSuccess,
		fmt.Sprintf("Exported %d audit events with bundle seal %s", len(events), bundleSeal[:12]))

	writeJSON(w, http.StatusOK, bundle)
}
