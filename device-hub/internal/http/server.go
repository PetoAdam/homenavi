package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/device-hub/internal/model"
	"github.com/PetoAdam/homenavi/shared/hdp"
	paho "github.com/eclipse/paho.mqtt.golang"
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
	Label             string   `json:"label"`
	Supported         bool     `json:"supported"`
	SupportsInterview bool     `json:"supports_interview"`
	DefaultTimeoutSec int      `json:"default_timeout_sec"`
	Instructions      []string `json:"instructions,omitempty"`
	CTALabel          string   `json:"cta_label,omitempty"`
	Notes             string   `json:"notes,omitempty"`
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

func (s *Server) handleHDPAdapterHello(_ paho.Client, msg mqttinfra.Message) {
	if s == nil || s.adapters == nil {
		return
	}
	s.adapters.upsertFromHello(msg.Payload())
}

func (s *Server) handleHDPAdapterStatus(_ paho.Client, msg mqttinfra.Message) {
	if s == nil || s.adapters == nil {
		return
	}
	s.adapters.upsertFromStatusTopic(msg.Topic(), msg.Payload())
}

func (s *Server) handleHDPMetadataEvent(_ paho.Client, msg mqttinfra.Message) {
	deviceID := strings.TrimPrefix(msg.Topic(), hdpMetadataPrefix)
	if deviceID == msg.Topic() || deviceID == "" {
		return
	}
	slog.Info("hdp metadata received", "device_id", deviceID)
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		slog.Debug("hdp metadata decode failed", "topic", msg.Topic(), "error", err)
		return
	}
	s.upsertMetadataFromHDP(deviceID, payload)
}

func (s *Server) handleHDPEvent(_ paho.Client, msg mqttinfra.Message) {
	if len(msg.Payload()) == 0 {
		return
	}
	var evt map[string]any
	if err := json.Unmarshal(msg.Payload(), &evt); err != nil {
		slog.Debug("hdp event decode failed", "topic", msg.Topic(), "error", err)
		return
	}
	deviceID := asString(evt["device_id"])
	if deviceID == "" {
		deviceID = strings.TrimPrefix(msg.Topic(), hdpEventPrefix)
	}
	if deviceID == "" {
		return
	}
	if asString(evt["event"]) == "device_removed" {
		ctx := context.Background()
		dev, _, err := s.resolveDevice(ctx, deviceID)
		if err != nil {
			slog.Warn("hdp device remove lookup failed", "device_id", deviceID, "error", err)
			return
		}
		if dev != nil {
			_ = s.repo.DeleteDeviceAndState(ctx, dev.ID.String())
		}
	}
}

func (s *Server) upsertMetadataFromHDP(deviceID string, payload map[string]any) {
	if deviceID == "" {
		return
	}
	ctx := context.Background()
	proto := normalizeProtocol(asString(payload["protocol"]))
	external := strings.TrimSpace(deviceID)
	if parsedProto, parsedExternal, ok := splitHDPDeviceID(deviceID); ok {
		proto = parsedProto
		external = parsedExternal
	}
	if proto == "" {
		slog.Warn("hdp metadata missing protocol", "device_id", deviceID)
		return
	}
	normExt, err := normalizeExternalID(proto, external)
	if err != nil {
		slog.Warn("hdp metadata normalize external failed", "device_id", deviceID, "protocol", proto, "error", err)
		return
	}
	dev, err := s.repo.GetByExternal(ctx, proto, normExt)
	if err != nil {
		slog.Warn("hdp metadata lookup failed", "device_id", deviceID, "protocol", proto, "external", normExt, "error", err)
		return
	}
	if dev == nil {
		dev = &model.Device{ExternalID: normExt, Protocol: proto}
	}
	changed := false
	if m := strings.TrimSpace(asString(payload["manufacturer"])); m != "" && dev.Manufacturer != m {
		dev.Manufacturer = m
		changed = true
	}
	if m := strings.TrimSpace(asString(payload["model"])); m != "" && dev.Model != m {
		dev.Model = m
		changed = true
	}
	if d := strings.TrimSpace(asString(payload["description"])); d != "" && dev.Description != d {
		dev.Description = d
		changed = true
	}
	if ic := strings.TrimSpace(asString(payload["icon"])); dev.Icon != ic {
		dev.Icon = ic
		changed = true
	}
	if dev.Protocol != proto {
		dev.Protocol = proto
		changed = true
	}
	if caps, ok := payload["capabilities"]; ok {
		if b, err := json.Marshal(caps); err == nil {
			dev.Capabilities = b
			changed = true
		}
	}
	if inputs, ok := payload["inputs"]; ok {
		if b, err := json.Marshal(inputs); err == nil {
			dev.Inputs = b
			changed = true
		}
	}
	if !changed && dev.ID != uuid.Nil {
		return
	}
	if err := s.repo.UpsertDevice(ctx, dev); err != nil {
		slog.Warn("hdp metadata upsert failed", "device_id", deviceID, "error", err)
		return
	}
}

