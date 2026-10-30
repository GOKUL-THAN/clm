package api

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

// Certificate lifecycle assessment.
//
// The certificates page answers "what do we have". This answers "what is about
// to hurt, and what should be done first" — which is the part of lifecycle
// management the gateway was missing. Everything here is derived from the
// existing inventory, so it needs no extra scan and no persistence.

// Risk bands. Expiry is graded rather than treated as a single 30-day cliff,
// because a 5-day certificate and a 29-day certificate call for very different
// urgency.
const (
	riskExpired  = "expired"
	riskCritical = "critical" // 7 days or fewer
	riskWarning  = "warning"  // 30 days or fewer
	riskNotice   = "notice"   // 60 days or fewer
	riskHealthy  = "healthy"
	riskUnknown  = "unknown" // no expiry known, e.g. a pending request
)

// LifecycleFinding is one actionable problem with one certificate.
type LifecycleFinding struct {
	CertID    string   `json:"cert_id"`
	Domain    string   `json:"domain"`
	Host      string   `json:"host,omitempty"`
	Port      int      `json:"port,omitempty"`
	Issuer    string   `json:"issuer,omitempty"`
	Risk      string   `json:"risk"`
	DaysLeft  int      `json:"days_left"`
	NotAfter  time.Time `json:"not_after,omitempty"`
	Issues    []string `json:"issues"`
	Action    string   `json:"action"`
	// Priority orders the worklist: lower sorts first.
	Priority int `json:"priority"`
}

// LifecycleReport is the gateway's lifecycle posture.
type LifecycleReport struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Total       int            `json:"total"`
	Risk        map[string]int `json:"risk"`
	Hygiene     map[string]int `json:"hygiene"`
	ByIssuer    []IssuerCount  `json:"by_issuer"`
	Expiring    []ExpiryBucket `json:"expiry_buckets"`
	Findings    []LifecycleFinding `json:"findings"`
	// RenewalQueue is the subset of findings that expiry alone puts at risk,
	// soonest first: the actual work queue.
	RenewalQueue []LifecycleFinding `json:"renewal_queue"`
	Summary      string             `json:"summary"`
}

// IssuerCount is a per-CA tally, which is what exposes an unexpected issuer or a
// concentration risk on one CA.
type IssuerCount struct {
	Issuer  string `json:"issuer"`
	Count   int    `json:"count"`
	AtRisk  int    `json:"at_risk"`
}

// ExpiryBucket groups certificates into reporting windows.
type ExpiryBucket struct {
	Window string `json:"window"`
	Count  int    `json:"count"`
}

// riskFor grades remaining validity.
func riskFor(c *Certificate) (string, int) {
	if c.NotAfter.IsZero() {
		return riskUnknown, 0
	}
	days := int(time.Until(c.NotAfter).Hours() / 24)
	switch {
	case days < 0:
		return riskExpired, days
	case days <= 7:
		return riskCritical, days
	case days <= 30:
		return riskWarning, days
	case days <= 60:
		return riskNotice, days
	default:
		return riskHealthy, days
	}
}

// riskPriority orders the worklist.
func riskPriority(risk string) int {
	switch risk {
	case riskExpired:
		return 0
	case riskCritical:
		return 1
	case riskWarning:
		return 2
	case riskNotice:
		return 3
	case riskUnknown:
		return 4
	default:
		return 5
	}
}

// assessCertificate collects every issue with one certificate.
func assessCertificate(c *Certificate) LifecycleFinding {
	risk, days := riskFor(c)
	issues := make([]string, 0, 4)

	switch risk {
	case riskExpired:
		issues = append(issues, "expired "+plural(-days, "day")+" ago")
	case riskCritical:
		issues = append(issues, "expires in "+plural(days, "day"))
	case riskWarning, riskNotice:
		issues = append(issues, "expires in "+plural(days, "day"))
	}

	if c.SelfSigned {
		issues = append(issues, "self-signed, so no public chain of trust")
	}
	if c.Issuer == "" && !c.NotAfter.IsZero() {
		issues = append(issues, "issuer could not be determined")
	}
	// A certificate valid for more than ~13 months has not been issuable
	// publicly since 2020, so it is either private or predates the change.
	if !c.NotBefore.IsZero() && !c.NotAfter.IsZero() {
		if c.NotAfter.Sub(c.NotBefore) > 400*24*time.Hour {
			issues = append(issues, "validity longer than 398 days, which public CAs no longer issue")
		}
	}

	action := "none required"
	switch {
	case risk == riskExpired:
		action = "replace now; this certificate is already failing clients"
	case risk == riskCritical:
		action = "renew immediately"
	case risk == riskWarning:
		action = "schedule renewal"
	case risk == riskNotice:
		action = "plan renewal"
	case c.SelfSigned:
		action = "replace with a CA-issued certificate"
	case risk == riskUnknown:
		action = "no expiry recorded; re-scan the host to populate it"
	}

	return LifecycleFinding{
		CertID:   c.ID,
		Domain:   c.Domain,
		Host:     c.Host,
		Port:     c.Port,
		Issuer:   c.Issuer,
		Risk:     risk,
		DaysLeft: days,
		NotAfter: c.NotAfter,
		Issues:   issues,
		Action:   action,
		Priority: riskPriority(risk),
	}
}

