package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/PetoAdam/homenavi/entity-registry-service/internal/autoimport"
	"github.com/PetoAdam/homenavi/entity-registry-service/internal/backfill"
	httptransport "github.com/PetoAdam/homenavi/entity-registry-service/internal/http"
	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/entity-registry-service/internal/realtime"
)

// App is the composed entity-registry-service application.
type App struct {
	server *http.Server
	repo   *dbinfra.Repository
	hub    *realtime.Hub
	db     *sql.DB
	cfg    Config
	logger *slog.Logger
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	database, err := dbinfra.Open(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	repo, err := dbinfra.New(database)
	if err != nil {
		return nil, fmt.Errorf("init repository: %w", err)
	}
	underlyingDB, err := database.DB()
	if err != nil {
		return nil, fmt.Errorf("resolve sql database: %w", err)
	}
	hub := realtime.NewHub()
	handler := httptransport.NewServer(repo, hub)
	return &App{
		server: &http.Server{Addr: ":" + cfg.Port, Handler: httptransport.NewRouter(handler), ReadHeaderTimeout: 5 * time.Second},
		repo:   repo,
		hub:    hub,
		db:     underlyingDB,
		cfg:    cfg,
		logger: logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	}()

	if a.cfg.AutoImport {
		backfill.Start(ctx, a.repo, a.cfg.DeviceHubURL, &http.Client{Timeout: 10 * time.Second})
		autoimport.Start(ctx, a.repo, a.cfg.MQTT.BrokerURL, a.hub)
		a.logger.Info("ers auto-import enabled", "broker", a.cfg.MQTT.BrokerURL, "device_hub", a.cfg.DeviceHubURL)
	}

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("entity-registry-service listening", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("entity-registry-service shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
