package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/PetoAdam/homenavi/shared/observability"
	httptransport "github.com/PetoAdam/homenavi/zigbee-adapter/internal/http"
	dbinfra "github.com/PetoAdam/homenavi/zigbee-adapter/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/zigbee-adapter/internal/infra/mqtt"
	redisinfra "github.com/PetoAdam/homenavi/zigbee-adapter/internal/infra/redis"
	"github.com/PetoAdam/homenavi/zigbee-adapter/internal/proto/zigbee"
)

// App is the composed zigbee-adapter application.
type App struct {
	server      *http.Server
	adapter     *zigbee.ZigbeeAdapter
	mqtt        *mqttinfra.Client
	rdb         *redisinfra.Client
	shutdownObs func()
	logger      *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	repo, err := dbinfra.Open(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}
	rdb, err := redisinfra.Connect(cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	cache := redisinfra.NewStateCache(rdb)
	mqttClient, err := mqttinfra.Connect(cfg.MQTTBrokerURL, "zigbee-adapter")
	if err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("connect mqtt: %w", err)
	}
	shutdownObs, promHandler, tracer := observability.SetupObservability("zigbee-adapter")
	adapter := zigbee.New(mqttClient, repo, cache)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           observability.WrapHandler(tracer, "zigbee-adapter", httptransport.NewRouter(httptransport.NewServer(promHandler))),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &App{server: server, adapter: adapter, mqtt: mqttClient, rdb: rdb, shutdownObs: shutdownObs, logger: logger}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.shutdownObs()
	defer a.mqtt.Close()
	defer func() { _ = a.rdb.Close() }()
	if err := a.adapter.Start(ctx); err != nil {
		return err
	}
	defer a.adapter.Stop()

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("zigbee-adapter listening", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("zigbee-adapter shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
