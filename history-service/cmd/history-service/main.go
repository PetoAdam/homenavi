package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"history-service/internal/config"
	"history-service/internal/httpapi"
	"history-service/internal/ingest"
	"history-service/internal/mqtt"
	"history-service/internal/store"
)

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	if strings.TrimSpace(cfg.MQTTBrokerURL) == "" {
		slog.Error("missing required env", "key", "MQTT_BROKER_URL")
		os.Exit(1)
	}
	if strings.TrimSpace(cfg.Postgres.User) == "" {
		slog.Error("missing required env", "key", "POSTGRES_USER")
		os.Exit(1)
	}
	if strings.TrimSpace(cfg.Postgres.DBName) == "" {
		slog.Error("missing required env", "key", "POSTGRES_DB")
		os.Exit(1)
	}
	if strings.TrimSpace(cfg.Postgres.Host) == "" {
		slog.Error("missing required env", "key", "POSTGRES_HOST")
		os.Exit(1)
	}
	if strings.TrimSpace(cfg.Postgres.Port) == "" {
		slog.Error("missing required env", "key", "POSTGRES_PORT")
		os.Exit(1)
	}

	db, err := store.OpenPostgres(cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.DBName, cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.SSLMode)
	if err != nil {
		slog.Error("db connect failed", "error", err)
		os.Exit(1)
	}
	repo, err := store.New(db)
	if err != nil {
		slog.Error("db migrate failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mq, err := mqtt.Connect(cfg.MQTTBrokerURL, cfg.MQTTClientID)
	if err != nil {
		slog.Error("mqtt connect failed", "error", err)
		os.Exit(1)
	}
	defer mq.Close()

	ing := &ingest.Ingestor{Repo: repo, StatePrefix: cfg.TopicPrefix, AllowRetains: cfg.IngestRetained}
	subTopic := strings.TrimRight(cfg.TopicPrefix, "/") + "/#"
	if err := mq.Subscribe(subTopic, func(m mqtt.Message) {
		ing.HandleMessage(ctx, m, time.Now().UTC())
	}); err != nil {
		slog.Error("mqtt subscribe failed", "topic", subTopic, "error", err)
		os.Exit(1)
	}
	slog.Info("history ingest subscribed", "topic", subTopic)

	srv := httpapi.New(repo)
	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}

	go func() {
		slog.Info("history-service listening", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			cancel()
		}
	}()

	stop := make(chan os.Signal, 2)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("shutdown requested")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	cancel()
}

func setupLogging(level string) {
	lvl := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
}
