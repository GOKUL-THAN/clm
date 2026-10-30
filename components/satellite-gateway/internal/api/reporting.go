package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Reporting and compliance.
//
// The lifecycle endpoint answers "what should I do next". This answers "can I
// show someone the state of the estate" — the auditor question. It is the same
// inventory viewed as a scored posture plus an export, because a compliance
// conversation ends in a spreadsheet whether or not the tool provides one.

// ComplianceRule is one checked control.
type ComplianceRule struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"` // high, medium, low
	Checked     int      `json:"checked"`
	Passed      int      `json:"passed"`
	Failed      int      `json:"failed"`
	// FailingDomains is capped; the export carries the full detail.
	FailingDomains []string `json:"failing_domains,omitempty"`
	Status         string   `json:"status"` // pass, fail, not_applicable
}

// ComplianceReport is the scored posture.
type ComplianceReport struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Gateway     string           `json:"gateway"`
	Total       int              `json:"total"`
	// Assessed counts live certificates; Excluded counts revoked records and
	// failed issuance attempts, which are held for history but not scored.
	Assessed int    `json:"assessed"`
	Excluded int    `json:"excluded"`
	Score    int    `json:"score"` // 0-100
	Grade    string `json:"grade"`
	Rules       []ComplianceRule `json:"rules"`
	Managed     map[string]int   `json:"managed"`
	ByEnv       []NameCount      `json:"by_environment"`
	ByIssuer    []NameCount      `json:"by_issuer"`
	BySource    []NameCount      `json:"by_source"`
	KeyStrength []NameCount      `json:"key_strength"`
	Summary     string           `json:"summary"`
	Caveats     []string         `json:"caveats"`
}

// NameCount is a labelled tally.
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

const maxFailingListed = 10

