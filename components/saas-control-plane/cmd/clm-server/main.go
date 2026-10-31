package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/m2p/clm/internal/api/rest"
	"github.com/m2p/clm/internal/saas/approval"
	"github.com/m2p/clm/internal/saas/cloudaccounts"
	"github.com/m2p/clm/internal/saas/deploy"
	"github.com/m2p/clm/internal/saas/identity"
	"github.com/m2p/clm/internal/saas/onboarding"
	"github.com/m2p/clm/internal/saas/sso"
	"github.com/m2p/clm/internal/saas/tenant"
	"github.com/m2p/clm/internal/saas/users"
	auditservice "github.com/m2p/clm/internal/services/audit"
	gatewayhealth "github.com/m2p/clm/internal/services/gateway-health"
	"github.com/m2p/clm/internal/store/postgres"
	"github.com/m2p/clm/pkg/config"
	"github.com/m2p/clm/pkg/utils"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfgPath := os.Getenv("CLM_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/server.yaml"
	}

	cfg, err := config.LoadServerConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := utils.NewLogger(cfg.LogLevel)
	defer logger.Sync()

	logger.Info("starting CLM SaaS control plane",
		zap.String("env", cfg.Env),
		zap.Int("port", cfg.Port),
	)

	// Initialize database
	db, err := postgres.NewDB(&cfg.Database)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Initialize repositories (control plane scope only)
	userRepo := postgres.NewUserRepository(db)
	tenantRepo := postgres.NewTenantRepository(db)
	satRepo := postgres.NewSatelliteRepository(db)
	auditRepo := postgres.NewAuditRepository(db)
	cloudAccountRepo := postgres.NewCloudAccountRepository(db)
	approvalRepo := postgres.NewApprovalRepository(db)

	// Initialize services
	userService := users.NewService(userRepo, cfg.JWT, logger)
	tenantService := tenant.NewService(tenantRepo, logger)
	ssoService := sso.NewService(userRepo, tenantRepo, cfg.JWT, logger)
	auditService := auditservice.NewService(auditRepo)
	approvalEngine := approval.NewEngine(approvalRepo, logger)
	onboardingService := onboarding.NewService(tenantRepo, userRepo, satRepo, logger)
	var cloudAccountService *cloudaccounts.Service
	if cfg.CloudAccounts.MasterKey != "" {
		cloudAccountService, err = cloudaccounts.NewService(cloudAccountRepo, cfg.CloudAccounts.MasterKey)
		if err != nil {
			logger.Fatal("invalid cloud account encryption configuration", zap.Error(err))
		}
	} else {
		logger.Warn("cloud account API disabled: encryption master key is not configured")
	}

	// Initialize deploy service with provisioners
	deployService := deploy.NewService(satRepo, logger)
	deployService.RegisterProvisioner(deploy.NewLightsailProvisioner(logger))
	deployService.RegisterProvisioner(deploy.NewEC2Provisioner(logger))

	// Keycloak realm administration — powers the Admin Portal's Identity
	// Management page. Optional: when KEYCLOAK_ADMIN_PASS is unset the
	// /api/v1/identity routes are simply not registered.
	var identityService *identity.Service
	if pass := os.Getenv("KEYCLOAK_ADMIN_PASS"); pass != "" {
		kcClient := identity.NewClient(identity.Config{
			BaseURL:    envOr("KEYCLOAK_URL", "http://keycloak:8180/auth"),
			Realm:      envOr("KEYCLOAK_REALM", "clm"),
			AdminRealm: envOr("KEYCLOAK_ADMIN_REALM", "master"),
			AdminUser:  envOr("KEYCLOAK_ADMIN_USER", "admin"),
			AdminPass:  pass,
		}, logger)
		identityService = identity.NewService(kcClient)

		// Probe once at boot so misconfiguration shows up in the logs rather
		// than as a mystery 502 in the portal.
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := identityService.Ping(probeCtx); err != nil {
			logger.Warn("keycloak admin API not reachable at startup; "+
				"Identity Management will retry per request", zap.Error(err))
		} else {
			logger.Info("keycloak admin API connected",
				zap.String("realm", kcClient.Realm()))
		}
		probeCancel()
	} else {
		logger.Info("KEYCLOAK_ADMIN_PASS not set — Identity Management disabled")
	}

	// Setup router
	deps := &rest.Dependencies{
		UserService:         userService,
		TenantService:       tenantService,
		SSOService:          ssoService,
		DeployService:       deployService,
		CloudAccountService: cloudAccountService,
		AuditService:        auditService,
		IdentityService:     identityService,
		ApprovalEngine:      approvalEngine,
		OnboardingService:   onboardingService,
		SatelliteRepo:       satRepo,
		Config:              cfg,
		Logger:              logger,
	}

	router := rest.NewRouter(deps)

	// Start background services
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	// Gateway health monitor — marks gateways offline if heartbeat missed for 2 minutes
	healthMonitor := gatewayhealth.NewMonitor(satRepo, 30*time.Second, 2*time.Minute, logger)
	go healthMonitor.Start(bgCtx)

	// Start HTTPS server. Development may explicitly disable TLS; production
	// validation requires TLS plus verified gateway client certificates.
	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		logger.Fatal("invalid TLS configuration", zap.Error(err))
	}
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      router,
		TLSConfig:    tlsConfig,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		var serveErr error
		if cfg.TLS.Enabled {
			logger.Info("HTTPS server listening", zap.String("addr", srv.Addr), zap.String("gateway_client_auth", cfg.TLS.ClientAuth))
			serveErr = srv.ListenAndServeTLS(cfg.TLS.CertPath, cfg.TLS.KeyPath)
		} else {
			logger.Warn("development HTTP server listening; TLS is disabled", zap.String("addr", srv.Addr))
			serveErr = srv.ListenAndServe()
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(serveErr))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")
	bgCancel() // Stop background services

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("server forced to shutdown", zap.Error(err))
	}

	logger.Info("server stopped")
}

// envOr reads an environment variable, falling back to def when unset or empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func buildTLSConfig(cfg *config.ServerConfig) (*tls.Config, error) {
	if !cfg.TLS.Enabled {
		return nil, nil
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	switch strings.ToLower(strings.TrimSpace(cfg.TLS.ClientAuth)) {
	case "", "none":
		tlsConfig.ClientAuth = tls.NoClientCert
	case "optional":
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	case "required":
		// Request and verify a certificate at the TLS layer, then require it only
		// on gateway routes in SatelliteAuth. Requiring it at the listener would
		// also block browser/JWT API clients on the shared port.
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	default:
		return nil, fmt.Errorf("unsupported TLS client_auth %q", cfg.TLS.ClientAuth)
	}
	if tlsConfig.ClientAuth != tls.NoClientCert {
		pem, err := os.ReadFile(cfg.TLS.ClientCAPath)
		if err != nil {
			return nil, fmt.Errorf("reading gateway client CA bundle: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("gateway client CA bundle contains no certificates")
		}
		tlsConfig.ClientCAs = pool
	}
	return tlsConfig, nil
}
