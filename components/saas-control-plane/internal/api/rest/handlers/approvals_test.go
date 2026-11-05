package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/internal/saas/approval"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

type approvalHandlerRepository struct {
	request    *models.ApprovalRequest
	getErr     error
	updated    bool
	listTenant uuid.UUID
}

func (r *approvalHandlerRepository) Create(_ context.Context, req *models.ApprovalRequest) error {
	r.request = req
	return nil
}

func (r *approvalHandlerRepository) GetByID(_ context.Context, tenantID, id uuid.UUID) (*models.ApprovalRequest, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.request == nil || r.request.ID != id || r.request.TenantID != tenantID {
		return nil, approval.ErrRequestNotFound
	}
	copy := *r.request
	return &copy, nil
}

func (r *approvalHandlerRepository) ListPending(_ context.Context, tenantID uuid.UUID) ([]*models.ApprovalRequest, error) {
	r.listTenant = tenantID
	return nil, nil
}

func (r *approvalHandlerRepository) Update(_ context.Context, req *models.ApprovalRequest) error {
	r.updated = true
	r.request = req
	return nil
}

func (r *approvalHandlerRepository) GetRules(context.Context, uuid.UUID) ([]*models.ApprovalRule, error) {
	return nil, nil
}

func TestApprovalHandlersEnforceTenantAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	callerTenant := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	targetTenant := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	requestID := uuid.New()
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		setup  func(*gin.Engine, *ApprovalHandler)
	}{
		{
			name:   "get request",
			method: http.MethodGet,
			path:   "/approvals/" + requestID.String(),
			status: http.StatusNotFound,
			setup: func(router *gin.Engine, handler *ApprovalHandler) {
				router.GET("/approvals/:id", handler.GetRequest)
			},
		},
		{
			name:   "approve request",
			method: http.MethodPost,
			path:   "/approvals/" + requestID.String() + "/approve",
			body:   `{"comment":"approved"}`,
			status: http.StatusNotFound,
			setup: func(router *gin.Engine, handler *ApprovalHandler) {
				router.POST("/approvals/:id/approve", handler.Approve)
			},
		},
		{
			name:   "reject request",
			method: http.MethodPost,
			path:   "/approvals/" + requestID.String() + "/reject",
			body:   `{"comment":"rejected"}`,
			status: http.StatusNotFound,
			setup: func(router *gin.Engine, handler *ApprovalHandler) {
				router.POST("/approvals/:id/reject", handler.Reject)
			},
		},
		{
			name:   "escalate request",
			method: http.MethodPost,
			path:   "/approvals/" + requestID.String() + "/escalate",
			body:   `{"comment":"escalate"}`,
			status: http.StatusNotFound,
			setup: func(router *gin.Engine, handler *ApprovalHandler) {
				router.POST("/approvals/:id/escalate", handler.Escalate)
			},
		},
		{
			name:   "list another tenant",
			method: http.MethodGet,
			path:   "/approvals/pending?tenant_id=" + targetTenant.String(),
			status: http.StatusForbidden,
			setup: func(router *gin.Engine, handler *ApprovalHandler) {
				router.GET("/approvals/pending", handler.ListPending)
			},
		},
		{
			name:   "submit for another tenant",
			method: http.MethodPost,
			path:   "/approvals?tenant_id=" + targetTenant.String(),
			body:   `{"request_type":"cert_create","justification":"test"}`,
			status: http.StatusForbidden,
			setup: func(router *gin.Engine, handler *ApprovalHandler) {
				router.POST("/approvals", handler.Submit)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &approvalHandlerRepository{
				request: &models.ApprovalRequest{
					ID:          requestID,
					TenantID:    targetTenant,
					Status:      models.ApprovalPending,
					RequestedBy: uuid.New(),
				},
			}
			handler := NewApprovalHandler(approval.NewEngine(repo, zap.NewNop()))
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("tenant_id", callerTenant)
				c.Set("user_id", uuid.New())
				c.Set("role", "approver")
				c.Next()
			})
			test.setup(router, handler)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			router.ServeHTTP(recorder, request)

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, test.status, recorder.Body.String())
			}
			if repo.updated {
				t.Fatal("cross-tenant request modified the approval")
			}
			if test.name == "submit for another tenant" && repo.request.ID != requestID {
				t.Fatal("cross-tenant submit created an approval")
			}
			if test.name == "list another tenant" && repo.listTenant != uuid.Nil {
				t.Fatalf("cross-tenant list reached repository with tenant %s", repo.listTenant)
			}
		})
	}
}

func TestApprovalGetRequestReturnsServerErrorForRepositoryFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repositoryErr := errors.New("database unavailable")
	repo := &approvalHandlerRepository{getErr: repositoryErr}
	handler := NewApprovalHandler(approval.NewEngine(repo, zap.NewNop()))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", uuid.New())
		c.Next()
	})
	router.GET("/approvals/:id", handler.GetRequest)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/approvals/"+uuid.NewString(), nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
}

func TestApprovalGetRequestReturnsNotFoundForMissingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &approvalHandlerRepository{getErr: core.ErrNotFound}
	handler := NewApprovalHandler(approval.NewEngine(repo, zap.NewNop()))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", uuid.New())
		c.Next()
	})
	router.GET("/approvals/:id", handler.GetRequest)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/approvals/"+uuid.NewString(), nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}