// buildComplianceReport scores the inventory against a fixed rule set.
//
// The rules are deliberately ones the gateway can actually verify from what it
// holds. Inventing a control that cannot be evidenced would make the score look
// better without meaning anything.
func buildComplianceReport(gateway string, certs []*Certificate) ComplianceReport {
	rep := ComplianceReport{
		GeneratedAt: time.Now(),
		Gateway:     gateway,
		Total:       len(certs),
		Managed:     map[string]int{},
		Rules:       []ComplianceRule{},
		Caveats:     []string{},
	}

	envs := map[string]int{}
	issuers := map[string]int{}
	sources := map[string]int{}
	keys := map[string]int{}

	rules := []*ComplianceRule{
		{ID: "CLM-01", Title: "No expired certificates in service", Severity: "high",
			Description: "An expired certificate breaks every client that validates it."},
		{ID: "CLM-02", Title: "No certificate expiring within 30 days without a renewal path", Severity: "high",
			Description: "A certificate near expiry that the gateway cannot renew needs manual action booked now."},
		{ID: "CLM-03", Title: "No self-signed certificates", Severity: "medium",
			Description: "A self-signed certificate gives clients no chain of trust."},
		{ID: "CLM-04", Title: "Keys are 2048-bit RSA equivalent or stronger", Severity: "high",
			Description: "Keys below 2048 bits are not considered secure."},
		{ID: "CLM-05", Title: "Validity does not exceed 398 days", Severity: "medium",
			Description: "Public CAs have not issued longer than 398 days since 2020."},
		{ID: "CLM-06", Title: "Issuer is identifiable", Severity: "low",
			Description: "An unidentifiable issuer cannot be assessed for trust or for CA concentration."},
	}
	byID := map[string]*ComplianceRule{}
	for _, r := range rules {
		byID[r.ID] = r
	}

	fail := func(id, domain string) {
		r := byID[id]
		r.Failed++
		if len(r.FailingDomains) < maxFailingListed {
			r.FailingDomains = append(r.FailingDomains, domain)
		}
	}

	assessed := 0
	skipped := 0

	for _, c := range certs {
		// A failed issuance attempt is a record of something that never existed,
		// and a revoked certificate has already been dealt with. Scoring either as
		// a live certificate would misstate the posture — a failed attempt with no
		// issuer would otherwise fail the "issuer identifiable" control.
		if c.Status == "failed" || !c.RevokedAt.IsZero() {
			skipped++
			continue
		}
		assessed++

		label := c.Domain
		if label == "" {
			label = c.Host
		}

		if c.ManagedBy != "" {
			rep.Managed[c.ManagedBy]++
		} else {
			rep.Managed["observed_only"]++
		}

		envs[nonEmpty(c.Environment, "unknown")]++
		issuers[nonEmpty(c.Issuer, "(unknown)")]++
		sources[nonEmpty(c.Source, "unknown")]++

		switch {
		case c.KeySize == 0:
			keys["unknown"]++
		case c.KeySize < 2048:
			keys["weak (<2048)"]++
		case c.KeySize < 4096:
			keys["2048-4095"]++
		default:
			keys["4096+"]++
		}

		// CLM-01 expired
		byID["CLM-01"].Checked++
		if !c.NotAfter.IsZero() && time.Until(c.NotAfter) < 0 {
			fail("CLM-01", label)
		}

		// CLM-02 near expiry with no automated renewal
		byID["CLM-02"].Checked++
		if !c.NotAfter.IsZero() {
			days := int(time.Until(c.NotAfter).Hours() / 24)
			if days >= 0 && days <= 30 && c.ManagedBy != "acme" {
				fail("CLM-02", label)
			}
		}

		// CLM-03 self-signed
		byID["CLM-03"].Checked++
		if c.SelfSigned {
			fail("CLM-03", label)
		}

		// CLM-04 key strength. Only counted where the size is known; scoring an
		// unknown as a pass would overstate the posture, and as a fail would
		// punish records discovery could not measure.
		if c.KeySize > 0 {
			byID["CLM-04"].Checked++
			if c.KeySize < 2048 {
				fail("CLM-04", label)
			}
		}

		// CLM-05 validity length
		if !c.NotBefore.IsZero() && !c.NotAfter.IsZero() {
			byID["CLM-05"].Checked++
			if c.NotAfter.Sub(c.NotBefore) > 400*24*time.Hour {
				fail("CLM-05", label)
			}
		}

		// CLM-06 issuer identifiable
		byID["CLM-06"].Checked++
		if c.Issuer == "" {
			fail("CLM-06", label)
		}
	}

	// Score: each rule contributes its pass rate, weighted by severity. Rules with
	// nothing to check are excluded rather than counted as perfect.
	weights := map[string]float64{"high": 3, "medium": 2, "low": 1}
	var weighted, totalWeight float64

	for _, r := range rules {
		r.Passed = r.Checked - r.Failed
		switch {
		case r.Checked == 0:
			r.Status = "not_applicable"
		case r.Failed == 0:
			r.Status = "pass"
		default:
			r.Status = "fail"
		}
		if r.Checked > 0 {
			w := weights[r.Severity]
			weighted += w * (float64(r.Passed) / float64(r.Checked))
			totalWeight += w
		}
		rep.Rules = append(rep.Rules, *r)
	}

	if totalWeight > 0 {
		rep.Score = int((weighted / totalWeight) * 100)
	} else {
		rep.Score = 0
	}
	rep.Grade = gradeFor(rep.Score, assessed)

	rep.ByEnv = toNameCounts(envs)
	rep.ByIssuer = toNameCounts(issuers)
	rep.BySource = toNameCounts(sources)
	rep.KeyStrength = toNameCounts(keys)

	failed := 0
	for _, r := range rep.Rules {
		if r.Status == "fail" {
			failed++
		}
	}
	rep.Assessed = assessed
	rep.Excluded = skipped

	switch {
	case rep.Total == 0:
		rep.Summary = "No certificates in the inventory, so there is nothing to report on yet."
	case assessed == 0:
		rep.Summary = fmt.Sprintf("None of the %d record(s) are live certificates: %d are revoked or failed issuance attempts.",
			rep.Total, skipped)
	default:
		rep.Summary = fmt.Sprintf("%d live certificate(s) assessed against %d control(s): %d failing. Score %d/100 (%s).",
			assessed, len(rep.Rules), failed, rep.Score, rep.Grade)
	}
	if skipped > 0 {
		rep.Caveats = append(rep.Caveats,
			fmt.Sprintf("%d record(s) excluded from scoring: revoked certificates or failed issuance attempts, neither of which is in service.", skipped))
	}

	// State the limits of the report rather than letting a reader assume more.
	rep.Caveats = append(rep.Caveats,
		"Only certificates this gateway has discovered or issued are in scope; anything never scanned is invisible to the score.")
	if keys["unknown"] > 0 {
		rep.Caveats = append(rep.Caveats,
			fmt.Sprintf("%d certificate(s) have no recorded key size and are excluded from the key-strength control.", keys["unknown"]))
	}
	if rep.Managed["observed_only"] > 0 {
		rep.Caveats = append(rep.Caveats,
			fmt.Sprintf("%d certificate(s) are observed only: the gateway holds no key for them and cannot renew or revoke them.", rep.Managed["observed_only"]))
	}

	return rep
}

