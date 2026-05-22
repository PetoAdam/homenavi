package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	httptransport "github.com/PetoAdam/homenavi/device-hub/internal/http"
	dbinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/shared/cachex"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
)

// App is the composed device-hub application.
type App struct {
	server      *http.Server
	mqtt        *mqttinfra.Client
	cache       *cachex.JSONStore
	shutdownObs func()
	logger      *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	repo, err := dbinfra.Open(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}
	mqttClient, err := mqttinfra.Connect(cfg.MQTT.BrokerURL)
	if err != nil {
		return nil, fmt.Errorf("connect mqtt: %w", err)
	}
	shutdownObs, promHandler, tracer, err := sharedobs.SetupObservability("device-hub")
	if err != nil {
		mqttClient.Close()
		return nil, fmt.Errorf("setup observability: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promHandler)
	var cacheStore *cachex.JSONStore
	if cfg.ListCacheTTL > 0 {
		cacheStore, err = cachex.NewJSONStore(context.Background(), cfg.Redis)
		if err != nil {
			logger.Warn("device-hub cache disabled", "error", err)
		}
	}
	handler := httptransport.NewServer(repo, mqttClient, httptransport.WithCache(cacheStore, cfg.ListCacheTTL))
	mux.Handle("/", httptransport.NewRouter(handler))

	return &App{
		server:      &http.Server{Addr: ":" + cfg.Port, Handler: sharedobs.WrapHandler(tracer, "device-hub", mux), ReadHeaderTimeout: 5 * time.Second},
		mqtt:        mqttClient,
		cache:       cacheStore,
		shutdownObs: shutdownObs,
		logger:      logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer func() {
		if a.cache != nil {
			_ = a.cache.Close()
		}
	}()
	defer a.mqtt.Close()
	defer a.shutdownObs()

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("device-hub listening", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("device-hub shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
