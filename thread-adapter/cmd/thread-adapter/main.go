package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"thread-adapter/internal/config"
	"thread-adapter/internal/mqtt"
	"thread-adapter/internal/observability"
	threadproto "thread-adapter/internal/proto/thread"
)

func main() {
	cfg := config.Load()

	mClient := mqtt.New(cfg.MQTTBrokerURL)

	shutdownObs, promHandler, tracer := observability.SetupObservability("thread-adapter")
	defer shutdownObs()

	adapter := threadproto.New(mClient, threadproto.Config{Enabled: true, AdapterID: cfg.AdapterID, Version: cfg.Version})
	if err := adapter.Start(context.Background()); err != nil {
		slog.Error("thread adapter start failed", "error", err)
		os.Exit(1)
	}
	slog.Info("thread adapter initialized")

	mux := http.NewServeMux()
	mux.Handle("/metrics", promHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: observability.WrapHandler(tracer, "thread-adapter", mux)}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("thread-adapter server error", "error", err)
		}
	}()
	slog.Info("thread-adapter started", "port", cfg.Port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	adapter.Stop()
	mClient.Disconnect()
	_ = srv.Shutdown(ctx)
	slog.Info("thread-adapter stopped")
}
