package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/PetoAdam/homenavi/user-service/internal/auth"
	httptransport "github.com/PetoAdam/homenavi/user-service/internal/http"
	dbinfra "github.com/PetoAdam/homenavi/user-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/user-service/internal/users"
)

// App is the composed user-service application.
type App struct {
	server      *http.Server
	shutdownObs func()
	logger      *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	if cfg.JWTPublicKeyPath == "" {
		return nil, fmt.Errorf("JWT_PUBLIC_KEY_PATH not set for user-service")
	}
	pubKey, err := auth.LoadRSAPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load JWT public key: %w", err)
	}

	repo, err := dbinfra.New(cfg.DB, logger)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	shutdownObs, promHandler, tracer, err := sharedobs.SetupObservability("user-service")
	if err != nil {
		return nil, fmt.Errorf("setup observability: %w", err)
	}
	service := users.NewService(repo)
	handler := httptransport.NewUsersHandler(service)
	router := httptransport.NewRouter(handler, promHandler, tracer, pubKey)

	return &App{
		server:      &http.Server{Addr: ":" + cfg.Port, Handler: router},
		shutdownObs: shutdownObs,
		logger:      logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.shutdownObs()

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("user service starting", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