// firstLine trims a multi-line error to something that fits in a table cell. CA
// errors in particular can run to several lines of JSON.
func firstLine(s string) string {
	if s == "" {
		return "no reason recorded"
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	return s
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return itoa(n) + " " + unit + "s"
}

// itoa avoids importing strconv for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// handleLifecycle returns the lifecycle assessment.
// GET /api/v1/lifecycle
func (s *Server) handleLifecycle(w http.ResponseWriter, r *http.Request) {
	certs := s.inventory.ListCerts()

	// Every slice is initialised so the response carries [] rather than null on
	// an empty inventory. A null forces each client to add a guard, and the UI
	// and any alerting script would both have to remember it.
	report := LifecycleReport{
		GeneratedAt:  time.Now(),
		Total:        len(certs),
		Risk:         map[string]int{},
		Hygiene:      map[string]int{},
		Findings:     make([]LifecycleFinding, 0, len(certs)),
		RenewalQueue: make([]LifecycleFinding, 0),
		ByIssuer:     make([]IssuerCount, 0),
		Expiring:     make([]ExpiryBucket, 0, 7),
	}

	issuers := map[string]*IssuerCount{}
	buckets := map[string]int{"expired": 0, "0-7d": 0, "8-30d": 0, "31-60d": 0, "61-90d": 0, "90d+": 0, "unknown": 0}

	for _, c := range certs {
		// Revoked certificates and failed issuance attempts are not live risks:
		// one has already been dealt with, the other never existed. Listing them
		// as work to do would bury the certificates that actually need attention.
		if !c.RevokedAt.IsZero() {
			report.Risk["revoked"]++
			continue
		}
		if c.Status == "failed" {
			report.Risk["failed_issuance"]++
			report.Findings = append(report.Findings, LifecycleFinding{
				CertID: c.ID, Domain: c.Domain, Issuer: c.Issuer,
				Risk: riskUnknown, Priority: riskPriority(riskUnknown),
				Issues: []string{"issuance failed: " + firstLine(c.LastError)},
				Action: "retry issuance once the cause is fixed",
			})
			continue
		}

		finding := assessCertificate(c)
		report.Risk[finding.Risk]++

		if c.SelfSigned {
			report.Hygiene["self_signed"]++
		}
		if c.Issuer == "" {
			report.Hygiene["unknown_issuer"]++
		}
		if len(c.SANs) == 0 && c.Source == "discovery" {
			report.Hygiene["no_sans"]++
		}
		if !c.NotBefore.IsZero() && !c.NotAfter.IsZero() &&
			c.NotAfter.Sub(c.NotBefore) > 400*24*time.Hour {
			report.Hygiene["over_long_validity"]++
		}

		name := c.Issuer
		if name == "" {
			name = "(unknown)"
		}
		if _, ok := issuers[name]; !ok {
			issuers[name] = &IssuerCount{Issuer: name}
		}
		issuers[name].Count++
		if finding.Priority <= 2 {
			issuers[name].AtRisk++
		}

		switch {
		case finding.Risk == riskUnknown:
			buckets["unknown"]++
		case finding.DaysLeft < 0:
			buckets["expired"]++
		case finding.DaysLeft <= 7:
			buckets["0-7d"]++
		case finding.DaysLeft <= 30:
			buckets["8-30d"]++
		case finding.DaysLeft <= 60:
			buckets["31-60d"]++
		case finding.DaysLeft <= 90:
			buckets["61-90d"]++
		default:
			buckets["90d+"]++
		}

		// Only surface certificates that need something doing.
		if len(finding.Issues) > 0 {
			report.Findings = append(report.Findings, finding)
		}
	}

	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Priority != report.Findings[j].Priority {
			return report.Findings[i].Priority < report.Findings[j].Priority
		}
		return report.Findings[i].DaysLeft < report.Findings[j].DaysLeft
	})

	for _, f := range report.Findings {
		if f.Priority <= 2 {
			report.RenewalQueue = append(report.RenewalQueue, f)
		}
	}

	for _, ic := range issuers {
		report.ByIssuer = append(report.ByIssuer, *ic)
	}
	sort.Slice(report.ByIssuer, func(i, j int) bool {
		if report.ByIssuer[i].Count != report.ByIssuer[j].Count {
			return report.ByIssuer[i].Count > report.ByIssuer[j].Count
		}
		return report.ByIssuer[i].Issuer < report.ByIssuer[j].Issuer
	})

	for _, window := range []string{"expired", "0-7d", "8-30d", "31-60d", "61-90d", "90d+", "unknown"} {
		report.Expiring = append(report.Expiring, ExpiryBucket{Window: window, Count: buckets[window]})
	}

	report.Summary = lifecycleSummary(report)
	writeJSON(w, http.StatusOK, report)
}

// lifecycleSummary states the posture in a sentence, so the API is useful to a
// human reading it directly and to an alert that only has room for one line.
func lifecycleSummary(r LifecycleReport) string {
	if r.Total == 0 {
		return "No certificates in the inventory yet. Run a discovery scan to populate it."
	}
	parts := make([]string, 0, 4)
	if n := r.Risk[riskExpired]; n > 0 {
		parts = append(parts, itoa(n)+" already expired")
	}
	if n := r.Risk[riskCritical]; n > 0 {
		parts = append(parts, itoa(n)+" expiring within 7 days")
	}
	if n := r.Risk[riskWarning]; n > 0 {
		parts = append(parts, itoa(n)+" within 30 days")
	}
	if n := r.Hygiene["self_signed"]; n > 0 {
		parts = append(parts, itoa(n)+" self-signed")
	}
	if len(parts) == 0 {
		return "All " + itoa(r.Total) + " tracked certificates are healthy for the next 60 days."
	}
	return itoa(r.Total) + " certificates tracked: " + strings.Join(parts, ", ") + "."
}
