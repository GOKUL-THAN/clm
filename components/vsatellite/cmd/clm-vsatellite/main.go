package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/m2p/clm/vsatellite/internal/agent"
	"github.com/m2p/clm/vsatellite/internal/deployer"
	"github.com/m2p/clm/vsatellite/internal/scanner"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
)

func main() {
	cfgPath := os.Getenv("CLM_VSATELLITE_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/vsatellite.yaml"
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			cfgPath = "/etc/clm/vsatellite.yaml"
		}
	}

	cfg := &agent.Config{
		HeartbeatSec: 30,
		Concurrency:  50,
		ScanPorts:    []int{443, 8443, 636, 6443},
		LogLevel:     "info",
		Capabilities: []string{"scan_cidr", "deploy_cert", "health_check"},
	}

	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse config %s: %v\n", cfgPath, err)
			os.Exit(1)
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "failed to read config %s: %v\n", cfgPath, err)
		os.Exit(1)
	}

	// Environment variable overrides
	if ep := os.Getenv("CLM_GATEWAY_ENDPOINT"); ep != "" {
		cfg.GatewayEndpoint = ep
	}
	if tok := os.Getenv("CLM_ENROLL_TOKEN"); tok != "" {
		cfg.Token = tok
	}
	if id := os.Getenv("CLM_VSATELLITE_ID"); id != "" {
		cfg.ID = id
	}

	logger := initLogger(cfg.LogLevel)
	defer logger.Sync()

	logger.Info("starting CLM vSatellite endpoint agent",
		zap.String("id", cfg.ID),
		zap.String("name", cfg.Name),
		zap.String("gateway_endpoint", cfg.GatewayEndpoint),
		zap.Strings("cidrs", cfg.CIDRs),
		zap.Strings("capabilities", cfg.Capabilities),
	)

	// Initialize local scanner
	scanTimeout := 5 * time.Second
	sc := scanner.NewScanner(cfg.Concurrency, scanTimeout, cfg.ScanPorts, logger)

	// Initialize local deployer
	dp := deployer.NewDeployer(logger)

	// Initialize agent runtime
	ag := agent.NewAgent(cfg, sc, dp, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received termination signal", zap.String("signal", sig.String()))
		cancel()
	}()

	ag.Run(ctx)
	logger.Info("vSatellite agent shutdown complete")
}

func initLogger(levelStr string) *zap.Logger {
	var lvl zapcore.Level
	switch levelStr {
	case "debug":
		lvl = zapcore.DebugLevel
	case "warn":
		lvl = zapcore.WarnLevel
	case "error":
		lvl = zapcore.ErrorLevel
	default:
		lvl = zapcore.InfoLevel
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.AddSync(os.Stdout),
		lvl,
	)

	return zap.New(core)
}
