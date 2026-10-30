package audit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
)

type memoryAuditRepo struct{ events []*models.AuditEvent }

func (r *memoryAuditRepo) Append(_ context.Context, event *models.AuditEvent) error {
	r.events = append(r.events, event)
	return nil
}
func (r *memoryAuditRepo) ListByTenant(_ context.Context, tenantID uuid.UUID, _ models.AuditFilter) ([]*models.AuditEvent, error) {
	var out []*models.AuditEvent
	for _, event := range r.events {
		if event.TenantID == tenantID {
			out = append(out, event)
		}
	}
	return out, nil
}
func TestAuditServiceTenantScopeAndSafeMetadata(t *testing.T) {
	repo := &memoryAuditRepo{}
	service := NewService(repo)
	tenantA, tenantB := uuid.New(), uuid.New()
	event := &models.AuditEvent{TenantID: tenantA, RequestID: "request", ActorID: uuid.New(), ActorRole: "tenant_admin", Action: "POST /api/v1/users", ResourceType: "users", Outcome: "success", SourceIP: "127.0.0.1", Metadata: json.RawMessage(`{"field":"role"}`)}
	if err := service.Record(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	events, err := service.List(context.Background(), tenantB, models.AuditFilter{})
	if err != nil || len(events) != 0 {
		t.Fatalf("cross tenant events=%d err=%v", len(events), err)
	}
	event.ID = uuid.Nil
	event.Metadata = json.RawMessage(`{"access_token":"secret"}`)
	if err := service.Record(context.Background(), event); err == nil {
		t.Fatal("unsafe metadata was accepted")
	}
}

func TestAuditServiceRejectsNestedSecretsAndSchemaOverflow(t *testing.T) {
	service := NewService(&memoryAuditRepo{})
	base := models.AuditEvent{TenantID: uuid.New(), RequestID: "request", ActorID: uuid.New(), ActorRole: "tenant_admin", Action: "POST /api/v1/users", ResourceType: "users", Outcome: "success", SourceIP: "127.0.0.1"}

	nested := base
	nested.Metadata = json.RawMessage(`{"changes":{"access_token":"must-not-be-recorded"}}`)
	if err := service.Record(context.Background(), &nested); err == nil {
		t.Fatal("nested secret metadata was accepted")
	}

	overflow := base
	overflow.RequestID = string(make([]byte, 129))
	if err := service.Record(context.Background(), &overflow); err == nil {
		t.Fatal("oversized request ID was accepted")
	}
}
