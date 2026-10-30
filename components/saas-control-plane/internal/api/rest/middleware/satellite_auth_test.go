package middleware

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/config"
	"github.com/m2p/clm/pkg/models"
)

type fakeSatelliteAuth struct {
	id, tenant uuid.UUID
	token      string
}

func (f fakeSatelliteAuth) AuthenticateSatellite(_ context.Context, id uuid.UUID, token string) (*models.SatelliteIdentity, error) {
	if id != f.id || token != f.token {
		return nil, errors.New("denied")
	}
	return &models.SatelliteIdentity{SatelliteID: f.id, TenantID: f.tenant}, nil
}
func (f fakeSatelliteAuth) GetByID(_ context.Context, id uuid.UUID) (*models.Satellite, error) {
	if id != f.id {
		return nil, errors.New("missing")
	}
	return &models.Satellite{ID: f.id, TenantID: f.tenant}, nil
}

func TestSatelliteAuthBindsCredentialToRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	satID, tenantID := uuid.New(), uuid.New()
	auth := fakeSatelliteAuth{id: satID, tenant: tenantID, token: "correct"}
	r := gin.New()
	r.GET("/satellites/:id", SatelliteAuth(auth, config.ServerConfig{}), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, tc := range []struct {
		name, id, token string
		want            int
	}{{"valid", satID.String(), "correct", 204}, {"wrong route", uuid.NewString(), "correct", 401}, {"wrong credential", satID.String(), "wrong", 401}, {"missing", satID.String(), "", 401}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/satellites/"+tc.id, nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestCertificateSANBinding(t *testing.T) {
	satID, tenantID := uuid.New(), uuid.New()
	matching, _ := url.Parse("spiffe://clm/tenants/" + tenantID.String() + "/satellites/" + satID.String())
	wrong, _ := url.Parse("spiffe://clm/tenants/" + tenantID.String() + "/satellites/" + uuid.NewString())
	if !certificateMatches(&x509.Certificate{URIs: []*url.URL{matching}}, tenantID, satID) {
		t.Fatal("matching URI SAN was rejected")
	}
	if certificateMatches(&x509.Certificate{URIs: []*url.URL{wrong}}, tenantID, satID) {
		t.Fatal("wrong satellite URI SAN was accepted")
	}
}

func TestProductionCannotUseLegacyGatewayAuth(t *testing.T) {
	cfg := config.ServerConfig{Env: "production", Gateway: config.GatewayTransportConfig{AllowInsecureLegacy: true}}
	if cfg.Validate() == nil {
		t.Fatal("production legacy gateway auth was accepted")
	}
}

func TestBearerCredentialRejectsOversizedInput(t *testing.T) {
	if _, ok := bearerCredential("Bearer " + strings.Repeat("x", maxSatelliteCredentialBytes+1)); ok {
		t.Fatal("oversized satellite credential was accepted")
	}
	if _, ok := bearerCredential(strings.Repeat("x", maxAuthorizationHeaderBytes+1)); ok {
		t.Fatal("oversized authorization header was accepted")
	}
}

type blockingSatelliteAuth struct {
	id, tenant uuid.UUID
	entered    chan struct{}
	release    chan struct{}
}

func (f *blockingSatelliteAuth) AuthenticateSatellite(ctx context.Context, id uuid.UUID, _ string) (*models.SatelliteIdentity, error) {
	if id != f.id {
		return nil, errors.New("denied")
	}
	select {
	case f.entered <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-f.release:
		return &models.SatelliteIdentity{SatelliteID: f.id, TenantID: f.tenant}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *blockingSatelliteAuth) GetByID(context.Context, uuid.UUID) (*models.Satellite, error) {
	return nil, errors.New("not used")
}

func TestSatelliteAuthBoundsConcurrentCredentialVerification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	satID, tenantID := uuid.New(), uuid.New()
	auth := &blockingSatelliteAuth{id: satID, tenant: tenantID, entered: make(chan struct{}, maxConcurrentSatelliteAuth), release: make(chan struct{})}
	r := gin.New()
	r.GET("/satellites/:id", SatelliteAuth(auth, config.ServerConfig{}), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	done := make(chan struct{}, maxConcurrentSatelliteAuth)
	for i := 0; i < maxConcurrentSatelliteAuth; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/satellites/"+satID.String(), nil)
			req.Header.Set("Authorization", "Bearer bounded")
			r.ServeHTTP(httptest.NewRecorder(), req)
			done <- struct{}{}
		}()
	}
	for i := 0; i < maxConcurrentSatelliteAuth; i++ {
		select {
		case <-auth.entered:
		case <-time.After(time.Second):
			t.Fatal("credential verifier did not fill bounded slots")
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/satellites/"+satID.String(), nil)
	req.Header.Set("Authorization", "Bearer overflow")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
	close(auth.release)
	for i := 0; i < maxConcurrentSatelliteAuth; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("blocked authentication did not finish")
		}
	}
}
