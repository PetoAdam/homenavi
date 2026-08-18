package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/PetoAdam/homenavi/automation-service/internal/auth"
	"github.com/PetoAdam/homenavi/automation-service/internal/engine"
	httptransport "github.com/PetoAdam/homenavi/automation-service/internal/http"
	dbinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/db"
	eventsinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/events"
	mqttinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/shared/cachex"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
)

// App is the composed automation-service application.
type App struct {
	server   *http.Server
	engine   *engine.Engine
	mqtt     *mqttinfra.Client
	cache    *cachex.JSONStore
	shutdown func()
	logger   *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	pubKey, err := auth.LoadRSAPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load JWT public key: %w", err)
	}

	database, err := dbinfra.Open(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	repo, err := dbinfra.New(database)
	if err != nil {
		return nil, fmt.Errorf("init repository: %w", err)
	}

	mqttClient, err := mqttinfra.Connect(cfg.MQTT)
	if err != nil {
		return nil, fmt.Errorf("connect mqtt: %w", err)
	}

	var runEvents engine.RunEventBus
	if cfg.MQTTSharedGroup != "" {
		runEvents, err = eventsinfra.NewRedisRunEventBus(context.Background(), cfg.Redis)
		if err != nil {
			mqttClient.Close()
			return nil, fmt.Errorf("connect shared run events: %w", err)
		}
	}
	var cacheStore *cachex.JSONStore
	if cfg.ListCacheTTL > 0 {
		cacheStore, err = cachex.NewJSONStore(context.Background(), cfg.Redis)
		if err != nil {
			logger.Warn("automation-service cache disabled", "error", err)
		}
	}
	eng := engine.New(repo, mqttClient, engine.Options{
		EmailServiceURL:     cfg.EmailServiceURL,
		ERSServiceURL:       cfg.ERSServiceURL,
		IntegrationProxyURL: cfg.IntegrationProxyURL,
		MQTTSharedGroup:     cfg.MQTTSharedGroup,
		RunEvents:           runEvents,
		SelectorStore:       cacheStore,
	})
	shutdown, promHandler, tracer, err := sharedobs.SetupObservability("automation-service")
	if err != nil {
		return nil, fmt.Errorf("setup observability: %w", err)
	}
	handler := httptransport.NewServer(
		repo,
		eng,
		pubKey,
		cfg.UserServiceURL,
		cfg.IntegrationProxyURL,
		&http.Client{Timeout: 10 * time.Second},
		httptransport.WithCache(cacheStore, cfg.ListCacheTTL),
	)
	router := httptransport.NewRouter(handler)

	return &App{
		server: &http.Server{
			Addr:              ":" + cfg.Port,
			Handler:           sharedobs.WithMetricsEndpoint(promHandler, tracer, "automation-service", router),
			ReadHeaderTimeout: 5 * time.Second,
		},
		engine:   eng,
		mqtt:     mqttClient,
		cache:    cacheStore,
		shutdown: shutdown,
		logger:   logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.engine.Start(ctx); err != nil {
		return fmt.Errorf("start engine: %w", err)
	}
	defer a.engine.Stop()
	defer a.mqtt.Close()
	defer func() {
		if a.shutdown != nil {
			a.shutdown()
		}
	}()
	defer func() {
		if a.cache != nil {
			_ = a.cache.Close()
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("automation-service listening", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("automation-service shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
