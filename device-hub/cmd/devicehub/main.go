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

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/redis/go-redis/v9"

	"device-hub/internal/config"
	"device-hub/internal/httpapi"
	mqttpkg "device-hub/internal/mqtt"
	zigbee "device-hub/internal/proto/zigbee"
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
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("redis init failed", "error", err)
		os.Exit(1)
	}
	cache := store.NewStateCache(rdb)
	mClient := mqttpkg.New(cfg.MQTTBrokerURL)
	_ = mClient.Subscribe("zigbee2mqtt/bridge/state", func(_ mqtt.Client, msg mqttpkg.Message) { slog.Info("bridge state", "payload", string(msg.Payload())) })
	events := make(chan any, 128)
	zAdapter := zigbee.New(mClient, repo, cache, events)
	if err := zAdapter.Start(context.Background()); err != nil {
		slog.Error("zigbee start failed", "error", err)
		os.Exit(1)
	}

	// Only health endpoint retained for k8s / docker health checks.
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	httpapi.NewServer(repo, mClient).Register(mux)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: mux}
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
