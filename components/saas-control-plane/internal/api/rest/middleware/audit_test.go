package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	auditservice "github.com/m2p/clm/internal/services/audit"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

type auditCaptureRepository struct {
	event *models.AuditEvent
	err   error
}

func (r *auditCaptureRepository) Append(_ context.Context, event *models.AuditEvent) error {
	r.event = event
	return r.err
}

func (r *auditCaptureRepository) ListByTenant(_ context.Context, _ uuid.UUID, _ models.AuditFilter) ([]*models.AuditEvent, error) {
	return nil, nil
}

func TestMutationAuditRecordsSafeTenantScopedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &auditCaptureRepository{}
	tenantID := uuid.New()
	actorID := uuid.New()
	targetID := uuid.New()
	router := gin.New()
	router.Use(testIdentity(tenantID, actorID, "tenant_admin"))
	router.Use(MutationAudit(auditservice.NewService(repo), zap.NewNop()))
	router.POST("/api/v1/users/:id/role", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"password": "response-is-not-audited"})
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/users/"+targetID.String()+"/role", strings.NewReader(`{"password":"request-is-not-audited"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "request-123")
	request.RemoteAddr = "192.0.2.10:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repo.event == nil {
		t.Fatal("audit event was not appended")
	}
	if repo.event.TenantID != tenantID || repo.event.ActorID != actorID || repo.event.RequestID != "request-123" {
		t.Fatalf("unexpected identity fields: %+v", repo.event)
	}
	if repo.event.Action != "POST /api/v1/users/:id/role" || repo.event.ResourceType != "users" || repo.event.ResourceID != targetID.String() || repo.event.Outcome != "success" {
		t.Fatalf("unexpected event: %+v", repo.event)
	}
	metadata := string(repo.event.Metadata)
	if strings.Contains(metadata, "password") || strings.Contains(metadata, "request-is-not-audited") || strings.Contains(metadata, "response-is-not-audited") {
		t.Fatalf("audit metadata leaked request or response data: %s", metadata)
	}
}

func TestMutationAuditWriteFailureIsVisible(t *testing.T) {
	repo := &auditCaptureRepository{err: errors.New("audit unavailable")}
	router := gin.New()
	router.Use(testIdentity(uuid.New(), uuid.New(), "tenant_admin"))
	router.Use(MutationAudit(auditservice.NewService(repo), zap.NewNop()))
	router.POST("/api/v1/users", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"id": uuid.NewString()})
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/users", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Audit-Write-Failed") != "true" {
		t.Fatal("audit failure response header is missing")
	}
	if strings.Contains(response.Body.String(), `"id"`) {
		t.Fatalf("buffered success response escaped after audit failure: %s", response.Body.String())
	}
}

func testIdentity(tenantID, actorID uuid.UUID, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		c.Set("user_id", actorID)
		c.Set("role", role)
		c.Next()
	}
}

func TestMutationAuditReplacesUnsafeRequestIDBeforeMutation(t *testing.T) {
	repo := &auditCaptureRepository{}
	router := gin.New()
	router.Use(testIdentity(uuid.New(), uuid.New(), "tenant_admin"))
	router.Use(MutationAudit(auditservice.NewService(repo), zap.NewNop()))
	router.POST("/api/v1/users", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodPost, "/api/v1/users", nil)
	request.Header.Set("X-Request-ID", strings.Repeat("x", 129)+"\nunsafe")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repo.event == nil || len(repo.event.RequestID) > 128 || repo.event.RequestID == request.Header.Get("X-Request-ID") {
		t.Fatalf("unsafe request ID was not replaced: %+v", repo.event)
	}
	if response.Header().Get("X-Request-ID") != repo.event.RequestID {
		t.Fatal("response and audit request IDs differ")
	}
}

func TestMutationAuditPreservesRejectedTenantlessPlatformRequest(t *testing.T) {
	repo := &auditCaptureRepository{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uuid.New())
		c.Set("role", "admin")
		c.Next()
	})
	router.Use(MutationAudit(auditservice.NewService(repo), zap.NewNop()))
	router.POST("/api/v1/users", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/users", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid request") {
		t.Fatalf("status = %d body = %s, want original 400", response.Code, response.Body.String())
	}
	if repo.event != nil {
		t.Fatal("tenantless rejected request should not create a falsely scoped event")
	}
}
