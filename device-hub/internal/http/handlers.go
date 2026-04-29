package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/google/uuid"
)

// IntegrationDescriptor surfaces adapter availability to the UI without
// hardcoding the list of protocols on the frontend.
type IntegrationDescriptor struct {
	Protocol string `json:"protocol"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Notes    string `json:"notes,omitempty"`
}

// PairingConfig describes how the UI should render pairing for a protocol.
// This lets the frontend avoid protocol-specific hardcoding.
type PairingConfig struct {
	Protocol          string   `json:"protocol"`
	SchemaVersion     string   `json:"schema_version"`
	Label             string   `json:"label"`
	Supported         bool     `json:"supported"`
	SupportsInterview bool     `json:"supports_interview"`
	DefaultTimeoutSec int      `json:"default_timeout_sec"`
	Instructions      []string `json:"instructions,omitempty"`
	CTALabel          string   `json:"cta_label,omitempty"`
	Notes             string   `json:"notes,omitempty"`
	Flow              any      `json:"flow,omitempty"`
}

type pairingMetadata struct {
	Icon         string `json:"icon,omitempty"`
	Description  string `json:"description,omitempty"`
	Type         string `json:"type,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
}

type pairingSession struct {
	ID                  string          `json:"id"`
	Protocol            string          `json:"protocol"`
	Mode                string          `json:"mode,omitempty"`
	FlowID              string          `json:"flow_id,omitempty"`
	Inputs              map[string]any  `json:"inputs,omitempty"`
	Stage               string          `json:"stage,omitempty"`
	Message             string          `json:"message,omitempty"`
	ErrorCode           string          `json:"error_code,omitempty"`
	RequiredInputs      []string        `json:"required_inputs,omitempty"`
	Status              string          `json:"status"`
	Active              bool            `json:"active"`
	StartedAt           time.Time       `json:"started_at"`
	ExpiresAt           time.Time       `json:"expires_at"`
	DeviceID            string          `json:"device_id,omitempty"`
	Metadata            pairingMetadata `json:"metadata,omitempty"`
	cancel              context.CancelFunc
	knownDevices        map[string]struct{} `json:"-"`
	candidateExternalID string              `json:"-"`
	awaitingInterview   bool                `json:"-"`
}

func (p *pairingSession) clone() pairingSession {
	if p == nil {
		return pairingSession{}
	}
	clone := *p
	clone.cancel = nil
	clone.knownDevices = nil
	if len(p.Inputs) > 0 {
		clone.Inputs = make(map[string]any, len(p.Inputs))
		for key, value := range p.Inputs {
			clone.Inputs[key] = value
		}
	}
	if len(p.RequiredInputs) > 0 {
		clone.RequiredInputs = append([]string(nil), p.RequiredInputs...)
	}
	clone.candidateExternalID = ""
	clone.awaitingInterview = false
	return clone
}

type Server struct {
	repo             *dbinfra.Repository
	mqtt             mqttinfra.ClientAPI
	adapters         *adapterRegistry
	pairingMu        sync.Mutex
	pairings         map[string]*pairingSession
	commandMu        sync.Mutex
	commandsByCorr   map[string]*pendingCommand
	commandsByDevice map[string]*pendingCommand
	commandTimeout   time.Duration
}

func NewServer(repo *dbinfra.Repository, mqtt mqttinfra.ClientAPI) *Server {
	if repo == nil {
		slog.Warn("NewServer initialized without repository; persistence operations will be unavailable")
	}
	if mqtt == nil {
		slog.Warn("NewServer initialized without mqtt client; publish/subscribe will be disabled")
	}
	return &Server{
		repo:             repo,
		mqtt:             mqtt,
		adapters:         newAdapterRegistry(0),
		pairings:         make(map[string]*pairingSession),
		commandsByCorr:   make(map[string]*pendingCommand),
		commandsByDevice: make(map[string]*pendingCommand),
		commandTimeout:   defaultCommandLifecycleTimeout,
	}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/hdp/devices", s.handleDeviceCollection)
	mux.HandleFunc("/api/hdp/devices/", s.handleDeviceRequest)
	mux.HandleFunc("/api/hdp/integrations", s.handleIntegrations)
	mux.HandleFunc("/api/hdp/pairing-config", s.handlePairingConfig)
	mux.HandleFunc("/api/hdp/pairings", s.handlePairings)
	if s.repo != nil {
		s.cleanupInvalidZigbeeDevices(context.Background())
	}
	if s.mqtt == nil {
		slog.Warn("mqtt client not configured; skipping mqtt subscriptions")
		return
	}
	if err := s.mqtt.Subscribe(hdpAdapterHelloTopic, s.handleHDPAdapterHello); err != nil {
		slog.Error("hdp adapter hello subscribe failed", "error", err)
	}
	if err := s.mqtt.Subscribe(hdpAdapterStatusPrefix+"#", s.handleHDPAdapterStatus); err != nil {
		slog.Error("hdp adapter status subscribe failed", "error", err)
	}
	if err := s.mqtt.Subscribe(hdpMetadataPrefix+"#", s.handleHDPMetadataEvent); err != nil {
		slog.Error("hdp metadata subscribe failed", "error", err)
	}
	if err := s.mqtt.Subscribe(hdpStatePrefix+"#", s.handleHDPStateEvent); err != nil {
		slog.Error("hdp state subscribe failed", "error", err)
	}
	if err := s.mqtt.Subscribe(hdpEventPrefix+"#", s.handleHDPEvent); err != nil {
		slog.Error("hdp event subscribe failed", "error", err)
	}
	if err := s.mqtt.Subscribe(hdpCommandResultPrefix+"#", s.handleHDPCommandResultEvent); err != nil {
		slog.Error("hdp command result subscribe failed", "error", err)
	}
	if err := s.mqtt.Subscribe(hdpPairingProgressPrefix+"#", s.handleHDPPairingProgressEvent); err != nil {
		slog.Error("hdp pairing progress subscribe failed", "error", err)
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.Register(mux)
	return mux
}

type deviceListItem struct {
	ID           uuid.UUID       `json:"id"`
	DeviceID     string          `json:"device_id"`
	Protocol     string          `json:"protocol"`
	ExternalID   string          `json:"external_id"`
	Type         string          `json:"type"`
	Manufacturer string          `json:"manufacturer"`
	Model        string          `json:"model"`
	Description  string          `json:"description"`
	Firmware     string          `json:"firmware"`
	Icon         string          `json:"icon"`
	Capabilities json.RawMessage `json:"capabilities,omitempty"`
	Inputs       json.RawMessage `json:"inputs,omitempty"`
	Online       bool            `json:"online"`
	LastSeen     time.Time       `json:"last_seen"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	State        json.RawMessage `json:"state"`
}

const (
	hdpSchema                = hdp.SchemaV1
	hdpMetadataPrefix        = hdp.MetadataPrefix
	hdpStatePrefix           = hdp.StatePrefix
	hdpEventPrefix           = hdp.EventPrefix
	hdpCommandPrefix         = hdp.CommandPrefix
	hdpCommandResultPrefix   = hdp.CommandResultPrefix
	hdpPairingCommandPrefix  = hdp.PairingCommandPrefix
	hdpPairingProgressPrefix = hdp.PairingProgressPrefix
)

var (
	errPairingActive      = errors.New("pairing already active")
	errPairingNotFound    = errors.New("pairing session not found")
	errPairingUnsupported = errors.New("protocol does not support pairing operations")
)
