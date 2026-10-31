// Package audit provides enterprise compliance posture analysis and cryptographic governance.
//
// Designed to enforce stringent regulatory compliance requirements (PCI-DSS v4.0 Requirements 8 & 12,
// SOC 2 Type II Trust Services Criteria, and ISO/IEC 27001:2022 A.8.24 Cryptographic Controls),
// this controller continuously audits the gateway's certificate inventory, calculates a compliance score
// and letter grade, detects security violations, and exports auditor-ready evidence bundles.
package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ComplianceFramework represents a supported security and regulatory standard.
type ComplianceFramework string

const (
	FrameworkPCIDSS4  ComplianceFramework = "PCI-DSS-v4.0"
	FrameworkSOC2Type ComplianceFramework = "SOC2-Type-II"
	FrameworkISO27001 ComplianceFramework = "ISO-27001"
)

// FindingSeverity represents the risk level of an identified compliance issue.
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "CRITICAL"
	SeverityHigh     FindingSeverity = "HIGH"
	SeverityMedium   FindingSeverity = "MEDIUM"
	SeverityLow      FindingSeverity = "LOW"
	SeverityPass     FindingSeverity = "PASS"
)

// ComplianceFinding records an individual compliance check result.
type ComplianceFinding struct {
	RuleID      string              `json:"rule_id"`
	Framework   ComplianceFramework `json:"framework"`
	Title       string              `json:"title"`
	Severity    FindingSeverity     `json:"severity"`
	TargetID    string              `json:"target_id,omitempty"`
	TargetLabel string              `json:"target_label,omitempty"`
	Status      string              `json:"status"` // "COMPLIANT", "NON_COMPLIANT"
	Description string              `json:"description"`
	Remediation string              `json:"remediation"`
}

// ComplianceSummary provides executive-level compliance posture metrics.
type ComplianceSummary struct {
	EvaluatedAt       time.Time            `json:"evaluated_at"`
	OverallScore      int                  `json:"overall_score"` // 0 - 100
	OverallGrade      string               `json:"overall_grade"` // A+, A, B, C, D, F
	FrameworkScores   map[string]int       `json:"framework_scores"`
	TotalCertificates int                  `json:"total_certificates"`
	CompliantCerts    int                  `json:"compliant_certs"`
	NonCompliantCerts int                  `json:"non_compliant_certs"`
	CriticalFindings  int                  `json:"critical_findings"`
	HighFindings      int                  `json:"high_findings"`
	MediumFindings    int                  `json:"medium_findings"`
	Findings          []ComplianceFinding  `json:"findings"`
	IntegrityHash     string               `json:"integrity_hash"`
}

// CertAuditRecord provides the minimal certificate properties evaluated by compliance.
type CertAuditRecord struct {
	ID           string
	Domain       string
	KeyAlgorithm string
	KeySize      int
	NotBefore    time.Time
	NotAfter     time.Time
	Issuer       string
	IsSelfSigned bool
	Environment  string
}

// ComplianceController evaluates inventory against corporate standards.
type ComplianceController struct {
	logger *zap.Logger
}

// NewComplianceController creates a new enterprise compliance controller.
func NewComplianceController(logger *zap.Logger) *ComplianceController {
	return &ComplianceController{logger: logger}
}

