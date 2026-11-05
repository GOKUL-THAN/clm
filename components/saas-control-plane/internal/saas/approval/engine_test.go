package approval

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

type approvalEngineRepository struct {
	rules    []*models.ApprovalRule
	rulesErr error
	created  *models.ApprovalRequest
	getErr   error
}

func (r *approvalEngineRepository) Create(_ context.Context, req *models.ApprovalRequest) error {
	r.created = req
	return nil
}

func (r *approvalEngineRepository) GetByID(context.Context, uuid.UUID, uuid.UUID) (*models.ApprovalRequest, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return nil, ErrRequestNotFound
}

func (r *approvalEngineRepository) ListPending(context.Context, uuid.UUID) ([]*models.ApprovalRequest, error) {
	return nil, nil
}

func (r *approvalEngineRepository) Update(context.Context, *models.ApprovalRequest) error {
	return nil
}

func (r *approvalEngineRepository) GetRules(context.Context, uuid.UUID) ([]*models.ApprovalRule, error) {
	return r.rules, r.rulesErr
}

func TestSubmitRequestFailsWhenApprovalRulesCannotBeLoaded(t *testing.T) {
	rulesErr := errors.New("approval rules database unavailable")
	repo := &approvalEngineRepository{rulesErr: rulesErr}
	engine := NewEngine(repo, zap.NewNop())

	_, err := engine.SubmitRequest(context.Background(), uuid.New(), &SubmitInput{
		RequestType:   "cert_renew",
		Environment:   "dev",
		Justification: "test",
	})
	if !errors.Is(err, rulesErr) {
		t.Fatalf("SubmitRequest error = %v, want %v", err, rulesErr)
	}
	if repo.created != nil {
		t.Fatal("approval request was created despite rule lookup failure")
	}
}

func TestSubmitRequestUsesDefaultsWhenTenantHasNoApprovalRules(t *testing.T) {
	repo := &approvalEngineRepository{}
	engine := NewEngine(repo, zap.NewNop())

	request, err := engine.SubmitRequest(context.Background(), uuid.New(), &SubmitInput{
		RequestType:   "cert_renew",
		Environment:   "dev",
		Justification: "test",
	})
	if err != nil {
		t.Fatalf("SubmitRequest returned error: %v", err)
	}
	if request.Level != models.ApprovalAuto || request.Status != models.ApprovalApproved {
		t.Fatalf("default approval = level %q status %q, want auto/approved", request.Level, request.Status)
	}
}

func TestApprovalActionsPropagateRepositoryLookupFailures(t *testing.T) {
	repositoryErr := errors.New("approval database unavailable")
	actions := map[string]func(*Engine) error{
		"approve": func(engine *Engine) error {
			_, err := engine.Approve(context.Background(), uuid.New(), uuid.New(), uuid.New(), "")
			return err
		},
		"reject": func(engine *Engine) error {
			_, err := engine.Reject(context.Background(), uuid.New(), uuid.New(), uuid.New(), "")
			return err
		},
		"escalate": func(engine *Engine) error {
			_, err := engine.Escalate(context.Background(), uuid.New(), uuid.New(), uuid.New(), "")
			return err
		},
	}

	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			engine := NewEngine(&approvalEngineRepository{getErr: repositoryErr}, zap.NewNop())
			if err := action(engine); !errors.Is(err, repositoryErr) {
				t.Fatalf("action error = %v, want underlying repository error %v", err, repositoryErr)
			}
		})
	}
}

func TestApprovalActionsMapRepositoryNotFound(t *testing.T) {
	actions := map[string]func(*Engine) error{
		"approve": func(engine *Engine) error {
			_, err := engine.Approve(context.Background(), uuid.New(), uuid.New(), uuid.New(), "")
			return err
		},
		"reject": func(engine *Engine) error {
			_, err := engine.Reject(context.Background(), uuid.New(), uuid.New(), uuid.New(), "")
			return err
		},
		"escalate": func(engine *Engine) error {
			_, err := engine.Escalate(context.Background(), uuid.New(), uuid.New(), uuid.New(), "")
			return err
		},
	}

	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			engine := NewEngine(&approvalEngineRepository{getErr: core.ErrNotFound}, zap.NewNop())
			if err := action(engine); !errors.Is(err, ErrRequestNotFound) {
				t.Fatalf("action error = %v, want %v", err, ErrRequestNotFound)
			}
		})
	}
}
