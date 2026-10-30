package gatewayhealth

import (
	"context"
	"time"

	"github.com/m2p/clm/internal/core"
	"go.uber.org/zap"
)

// Monitor periodically checks all registered gateways and marks
// those that have missed heartbeats as offline.
type Monitor struct {
	satRepo        core.SatelliteRepository
	checkInterval  time.Duration
	offlineTimeout time.Duration // duration without heartbeat before marking offline
	logger         *zap.Logger
}

// NewMonitor creates a gateway health monitor.
// checkInterval: how often to scan (e.g., 30s)
// offlineTimeout: how long without heartbeat before marking offline (e.g., 2m)
func NewMonitor(satRepo core.SatelliteRepository, checkInterval, offlineTimeout time.Duration, logger *zap.Logger) *Monitor {
	return &Monitor{
		satRepo:        satRepo,
		checkInterval:  checkInterval,
		offlineTimeout: offlineTimeout,
		logger:         logger,
	}
}

// Start begins the health monitoring loop. Blocks until ctx is cancelled.
func (m *Monitor) Start(ctx context.Context) {
	m.logger.Info("gateway health monitor started",
		zap.Duration("check_interval", m.checkInterval),
		zap.Duration("offline_timeout", m.offlineTimeout),
	)

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("gateway health monitor stopped")
			return
		case <-ticker.C:
			m.checkGateways(ctx)
		}
	}
}

func (m *Monitor) checkGateways(ctx context.Context) {
	// List all gateways across all tenants that are not already offline
	// For now, we use a simplified approach: check satellites with status != "offline"
	// In production, this would query with a filter
	satellites, err := m.satRepo.ListAll(ctx)
	if err != nil {
		m.logger.Debug("gateway health check: could not list satellites", zap.Error(err))
		return
	}

	now := time.Now()
	offlineCount := 0

	for _, sat := range satellites {
		if sat.Status == "offline" || sat.Status == "registering" {
			continue
		}

		// Check if heartbeat is stale
		if sat.LastHeartbeat == nil || now.Sub(*sat.LastHeartbeat) > m.offlineTimeout {
			sat.Status = "offline"
			sat.UpdatedAt = now
			if err := m.satRepo.Update(ctx, sat); err != nil {
				m.logger.Warn("failed to mark gateway offline",
					zap.String("satellite_id", sat.ID.String()),
					zap.Error(err),
				)
				continue
			}
			offlineCount++
			m.logger.Warn("gateway marked offline (missed heartbeat)",
				zap.String("name", sat.Name),
				zap.String("satellite_id", sat.ID.String()),
				zap.Time("last_heartbeat", *sat.LastHeartbeat),
			)
		}
	}

	if offlineCount > 0 {
		m.logger.Info("gateway health check complete",
			zap.Int("total", len(satellites)),
			zap.Int("marked_offline", offlineCount),
		)
	}
}
