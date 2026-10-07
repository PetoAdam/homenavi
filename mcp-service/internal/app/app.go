package app

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/PetoAdam/homenavi/mcp-service/internal/mcpserver"
	"github.com/PetoAdam/homenavi/shared/authx"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/golang-jwt/jwt/v5"
)

type App struct {
	server      *http.Server
	shutdown    func() error
	shutdownObs func()
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	var publicKey *rsa.PublicKey
	if cfg.Enabled {
		keyData, err := os.ReadFile(cfg.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read MCP JWT public key: %w", err)
		}
		publicKey, err = jwt.ParseRSAPublicKeyFromPEM(keyData)
		if err != nil {
			return nil, fmt.Errorf("parse MCP JWT public key: %w", err)
		}
	}
	redisConfig, err := redisx.LoadConfig(redisx.Config{Addrs: []string{"redis:6379"}})
	if err != nil {
		return nil, fmt.Errorf("load Redis config: %w", err)
	}
	redisClient, err := redisx.Connect(context.Background(), redisConfig)
	if err != nil {
		return nil, fmt.Errorf("connect Redis: %w", err)
	}
	handler := mcpserver.New(mcpserver.Config{Enabled: cfg.Enabled, Issuer: cfg.Issuer, Resource: cfg.Resource, PublicKey: publicKey, DeviceHubURL: cfg.DeviceHubURL, HistoryURL: cfg.HistoryURL, EntityRegistryURL: cfg.EntityRegistryURL, AutomationURL: cfg.AutomationURL, AuthServiceURL: cfg.AuthServiceURL, AllowedOrigins: cfg.AllowedOrigins, EnabledTools: cfg.EnabledTools, RateLimitPerMinute: cfg.RateLimitPerMinute, Logger: logger, SessionValidator: func(ctx context.Context, sessionID string) (bool, error) {
		status, err := redisClient.Get(ctx, authx.SessionStatusKey(sessionID)).Result()
		return status == authx.SessionStatusActive, err
	}, RateLimiter: func(ctx context.Context, sessionID string) (bool, error) {
		window := time.Now().UTC().Truncate(time.Minute).Unix()
		key := "mcp_rate:" + sessionID + ":" + strconv.FormatInt(window, 10)
		count, err := redisClient.Incr(ctx, key).Result()
		if err != nil {
			return false, err
		}
		if count == 1 {
			if err := redisClient.Expire(ctx, key, 2*time.Minute).Err(); err != nil {
				return false, err
			}
		}
		return count <= int64(cfg.RateLimitPerMinute), nil
	}})
	shutdownObs, promHandler, tracer, err := sharedobs.SetupObservability("mcp-service")
	if err != nil {
		_ = redisClient.Close()
		return nil, fmt.Errorf("setup observability: %w", err)
	}
	return &App{server: &http.Server{Addr: ":" + cfg.Port, Handler: sharedobs.WithMetricsEndpoint(promHandler, tracer, "mcp-service", handler)}, shutdown: redisClient.Close, shutdownObs: shutdownObs}, nil
}

func (a *App) Run() error {
	defer a.shutdownObs()
	defer a.shutdown()
	return a.server.ListenAndServe()
}
