package adapter

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/PetoAdam/homenavi/shared/mqttx"
)

const heartbeatInterval = 20 * time.Second

// Client is the subset of MQTT behavior the adapter needs.
type Client interface {
	Publish(topic string, payload []byte) error
	PublishWith(topic string, payload []byte, retain bool) error
	Subscribe(topic string, cb mqttx.Handler) error
}

// Service is a minimal Thread placeholder that keeps observability and health
// endpoints alive while the protocol implementation is built.
type Service struct {
	client    Client
	enabled   bool
	adapterID string
	version   string
	ctx       context.Context
	cancel    context.CancelFunc
}

func New(client Client, cfg Config) *Service {
	return &Service{client: client, enabled: cfg.Enabled, adapterID: cfg.AdapterID, version: cfg.Version}
}

func (s *Service) Start(ctx context.Context) error {
	if !s.enabled {
		slog.Info("mock adapter disabled", "status", "placeholder")
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.publishHello()
	s.publishStatus("online", "placeholder")
	if err := s.client.Subscribe(hdp.PairingCommandPrefix+"mock", s.handlePairingCommand); err != nil {
		slog.Warn("mock adapter pairing subscribe failed", "error", err)
	}
	if err := s.client.Subscribe(hdp.CommandPrefix+"mock/#", s.handleDeviceCommand); err != nil {
		slog.Warn("mock adapter command subscribe failed", "error", err)
	}
	go s.runHeartbeat()
	slog.Info("mock adapter placeholder running", "status", "planned")
	return nil
}

func (s *Service) Stop() {
	slog.Info("mock adapter stopping")
	if s.cancel != nil {
		s.cancel()
	}
	s.publishStatus("offline", "shutdown")
}

func (s *Service) runHeartbeat() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.publishStatus("online", "heartbeat")
		}
	}
}

func (s *Service) hdpDeviceID(deviceID string) string {
	id := strings.Trim(strings.TrimSpace(deviceID), "/")
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, "mock/") {
		return id
	}
	parts := strings.Split(id, "/")
	suffix := strings.TrimSpace(parts[len(parts)-1])
	if suffix == "" {
		return ""
	}
	return "mock/" + suffix
}

func (s *Service) externalFromHDP(deviceID string) (string, string) {
	id := strings.Trim(strings.TrimSpace(deviceID), "/")
	if id == "" {
		return "", ""
	}
	parts := strings.Split(id, "/")
	if len(parts) == 1 {
		return "mock", parts[0]
	}
	proto := strings.ToLower(parts[0])
	if len(parts) >= 3 {
		return proto, strings.Join(parts[2:], "/")
	}
	return proto, strings.Join(parts[1:], "/")
}

var _ Client = (*mqttx.Client)(nil)
