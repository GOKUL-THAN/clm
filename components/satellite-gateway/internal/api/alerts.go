package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/satellite/state"
	"go.uber.org/zap"
)

// Proactive expiry and risk alerting.
//
// The gateway could already describe risk on request, but nothing told anyone
// about it. A certificate expiring at 03:00 on a Sunday produced no signal until
// somebody happened to open the portal, which is the failure mode certificate
// lifecycle management exists to prevent.
//
// Two design points worth stating:
//
//   - Alerts are deduplicated with a cooldown per certificate and severity.
//     Without it, an evaluation every 15 minutes would send the same "expires in
//     20 days" message 96 times a day, and the recipients would filter the
//     channel within a week.
//   - Severity escalation bypasses the cooldown. Going from warning to critical
//     is new information and must not be swallowed by a cooldown started while
//     the certificate was merely a warning.

// Alert is one raised condition.
type Alert struct {
	ID       string    `json:"id"`
	CertID   string    `json:"cert_id,omitempty"`
	Domain   string    `json:"domain"`
	Host     string    `json:"host,omitempty"`
	Severity string    `json:"severity"` // critical, warning, info
	Kind     string    `json:"kind"`     // expired, expiry, self_signed, weak_key
	Message  string    `json:"message"`
	DaysLeft int       `json:"days_left"`
	Issuer   string    `json:"issuer,omitempty"`
	RaisedAt time.Time `json:"raised_at"`
	// Delivered records whether at least one channel accepted the alert, and
	// DeliveryError why not. A silently dropped alert is worse than none, because
	// the absence of a message reads as "nothing is wrong".
	Delivered     bool   `json:"delivered"`
	DeliveryError string `json:"delivery_error,omitempty"`
	Channels      []string `json:"channels,omitempty"`
}

// AlertConfig is the runtime alerting configuration.
type AlertConfig struct {
	Enabled           bool
	WebhookURL        string
	SlackWebhookURL   string
	WarnDays          int
	CriticalDays      int
	Cooldown          time.Duration
	AlertOnSelfSigned bool
	AlertOnWeakKey    bool
}

// Alerter evaluates the inventory and delivers notifications.
type Alerter struct {
	cfg    AlertConfig
	client *http.Client
	store  *state.Store
	logger *zap.Logger

	mu      sync.RWMutex
	history []*Alert
	// lastSent keys "certID|kind" to the severity and time last delivered, which
	// is what makes both the cooldown and the escalation rule work.
	lastSent map[string]sentRecord
	// stats are cumulative counters for the portal.
	raised    int
	delivered int
	failed    int
	lastRun   time.Time
}

type sentRecord struct {
	Severity string    `json:"severity"`
	At       time.Time `json:"at"`
}

// alertSnapshot is the on-disk shape.
type alertSnapshot struct {
	History  []*Alert              `json:"history"`
	LastSent map[string]sentRecord `json:"last_sent"`
	Raised   int                   `json:"raised"`
	Delivered int                  `json:"delivered"`
	Failed   int                   `json:"failed"`
}

const (
	alertCollection = "alerts"
	// maxAlertHistory bounds memory and the snapshot. Alerting is a notification
	// path, not an audit log; the inventory holds the durable truth.
	maxAlertHistory = 500
)

// severityRank orders severities so escalation can be detected.
func severityRank(s string) int {
	switch s {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	}
	return 0
}

// NewAlerter builds an alerter. It is safe to use with Enabled false, in which
// case evaluation still records alerts but delivers nothing — useful for seeing
// what would fire before pointing it at a channel.
func NewAlerter(cfg AlertConfig, st *state.Store, logger *zap.Logger) *Alerter {
	if cfg.WarnDays <= 0 {
		cfg.WarnDays = 30
	}
	if cfg.CriticalDays <= 0 {
		cfg.CriticalDays = 7
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 24 * time.Hour
	}

	a := &Alerter{
		cfg:      cfg,
		client:   &http.Client{Timeout: 15 * time.Second},
		store:    st,
		logger:   logger,
		history:  make([]*Alert, 0, 64),
		lastSent: map[string]sentRecord{},
	}

	if st.Enabled() {
		var snap alertSnapshot
		if err := st.Load(alertCollection, &snap); err == nil {
			a.history = snap.History
			if snap.LastSent != nil {
				a.lastSent = snap.LastSent
			}
			a.raised, a.delivered, a.failed = snap.Raised, snap.Delivered, snap.Failed
			if len(a.history) > 0 {
				logger.Info("alert history restored", zap.Int("alerts", len(a.history)))
			}
		}
	}
	return a
}

