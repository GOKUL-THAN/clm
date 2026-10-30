package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/m2p/clm/internal/satellite"
	"github.com/m2p/clm/internal/satellite/api"
	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/ca"
	"github.com/m2p/clm/internal/satellite/connector"
	"github.com/m2p/clm/internal/satellite/credstore"
	cryptoengine "github.com/m2p/clm/internal/satellite/crypto"
	"github.com/m2p/clm/internal/satellite/deployment"
	clouddeployer "github.com/m2p/clm/internal/satellite/deployment/deployers/cloud"
	datadeployer "github.com/m2p/clm/internal/satellite/deployment/deployers/data"
	linuxdeployer "github.com/m2p/clm/internal/satellite/deployment/deployers/os"
	"github.com/m2p/clm/internal/satellite/discovery/scanners"
	"github.com/m2p/clm/internal/satellite/dns01"
	"github.com/m2p/clm/internal/satellite/policy"
	"github.com/m2p/clm/internal/satellite/secrets"
	secretproviders "github.com/m2p/clm/internal/satellite/secrets/providers"
	"github.com/m2p/clm/internal/satellite/state"
	"github.com/m2p/clm/internal/satellite/vsatellite"
	"github.com/m2p/clm/pkg/config"
	"github.com/m2p/clm/pkg/utils"
	"go.uber.org/zap"
)

// satelliteVersion is reported to the control plane and the portal API.
const satelliteVersion = "1.0.0"

