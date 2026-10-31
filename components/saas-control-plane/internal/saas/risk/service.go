// Package risk provides corporate cryptographic risk register and posture quantification.
//
// Patterned after enterprise risk management frameworks (Deloitte, JPMorgan Chase, NIST IR 8547),
// this controller calculates quantitative risk scores, identifies Post-Quantum Cryptography (PQC)
// transition exposures, flags single-CA dependency risks (SPOF), and measures wildcard blast radiuses.
package risk

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type RiskCategory string

const (
	RiskPQCQuantumExposed   RiskCategory = "PQC_VULNERABILITY"
	RiskSingleCAPointOfFail RiskCategory = "SINGLE_CA_DEPENDENCY"
	RiskWildcardBlastRadius RiskCategory = "WILDCARD_BLAST_RADIUS"
	RiskImminentExpiry      RiskCategory = "IMMINENT_EXPIRATION_OUTAGE"
	RiskWeakCipherOrKey     RiskCategory = "WEAK_CRYPTOGRAPHIC_PRIMITIVE"
)

// CryptographicRiskItem represents an evaluated risk vector in the register.
type CryptographicRiskItem struct {
	ID          string       `json:"id"`
	Category    RiskCategory `json:"category"`
	Severity    string       `json:"severity"` // CRITICAL, HIGH, MEDIUM, LOW
	Score       int          `json:"score"`    // 1 - 100
	Affected    string       `json:"affected"` // Domain, CA account, or Algorithm
	Description string       `json:"description"`
	Remediation string       `json:"remediation"`
}

// RiskPostureSummary provides executive risk metrics.
type RiskPostureSummary struct {
	EvaluatedAt         time.Time                `json:"evaluated_at"`
	OverallRiskScore    int                      `json:"overall_risk_score"` // 0 (pesticide clean) to 100 (critical risk)
	RiskRating          string                   `json:"risk_rating"`        // LOW, MODERATE, HIGH, SEVERE
	PQCTransitionReady  bool                     `json:"pqc_transition_ready"`
	TotalIdentifiedRisks int                     `json:"total_identified_risks"`
	CriticalRisks       int                      `json:"critical_risks"`
	HighRisks           int                      `json:"high_risks"`
	MediumRisks         int                      `json:"medium_risks"`
	Items               []*CryptographicRiskItem `json:"items"`
}

// CertInput represents minimal certificate data for risk evaluation.
type CertInput struct {
	ID           string
	Domain       string
	KeyAlgorithm string
	KeySize      int
	DaysLeft     int
	Issuer       string
	IsWildcard   bool
}

// RiskController evaluates enterprise cryptographic exposure.
type RiskController struct {
	mu     sync.RWMutex
	logger *zap.Logger
}

// NewRiskController creates a new risk controller.
func NewRiskController(logger *zap.Logger) *RiskController {
	return &RiskController{logger: logger}
}

// EvaluateRisk analyzes a tenant's certificate fleet and produces an executive risk report.
func (c *RiskController) EvaluateRisk(ctx context.Context, tenantID uuid.UUID, certs []CertInput) *RiskPostureSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	summary := &RiskPostureSummary{
		EvaluatedAt: time.Now(),
		Items:       make([]*CryptographicRiskItem, 0),
	}

	if len(certs) == 0 {
		summary.OverallRiskScore = 0
		summary.RiskRating = "LOW"
		summary.PQCTransitionReady = true
		return summary
	}

	caCounts := make(map[string]int)
	pqcVulnerableCount := 0
	totalScore := 0

	for _, cert := range certs {
		caCounts[cert.Issuer]++

		// 1. Post-Quantum Cryptography Risk (All standard RSA and ECC are vulnerable to Shor's algorithm)
		if cert.KeyAlgorithm == "RSA" || cert.KeyAlgorithm == "ECDSA" {
			pqcVulnerableCount++
			if cert.KeySize < 2048 {
				totalScore += 25
				summary.Items = append(summary.Items, &CryptographicRiskItem{
					ID:          "RISK-WEAK-" + cert.ID,
					Category:    RiskWeakCipherOrKey,
					Severity:    "CRITICAL",
					Score:       90,
					Affected:    cert.Domain,
					Description: "RSA key length below minimum standard (2048-bit)",
					Remediation: "Re-issue certificate with RSA-4096 or ECDSA-P384.",
				})
			}
		}

		// 2. Wildcard Blast Radius Risk
		if cert.IsWildcard {
			totalScore += 15
			summary.Items = append(summary.Items, &CryptographicRiskItem{
				ID:          "RISK-WILD-" + cert.ID,
				Category:    RiskWildcardBlastRadius,
				Severity:    "HIGH",
				Score:       70,
				Affected:    cert.Domain,
				Description: "Broad wildcard certificate exposes all subdomains to single private key compromise",
				Remediation: "Segregate critical APIs into individual domain-scoped certificates.",
			})
		}

		// 3. Imminent Outage Risk (< 7 days)
		if cert.DaysLeft <= 7 {
			totalScore += 35
			summary.Items = append(summary.Items, &CryptographicRiskItem{
				ID:          "RISK-EXP-" + cert.ID,
				Category:    RiskImminentExpiry,
				Severity:    "CRITICAL",
				Score:       95,
				Affected:    cert.Domain,
				Description: "Imminent expiration without confirmed deployment",
				Remediation: "Trigger immediate automated ACME rotation pipeline.",
			})
		}
	}

	// 4. Single CA Dependency Risk (SPOF)
	for issuer, count := range caCounts {
		ratio := float64(count) / float64(len(certs))
		if ratio >= 0.90 && len(certs) >= 5 {
			totalScore += 20
			summary.Items = append(summary.Items, &CryptographicRiskItem{
				ID:          "RISK-SPOF-CA",
				Category:    RiskSingleCAPointOfFail,
				Severity:    "HIGH",
				Score:       65,
				Affected:    issuer,
				Description: "Over 90% of certificates depend on a single CA provider",
				Remediation: "Establish dual-vendor strategy with secondary commercial CA or ACME provider.",
			})
		}
	}

	// Calculate overall score (normalized 0 - 100)
	if len(summary.Items) > 0 {
		normalizedScore := totalScore / len(summary.Items)
		if normalizedScore > 100 {
			normalizedScore = 100
		}
		summary.OverallRiskScore = normalizedScore
	} else {
		summary.OverallRiskScore = 10
	}

	// Tally severity
	for _, it := range summary.Items {
		switch it.Severity {
		case "CRITICAL":
			summary.CriticalRisks++
		case "HIGH":
			summary.HighRisks++
		case "MEDIUM":
			summary.MediumRisks++
		}
	}
	summary.TotalIdentifiedRisks = len(summary.Items)

	// Rating
	if summary.OverallRiskScore >= 75 {
		summary.RiskRating = "SEVERE"
	} else if summary.OverallRiskScore >= 50 {
		summary.RiskRating = "HIGH"
	} else if summary.OverallRiskScore >= 25 {
		summary.RiskRating = "MODERATE"
	} else {
		summary.RiskRating = "LOW"
	}

	summary.PQCTransitionReady = (pqcVulnerableCount == 0)

	c.logger.Info("cryptographic risk evaluation completed",
		zap.String("tenant_id", tenantID.String()),
		zap.Int("score", summary.OverallRiskScore),
		zap.String("rating", summary.RiskRating),
	)

	return summary
}