func (s *Server) handleHDPStateEvent(_ paho.Client, msg mqttinfra.Message) {
	topic := strings.TrimPrefix(msg.Topic(), hdpStatePrefix)
	if topic == msg.Topic() {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		slog.Debug("hdp state decode failed", "topic", msg.Topic(), "error", err)
		return
	}
	deviceID := strings.TrimSpace(asString(payload["device_id"]))
	if deviceID == "" {
		deviceID = strings.TrimSpace(topic)
	}
	state := map[string]any{}
	if st, ok := payload["state"].(map[string]any); ok && len(st) > 0 {
		state = st
	}
	ts := parseTimestamp(payload["ts"])
	corr := strings.TrimSpace(asString(payload["corr"]))
	if len(state) == 0 {
		return
	}
	slog.Info("hdp state received", "device_id", deviceID, "corr", corr, "ts", ts, "keys", len(state))
	s.consumeState(deviceID, state, corr, ts, false)
}

func parseTimestamp(raw any) int64 {
	switch v := raw.(type) {
	case float64:
		if v <= 0 {
			return time.Now().UnixMilli()
		}
		return int64(v)
	case int64:
		if v <= 0 {
			return time.Now().UnixMilli()
		}
		return v
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return time.Now().UnixMilli()
		}
		return int64(f)
	default:
		return time.Now().UnixMilli()
	}
}

func asString(raw any) string {
	if raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", raw)
	}
}

func normalizeProtocol(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func (s *Server) consumeState(deviceTopicID string, state map[string]any, corr string, ts int64, publishHDP bool) {
	deviceKey := strings.TrimSpace(deviceTopicID)
	if deviceKey == "" || len(state) == 0 {
		return
	}
	ctx := context.Background()
	deviceUUID := deviceKey
	externalID := deviceKey
	dev, hdpID, err := s.resolveDevice(ctx, deviceKey)
	if err != nil {
		slog.Warn("state lookup failed", "device_id", deviceKey, "error", err)
	}
	if dev != nil {
		deviceUUID = dev.ID.String()
		if hdpID != "" {
			externalID = hdpID
		}
		_ = s.repo.TouchOnline(ctx, deviceUUID)
		if b, err := json.Marshal(state); err == nil {
			if err := s.repo.SaveDeviceState(ctx, deviceUUID, b); err != nil {
				slog.Warn("state persist failed", "device_id", deviceUUID, "error", err)
			}
		}
	}
	slog.Info("state persisted", "device_id", deviceUUID, "external_id", externalID, "corr", corr, "ts", ts, "keys", len(state))
	s.processCommandStateLifecycle(externalID, state, corr)
	if !publishHDP {
		return
	}
	envelope := map[string]any{
		"schema":    hdpSchema,
		"type":      "state",
		"device_id": externalID,
		"ts":        ts,
		"state":     state,
	}
	if corr != "" {
		envelope["corr"] = corr
	}
	if data, err := json.Marshal(envelope); err == nil {
		if err := s.mqtt.PublishWith(hdpStatePrefix+externalID, data, true); err != nil {
			slog.Warn("hdp state publish failed", "device_id", externalID, "error", err)
		}
	}
}

func normalizeExternalID(protocol, external string) (string, error) {
	proto := normalizeProtocol(protocol)
	ext := strings.TrimSpace(external)
	if proto == "" || ext == "" {
		return "", errors.New("protocol and external_id are required")
	}
	segments := strings.Split(ext, "/")
	filtered := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			filtered = append(filtered, seg)
		}
	}
	if len(filtered) == 0 {
		return "", fmt.Errorf("external_id suffix is required")
	}
	if strings.EqualFold(filtered[0], proto) {
		filtered = filtered[1:]
	}
	if len(filtered) == 0 {
		return "", fmt.Errorf("external_id suffix is required")
	}
	normalized := strings.Join(filtered, "/")
	if proto == "zigbee" {
		normalized = strings.ToLower(normalized)
		if !zigbeeIEEEExternalIDRe.MatchString(normalized) {
			return "", fmt.Errorf("invalid zigbee external_id: %q", normalized)
		}
		return normalized, nil
	}
	return normalized, nil
}

