package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/PetoAdam/homenavi/weather-service/internal/forecast"
	httptransport "github.com/PetoAdam/homenavi/weather-service/internal/http"
	cacheinfra "github.com/PetoAdam/homenavi/weather-service/internal/infra/cache"
	"github.com/PetoAdam/homenavi/weather-service/internal/infra/openweather"
)

// App is the composed weather-service application.
type App struct {
	server   *http.Server
	shutdown func()
	logger   *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	provider := openweather.New(cfg.OpenWeatherAPIKey)
	cache := cacheinfra.NewMemoryCache(cfg.CacheTTL)
	service := forecast.NewService(provider, cache)
	handler := httptransport.NewHandler(service)
	router := httptransport.NewRouter(handler)
	shutdown, promHandler, tracer, err := sharedobs.SetupObservability("weather-service")
	if err != nil {
		return nil, fmt.Errorf("setup observability: %w", err)
	}

	return &App{
		server: &http.Server{
			Addr:         ":" + cfg.Port,
			Handler:      sharedobs.WithMetricsEndpoint(promHandler, tracer, "weather-service", router),
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		shutdown: shutdown,
		logger:   logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer func() {
		if a.shutdown != nil {
			a.shutdown()
		}
	}()
	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("weather-service started", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("shutting down weather-service")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown error: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
