package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGatewayDeploymentCreateHandlerFailsClosedInProduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	handler := gatewayDeploymentCreateHandler("production", func(c *gin.Context) {
		called = true
		c.Status(http.StatusCreated)
	})
	router := gin.New()
	router.POST("/api/v1/satellites/deploy", handler)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/satellites/deploy", nil))
	if called {
		t.Fatal("legacy cloud deployment handler ran in production")
	}
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "pre-provisioned satellite credential") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGatewayDeploymentCreateHandlerFailsClosedOutsideProduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	handler := gatewayDeploymentCreateHandler("development", func(c *gin.Context) {
		called = true
		c.Status(http.StatusCreated)
	})
	router := gin.New()
	router.POST("/api/v1/satellites/deploy", handler)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/satellites/deploy", nil))
	if called {
		t.Fatal("legacy cloud deployment handler ran outside production")
	}
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "pre-provisioned satellite credential") {
		t.Fatalf("called=%v status=%d body=%s", called, recorder.Code, recorder.Body.String())
	}
}
