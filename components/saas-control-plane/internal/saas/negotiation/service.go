package negotiation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// Service defines negotiation and redline collaboration operations
type Service interface {
	CreateSession(ctx context.Context, contractID uuid.UUID, turnaroundDeadline time.Time) (*models.ContractRedline, error)
	ListRedlines(ctx context.Context, contractID uuid.UUID) ([]*models.ContractRedline, error)
	ProposeRedline(ctx context.Context, contractID uuid.UUID, redline *models.ContractRedline) (*models.ContractRedline, error)
	ResolveRedline(ctx context.Context, redlineID uuid.UUID, accepted bool, comment string) (*models.ContractRedline, error)
	GenerateCounterpartyToken(ctx context.Context, contractID uuid.UUID) (string, error)
}

type serviceImpl struct {
	logger   *zap.Logger
	mu       sync.RWMutex
	redlines map[uuid.UUID]*models.ContractRedline
	tokens   map[uuid.UUID]string
}

// NewService creates a negotiation service instance
func NewService(logger *zap.Logger) Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &serviceImpl{
		logger:   logger,
		redlines: make(map[uuid.UUID]*models.ContractRedline),
		tokens:   make(map[uuid.UUID]string),
	}
}

func (s *serviceImpl) CreateSession(ctx context.Context, contractID uuid.UUID, turnaroundDeadline time.Time) (*models.ContractRedline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tok, _ := s.generateRandomToken()
	s.tokens[contractID] = tok

	r := &models.ContractRedline{
		ID:           uuid.New(),
		ContractID:   contractID,
		AuthorName:   "Legal Reviewer",
		AuthorType:   "internal",
		ChangeType:   "baseline",
		ClauseTitle:  "Initial Baseline Negotiation Session",
		ProposedText: "Workspace opened for counterparty redlines",
		Status:       "proposed",
		CreatedAt:    time.Now().UTC(),
	}
	s.redlines[r.ID] = r
	return r, nil
}

func (s *serviceImpl) ListRedlines(ctx context.Context, contractID uuid.UUID) ([]*models.ContractRedline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*models.ContractRedline
	for _, r := range s.redlines {
		if r.ContractID == contractID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *serviceImpl) ProposeRedline(ctx context.Context, contractID uuid.UUID, redline *models.ContractRedline) (*models.ContractRedline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if redline.ID == uuid.Nil {
		redline.ID = uuid.New()
	}
	redline.ContractID = contractID
	redline.Status = "proposed"
	redline.CreatedAt = time.Now().UTC()
	s.redlines[redline.ID] = redline
	return redline, nil
}

func (s *serviceImpl) ResolveRedline(ctx context.Context, redlineID uuid.UUID, accepted bool, comment string) (*models.ContractRedline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.redlines[redlineID]
	if !ok {
		return nil, errors.New("redline not found")
	}
	if accepted {
		r.Status = "accepted"
	} else {
		r.Status = "rejected"
	}
	r.ReviewComments = comment
	now := time.Now().UTC()
	r.ResolvedAt = &now
	return r, nil
}

func (s *serviceImpl) GenerateCounterpartyToken(ctx context.Context, contractID uuid.UUID) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tok, ok := s.tokens[contractID]; ok {
		return tok, nil
	}
	tok, err := s.generateRandomToken()
	if err != nil {
		return "", err
	}
	s.tokens[contractID] = tok
	return tok, nil
}

func (s *serviceImpl) generateRandomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