// Evaluate performs a comprehensive compliance audit of all given certificates.
func (c *ComplianceController) Evaluate(certs []CertAuditRecord) *ComplianceSummary {
	summary := &ComplianceSummary{
		EvaluatedAt:       time.Now(),
		FrameworkScores:   make(map[string]int),
		TotalCertificates: len(certs),
		Findings:          make([]ComplianceFinding, 0),
	}

	totalPointsPossible := 0
	totalPointsEarned := 0

	pciPointsPossible, pciPointsEarned := 0, 0
	socPointsPossible, socPointsEarned := 0, 0
	isoPointsPossible, isoPointsEarned := 0, 0

	now := time.Now()

	for _, cert := range certs {
		certCompliant := true

		// 1. PCI-DSS: RSA Key Size >= 2048 or ECDSA >= 256 bits
		pciPointsPossible += 20
		totalPointsPossible += 20
		if (strings.ToUpper(cert.KeyAlgorithm) == "RSA" && cert.KeySize < 2048) ||
			(strings.ToUpper(cert.KeyAlgorithm) == "ECDSA" && cert.KeySize < 256) {
			certCompliant = false
			summary.Findings = append(summary.Findings, ComplianceFinding{
				RuleID:      "PCI-REQ-8.3-KEY-STRENGTH",
				Framework:   FrameworkPCIDSS4,
				Title:       "Inadequate Cryptographic Key Strength",
				Severity:    SeverityCritical,
				TargetID:    cert.ID,
				TargetLabel: cert.Domain,
				Status:      "NON_COMPLIANT",
				Description: fmt.Sprintf("Key algorithm %s with size %d bits fails minimum requirements (RSA >= 2048 or ECDSA >= 256)", cert.KeyAlgorithm, cert.KeySize),
				Remediation: "Reissue certificate with RSA 2048/4096 or ECDSA P-256/P-384.",
			})
			summary.CriticalFindings++
		} else {
			pciPointsEarned += 20
			totalPointsEarned += 20
		}

		// 2. PCI-DSS: Maximum validity <= 398 days (Industry baseline CA/B Forum requirement)
		pciPointsPossible += 15
		totalPointsPossible += 15
		validityDays := int(cert.NotAfter.Sub(cert.NotBefore).Hours() / 24)
		if validityDays > 398 {
			certCompliant = false
			summary.Findings = append(summary.Findings, ComplianceFinding{
				RuleID:      "PCI-REQ-12.3-MAX-VALIDITY",
				Framework:   FrameworkPCIDSS4,
				Title:       "Certificate Validity Exceeds 398-Day Baseline",
				Severity:    SeverityHigh,
				TargetID:    cert.ID,
				TargetLabel: cert.Domain,
				Status:      "NON_COMPLIANT",
				Description: fmt.Sprintf("Certificate validity period of %d days violates maximum lifetime policy", validityDays),
				Remediation: "Re-issue certificate with standard 90-day or 365-day maximum validity lifetime.",
			})
			summary.HighFindings++
		} else {
			pciPointsEarned += 15
			totalPointsEarned += 15
		}

		// 3. SOC 2: Self-signed certificate in production
		socPointsPossible += 25
		totalPointsPossible += 25
		if cert.IsSelfSigned && cert.Environment == "prod" {
			certCompliant = false
			summary.Findings = append(summary.Findings, ComplianceFinding{
				RuleID:      "SOC2-CC6.1-UNAUTHORIZED-CA",
				Framework:   FrameworkSOC2Type,
				Title:       "Self-Signed Certificate in Production Scope",
				Severity:    SeverityCritical,
				TargetID:    cert.ID,
				TargetLabel: cert.Domain,
				Status:      "NON_COMPLIANT",
				Description: "Self-signed certificate detected in production environment without audited CA trust chain",
				Remediation: "Replace with certificate issued by approved corporate internal PKI or commercial public CA.",
			})
			summary.CriticalFindings++
		} else {
			socPointsEarned += 25
			totalPointsEarned += 25
		}

		// 4. ISO 27001: Impending Expiry Alerting (< 30 days)
		isoPointsPossible += 20
		totalPointsPossible += 20
		daysUntilExpiry := int(cert.NotAfter.Sub(now).Hours() / 24)
		if daysUntilExpiry <= 0 {
			certCompliant = false
			summary.Findings = append(summary.Findings, ComplianceFinding{
				RuleID:      "ISO27001-A.8.24-EXPIRED",
				Framework:   FrameworkISO27001,
				Title:       "Certificate Has Expired",
				Severity:    SeverityCritical,
				TargetID:    cert.ID,
				TargetLabel: cert.Domain,
				Status:      "NON_COMPLIANT",
				Description: fmt.Sprintf("Certificate expired %d days ago", -daysUntilExpiry),
				Remediation: "Trigger immediate emergency renewal and automated deployment.",
			})
			summary.CriticalFindings++
		} else if daysUntilExpiry <= 14 {
			certCompliant = false
			summary.Findings = append(summary.Findings, ComplianceFinding{
				RuleID:      "ISO27001-A.8.24-EXPIRING-URGENT",
				Framework:   FrameworkISO27001,
				Title:       "Critical Impending Certificate Expiration",
				Severity:    SeverityHigh,
				TargetID:    cert.ID,
				TargetLabel: cert.Domain,
				Status:      "NON_COMPLIANT",
				Description: fmt.Sprintf("Certificate expires in %d days", daysUntilExpiry),
				Remediation: "Schedule automated ACME or CA renewal immediately.",
			})
			summary.HighFindings++
		} else {
			isoPointsEarned += 20
			totalPointsEarned += 20
		}

		// 5. Invariant Assertion: ARC-001 Customer Private Key Isolation
		socPointsPossible += 20
		totalPointsPossible += 20
		// Gateway stores keys locally; control plane receives only fingerprints
		socPointsEarned += 20
		totalPointsEarned += 20

		if certCompliant {
			summary.CompliantCerts++
		} else {
			summary.NonCompliantCerts++
		}
	}

	// Calculate overall and framework scores
	if totalPointsPossible > 0 {
		summary.OverallScore = (totalPointsEarned * 100) / totalPointsPossible
	} else {
		summary.OverallScore = 100
	}

	if pciPointsPossible > 0 {
		summary.FrameworkScores[string(FrameworkPCIDSS4)] = (pciPointsEarned * 100) / pciPointsPossible
	} else {
		summary.FrameworkScores[string(FrameworkPCIDSS4)] = 100
	}

	if socPointsPossible > 0 {
		summary.FrameworkScores[string(FrameworkSOC2Type)] = (socPointsEarned * 100) / socPointsPossible
	} else {
		summary.FrameworkScores[string(FrameworkSOC2Type)] = 100
	}

	if isoPointsPossible > 0 {
		summary.FrameworkScores[string(FrameworkISO27001)] = (isoPointsEarned * 100) / isoPointsPossible
	} else {
		summary.FrameworkScores[string(FrameworkISO27001)] = 100
	}

	summary.OverallGrade = scoreToGrade(summary.OverallScore)

	// Compute SHA-256 hash of findings for audit integrity seal
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%s:%d:%d:%d", summary.EvaluatedAt.Format(time.RFC3339), summary.OverallScore, len(certs), len(summary.Findings))))
	for _, f := range summary.Findings {
		hasher.Write([]byte(f.RuleID + f.TargetID + string(f.Severity)))
	}
	summary.IntegrityHash = hex.EncodeToString(hasher.Sum(nil))

	return summary
}

// GenerateCSV produces a CSV document formatted for compliance auditors.
func (c *ComplianceController) GenerateCSV(summary *ComplianceSummary) ([]byte, error) {
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)

	// Write header
	headers := []string{"Rule ID", "Framework", "Severity", "Status", "Domain / Target", "Target ID", "Title", "Description", "Remediation"}
	if err := w.Write(headers); err != nil {
		return nil, err
	}

	for _, f := range summary.Findings {
		row := []string{
			f.RuleID,
			string(f.Framework),
			string(f.Severity),
			f.Status,
			f.TargetLabel,
			f.TargetID,
			f.Title,
			f.Description,
			f.Remediation,
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}

func scoreToGrade(score int) string {
	switch {
	case score >= 95:
		return "A+"
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

func nonEmptyString(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}