func main() {
	// Load configuration
	cfgPath := os.Getenv("CLM_SATELLITE_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/satellite.yaml"
	}

	cfg, err := config.LoadSatelliteConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := utils.NewLogger(cfg.LogLevel)
	defer logger.Sync()

	logger.Info("starting CLM satellite",
		zap.String("id", cfg.ID),
		zap.String("name", cfg.Name),
		zap.String("connector_mode", cfg.ConnectorMode),
	)

	// Initialize crypto engine
	cryptoEng := cryptoengine.NewEngine(logger)

	// Initialize secrets store based on config
	var secretsStore *secrets.Manager
	switch cfg.Secrets.Provider {
	case "aws":
		awsStore := secretproviders.NewAWSSecretsManager(
			cfg.Secrets.Region,
			cfg.Secrets.KMSKeyID,
			cfg.Secrets.Prefix,
			logger,
		)
		secretsStore = secrets.NewManager(awsStore, cfg.Secrets.Prefix, logger)
	default:
		awsStore := secretproviders.NewAWSSecretsManager(
			cfg.Secrets.Region,
			cfg.Secrets.KMSKeyID,
			cfg.Secrets.Prefix,
			logger,
		)
		secretsStore = secrets.NewManager(awsStore, cfg.Secrets.Prefix, logger)
	}

	// Initialize network scanner
	ports := cfg.Discovery.Ports
	if len(ports) == 0 {
		ports = []int{443, 8443, 636, 6443, 9093, 5671}
	}
	concurrency := cfg.Discovery.Concurrency
	if concurrency == 0 {
		concurrency = 50
	}
	scanner := scanners.NewNetworkScanner(concurrency, 5*time.Second, ports, logger)

	// DNS discovery finds certificates by name. It uses a lower concurrency than
	// the address sweep because each candidate costs a DNS query before any
	// handshake, and resolvers rate-limit far more readily than TCP endpoints.
	// CLM_DNS_SERVER points lookups at an internal resolver when set; otherwise
	// the host's own resolver is used.
	// Sized for a full-width enumeration: 2000 candidate names have to resolve
	// inside the request timeout, and at a low concurrency they cannot.
	dnsConcurrency := cfg.Discovery.DNSConcurrency
	if dnsConcurrency <= 0 {
		dnsConcurrency = 64
	}
	dnsServer := cfg.Discovery.DNSServer
	if env := os.Getenv("CLM_DNS_SERVER"); env != "" {
		dnsServer = env
	}
	dnsScanner := scanners.NewDNSScanner(
		dnsConcurrency, 5*time.Second, []int{443, 8443}, dnsServer, logger)

	// Durable state. Without this the certificate inventory, every approval
	// decision and the whole vSatellite fleet are lost on restart.
	stateDir := cfg.State.Dir
	if env := os.Getenv("CLM_STATE_DIR"); env != "" {
		stateDir = env
	}
	stateStore, err := state.New(stateDir, logger)
	if err != nil {
		logger.Fatal("state directory is unusable", zap.Error(err))
	}
	commandJournal, err := state.NewCommandJournal(stateStore)
	if err != nil {
		logger.Fatal("command journal is unusable", zap.Error(err))
	}

	// Alerting. Built even when disabled so evaluation still records what would
	// have fired, which is how an operator can see the effect before wiring a
	// channel to a team's channel.
	alertCooldown := parseDurationOr(cfg.Alerting.Cooldown, 24*time.Hour, "alerting.cooldown", logger)
	alerter := api.NewAlerter(api.AlertConfig{
		Enabled:           cfg.Alerting.Enabled,
		WebhookURL:        envOrValue("CLM_ALERT_WEBHOOK", cfg.Alerting.WebhookURL),
		SlackWebhookURL:   envOrValue("CLM_SLACK_WEBHOOK", cfg.Alerting.SlackWebhookURL),
		WarnDays:          cfg.Alerting.WarnDays,
		CriticalDays:      cfg.Alerting.CriticalDays,
		Cooldown:          alertCooldown,
		AlertOnSelfSigned: cfg.Alerting.AlertOnSelfSigned,
		AlertOnWeakKey:    cfg.Alerting.AlertOnWeakKey,
	}, stateStore, logger)

	// ACME. Requires somewhere to keep issued private keys, so it is only built
	// when state persistence is configured — issuing a certificate whose key
	// vanishes on restart would be worse than not issuing one.
	var acmeManager *api.ACMEManager
	if cfg.ACME.Enabled {
		switch {
		case !stateStore.Enabled():
			logger.Error("acme.enabled is set but state.dir is not; issuance is disabled " +
				"because there is nowhere durable to store private keys")
		case !cfg.ACME.AcceptTOS:
			logger.Error("acme.enabled is set but acme.accept_tos is false; issuance is " +
				"disabled because an ACME account cannot be created without agreeing to the CA terms")
		default:
			dnsSolver, dnsErr := newDNS01Solver(dns01.Config{
				Provider:           cfg.ACME.DNS01.Provider,
				Zone:               cfg.ACME.DNS01.Zone,
				Nameserver:         cfg.ACME.DNS01.Nameserver,
				CredentialRef:      cfg.ACME.DNS01.CredentialRef,
				Resolvers:          cfg.ACME.DNS01.Resolvers,
				TTL:                cfg.ACME.DNS01.TTL,
				PollInterval:       parseDurationOr(cfg.ACME.DNS01.PollInterval, 2*time.Second, "acme.dns01.poll_interval", logger),
				PropagationTimeout: parseDurationOr(cfg.ACME.DNS01.PropagationTimeout, 2*time.Minute, "acme.dns01.propagation_timeout", logger),
				CleanupTimeout:     parseDurationOr(cfg.ACME.DNS01.CleanupTimeout, 15*time.Second, "acme.dns01.cleanup_timeout", logger),
			}, cfg.ACME.ChallengeType)
			if dnsErr != nil {
				logger.Error("DNS-01 could not be initialised; ACME issuance is disabled", zap.Error(dnsErr))
				break
			}
			acmeManager, err = api.NewACMEManager(api.ACMEConfig{
				Enabled:            true,
				Environment:        cfg.Env,
				DirectoryURL:       cfg.ACME.DirectoryURL,
				Email:              cfg.ACME.Email,
				AcceptTOS:          cfg.ACME.AcceptTOS,
				KeyType:            cfg.ACME.KeyType,
				RenewBeforeDays:    cfg.ACME.RenewBeforeDays,
				InsecureSkipVerify: cfg.ACME.InsecureSkipVerify,
				ChallengeType:      cfg.ACME.ChallengeType,
				DNS01:              dnsSolver,
				KeyDir:             filepath.Join(stateStore.Dir(), "keys"),
			}, logger)
			if err != nil {
				logger.Error("ACME could not be initialised; issuance is disabled", zap.Error(err))
				acmeManager = nil
			} else {
				logger.Info("ACME issuance enabled",
					zap.String("directory", cfg.ACME.DirectoryURL),
					zap.Int("renew_before_days", cfg.ACME.RenewBeforeDays))
			}
		}
	} else {
		logger.Info("ACME issuance is disabled (set acme.enabled to turn it on)")
	}

	// Multi-vendor CA registry. Credentials come from the environment only: a CA
	// API key in a config file ends up in a backup, a container image or a support
	// bundle, and none of those should carry the ability to buy certificates.
	caRegistry := ca.NewRegistry()
	keyDir := ""
	if stateStore.Enabled() {
		keyDir = filepath.Join(stateStore.Dir(), "keys")
	}

	// Credential storage for CA accounts registered through the portal and for
	// deployment targets.
	credentials := credstore.New(stateStore, logger)

	// Audit trail: an append-only record of who did what, separate from the
	// snapshot-based state store because an audit log must never be rewritable
	// the way a full-snapshot collection is. See internal/audit for why.
	auditDir := ""
	if stateStore.Enabled() {
		auditDir = filepath.Join(stateStore.Dir(), "audit")
	}
	auditLogger, err := audit.New(auditDir, logger)
	if err != nil {
		logger.Error("audit trail could not be initialised; actions taken on this "+
			"gateway will not be recorded", zap.Error(err))
		auditLogger, _ = audit.New("", logger) // disabled logger, safe to use nil-style
	}

	// Certificate policy engine: enforceable issuance rules, persisted the same
	// way credentials and endpoints are — a full snapshot under state.dir,
	// because the policy set itself is small and changes rarely, unlike the
	// audit trail above.
	const policyCollection = "policies"
	policyStore := policy.NewStore(logger, func(policies []*policy.Policy) {
		if !stateStore.Enabled() {
			return
		}
		stateStore.Save(policyCollection, func() any {
			return map[string]any{"policies": policies}
		})
	})
	if stateStore.Enabled() {
		var snap struct {
			Policies []*policy.Policy `json:"policies"`
		}
		if err := stateStore.Load(policyCollection, &snap); err != nil {
			logger.Error("loading certificate policies failed", zap.Error(err))
		} else {
			policyStore.Restore(snap.Policies)
		}
	}

	// The ACME implementation lives in the api package, so the ca package cannot
	// construct one itself. Installing the builder here keeps that dependency
	// pointing one way while still letting an ACME account be registered at
	// runtime like any other vendor.
	ca.SetACMEBuilder(func(buildCfg ca.ACMEBuildConfig, l *zap.Logger) (ca.Provider, error) {
		return newRuntimeACMEProvider(buildCfg, cfg.Env, l)
	})

	if acmeManager != nil {
		if err := caRegistry.Register("acme", "letsencrypt-compatible", true,
			api.NewACMEProvider(acmeManager, logger)); err != nil {
			logger.Error("registering the ACME account failed", zap.Error(err))
		}
	}

	if key, secret := os.Getenv("CLM_GODADDY_KEY"), os.Getenv("CLM_GODADDY_SECRET"); key != "" && secret != "" {
		gdEnv := os.Getenv("CLM_GODADDY_ENV")
		if gdEnv == "" {
			// Default to the sandbox. A production default would let a
			// misconfiguration buy real certificates.
			gdEnv = "ote"
		}
		gd, gerr := ca.NewGoDaddyProvider(ca.GoDaddyConfig{
			Key: key, Secret: secret, Environment: gdEnv,
			ShopperID: os.Getenv("CLM_GODADDY_SHOPPER_ID"),
			BaseURL:   os.Getenv("CLM_GODADDY_BASE_URL"),
		}, logger)
		if gerr != nil {
			logger.Error("GoDaddy CA account not registered", zap.Error(gerr))
		} else if err := caRegistry.Register("godaddy", gd.Environment(), false, gd); err != nil {
			logger.Error("registering the GoDaddy account failed", zap.Error(err))
		} else {
			logger.Info("GoDaddy CA account registered",
				zap.String("environment", gd.Environment()))
		}
	} else {
		logger.Info("GoDaddy CA account not configured " +
			"(set CLM_GODADDY_KEY and CLM_GODADDY_SECRET)")
	}

	if apiUser, apiKey := os.Getenv("CLM_NAMECHEAP_API_USER"), os.Getenv("CLM_NAMECHEAP_API_KEY"); apiUser != "" && apiKey != "" {
		// Namecheap rejects any call from an address that is not allow-listed on
		// the account, so the client IP is required rather than guessed.
		nc, nerr := ca.NewNamecheapProvider(ca.NamecheapConfig{
			APIUser:  apiUser,
			APIKey:   apiKey,
			UserName: os.Getenv("CLM_NAMECHEAP_USERNAME"),
			ClientIP: os.Getenv("CLM_NAMECHEAP_CLIENT_IP"),
			Sandbox:  os.Getenv("CLM_NAMECHEAP_SANDBOX") != "false",
			BaseURL:  os.Getenv("CLM_NAMECHEAP_BASE_URL"),
		}, logger)
		if nerr != nil {
			logger.Error("Namecheap CA account not registered", zap.Error(nerr))
		} else if err := caRegistry.Register("namecheap", nc.Environment(), false, nc); err != nil {
			logger.Error("registering the Namecheap account failed", zap.Error(err))
		} else {
			logger.Info("Namecheap CA account registered",
				zap.String("environment", nc.Environment()))
		}
	} else {
		logger.Info("Namecheap CA account not configured " +
			"(set CLM_NAMECHEAP_API_USER, CLM_NAMECHEAP_API_KEY and CLM_NAMECHEAP_CLIENT_IP)")
	}

	logger.Info("CA accounts available",
		zap.Strings("accounts", caRegistry.Names()),
		zap.String("default", caRegistry.Default()))

	// Initialize deployment executor with deployers
	deployExecutor := deployment.NewExecutor(logger)
	deployExecutor.RegisterDeployer(linuxdeployer.NewLinuxDeployer(logger))
	deployExecutor.RegisterDeployer(datadeployer.NewPostgreSQLDeployer(logger))

	// AWS ACM / ALB. This deployer was written but never registered, so the
	// target type was unreachable no matter what a caller asked for. It needs AWS
	// credentials at deploy time; registering it without them fails per-request
	// with a clear AWS error, which is better than the endpoint pretending the
	// target type does not exist.
	acmRegion := cfg.Secrets.Region
	if acmRegion == "" {
		acmRegion = "ap-south-1"
	}
	deployExecutor.RegisterDeployer(clouddeployer.NewAWSACMDeployer(acmRegion, logger))

	// GCP and Azure. Both take their credentials per request rather than at
	// construction, so one instance serves every project or subscription and
	// nothing sensitive is held in memory between deployments.
	deployExecutor.RegisterDeployer(clouddeployer.NewGCPDeployer(logger))
	deployExecutor.RegisterDeployer(clouddeployer.NewAzureDeployer(logger))

	// Initialize SaaS connector
	conn, err := connector.New(&connector.Config{
		SatelliteID:        cfg.ID,
		SaaSEndpoint:       cfg.SaaSEndpoint,
		Token:              os.Getenv("CLM_SATELLITE_TOKEN"),
		TLSCert:            cfg.TLSCertPath,
		TLSKey:             cfg.TLSKeyPath,
		TLSCA:              cfg.TLSCAPath,
		TLSServerName:      cfg.TLSServerName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		Mode:               cfg.ConnectorMode,
		Environment:        cfg.Env,
		MaxResultBytes:     cfg.Commands.MaxResultBytes,
		NetworkRetries:     cfg.Commands.NetworkRetries,
	}, logger)
	if err != nil {
		logger.Fatal("satellite connector configuration is invalid", zap.Error(err))
	}

	// Create and start runtime
	runtime := satellite.NewRuntime(cfg, conn, cryptoEng, secretsStore, scanner, dnsScanner, deployExecutor, commandJournal, logger)

	// Start vSatellite coordinator API (port 9090 for vSatellite agents to connect)
	vsatCoordinator := vsatellite.NewCoordinator(logger)
	vsatAPI := vsatellite.NewAPI(vsatCoordinator, logger)
	mux := http.NewServeMux()
	vsatAPI.RegisterRoutes(mux)

	// Prepare the Gateway Portal REST API on :8080. nginx proxies /api/ and
	// /health here, so without it the portal cannot load any data.
	portalAddr := os.Getenv("CLM_GATEWAY_API_ADDR")
	if portalAddr == "" {
		portalAddr = ":8080"
	}
	jwksURL := os.Getenv("CLM_JWKS_URL")
	if jwksURL == "" {
		jwksURL = strings.TrimSuffix(cfg.SaaSEndpoint, "/") +
			"/auth/realms/clm/protocol/openid-connect/certs"
	}
	portalAPI := api.NewServer(api.Options{
		SatelliteID:   cfg.ID,
		SatelliteName: cfg.Name,
		SaaSEndpoint:  cfg.SaaSEndpoint,
		Version:       satelliteVersion,
		Auth:          api.NewAuthenticator(os.Getenv("CLM_JWT_SECRET"), jwksURL, logger),
		Scanner:       scanner,
		DNSScanner:    dnsScanner,
		ACME:          acmeManager,
		Alerter:       alerter,
		CARegistry:    caRegistry,
		Credentials:   credentials,
		EndpointStore: api.NewEndpointStore(stateStore, logger),
		Audit:         auditLogger,
		Policies:      policyStore,
		KeyDir:        keyDir,
		Deployer:      deployExecutor,
		Agents:        vsatCoordinator,
		Logger:        logger,
	})

	// Restore lifecycle state before any listener or scheduler can mutate it.
	// Corrupt or unreadable inventory is a startup failure: serving with an empty
	// lineage could activate or overwrite the wrong certificate version.
	if err := portalAPI.Inventory().AttachStore(stateStore, logger); err != nil {
		logger.Fatal("restoring gateway inventory failed", zap.Error(err))
	}
	vsatCoordinator.AttachStore(stateStore)

	// CA accounts registered through the portal, re-created from stored
	// credentials. Done after the environment-configured accounts so an
	// explicitly configured account keeps priority for the default.
	portalAPI.RestoreCAAccounts()
	logger.Info("CA accounts after restore",
		zap.Strings("accounts", caRegistry.Names()),
		zap.String("default", caRegistry.Default()))

	go func() {
		logger.Info("vSatellite API listening on :9090")
		if err := http.ListenAndServe(":9090", mux); err != nil {
			logger.Error("vSatellite API error", zap.Error(err))
		}
	}()
	go func() {
		logger.Info("gateway portal API listening",
			zap.String("addr", portalAddr), zap.String("jwks", jwksURL))
		if err := http.ListenAndServe(portalAddr, portalAPI.Handler()); err != nil {
			logger.Error("gateway portal API error", zap.Error(err))
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Background processes: automated discovery, expiry alerting and renewal.
	// These are what turn the gateway from something an operator drives by hand
	// into something that manages the lifecycle on its own.
	scheduler := api.NewScheduler(api.SchedulerConfig{
		DiscoveryEnabled:  cfg.Discovery.Enabled,
		DiscoveryInterval: parseDurationOr(cfg.Discovery.Interval, 6*time.Hour, "discovery.interval", logger),
		Networks:          cfg.Discovery.Networks,
		Domains:           cfg.Discovery.Domains,
		DNSServer:         dnsServer,
		Enumerate:         cfg.Discovery.Enumerate,
		MaxNames:          cfg.Discovery.MaxNames,
		ScanACM:           cfg.Discovery.ScanACM,
		ACMRegions:        cfg.Discovery.ACMRegions,

		AlertEnabled:  cfg.Alerting.Enabled,
		AlertInterval: parseDurationOr(cfg.Alerting.Interval, time.Hour, "alerting.interval", logger),

		RenewEnabled:  acmeManager != nil,
		RenewInterval: parseDurationOr(cfg.ACME.RenewInterval, 12*time.Hour, "acme.renew_interval", logger),

		// Only worth polling when a CA that issues asynchronously is configured;
		// ACME completes inside the request.
		CAOrderPollEnabled: caRegistry.Count() > 0,
		CAOrderInterval:    parseDurationOr(cfg.ACME.OrderPollInterval, 15*time.Minute, "acme.order_poll_interval", logger),
	}, portalAPI, logger)
	portalAPI.SetScheduler(scheduler)
	scheduler.Start(ctx)

	// Monitor agent health
	go vsatCoordinator.MonitorAgents(ctx, 2*time.Minute)

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	if err := runtime.Start(ctx); err != nil {
		logger.Fatal("satellite runtime error", zap.Error(err))
	}

	// Write state synchronously on the way out, so a clean stop does not lose the
	// last few seconds of changes sitting in the debounce window.
	portalAPI.Inventory().FlushState()
	vsatCoordinator.FlushState()
	credentials.FlushState()
	stateStore.Flush()
	if err := portalAPI.Audit().Close(); err != nil {
		logger.Warn("closing the audit log failed", zap.Error(err))
	}

	logger.Info("satellite stopped")
}

// newRuntimeACMEProvider enforces TLS policy using the gateway environment,
// never the account's staging/production label. Otherwise a production gateway
// could register an account labelled staging and disable certificate validation.
func newRuntimeACMEProvider(buildCfg ca.ACMEBuildConfig, gatewayEnvironment string, logger *zap.Logger) (ca.Provider, error) {
	if buildCfg.KeyDir == "" {
		return nil, fmt.Errorf("state.dir must be set so issued private keys can be stored")
	}
	dnsSolver, err := newDNS01Solver(dns01.Config{
		Provider:           buildCfg.DNSProvider,
		Zone:               buildCfg.DNSZone,
		Nameserver:         buildCfg.DNSNameserver,
		CredentialRef:      buildCfg.DNSCredentialRef,
		Resolvers:          buildCfg.DNSResolvers,
		TTL:                buildCfg.DNSTTL,
		PollInterval:       parseDurationOr(buildCfg.DNSPollInterval, 2*time.Second, "dns_poll_interval", logger),
		PropagationTimeout: parseDurationOr(buildCfg.DNSPropagationTimeout, 2*time.Minute, "dns_propagation_timeout", logger),
		CleanupTimeout:     parseDurationOr(buildCfg.DNSCleanupTimeout, 15*time.Second, "dns_cleanup_timeout", logger),
	}, buildCfg.ChallengeType)
	if err != nil {
		return nil, err
	}
	manager, err := api.NewACMEManager(api.ACMEConfig{
		Enabled: true, Environment: gatewayEnvironment,
		DirectoryURL: buildCfg.DirectoryURL, Email: buildCfg.Email,
		AcceptTOS: buildCfg.AcceptTOS, KeyType: buildCfg.KeyType,
		InsecureSkipVerify: buildCfg.InsecureSkipVerify,
		ChallengeType:      buildCfg.ChallengeType, DNS01: dnsSolver,
		KeyDir: buildCfg.KeyDir,
	}, logger)
	if err != nil {
		return nil, err
	}
	return api.NewACMEProvider(manager, logger), nil
}

// newDNS01Solver builds only live adapters. Merely naming a planned cloud
// provider fails during configuration rather than halfway through an ACME order.
func newDNS01Solver(cfg dns01.Config, challengeType string) (*dns01.Solver, error) {
	if strings.TrimSpace(cfg.Provider) == "" {
		if strings.EqualFold(strings.TrimSpace(challengeType), "dns-01") {
			return nil, fmt.Errorf("dns-01 challenge_type requires a DNS-01 provider")
		}
		return nil, nil
	}
	return dns01.New(cfg)
}

// parseDurationOr parses a Go duration, falling back with a warning rather than
// failing to start. A malformed interval in a config file should not take the
// gateway down, but it must not be silently ignored either.
func parseDurationOr(v string, fallback time.Duration, field string, logger *zap.Logger) time.Duration {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		logger.Warn("could not parse duration, using the default",
			zap.String("field", field), zap.String("value", v),
			zap.Duration("default", fallback), zap.Error(err))
		return fallback
	}
	return d
}

// envOrValue prefers an environment variable, which is how secrets such as a
// webhook URL stay out of the config file.
func envOrValue(env, value string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return value
}
