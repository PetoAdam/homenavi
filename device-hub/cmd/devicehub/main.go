package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PetoAdam/homenavi/device-hub/internal/config"
	"github.com/PetoAdam/homenavi/device-hub/internal/httpapi"
	mqttpkg "github.com/PetoAdam/homenavi/device-hub/internal/mqtt"
	"github.com/PetoAdam/homenavi/device-hub/internal/store"
	"github.com/PetoAdam/homenavi/shared/dbx"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
)

func main() {
	cfg := config.Load()
	dsn := dbx.BuildPostgresDSN(dbx.PostgresConfig{Host: cfg.Postgres.Host, User: cfg.Postgres.User, Password: cfg.Postgres.Password, DBName: cfg.Postgres.DBName, Port: cfg.Postgres.Port, SSLMode: cfg.Postgres.SSLMode})
	repo, err := store.NewRepository(dsn)
	if err != nil {
		slog.Error("db init failed", "error", err)
		os.Exit(1)
	}
	mClient := mqttpkg.New(cfg.MQTTBrokerURL)

	// Integrations are discovered dynamically from adapters via HDP status frames.
	_ = cfg

	shutdownObs, promHandler, tracer := sharedobs.SetupObservability("device-hub")
	defer shutdownObs()

	// Only health endpoint retained for k8s / docker health checks.
	mux := http.NewServeMux()
	mux.Handle("/metrics", promHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	httpapi.NewServer(repo, mClient).Register(mux)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: sharedobs.WrapHandler(tracer, "device-hub", mux)}
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
	mClient.Close()
	_ = srv.Shutdown(ctx)
	slog.Info("device-hub stopped")
}
