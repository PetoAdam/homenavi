package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/PetoAdam/homenavi/shared/mqttx"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/PetoAdam/homenavi/thread-adapter/internal/adapter"
	httptransport "github.com/PetoAdam/homenavi/thread-adapter/internal/http"
)

// App is the composed thread-adapter application.
type App struct {
	server      *http.Server
	adapter     *adapter.Service
	mqttClient  *mqttx.Client
	shutdownObs func()
	logger      *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	mqttClient, err := mqttx.Connect(mqttx.Options{BrokerURL: cfg.MQTTBrokerURL, ClientIDPrefix: "thread-adapter"})
	if err != nil {
		return nil, fmt.Errorf("connect mqtt: %w", err)
	}
	shutdownObs, promHandler, tracer := sharedobs.SetupObservability("thread-adapter")
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
		a.logger.Info("thread-adapter started", "addr", a.server.Addr)
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
