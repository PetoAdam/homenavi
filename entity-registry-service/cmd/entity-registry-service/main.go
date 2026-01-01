package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"entity-registry-service/internal/autoimport"
	"entity-registry-service/internal/backfill"
	"entity-registry-service/internal/config"
	"entity-registry-service/internal/httpapi"
	"entity-registry-service/internal/realtime"
	"entity-registry-service/internal/store"
)

func main() {
	cfg := config.Load()

	db, err := store.OpenPostgres(
		cfg.Postgres.User,
		cfg.Postgres.Password,
		cfg.Postgres.DBName,
		cfg.Postgres.Host,
		cfg.Postgres.Port,
		cfg.Postgres.SSLMode,
	)
	if err != nil {
		slog.Error("db connect failed", "error", err)
		os.Exit(1)
	}

	repo, err := store.New(db)
	if err != nil {
		slog.Error("db init failed", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	hub := realtime.NewHub()
	srv := httpapi.NewServer(repo, hub)
	srv.Register(mux)

	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: mux}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cfg.AutoImport {
		backfill.Start(ctx, repo, cfg.DeviceHubURL, &http.Client{Timeout: 10 * time.Second})
		autoimport.Start(ctx, repo, cfg.MQTTBrokerURL, hub)
		slog.Info("ers auto-import enabled", "broker", cfg.MQTTBrokerURL, "device_hub", cfg.DeviceHubURL)
	}

	go func() {
		slog.Info("entity-registry-service started", "port", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("graceful shutdown failed", "error", err)
	}

	slog.Info("entity-registry-service stopped")
}