// Channels reports which delivery channels are configured, for the portal.
func (a *Alerter) Channels() []string {
	out := []string{}
	if a.cfg.WebhookURL != "" {
		out = append(out, "webhook")
	}
	if a.cfg.SlackWebhookURL != "" {
		out = append(out, "slack")
	}
	return out
}

// Evaluate walks the inventory, raises alerts for anything that qualifies, and
// delivers those not suppressed by the cooldown. It returns the alerts raised.
func (a *Alerter) Evaluate(ctx context.Context, certs []*Certificate) []*Alert {
	raised := make([]*Alert, 0)

	for _, c := range certs {
		// A revoked certificate is not a live risk; alerting on it would be noise
		// after someone has already acted.
		if !c.RevokedAt.IsZero() {
			continue
		}
		for _, cand := range a.candidates(c) {
			if a.shouldSend(cand) {
				a.deliver(ctx, cand)
			} else {
				cand.Channels = []string{"suppressed (cooldown)"}
			}
			a.record(cand)
			raised = append(raised, cand)
		}
	}

	a.mu.Lock()
	a.lastRun = time.Now()
	a.mu.Unlock()
	a.persist()

	return raised
}

// candidates builds the alerts a single certificate justifies.
func (a *Alerter) candidates(c *Certificate) []*Alert {
	out := make([]*Alert, 0, 2)
	base := func(kind, severity, msg string, days int) *Alert {
		return &Alert{
			ID:       uuid.NewString(),
			CertID:   c.ID,
			Domain:   c.Domain,
			Host:     c.Host,
			Severity: severity,
			Kind:     kind,
			Message:  msg,
			DaysLeft: days,
			Issuer:   c.Issuer,
			RaisedAt: time.Now(),
		}
	}

	if !c.NotAfter.IsZero() {
		days := int(time.Until(c.NotAfter).Hours() / 24)
		switch {
		case days < 0:
			out = append(out, base("expired", "critical",
				fmt.Sprintf("%s expired %d day(s) ago and is failing clients now", c.Domain, -days), days))
		case days <= a.cfg.CriticalDays:
			out = append(out, base("expiry", "critical",
				fmt.Sprintf("%s expires in %d day(s)", c.Domain, days), days))
		case days <= a.cfg.WarnDays:
			out = append(out, base("expiry", "warning",
				fmt.Sprintf("%s expires in %d day(s)", c.Domain, days), days))
		}
	}

	if a.cfg.AlertOnSelfSigned && c.SelfSigned {
		out = append(out, base("self_signed", "warning",
			fmt.Sprintf("%s is self-signed, so clients have no chain of trust", c.Domain), c.DaysLeft))
	}
	if a.cfg.AlertOnWeakKey && c.KeySize > 0 && c.KeySize < 2048 {
		out = append(out, base("weak_key", "critical",
			fmt.Sprintf("%s uses a %d-bit key, below the 2048-bit minimum", c.Domain, c.KeySize), c.DaysLeft))
	}

	return out
}

// shouldSend applies the cooldown, allowing escalation through.
func (a *Alerter) shouldSend(al *Alert) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	prev, ok := a.lastSent[al.CertID+"|"+al.Kind]
	if !ok {
		return true
	}
	// Escalation is new information; never suppress it.
	if severityRank(al.Severity) > severityRank(prev.Severity) {
		return true
	}
	return time.Since(prev.At) >= a.cfg.Cooldown
}

// deliver posts the alert to every configured channel. An alert is considered
// delivered if any channel accepted it.
func (a *Alerter) deliver(ctx context.Context, al *Alert) {
	if !a.cfg.Enabled {
		al.Channels = []string{"disabled"}
		return
	}
	channels := []string{}
	var lastErr error

	if a.cfg.WebhookURL != "" {
		if err := a.postJSON(ctx, a.cfg.WebhookURL, al); err != nil {
			lastErr = err
			a.logger.Warn("alert webhook delivery failed",
				zap.String("domain", al.Domain), zap.Error(err))
		} else {
			channels = append(channels, "webhook")
		}
	}
	if a.cfg.SlackWebhookURL != "" {
		if err := a.postJSON(ctx, a.cfg.SlackWebhookURL, slackPayload(al)); err != nil {
			lastErr = err
			a.logger.Warn("alert slack delivery failed",
				zap.String("domain", al.Domain), zap.Error(err))
		} else {
			channels = append(channels, "slack")
		}
	}

	if len(channels) == 0 {
		al.Delivered = false
		if lastErr != nil {
			al.DeliveryError = lastErr.Error()
		} else {
			al.DeliveryError = "no delivery channel configured"
		}
		al.Channels = []string{}
		a.mu.Lock()
		a.failed++
		a.mu.Unlock()
		return
	}

	al.Delivered = true
	al.Channels = channels

	a.mu.Lock()
	a.delivered++
	a.lastSent[al.CertID+"|"+al.Kind] = sentRecord{Severity: al.Severity, At: time.Now()}
	a.mu.Unlock()

	a.logger.Info("alert delivered",
		zap.String("domain", al.Domain),
		zap.String("severity", al.Severity),
		zap.String("kind", al.Kind),
		zap.Strings("channels", channels))
}

