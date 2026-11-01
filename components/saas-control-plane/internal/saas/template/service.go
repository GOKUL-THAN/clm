package template

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

// Service defines template authoring and automated document assembly
type Service interface {
	ListTemplates(ctx context.Context) ([]*models.ContractTemplate, error)
	CreateTemplate(ctx context.Context, tpl *models.ContractTemplate) (*models.ContractTemplate, error)
	ListClauses(ctx context.Context, category string) ([]*models.Clause, error)
	CreateClause(ctx context.Context, cl *models.Clause) (*models.Clause, error)
	AssembleDocument(ctx context.Context, templateID uuid.UUID, variables map[string]string, clauseIDs []uuid.UUID) (string, error)
}

type serviceImpl struct {
	logger    *zap.Logger
	mu        sync.RWMutex
	templates map[uuid.UUID]*models.ContractTemplate
	clauses   map[uuid.UUID]*models.Clause
}

// NewService instantiates template service
func NewService(logger *zap.Logger) Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &serviceImpl{
		logger:    logger,
		templates: make(map[uuid.UUID]*models.ContractTemplate),
		clauses:   make(map[uuid.UUID]*models.Clause),
	}
}

func (s *serviceImpl) ListTemplates(ctx context.Context) ([]*models.ContractTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*models.ContractTemplate
	for _, t := range s.templates {
		out = append(out, t)
	}
	return out, nil
}

func (s *serviceImpl) CreateTemplate(ctx context.Context, tpl *models.ContractTemplate) (*models.ContractTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tpl.ID == uuid.Nil {
		tpl.ID = uuid.New()
	}
	tpl.CreatedAt = time.Now().UTC()
	tpl.UpdatedAt = tpl.CreatedAt
	s.templates[tpl.ID] = tpl
	return tpl, nil
}

func (s *serviceImpl) ListClauses(ctx context.Context, category string) ([]*models.Clause, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*models.Clause
	for _, c := range s.clauses {
		if category != "" && !strings.EqualFold(c.Category, category) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *serviceImpl) CreateClause(ctx context.Context, cl *models.Clause) (*models.Clause, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cl.ID == uuid.Nil {
		cl.ID = uuid.New()
	}
	cl.CreatedAt = time.Now().UTC()
	cl.UpdatedAt = cl.CreatedAt
	s.clauses[cl.ID] = cl
	return cl, nil
}

func (s *serviceImpl) AssembleDocument(ctx context.Context, templateID uuid.UUID, variables map[string]string, clauseIDs []uuid.UUID) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tpl, ok := s.templates[templateID]
	if !ok {
		return "", errors.New("template not found")
	}

	body := tpl.ContentBody
	for k, v := range variables {
		body = strings.ReplaceAll(body, "{{"+k+"}}", v)
	}

	if len(clauseIDs) > 0 {
		body += "\n\n--- PRE-APPROVED ATTACHED CLAUSES ---\n"
		for _, cid := range clauseIDs {
			if cl, found := s.clauses[cid]; found {
				body += fmt.Sprintf("\n[%s]\n%s\n", cl.Title, cl.StandardText)
			}
		}
	}

	return body, nil
}

