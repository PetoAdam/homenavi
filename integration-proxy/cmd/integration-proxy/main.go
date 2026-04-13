package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/PetoAdam/homenavi/integration-proxy/internal/app"
)

func main() {
	baseCfg := app.LoadConfig()
	var (
		listenAddr  = flag.String("listen", baseCfg.ListenAddr, "listen address")
		configPath  = flag.String("config", baseCfg.ConfigPath, "path to integrations yaml")
		schemaPath  = flag.String("schema", baseCfg.SchemaPath, "path to integration manifest jsonschema")
		refresh     = flag.Duration("refresh", baseCfg.RefreshInterval, "manifest refresh interval")
		updateEvery = flag.Duration("updates-refresh", baseCfg.UpdateCheckInterval, "integration update check interval (0 disables)")
	)
	flag.Parse()

	logger := log.New(os.Stdout, "integration-proxy ", log.LstdFlags|log.LUTC)
	cfg := baseCfg
	cfg.ListenAddr = *listenAddr
	cfg.ConfigPath = *configPath
	cfg.SchemaPath = *schemaPath
	cfg.RefreshInterval = *refresh
	cfg.UpdateCheckInterval = *updateEvery
	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Fatalf("application init: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		logger.Fatalf("server error: %v", err)
	}
}
