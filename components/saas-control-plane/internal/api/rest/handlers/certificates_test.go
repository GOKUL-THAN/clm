package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
)

type certificateLookupRepository struct {
	core.CertificateRepository
	err error
}

func (r *certificateLookupRepository) GetByIDForTenant(context.Context, uuid.UUID, uuid.UUID) (*models.Certificate, error) {
	return nil, r.err
}

func TestCertificateLookupHandlersDistinguishNotFoundFromRepositoryFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		method string
		path   string
		setup  func(*gin.Engine, *CertHandler)
		err    error
		status int
	}{
		{
			name:   "get missing certificate",
			method: http.MethodGet,
			path:   "/certificates/",
			setup: func(router *gin.Engine, handler *CertHandler) {
				router.GET("/certificates/:id", handler.GetCertificate)
			},
			err:    core.ErrNotFound,
			status: http.StatusNotFound,
		},
		{
			name:   "get repository failure",
			method: http.MethodGet,
			path:   "/certificates/",
			setup: func(router *gin.Engine, handler *CertHandler) {
				router.GET("/certificates/:id", handler.GetCertificate)
			},
			err:    errors.New("database unavailable"),
			status: http.StatusInternalServerError,
		},
		{
			name:   "revoke missing certificate",
			method: http.MethodPost,
			path:   "/certificates/",
			setup: func(router *gin.Engine, handler *CertHandler) {
				router.POST("/certificates/:id/revoke", handler.RevokeCertificate)
			},
			err:    core.ErrNotFound,
			status: http.StatusNotFound,
		},
		{
			name:   "revoke repository failure",
			method: http.MethodPost,
			path:   "/certificates/",
			setup: func(router *gin.Engine, handler *CertHandler) {
				router.POST("/certificates/:id/revoke", handler.RevokeCertificate)
			},
			err:    errors.New("database unavailable"),
			status: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &certificateLookupRepository{err: test.err}
			handler := NewCertHandler(repo, nil)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("tenant_id", uuid.New())
				c.Next()
			})
			test.setup(router, handler)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path+uuid.NewString()+"/revoke", nil)
			if test.method == http.MethodGet {
				request = httptest.NewRequest(test.method, test.path+uuid.NewString(), nil)
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}
}
