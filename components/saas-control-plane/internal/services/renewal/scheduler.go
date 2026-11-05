package renewal

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// Scheduler periodically checks for certificates nearing expiry and queues renewal commands.
type Scheduler struct {
	certRepo  core.CertificateSystemRepository
	satRepo   core.SatelliteRepository
	interval  time.Duration
	threshold int // days before expiry to trigger renewal
	logger    *zap.Logger
}

// NewScheduler creates a new renewal scheduler.
func NewScheduler(
	certRepo core.CertificateSystemRepository,
	satRepo core.SatelliteRepository,
	interval time.Duration,
	thresholdDays int,
	logger *zap.Logger,
) *Scheduler {
	return &Scheduler{
		certRepo:  certRepo,
		satRepo:   satRepo,
		interval:  interval,
		threshold: thresholdDays,
		logger:    logger,
	}
}

// Start begins the renewal check loop.
func (s *Scheduler) Start(ctx context.Context) {
	s.logger.Info("renewal scheduler started",
		zap.Duration("interval", s.interval),
		zap.Int("threshold_days", s.threshold),
	)

	// Run immediately on start
	s.checkAndRenew(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("renewal scheduler stopped")
			return
		case <-ticker.C:
			s.checkAndRenew(ctx)
		}
	}
}

type renewalPayload struct {
	CertID       string              `json:"cert_id"`
	Domain       string              `json:"domain"`
	SANs         []string            `json:"sans"`
	KeyAlgorithm models.KeyAlgorithm `json:"key_algorithm"`
	Environment  string              `json:"environment"`
	TenantID     string              `json:"tenant_id"`
}

// checkAndRenew finds expiring certs and queues renewal commands to satellites.
func (s *Scheduler) checkAndRenew(ctx context.Context) {
	s.logger.Debug("checking for certificates needing renewal")

	filter := &core.CertFilter{
		Status:     []models.CertStatus{models.CertStatusActive},
		ExpiringIn: s.threshold,
		Page:       1,
		PageSize:   500,
	}

	certs, total, err := s.certRepo.ListAllTenants(ctx, filter)
	if err != nil {
		s.logger.Error("failed to query expiring certificates", zap.Error(err))
		return
	}
	if total == 0 {
		s.logger.Debug("no certificates needing renewal")
		return
	}

	s.logger.Info("found certificates needing renewal",
		zap.Int("count", total),
		zap.Int("threshold_days", s.threshold),
	)

	renewing := 0
	for _, cert := range certs {
		if cert.Status != models.CertStatusActive {
			continue
		}

		daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)
		if daysLeft > s.threshold {
			continue
		}

		satelliteID, err := uuid.Parse(cert.SatelliteID)
		if err != nil {
			s.logger.Warn("certificate has invalid satellite ID, skipping renewal",
				zap.String("cert_id", cert.ID.String()),
				zap.String("domain", cert.Domain),
				zap.String("satellite", cert.SatelliteID),
				zap.Error(err),
			)
			continue
		}

		payloadJSON, err := json.Marshal(renewalPayload{
			CertID:       cert.ID.String(),
			Domain:       cert.Domain,
			SANs:         cert.SANs,
			KeyAlgorithm: cert.KeyAlgorithm,
			Environment:  cert.Environment,
			TenantID:     cert.TenantID.String(),
		})
		if err != nil {
			s.logger.Error("failed to serialize renewal command",
				zap.String("cert_id", cert.ID.String()),
				zap.Error(err),
			)
			continue
		}

		cmd := &models.SatelliteCommand{
			ID:             uuid.New(),
			TenantID:       cert.TenantID,
			SatelliteID:    satelliteID,
			IdempotencyKey: "renew:" + cert.ID.String() + ":" + cert.NotAfter.UTC().Format(time.RFC3339),
			Type:           "renew_cert",
			Payload:        string(payloadJSON),
			Status:         models.CommandQueued,
			Priority:       2,
		}

		queuedCommand, created, err := s.satRepo.QueueCommandIdempotent(ctx, cert.TenantID, cmd)
		if err != nil {
			s.logger.Error("failed to queue renewal command",
				zap.String("cert_id", cert.ID.String()),
				zap.Error(err),
			)
			continue
		}
		if !created && queuedCommand.Status != models.CommandQueued && queuedCommand.Status != models.CommandLeased && queuedCommand.Status != models.CommandRunning {
			s.logger.Warn("renewal command is already terminal, leaving certificate status unchanged",
				zap.String("cert_id", cert.ID.String()),
				zap.String("command_id", queuedCommand.ID.String()),
				zap.String("command_status", string(queuedCommand.Status)),
			)
			continue
		}

		cert.Status = models.CertStatusRenewing
		cert.UpdatedAt = time.Now().UTC()
		if err := s.certRepo.Update(ctx, cert); err != nil {
			s.logger.Error("failed to mark certificate as renewing",
				zap.String("cert_id", cert.ID.String()),
				zap.String("command_id", queuedCommand.ID.String()),
				zap.Error(err),
			)
			continue
		}

		renewing++
		s.logger.Info("renewal queued",
			zap.String("cert_id", cert.ID.String()),
			zap.String("domain", cert.Domain),
			zap.Int("days_left", daysLeft),
			zap.String("satellite", cert.SatelliteID),
			zap.Bool("command_created", created),
		)
	}

	if renewing > 0 {
		s.logger.Info("renewal batch complete",
			zap.Int("renewing", renewing),
			zap.Int("total_expiring", total),
		)
	}
}
