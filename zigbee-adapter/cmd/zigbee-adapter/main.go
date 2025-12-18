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

	"github.com/redis/go-redis/v9"

	"zigbee-adapter/internal/config"
	"zigbee-adapter/internal/mqtt"
	"zigbee-adapter/internal/observability"
	"zigbee-adapter/internal/proto/zigbee"
	"zigbee-adapter/internal/store"
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

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("redis init failed", "error", err)
		os.Exit(1)
	}
	cache := store.NewStateCache(rdb)

	mClient := mqtt.New(cfg.MQTTBrokerURL)

	shutdownObs, promHandler, tracer := observability.SetupObservability("zigbee-adapter")
	defer shutdownObs()

	zAdapter := zigbee.New(mClient, repo, cache)
	if err := zAdapter.Start(context.Background()); err != nil {
		slog.Error("zigbee start failed", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: observability.WrapHandler(tracer, "zigbee-adapter", mux)}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("adapter server error", "error", err)
		}
	}()
	slog.Info("zigbee-adapter started", "port", cfg.Port, "adapter_id", cfg.AdapterID)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	zAdapter.Stop()
	mClient.Disconnect()
	_ = rdb.Close()
	_ = srv.Shutdown(ctx)
	slog.Info("zigbee-adapter stopped")
}