func (a *Alerter) postJSON(ctx context.Context, url string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding alert: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("channel returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// slackPayload shapes an alert for a Slack incoming webhook.
func slackPayload(al *Alert) map[string]any {
	icon := ":warning:"
	if al.Severity == "critical" {
		icon = ":rotating_light:"
	}
	return map[string]any{
		"text": fmt.Sprintf("%s *CLM %s*: %s", icon, al.Severity, al.Message),
		"attachments": []map[string]any{{
			"color": map[string]string{"critical": "danger", "warning": "warning"}[al.Severity],
			"fields": []map[string]any{
				{"title": "Domain", "value": al.Domain, "short": true},
				{"title": "Kind", "value": al.Kind, "short": true},
				{"title": "Days left", "value": fmt.Sprintf("%d", al.DaysLeft), "short": true},
				{"title": "Issuer", "value": nonEmpty(al.Issuer, "unknown"), "short": true},
			},
		}},
	}
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// record appends to the bounded history.
func (a *Alerter) record(al *Alert) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.raised++
	a.history = append(a.history, al)
	if len(a.history) > maxAlertHistory {
		a.history = a.history[len(a.history)-maxAlertHistory:]
	}
}

func (a *Alerter) persist() {
	if a.store == nil || !a.store.Enabled() {
		return
	}
	a.store.Save(alertCollection, func() any {
		a.mu.RLock()
		defer a.mu.RUnlock()
		return alertSnapshot{
			History: a.history, LastSent: a.lastSent,
			Raised: a.raised, Delivered: a.delivered, Failed: a.failed,
		}
	})
}

// History returns recent alerts, newest first.
func (a *Alerter) History(limit int) []*Alert {
	a.mu.RLock()
	defer a.mu.RUnlock()

	out := make([]*Alert, len(a.history))
	copy(out, a.history)
	sort.Slice(out, func(i, j int) bool { return out[i].RaisedAt.After(out[j].RaisedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Stats summarises alerting for the portal.
func (a *Alerter) Stats() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	bySeverity := map[string]int{}
	for _, al := range a.history {
		bySeverity[al.Severity]++
	}
	return map[string]any{
		"enabled":       a.cfg.Enabled,
		"channels":      a.Channels(),
		"warn_days":     a.cfg.WarnDays,
		"critical_days": a.cfg.CriticalDays,
		"cooldown":      a.cfg.Cooldown.String(),
		"raised":        a.raised,
		"delivered":     a.delivered,
		"failed":        a.failed,
		"by_severity":   bySeverity,
		"last_run":      a.lastRun,
		"history_size":  len(a.history),
	}
}

// --- HTTP handlers ---

// handleListAlerts returns alert history and stats.
// GET /api/v1/alerts
func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	if s.alerter == nil {
		writeErr(w, http.StatusServiceUnavailable, "alerting is not configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"alerts": s.alerter.History(200),
		"stats":  s.alerter.Stats(),
	})
}

// handleTestAlert delivers a synthetic alert so a channel can be proved end to
// end without waiting for a certificate to approach expiry.
// POST /api/v1/alerts/test
func (s *Server) handleTestAlert(w http.ResponseWriter, r *http.Request) {
	if s.alerter == nil {
		writeErr(w, http.StatusServiceUnavailable, "alerting is not configured")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "sending a test alert requires an admin role")
		return
	}

	al := &Alert{
		ID:       uuid.NewString(),
		Domain:   "test.clm.local",
		Severity: "info",
		Kind:     "test",
		Message:  "CLM test alert from " + s.satelliteName + " — delivery is working",
		RaisedAt: time.Now(),
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	// Deliberately bypasses the cooldown: a test that silently does nothing
	// because of a cooldown would be worse than no test at all.
	s.alerter.deliver(ctx, al)
	s.alerter.record(al)
	s.alerter.persist()

	status := http.StatusOK
	if !al.Delivered {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, map[string]any{
		"delivered": al.Delivered,
		"channels":  al.Channels,
		"error":     al.DeliveryError,
		"alert":     al,
	})
}
