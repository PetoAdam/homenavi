package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"homenavi/integration-proxy/internal/auth"
	"homenavi/integration-proxy/internal/config"
	"homenavi/integration-proxy/internal/server"
)

func main() {
	var (
		listenAddr = flag.String("listen", ":8099", "listen address")
		configPath = flag.String("config", getenv("INTEGRATIONS_CONFIG_PATH", "/config/integrations.yaml"), "path to integrations yaml")
		schemaPath = flag.String("schema", getenv("INTEGRATIONS_SCHEMA_PATH", "/config/homenavi-integration.schema.json"), "path to integration manifest jsonschema")
		refresh    = flag.Duration("refresh", 30*time.Second, "manifest refresh interval")
	)
	flag.Parse()

	logger := log.New(os.Stdout, "integration-proxy ", log.LstdFlags|log.LUTC)
	pubKeyPath := strings.TrimSpace(os.Getenv("JWT_PUBLIC_KEY_PATH"))
	if pubKeyPath == "" {
		logger.Fatalf("JWT_PUBLIC_KEY_PATH is required to protect /integrations/*")
	}
	pubKey, err := auth.LoadRSAPublicKey(pubKeyPath)
	if err != nil {
		logger.Fatalf("load JWT public key: %v", err)
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	validator, err := server.LoadSchema(*schemaPath)
	if err != nil {
		logger.Fatalf("load schema: %v", err)
	}

	s := server.New(logger, validator, *schemaPath, *configPath)
	for _, ic := range cfg.Integrations {
		if err := s.AddIntegration(ic); err != nil {
			logger.Fatalf("add integration %q: %v", ic.ID, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.StartRefreshLoop(ctx, *refresh)

	h := auth.RequireResident(pubKey)(s.Routes())
	srv := &http.Server{Addr: *listenAddr, Handler: h}
	logger.Printf("listening on %s", *listenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("server error: %v", err)
	}
}

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}
