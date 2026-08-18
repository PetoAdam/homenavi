package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	httptransport "github.com/PetoAdam/homenavi/history-service/internal/http"
	dbinfra "github.com/PetoAdam/homenavi/history-service/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/history-service/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/history-service/internal/ingest"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
)

// App is the composed history-service application.
type App struct {
	server   *http.Server
	mqtt     *mqttinfra.Client
	shutdown func()
	logger   *slog.Logger
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
	mqttClient, err := mqttinfra.Connect(cfg.MQTT)
	if err != nil {
		return nil, fmt.Errorf("connect mqtt: %w", err)
	}
	shutdown, promHandler, tracer, err := sharedobs.SetupObservability("history-service")
	if err != nil {
		mqttClient.Close()
		return nil, fmt.Errorf("setup observability: %w", err)
	}
	ingestor := &ingest.Ingestor{Repo: repo, StatePrefix: cfg.TopicPrefix, AllowRetains: cfg.IngestRetained}
	subTopic := strings.TrimRight(cfg.TopicPrefix, "/") + "/#"
	subscribe := mqttClient.Subscribe
	if strings.TrimSpace(cfg.MQTTSharedGroup) != "" {
		subscribe = func(topic string, handler func(mqttinfra.Message)) error {
			return mqttClient.SubscribeShared(cfg.MQTTSharedGroup, topic, handler)
		}
	}
	if err := subscribe(subTopic, func(m mqttinfra.Message) {
		ingestor.HandleMessage(context.Background(), m, time.Now().UTC())
	}); err != nil {
		mqttClient.Close()
		return nil, fmt.Errorf("subscribe mqtt topic %s: %w", subTopic, err)
	}
	logger.Info("history ingest subscribed", "topic", subTopic, "shared_group", cfg.MQTTSharedGroup)

	handler := httptransport.NewServer(repo)
	router := httptransport.NewRouter(handler)
	return &App{
		server:   &http.Server{Addr: ":" + cfg.Port, Handler: sharedobs.WithMetricsEndpoint(promHandler, tracer, "history-service", router), ReadHeaderTimeout: 5 * time.Second},
		mqtt:     mqttClient,
		shutdown: shutdown,
		logger:   logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.mqtt.Close()
	defer func() {
		if a.shutdown != nil {
			a.shutdown()
		}
	}()
	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("history-service listening", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		a.logger.Info("history-service shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
