package sso

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/config"
	"go.uber.org/zap"
)

func TestSSOConfigTenantIsolation(t *testing.T) {
	service := NewService(nil, nil, config.JWTConfig{}, zap.NewNop())
	tenantA := uuid.New()
	tenantB := uuid.New()
	cfg := &SSOConfig{Provider: ProviderOIDC, Enabled: true}

	if err := service.RegisterTenantSSO(tenantA, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.TenantID != tenantA {
		t.Fatalf("config tenant = %s, want %s", cfg.TenantID, tenantA)
	}
	if _, err := service.GetTenantSSO(tenantB); !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("cross-tenant config read error = %v", err)
	}
	if providers := service.ListProviders(tenantB); len(providers) != 0 {
		t.Fatalf("cross-tenant provider list returned %+v", providers)
	}
	service.RemoveTenantSSO(tenantB)
	if _, err := service.GetTenantSSO(tenantA); err != nil {
		t.Fatalf("cross-tenant delete removed owner config: %v", err)
	}
}