// gradeFor turns a score into a letter. An empty inventory gets no grade, because
// scoring nothing as an A would be actively misleading.
func gradeFor(score, total int) string {
	if total == 0 {
		return "n/a"
	}
	switch {
	case score >= 95:
		return "A"
	case score >= 85:
		return "B"
	case score >= 70:
		return "C"
	case score >= 50:
		return "D"
	default:
		return "E"
	}
}

func toNameCounts(m map[string]int) []NameCount {
	out := make([]NameCount, 0, len(m))
	for k, v := range m {
		out = append(out, NameCount{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// --- HTTP handlers ---

// handleComplianceReport returns the scored posture.
// GET /api/v1/reports/compliance
func (s *Server) handleComplianceReport(w http.ResponseWriter, r *http.Request) {
	report := buildComplianceReport(s.satelliteName, s.inventory.ListCerts())
	writeJSON(w, http.StatusOK, report)
}

// handleExport returns the inventory as CSV or JSON.
// GET /api/v1/reports/export?format=csv|json
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	certs := s.inventory.ListCerts()
	format := strings.ToLower(r.URL.Query().Get("format"))
	stamp := time.Now().UTC().Format("20060102-150405")

	if format == "" || format == "json" {
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="clm-inventory-%s.json"`, stamp))
		writeJSON(w, http.StatusOK, map[string]any{
			"gateway":     s.satelliteName,
			"exported_at": time.Now(),
			"total":       len(certs),
			"certificates": certs,
		})
		return
	}

	if format != "csv" {
		writeErr(w, http.StatusBadRequest, "format must be csv or json")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="clm-inventory-%s.csv"`, stamp))

	cw := csv.NewWriter(w)
	defer cw.Flush()

	// The private key is deliberately absent, and so is the chain: an export is
	// the most likely artefact to be emailed around.
	_ = cw.Write([]string{
		"domain", "host", "port", "status", "days_left", "not_before", "not_after",
		"issuer", "serial", "key_algorithm", "key_size", "self_signed",
		"environment", "source", "discovery_method", "managed_by",
		"renewal_count", "revoked_at", "sans",
	})

	for _, c := range certs {
		_ = cw.Write([]string{
			c.Domain,
			c.Host,
			itoaOrEmpty(c.Port),
			c.Status,
			strconv.Itoa(c.DaysLeft),
			timeOrEmpty(c.NotBefore),
			timeOrEmpty(c.NotAfter),
			c.Issuer,
			c.SerialNumber,
			c.KeyAlgorithm,
			itoaOrEmpty(c.KeySize),
			strconv.FormatBool(c.SelfSigned),
			c.Environment,
			c.Source,
			c.DiscoveryMethod,
			c.ManagedBy,
			strconv.Itoa(c.RenewalCount),
			timeOrEmpty(c.RevokedAt),
			strings.Join(c.SANs, " "),
		})
	}
}

func itoaOrEmpty(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func timeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
