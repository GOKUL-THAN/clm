package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// AlertLevel represents severity of an alert.
type AlertLevel string

const (
	AlertCritical AlertLevel = "critical"
	AlertWarning  AlertLevel = "warning"
	AlertInfo     AlertLevel = "info"
)

// Alert represents a monitoring alert.
type Alert struct {
	ID        string     `json:"id"`
	Level     AlertLevel `json:"level"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	CertID    string     `json:"cert_id,omitempty"`
	Domain    string     `json:"domain,omitempty"`
	DaysLeft  int        `json:"days_left,omitempty"`
	Timestamp time.Time  `json:"timestamp"`
}

// AlertConfig holds alert delivery configuration.
type AlertConfig struct {
	WebhookURL    string `yaml:"webhook_url"`
	SlackURL      string `yaml:"slack_url"`
	EmailEndpoint string `yaml:"email_endpoint"`
	CriticalDays  int    `yaml:"critical_days"` // e.g., 7
	WarningDays   int    `yaml:"warning_days"`  // e.g., 14
	InfoDays      int    `yaml:"info_days"`     // e.g., 30
	CheckInterval time.Duration
}

// Monitor periodically checks certificate health and sends alerts.
type Monitor struct {
	certRepo   core.CertificateTenantScanner
	satRepo    core.SatelliteRepository
	config     *AlertConfig
	logger     *zap.Logger
	httpClient *http.Client
}

// NewMonitor creates a new certificate monitor.
func NewMonitor(
	certRepo core.CertificateTenantScanner,
	satRepo core.SatelliteRepository,
	config *AlertConfig,
	logger *zap.Logger,
) *Monitor {
	if config.CriticalDays == 0 {
		config.CriticalDays = 7
	}
	if config.WarningDays == 0 {
		config.WarningDays = 14
	}
	if config.InfoDays == 0 {
		config.InfoDays = 30
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 1 * time.Hour
	}
	return &Monitor{
		certRepo:   certRepo,
		satRepo:    satRepo,
		config:     config,
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return errors.New("redirects prohibited") }},
	}
}

// Start begins the monitoring loop.
func (m *Monitor) Start(ctx context.Context) {
	m.logger.Info("certificate monitor started",
		zap.Duration("interval", m.config.CheckInterval),
		zap.Int("critical_days", m.config.CriticalDays),
		zap.Int("warning_days", m.config.WarningDays),
	)

	// Run immediately
	m.checkCertificates(ctx)
	m.checkSatellites(ctx)

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("certificate monitor stopped")
			return
		case <-ticker.C:
			m.checkCertificates(ctx)
			m.checkSatellites(ctx)
		}
	}
}

// checkCertificates scans for expiring, expired, or problematic certificates.
func (m *Monitor) checkCertificates(ctx context.Context) {
	// Check for certificates expiring within info threshold
	filter := &core.CertFilter{
		Status:     []models.CertStatus{models.CertStatusActive},
		ExpiringIn: m.config.InfoDays,
		Page:       1,
		PageSize:   1000,
	}

	certs, _, err := m.certRepo.ListAllTenants(ctx, filter)
	if err != nil {
		m.logger.Error("monitor: failed to query certificates", zap.Error(err))
		return
	}

	var alerts []Alert
	for _, cert := range certs {
		daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)

		var level AlertLevel
		var title string

		switch {
		case daysLeft <= 0:
			level = AlertCritical
			title = "Certificate EXPIRED"
		case daysLeft <= m.config.CriticalDays:
			level = AlertCritical
			title = fmt.Sprintf("Certificate expires in %d days", daysLeft)
		case daysLeft <= m.config.WarningDays:
			level = AlertWarning
			title = fmt.Sprintf("Certificate expires in %d days", daysLeft)
		default:
			level = AlertInfo
			title = fmt.Sprintf("Certificate expires in %d days", daysLeft)
		}

		alert := Alert{
			ID:        fmt.Sprintf("cert-expiry-%s", cert.ID.String()[:8]),
			Level:     level,
			Title:     title,
			Message:   fmt.Sprintf("Domain: %s | Issuer: %s | Env: %s | Expires: %s", cert.Domain, cert.Issuer, cert.Environment, cert.NotAfter.Format("2006-01-02")),
			CertID:    cert.ID.String(),
			Domain:    cert.Domain,
			DaysLeft:  daysLeft,
			Timestamp: time.Now(),
		}
		alerts = append(alerts, alert)
	}

	// Send alerts for critical and warning
	for _, alert := range alerts {
		if alert.Level == AlertCritical || alert.Level == AlertWarning {
			m.sendAlert(alert)
		}
	}

	if len(alerts) > 0 {
		m.logger.Info("monitoring check complete",
			zap.Int("alerts", len(alerts)),
			zap.Int("critical", countByLevel(alerts, AlertCritical)),
			zap.Int("warning", countByLevel(alerts, AlertWarning)),
		)
	}
}

// checkSatellites verifies satellite connectivity.
func (m *Monitor) checkSatellites(ctx context.Context) {
	// Check all satellites - find ones with stale heartbeats
	sats, err := m.satRepo.ListByTenant(ctx, uuid.Nil)
	if err != nil {
		return
	}

	for _, sat := range sats {
		if sat.LastHeartbeat == nil {
			continue
		}
		if time.Since(*sat.LastHeartbeat) > 5*time.Minute && sat.Status == "online" {
			alert := Alert{
				ID:        fmt.Sprintf("sat-offline-%s", sat.ID.String()[:8]),
				Level:     AlertWarning,
				Title:     "Satellite connection lost",
				Message:   fmt.Sprintf("Satellite '%s' (region: %s) has not sent a heartbeat in %v", sat.Name, sat.Region, time.Since(*sat.LastHeartbeat).Round(time.Second)),
				Timestamp: time.Now(),
			}
			m.sendAlert(alert)
		}
	}
}

// sendAlert delivers an alert via configured channels.
func (m *Monitor) sendAlert(alert Alert) {
	m.logger.Warn("ALERT",
		zap.String("level", string(alert.Level)),
		zap.String("title", alert.Title),
		zap.String("message", alert.Message),
	)

	// Send to webhook if configured
	if m.config.WebhookURL != "" {
		go m.sendWebhook(alert)
	}

	// Send to Slack if configured
	if m.config.SlackURL != "" {
		go m.sendSlack(alert)
	}
}

func (m *Monitor) sendWebhook(alert Alert) {
	payload, err := json.Marshal(alert)
	if err != nil {
		m.logger.Error("could not encode webhook alert", zap.Error(err))
		return
	}
	resp, err := m.httpClient.Post(m.config.WebhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		m.logger.Debug("webhook delivery failed", zap.Error(err))
		return
	}
	if err := resp.Body.Close(); err != nil {
		m.logger.Debug("closing webhook response failed", zap.Error(err))
	}
}

func (m *Monitor) sendSlack(alert Alert) {
	emoji := map[AlertLevel]string{AlertCritical: ":red_circle:", AlertWarning: ":warning:", AlertInfo: ":information_source:"}
	slackPayload := map[string]string{
		"text": fmt.Sprintf("%s *%s*\n%s", emoji[alert.Level], alert.Title, alert.Message),
	}
	payload, err := json.Marshal(slackPayload)
	if err != nil {
		m.logger.Error("could not encode Slack alert", zap.Error(err))
		return
	}
	resp, err := m.httpClient.Post(m.config.SlackURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		m.logger.Debug("slack delivery failed", zap.Error(err))
		return
	}
	if err := resp.Body.Close(); err != nil {
		m.logger.Debug("closing Slack response failed", zap.Error(err))
	}
}

func countByLevel(alerts []Alert, level AlertLevel) int {
	count := 0
	for _, a := range alerts {
		if a.Level == level {
			count++
		}
	}
	return count
}
