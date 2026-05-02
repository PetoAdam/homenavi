package adapter

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/PetoAdam/homenavi/shared/mqttx"
)

// matterDevice holds the last-known state and metadata of a commissioned Matter node.
type matterDevice struct {
	ExternalID   string
	Type         string
	Manufacturer string
	Model        string
	Icon         string
	State        map[string]any
}

const heartbeatInterval = 20 * time.Second

// Client is the subset of MQTT behavior the adapter needs.
type Client interface {
	Publish(topic string, payload []byte) error
	PublishWith(topic string, payload []byte, retain bool) error
	Subscribe(topic string, cb mqttx.Handler) error
}

// Service orchestrates Matter adapter lifecycle: hello/status publishing,
// pairing command handling with full validation and synthetic commissioning,
// device command routing with on/off, level, and color-temp cluster support.
type Service struct {
	client                   Client
	httpClient               *http.Client
	enabled                  bool
	adapterID                string
	version                  string
	defaultTimeoutSec        int
	defaultNetworkPath       string
	commissioningInterface   string
	enableBLE                bool
	enableThread             bool
	enableOnNetwork          bool
	commissionerEnabled      bool
	commissionerCommand      string
	commissionerArgs         []string
	commissionerTimeout      time.Duration
	otbrBaseURL              string
	otbrExpectedState        string
	threadBorderRouterHost   string
	threadBorderRouterPort   int
	threadDatasetSource      string
	threadOperationalDataset string
	ctx                      context.Context
	cancel                   context.CancelFunc
	pairingCancel            context.CancelFunc
	pairingMu                sync.Mutex
	devices                  sync.Map // key: externalID (e.g. "matter-device-001"), value: matterDevice
}

func New(client Client, cfg Config) *Service {
	defaultNetworkPath := strings.TrimSpace(strings.ToLower(cfg.DefaultNetworkPath))
	if defaultNetworkPath == "" {
		defaultNetworkPath = "on_network"
	}
	defaultTimeoutSec := cfg.DefaultTimeoutSec
	if defaultTimeoutSec <= 0 {
		defaultTimeoutSec = 300
	}
	return &Service{
		client:                   client,
		httpClient:               &http.Client{Timeout: 5 * time.Second},
		enabled:                  cfg.Enabled,
		adapterID:                cfg.AdapterID,
		version:                  cfg.Version,
		defaultTimeoutSec:        defaultTimeoutSec,
		defaultNetworkPath:       defaultNetworkPath,
		commissioningInterface:   strings.TrimSpace(cfg.CommissioningInterface),
		enableBLE:                cfg.EnableBLE,
		enableThread:             cfg.EnableThread,
		enableOnNetwork:          cfg.EnableOnNetwork,
		commissionerEnabled:      cfg.CommissionerEnabled,
		commissionerCommand:      strings.TrimSpace(cfg.CommissionerCommand),
		commissionerArgs:         append([]string(nil), cfg.CommissionerArgs...),
		commissionerTimeout:      cfg.CommissionerTimeout,
		otbrBaseURL:              strings.TrimSpace(cfg.OTBRBaseURL),
		otbrExpectedState:        strings.TrimSpace(cfg.OTBRExpectedState),
		threadBorderRouterHost:   strings.TrimSpace(cfg.ThreadBorderRouterHost),
		threadBorderRouterPort:   cfg.ThreadBorderRouterPort,
		threadDatasetSource:      strings.TrimSpace(cfg.ThreadDatasetSource),
		threadOperationalDataset: strings.TrimSpace(cfg.ThreadOperationalDataset),
	}
}

func (s *Service) Start(ctx context.Context) error {
	if !s.enabled {
		slog.Info("matter adapter disabled", "status", "placeholder")
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.publishHello()
	s.publishStatus("online", "starting")
	if err := s.client.Subscribe(hdp.PairingCommandPrefix+"matter", s.handlePairingCommand); err != nil {
		slog.Warn("matter adapter pairing subscribe failed", "error", err)
	}
	if err := s.client.Subscribe(hdp.CommandPrefix+"matter/#", s.handleDeviceCommand); err != nil {
		slog.Warn("matter adapter command subscribe failed", "error", err)
	}
	go s.runHeartbeat()
	slog.Info("matter adapter running", "adapter_id", s.adapterID, "version", s.version)
	return nil
}

func (s *Service) Stop() {
	slog.Info("matter adapter stopping")
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
	if strings.HasPrefix(id, "matter/") {
		return id
	}
	parts := strings.Split(id, "/")
	suffix := strings.TrimSpace(parts[len(parts)-1])
	if suffix == "" {
		return ""
	}
	return "matter/" + suffix
}

func (s *Service) externalFromHDP(deviceID string) (string, string) {
	id := strings.Trim(strings.TrimSpace(deviceID), "/")
	if id == "" {
		return "", ""
	}
	parts := strings.Split(id, "/")
	if len(parts) == 1 {
		return "matter", parts[0]
	}
	proto := strings.ToLower(parts[0])
	if len(parts) >= 3 {
		return proto, strings.Join(parts[2:], "/")
	}
	return proto, strings.Join(parts[1:], "/")
}

var _ Client = (*mqttx.Client)(nil)
