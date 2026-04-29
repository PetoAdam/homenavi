package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/PetoAdam/homenavi/mock-adapter/internal/adapter"
	httptransport "github.com/PetoAdam/homenavi/mock-adapter/internal/http"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
)

// App is the composed mock-adapter application.
type App struct {
	server      *http.Server
	adapter     *adapter.Service
	mqttClient  *mqttx.Client
	shutdownObs func()
	logger      *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	mqttClient, err := mqttx.Connect(mqttx.Options{BrokerURL: cfg.MQTT.BrokerURL, ClientIDPrefix: "mock-adapter"})
	if err != nil {
		return nil, fmt.Errorf("connect mqtt: %w", err)
	}
	shutdownObs, promHandler, tracer, err := sharedobs.SetupObservability("mock-adapter")
	if err != nil {
		return nil, fmt.Errorf("setup observability: %w", err)
	}
	adapterSvc := adapter.New(mqttClient, adapter.Config{Enabled: true, AdapterID: cfg.AdapterID, Version: cfg.Version})
	router := httptransport.NewRouter(promHandler, tracer)

	return &App{
		server:      &http.Server{Addr: ":" + cfg.Port, Handler: router},
		adapter:     adapterSvc,
		mqttClient:  mqttClient,
		shutdownObs: shutdownObs,
		logger:      logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.shutdownObs()
	if err := a.adapter.Start(context.Background()); err != nil {
		return fmt.Errorf("start adapter: %w", err)
	}
	defer a.adapter.Stop()
	defer a.mqttClient.Close()

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("mock-adapter started", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
