package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	proxyauth "github.com/PetoAdam/homenavi/integration-proxy/internal/auth"
	integrationconfig "github.com/PetoAdam/homenavi/integration-proxy/internal/config"
	httptransport "github.com/PetoAdam/homenavi/integration-proxy/internal/http"
)

// App is the composed integration-proxy application.
type App struct {
	proxy  *httptransport.Server
	server *http.Server
	logger *log.Logger
	cfg    Config
}

func New(cfg Config, logger *log.Logger) (*App, error) {
	if strings.TrimSpace(cfg.JWTPublicKeyPath) == "" {
		return nil, fmt.Errorf("JWT_PUBLIC_KEY_PATH is required to protect /integrations/*")
	}
	pubKey, err := proxyauth.LoadRSAPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load JWT public key: %w", err)
	}
	installed, err := integrationconfig.Load(cfg.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	validator, err := httptransport.LoadSchema(cfg.SchemaPath)
	if err != nil {
		return nil, fmt.Errorf("load schema: %w", err)
	}
	proxyServer := httptransport.NewServer(logger, validator, pubKey, cfg.SchemaPath, cfg.ConfigPath)
	for _, ic := range installed.Integrations {
		if err := proxyServer.AddIntegration(ic); err != nil {
			return nil, fmt.Errorf("add integration %q: %w", ic.ID, err)
		}
	}
	return &App{
		proxy:  proxyServer,
		server: &http.Server{Addr: cfg.ListenAddr, Handler: httptransport.NewRouter(proxyServer, pubKey), ReadHeaderTimeout: 5 * time.Second},
		logger: logger,
		cfg:    cfg,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go a.proxy.StartRefreshLoop(ctx, a.cfg.RefreshInterval)
	go a.proxy.StartUpdateLoop(ctx, a.cfg.UpdateCheckInterval)
	go func() {
		a.logger.Printf("listening on %s", a.cfg.ListenAddr)
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
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
