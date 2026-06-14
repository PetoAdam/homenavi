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
	mqttinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/shared/cachex"
)

// App is the composed automation-service application.
type App struct {
	server *http.Server
	engine *engine.Engine
	mqtt   *mqttinfra.Client
	cache  *cachex.JSONStore
	logger *slog.Logger
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

	mqttClient, err := mqttinfra.Connect(cfg.MQTT.BrokerURL, cfg.MQTT.ClientID)
	if err != nil {
		return nil, fmt.Errorf("connect mqtt: %w", err)
	}

	eng := engine.New(repo, mqttClient, engine.Options{
		EmailServiceURL:     cfg.EmailServiceURL,
		ERSServiceURL:       cfg.ERSServiceURL,
		IntegrationProxyURL: cfg.IntegrationProxyURL,
	})
	var cacheStore *cachex.JSONStore
	if cfg.ListCacheTTL > 0 {
		cacheStore, err = cachex.NewJSONStore(context.Background(), cfg.Redis)
		if err != nil {
			logger.Warn("automation-service workflow cache disabled", "error", err)
		}
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

	return &App{
		server: &http.Server{
			Addr:              ":" + cfg.Port,
			Handler:           httptransport.NewRouter(handler),
			ReadHeaderTimeout: 5 * time.Second,
		},
		engine: eng,
		mqtt:   mqttClient,
		cache:  cacheStore,
		logger: logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.engine.Start(ctx); err != nil {
		return fmt.Errorf("start engine: %w", err)
	}
	defer a.engine.Stop()
	defer a.mqtt.Close()
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
