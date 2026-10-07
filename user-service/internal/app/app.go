package app

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/PetoAdam/homenavi/shared/authx"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/PetoAdam/homenavi/user-service/internal/auth"
	httptransport "github.com/PetoAdam/homenavi/user-service/internal/http"
	dbinfra "github.com/PetoAdam/homenavi/user-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/user-service/internal/users"
	"github.com/redis/go-redis/v9"
)

// App is the composed user-service application.
type App struct {
	server      *http.Server
	shutdownObs func()
	redisClient io.Closer
	logger      *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	if len(cfg.JWTPublicKeyPaths) == 0 {
		return nil, fmt.Errorf("JWT_PUBLIC_KEY_PATH or JWT_PUBLIC_KEY_PATHS not set for user-service")
	}
	publicKeys := make([]*rsa.PublicKey, 0, len(cfg.JWTPublicKeyPaths))
	for _, path := range cfg.JWTPublicKeyPaths {
		pubKey, err := auth.LoadRSAPublicKey(path)
		if err != nil {
			return nil, fmt.Errorf("load JWT public key %q: %w", path, err)
		}
		publicKeys = append(publicKeys, pubKey)
	}
	publicKeySet, err := auth.NewRSAPublicKeySet(publicKeys...)
	if err != nil {
		return nil, fmt.Errorf("create JWT public key set: %w", err)
	}
	redisConfig, err := redisx.LoadConfig(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("load Redis config: %w", err)
	}
	redisClient, err := redisx.Connect(context.Background(), redisConfig)
	if err != nil {
		return nil, fmt.Errorf("connect Redis: %w", err)
	}

	repo, err := dbinfra.New(cfg.DB, logger)
	if err != nil {
		_ = redisClient.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}

	shutdownObs, promHandler, tracer, err := sharedobs.SetupObservability("user-service")
	if err != nil {
		_ = redisClient.Close()
		return nil, fmt.Errorf("setup observability: %w", err)
	}
	service := users.NewService(repo)
	handler := httptransport.NewUsersHandler(service)
	router := httptransport.NewRouter(handler, promHandler, tracer, publicKeySet, auth.ValidationConfig{
		Issuer:          cfg.JWTIssuer,
		APIAudience:     cfg.JWTAPIAudience,
		ServiceAudience: cfg.JWTServiceAudience,
		SessionValidator: func(ctx context.Context, sessionID string) (bool, error) {
			status, err := redisClient.Get(ctx, authx.SessionStatusKey(sessionID)).Result()
			if errors.Is(err, redis.Nil) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return status == authx.SessionStatusActive, nil
		},
	})

	return &App{
		server:      &http.Server{Addr: ":" + cfg.Port, Handler: router},
		shutdownObs: shutdownObs,
		redisClient: redisClient,
		logger:      logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.shutdownObs()
	defer a.redisClient.Close()

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
