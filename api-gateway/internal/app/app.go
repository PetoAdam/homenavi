package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	httptransport "github.com/PetoAdam/homenavi/api-gateway/internal/http"
	apiMiddleware "github.com/PetoAdam/homenavi/api-gateway/internal/middleware"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/redis/go-redis/v9"
)

// App is the composed api-gateway application.
type App struct {
	server      *http.Server
	redisClient redis.UniversalClient
	shutdownObs func()
	logger      *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	pubKey, err := apiMiddleware.LoadRSAPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load JWT public key: %w", err)
	}

	redisClient, err := redisx.Connect(context.Background(), cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	shutdown, promHandler, tracer, err := sharedobs.SetupObservability("api-gateway")
	if err != nil {
		_ = redisClient.Close()
		return nil, fmt.Errorf("setup observability: %w", err)
	}
	wsRouter := httptransport.NewWebSocketRouter(cfg.Gateway, redisClient, pubKey)
	mainRouter := httptransport.NewMainRouter(cfg.Gateway, redisClient, pubKey, promHandler, tracer, cfg.CORSAllowOrigins)
	root := httptransport.NewRootRouter(wsRouter, mainRouter)

	return &App{
		server:      &http.Server{Addr: cfg.Gateway.ListenAddr, Handler: root},
		redisClient: redisClient,
		shutdownObs: shutdown,
		logger:      logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.shutdownObs()
	defer a.redisClient.Close()

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("api gateway starting", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