var zigbeeIEEEExternalIDRe = regexp.MustCompile(`^0x[0-9a-f]{16}$`)

func (s *Server) cleanupInvalidZigbeeDevices(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	devices, err := s.repo.List(ctx)
	if err != nil {
		slog.Warn("zigbee cleanup skipped; device list failed", "error", err)
		return
	}
	for _, dev := range devices {
		if normalizeProtocol(dev.Protocol) != "zigbee" {
			continue
		}
		norm, err := normalizeExternalID("zigbee", dev.ExternalID)
		if err == nil && norm != "" {
			continue
		}
		if err := s.repo.DeleteDeviceAndState(ctx, dev.ID.String()); err != nil {
			slog.Warn("invalid zigbee device delete failed", "device_id", dev.ID.String(), "external_id", dev.ExternalID, "error", err)
			continue
		}
		slog.Warn("invalid zigbee device removed", "device_id", dev.ID.String(), "external_id", dev.ExternalID)
	}
}

func canonicalHDPDeviceID(protocol, external string) string {
	proto := normalizeProtocol(protocol)
	ext := strings.Trim(strings.TrimSpace(external), "/")
	if ext == "" {
		return ""
	}
	if proto != "" && strings.HasPrefix(ext, proto+"/") {
		return ext
	}
	if proto != "" {
		return fmt.Sprintf("%s/%s", proto, ext)
	}
	return ext
}

func (s *Server) resolveDevice(ctx context.Context, deviceID string) (dev *model.Device, hdpID string, err error) {
	if s == nil || s.repo == nil {
		return nil, "", errors.New("repository unavailable")
	}
	trimmed := strings.TrimSpace(deviceID)
	if trimmed == "" {
		return nil, "", nil
	}
	if proto, ext, ok := splitHDPDeviceID(trimmed); ok {
		normExt, err := normalizeExternalID(proto, ext)
		if err != nil {
			return nil, "", err
		}
		found, err := s.repo.GetByExternal(ctx, proto, normExt)
		if err != nil {
			return nil, "", err
		}
		return found, canonicalHDPDeviceID(proto, normExt), nil
	}
	found, err := s.repo.GetByID(ctx, trimmed)
	if err != nil {
		return nil, "", err
	}
	if found == nil {
		return nil, "", nil
	}
	return found, canonicalHDPDeviceID(found.Protocol, found.ExternalID), nil
}

func splitHDPDeviceID(deviceID string) (protocol string, external string, ok bool) {
	trimmed := strings.Trim(strings.TrimSpace(deviceID), "/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", trimmed, false
	}
	proto := normalizeProtocol(parts[0])
	ext := strings.Trim(parts[1], "/")
	if proto == "" || ext == "" {
		return "", "", false
	}
	return proto, ext, true
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
