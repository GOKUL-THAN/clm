package obligation

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// Service defines obligation tracking and SLA management
type Service interface {
	ListObligations(ctx context.Context, contractID *uuid.UUID) ([]*models.ContractObligation, error)
	CreateObligation(ctx context.Context, contractID uuid.UUID, obl *models.ContractObligation) (*models.ContractObligation, error)
	FulfillObligation(ctx context.Context, oblID uuid.UUID, verifiedBy, notes string) (*models.ContractObligation, error)
	CalculateComplianceRate(ctx context.Context, contractID *uuid.UUID) (float64, error)
}

type serviceImpl struct {
	logger      *zap.Logger
	mu          sync.RWMutex
	obligations map[uuid.UUID]*models.ContractObligation
}

// NewService instantiates obligation service
func NewService(logger *zap.Logger) Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &serviceImpl{
		logger:      logger,
		obligations: make(map[uuid.UUID]*models.ContractObligation),
	}
}

func (s *serviceImpl) ListObligations(ctx context.Context, contractID *uuid.UUID) ([]*models.ContractObligation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*models.ContractObligation
	for _, o := range s.obligations {
		if contractID != nil && o.ContractID != *contractID {
			continue
		}
		out = append(out, o)
	}
	return out, nil
}

func (s *serviceImpl) CreateObligation(ctx context.Context, contractID uuid.UUID, obl *models.ContractObligation) (*models.ContractObligation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if obl.ID == uuid.Nil {
		obl.ID = uuid.New()
	}
	obl.ContractID = contractID
	if obl.Status == "" {
		obl.Status = "in_progress"
	}
	obl.CreatedAt = time.Now().UTC()
	obl.UpdatedAt = obl.CreatedAt
	s.obligations[obl.ID] = obl
	return obl, nil
}

func (s *serviceImpl) FulfillObligation(ctx context.Context, oblID uuid.UUID, verifiedBy, notes string) (*models.ContractObligation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	o, ok := s.obligations[oblID]
	if !ok {
		return nil, errors.New("obligation not found")
	}

	now := time.Now().UTC()
	o.Status = "fulfilled"
	o.CompletedAt = &now
	o.VerifiedBy = verifiedBy
	o.EvidenceNotes = notes
	o.UpdatedAt = now
	return o, nil
}

func (s *serviceImpl) CalculateComplianceRate(ctx context.Context, contractID *uuid.UUID) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := 0
	fulfilled := 0

	for _, o := range s.obligations {
		if contractID != nil && o.ContractID != *contractID {
			continue
		}
		total++
		if o.Status == "fulfilled" {
			fulfilled++
		}
	}

	if total == 0 {
		return 100.0, nil
	}
	return (float64(fulfilled) / float64(total)) * 100.0, nil
}

