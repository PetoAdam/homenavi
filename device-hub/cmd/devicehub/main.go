package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"device-hub/internal/config"
	"device-hub/internal/httpapi"
	mqttpkg "device-hub/internal/mqtt"
	"device-hub/internal/observability"
	"device-hub/internal/store"
)

func main() {
	cfg := config.Load()
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		cfg.Postgres.Host, cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.DBName, cfg.Postgres.Port)
	repo, err := store.NewRepository(dsn)
	if err != nil {
		slog.Error("db init failed", "error", err)
		os.Exit(1)
	}
	mClient := mqttpkg.New(cfg.MQTTBrokerURL)

	// Integrations are discovered dynamically from adapters via HDP status frames.
	_ = cfg

	shutdownObs, promHandler, tracer := observability.SetupObservability("device-hub")
	defer shutdownObs()

	// Only health endpoint retained for k8s / docker health checks.
	mux := http.NewServeMux()
	mux.Handle("/metrics", promHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	httpapi.NewServer(repo, mClient).Register(mux)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: observability.WrapHandler(tracer, "device-hub", mux)}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()
	slog.Info("device-hub started", "port", cfg.Port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	slog.Info("device-hub stopped")
}
