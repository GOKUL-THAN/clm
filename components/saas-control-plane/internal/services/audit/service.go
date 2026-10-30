package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
)

var ErrInvalidEvent = errors.New("invalid audit event")

type Service struct{ repo core.AuditRepository }

func NewService(repo core.AuditRepository) *Service { return &Service{repo: repo} }

func (s *Service) Record(ctx context.Context, event *models.AuditEvent) error {
	if event == nil || event.TenantID == uuid.Nil || event.ActorID == uuid.Nil ||
		!bounded(event.RequestID, 128) || !bounded(event.ActorRole, 64) ||
		!bounded(event.Action, 128) || !bounded(event.ResourceType, 128) ||
		len(event.ResourceID) > 255 || !bounded(event.SourceIP, 64) ||
		(event.Outcome != "success" && event.Outcome != "failure") {
		return ErrInvalidEvent
	}
	if len(event.Metadata) == 0 {
		event.Metadata = json.RawMessage(`{}`)
	}
	if len(event.Metadata) > 4096 || !json.Valid(event.Metadata) {
		return ErrInvalidEvent
	}
	var metadata map[string]any
	if err := json.Unmarshal(event.Metadata, &metadata); err != nil {
		return ErrInvalidEvent
	}
	allowed := map[string]struct{}{
		"field": {}, "fields": {}, "change": {}, "changes": {},
		"reason": {}, "status": {},
	}
	for key, value := range metadata {
		if _, ok := allowed[key]; !ok || containsSensitiveMetadata(key, value) {
			return ErrInvalidEvent
		}
	}
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	return s.repo.Append(ctx, event)
}

func bounded(value string, maximum int) bool {
	length := len(strings.TrimSpace(value))
	return length > 0 && length <= maximum
}

func containsSensitiveMetadata(key string, value any) bool {
	lower := strings.ToLower(key)
	for _, forbidden := range []string{"password", "secret", "credential", "token", "authorization", "body"} {
		if strings.Contains(lower, forbidden) {
			return true
		}
	}
	switch typed := value.(type) {
	case map[string]any:
		for nestedKey, nestedValue := range typed {
			if containsSensitiveMetadata(nestedKey, nestedValue) {
				return true
			}
		}
	case []any:
		for _, nestedValue := range typed {
			if containsSensitiveMetadata("value", nestedValue) {
				return true
			}
		}
	}
	return false
}
func (s *Service) List(ctx context.Context, tenantID uuid.UUID, filter models.AuditFilter) ([]*models.AuditEvent, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidEvent
	}
	return s.repo.ListByTenant(ctx, tenantID, filter)
}
