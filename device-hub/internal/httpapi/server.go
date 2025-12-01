package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"device-hub/internal/model"
	"device-hub/internal/mqtt"
	"device-hub/internal/store"
)

// IntegrationDescriptor surfaces adapter availability to the UI without
// hardcoding the list of protocols on the frontend.
type IntegrationDescriptor struct {
	Protocol string `json:"protocol"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Notes    string `json:"notes,omitempty"`
}

type pairingMetadata struct {
	Name         string `json:"name,omitempty"`
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
	repo         *store.Repository
	mqtt         *mqtt.Client
	integrations []IntegrationDescriptor
	pairingMu    sync.Mutex
	pairings     map[string]*pairingSession
}

func NewServer(repo *store.Repository, mqtt *mqtt.Client, integrations []IntegrationDescriptor) *Server {
	return &Server{
		repo:         repo,
		mqtt:         mqtt,
		integrations: integrations,
		pairings:     make(map[string]*pairingSession),
	}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/devicehub/devices", s.handleDeviceCollection)
	mux.HandleFunc("/api/devicehub/devices/", s.handleDeviceRequest)
	mux.HandleFunc("/api/devicehub/integrations", s.handleIntegrations)
	mux.HandleFunc("/api/devicehub/pairings", s.handlePairings)
	if err := s.mqtt.Subscribe(deviceUpsertTopic, s.handleDeviceUpsertEvent); err != nil {
		slog.Error("device upsert subscribe failed", "error", err)
	}
	if err := s.mqtt.Subscribe(pairingProgressTopic, s.handlePairingProgressEvent); err != nil {
		slog.Error("pairing progress subscribe failed", "error", err)
	}
}

func (s *Server) handleDeviceUpsertEvent(_ paho.Client, msg mqtt.Message) {
	payload := msg.Payload()
	if len(payload) == 0 {
		return
	}
	var dev model.Device
	if err := json.Unmarshal(payload, &dev); err != nil {
		slog.Debug("device upsert decode failed", "error", err)
		return
	}
	s.handlePairingCandidate(&dev)
}

type commandRequest struct {
	State map[string]any `json:"state"`
	Input *struct {
		ID    string `json:"id"`
		Value any    `json:"value"`
	} `json:"input"`
	TransitionMs *int `json:"transition_ms"`
}

type commandResponse struct {
	Status       string `json:"status"`
	DeviceID     string `json:"device_id"`
	TransitionMs *int   `json:"transition_ms,omitempty"`
}

type renameRequest struct {
}

type deviceUpdateRequest struct {
	Name *string `json:"name"`
	Icon *string `json:"icon"`
}

type deviceUpdateResponse struct {
	Status   string `json:"status"`
	DeviceID string `json:"device_id"`
	Name     string `json:"name,omitempty"`
	Icon     string `json:"icon,omitempty"`
}

type deviceCreateRequest struct {
	Protocol     string          `json:"protocol"`
	ExternalID   string          `json:"external_id"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	Manufacturer string          `json:"manufacturer"`
	Model        string          `json:"model"`
	Description  string          `json:"description"`
	Firmware     string          `json:"firmware"`
	Icon         string          `json:"icon"`
	Capabilities json.RawMessage `json:"capabilities"`
	Inputs       json.RawMessage `json:"inputs"`
}

type refreshRequest struct {
	Metadata   *bool    `json:"metadata,omitempty"`
	State      *bool    `json:"state,omitempty"`
	Properties []string `json:"properties"`
}

type deviceListItem struct {
	ID           uuid.UUID       `json:"id"`
	Protocol     string          `json:"protocol"`
	ExternalID   string          `json:"external_id"`
	Name         string          `json:"name"`
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
	deviceUpsertTopic        = "homenavi/devicehub/events/device.upsert"
	deviceRemovedTopic       = "homenavi/devicehub/events/device.removed"
	pairingEventTopic        = "homenavi/devicehub/events/pairing"
	pairingCommandTopic      = "homenavi/devicehub/commands/pairing"
	deviceRemoveCommandTopic = "homenavi/devicehub/commands/device.remove"
	pairingProgressTopic     = "homenavi/devicehub/events/pairing.progress"
)

var (
	errPairingActive      = errors.New("pairing already active")
	errPairingNotFound    = errors.New("pairing session not found")
	errPairingUnsupported = errors.New("protocol does not support pairing operations")
)

func (s *Server) handleDeviceCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleDeviceList(w, r)
	case http.MethodPost:
		s.handleDeviceCreate(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDeviceList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	devices, err := s.repo.List(ctx)
	if err != nil {
		slog.Error("device list query failed", "error", err)
		http.Error(w, "failed to load devices", http.StatusInternalServerError)
		return
	}
	items := make([]deviceListItem, 0, len(devices))
	for _, d := range devices {
		stateJSON, err := s.repo.GetDeviceState(ctx, d.ID.String())
		if err != nil {
			slog.Warn("device state lookup failed", "device_id", d.ID.String(), "error", err)
			stateJSON = nil
		}
		if len(stateJSON) == 0 {
			stateJSON = []byte(`{}`)
		}
		item := deviceListItem{
			ID:           d.ID,
			Protocol:     d.Protocol,
			ExternalID:   d.ExternalID,
			Name:         d.Name,
			Type:         d.Type,
			Manufacturer: d.Manufacturer,
			Model:        d.Model,
			Description:  d.Description,
			Firmware:     d.Firmware,
			Icon:         d.Icon,
			Online:       d.Online,
			LastSeen:     d.LastSeen,
			CreatedAt:    d.CreatedAt,
			UpdatedAt:    d.UpdatedAt,
			State:        json.RawMessage(append([]byte(nil), stateJSON...)),
		}
		if len(d.Capabilities) > 0 {
			item.Capabilities = json.RawMessage(append([]byte(nil), d.Capabilities...))
		}
		if len(d.Inputs) > 0 {
			item.Inputs = json.RawMessage(append([]byte(nil), d.Inputs...))
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleDeviceCreate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 32*1024))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	if len(body) == 0 {
		http.Error(w, "request body required", http.StatusBadRequest)
		return
	}
	var req deviceCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	protocol := strings.ToLower(strings.TrimSpace(req.Protocol))
	external := strings.TrimSpace(req.ExternalID)
	if protocol == "" || external == "" {
		http.Error(w, "protocol and external_id are required", http.StatusBadRequest)
		return
	}
	dev := &model.Device{
		Protocol:     protocol,
		ExternalID:   external,
		Name:         strings.TrimSpace(req.Name),
		Type:         strings.TrimSpace(req.Type),
		Manufacturer: strings.TrimSpace(req.Manufacturer),
		Model:        strings.TrimSpace(req.Model),
		Description:  strings.TrimSpace(req.Description),
		Firmware:     strings.TrimSpace(req.Firmware),
		Icon:         strings.TrimSpace(req.Icon),
		Online:       false,
	}
	if len(req.Capabilities) > 0 {
		if !json.Valid(req.Capabilities) {
			http.Error(w, "capabilities must be valid json", http.StatusBadRequest)
			return
		}
		dev.Capabilities = datatypes.JSON(append([]byte(nil), req.Capabilities...))
	}
	if len(req.Inputs) > 0 {
		if !json.Valid(req.Inputs) {
			http.Error(w, "inputs must be valid json", http.StatusBadRequest)
			return
		}
		dev.Inputs = datatypes.JSON(append([]byte(nil), req.Inputs...))
	}
	if err := s.repo.UpsertDevice(r.Context(), dev); err != nil {
		slog.Error("device create failed", "protocol", protocol, "external", external, "error", err)
		http.Error(w, "failed to create device", http.StatusInternalServerError)
		return
	}
	s.publishDeviceMetadata(dev)
	item, err := s.buildDeviceItem(r.Context(), dev)
	if err != nil {
		http.Error(w, "failed to encode device", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleDeviceRequest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/devicehub/devices/")
	if path == r.URL.Path {
		http.NotFound(w, r)
		return
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		s.handleDeviceList(w, r)
		return
	}
	deviceID := segments[0]
	if len(segments) == 1 {
		s.handleDevice(w, r, deviceID)
		return
	}
	switch segments[1] {
	case "commands":
		s.handleDeviceCommand(w, r, deviceID)
	case "refresh":
		s.handleDeviceRefresh(w, r, deviceID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request, deviceID string) {
	switch r.Method {
	case http.MethodGet:
		s.handleDeviceGet(w, r, deviceID)
	case http.MethodPatch:
		s.handleDevicePatch(w, r, deviceID)
	case http.MethodDelete:
		s.handleDeviceDelete(w, r, deviceID)
	default:
		w.Header().Set("Allow", "GET, PATCH, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDevicePatch(w http.ResponseWriter, r *http.Request, deviceID string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	if len(body) == 0 {
		http.Error(w, "request body required", http.StatusBadRequest)
		return
	}
	var req deviceUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Name == nil && req.Icon == nil {
		http.Error(w, "no updatable fields provided", http.StatusBadRequest)
		return
	}
	dev, err := s.repo.GetByID(r.Context(), deviceID)
	if err != nil {
		slog.Error("device update lookup failed", "device_id", deviceID, "error", err)
		http.Error(w, "device lookup failed", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	updated := false
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			http.Error(w, "name cannot be empty", http.StatusBadRequest)
			return
		}
		if dev.Name != trimmed {
			dev.Name = trimmed
			updated = true
		}
	}
	if req.Icon != nil {
		icon := strings.TrimSpace(*req.Icon)
		if dev.Icon != icon {
			dev.Icon = icon
			updated = true
		}
	}
	if !updated {
		writeJSON(w, http.StatusOK, deviceUpdateResponse{Status: "unchanged", DeviceID: dev.ID.String(), Name: dev.Name, Icon: dev.Icon})
		return
	}
	if err := s.repo.UpsertDevice(r.Context(), dev); err != nil {
		slog.Error("device update failed", "device_id", deviceID, "error", err)
		http.Error(w, "failed to update device", http.StatusInternalServerError)
		return
	}
	s.publishDeviceMetadata(dev)
	writeJSON(w, http.StatusOK, deviceUpdateResponse{Status: "updated", DeviceID: dev.ID.String(), Name: dev.Name, Icon: dev.Icon})
}

func (s *Server) handleDeviceGet(w http.ResponseWriter, r *http.Request, deviceID string) {
	dev, err := s.repo.GetByID(r.Context(), deviceID)
	if err != nil {
		slog.Error("device get failed", "device_id", deviceID, "error", err)
		http.Error(w, "device lookup failed", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	item, err := s.buildDeviceItem(r.Context(), dev)
	if err != nil {
		http.Error(w, "failed to encode device", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleDeviceDelete(w http.ResponseWriter, r *http.Request, deviceID string) {
	dev, err := s.repo.GetByID(r.Context(), deviceID)
	if err != nil {
		slog.Error("device delete lookup failed", "device_id", deviceID, "error", err)
		http.Error(w, "device lookup failed", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	protocol := normalizeProtocol(dev.Protocol)
	if protocol == "zigbee" {
		if err := s.requestProtocolRemoval(dev); err != nil {
			slog.Error("protocol removal failed", "device_id", dev.ID, "protocol", protocol, "error", err)
			http.Error(w, "failed to request protocol removal", http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "device_id": dev.ID.String(), "protocol": protocol})
		return
	}
	if err := s.repo.DeleteDeviceAndState(r.Context(), deviceID); err != nil {
		slog.Error("device delete failed", "device_id", deviceID, "error", err)
		http.Error(w, "failed to delete device", http.StatusInternalServerError)
		return
	}
	s.publishDeviceRemoval(dev, "api-delete")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeviceCommand(w http.ResponseWriter, r *http.Request, deviceID string) {
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		w.Header().Set("Allow", "POST, PATCH")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	if len(body) == 0 {
		http.Error(w, "request body required", http.StatusBadRequest)
		return
	}
	var req commandRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if len(req.State) == 0 && req.Input == nil {
		http.Error(w, "state or input required", http.StatusBadRequest)
		return
	}
	dev, err := s.repo.GetByID(r.Context(), deviceID)
	if err != nil {
		slog.Error("command lookup failed", "device_id", deviceID, "error", err)
		http.Error(w, "device lookup failed", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}

	statePatch := make(map[string]any)
	for k, v := range req.State {
		if k != "" && v != nil {
			statePatch[k] = v
		}
	}
	if req.Input != nil {
		if err := s.applyInput(dev, statePatch, req.Input.ID, req.Input.Value); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errInputNotFound) {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
	}
	if len(statePatch) == 0 {
		http.Error(w, "resolved state is empty", http.StatusBadRequest)
		return
	}
	payload := map[string]any{
		"device_id": dev.ID.String(),
		"state":     statePatch,
	}
	if req.TransitionMs != nil {
		// Zigbee2MQTT expects transition in seconds, accept ms from clients for precision.
		statePatch["transition"] = float64(*req.TransitionMs) / 1000.0
	}
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "failed to encode command", http.StatusInternalServerError)
		return
	}
	if err := s.mqtt.Publish("homenavi/devicehub/commands/device.set", data); err != nil {
		slog.Error("publish command failed", "device_id", deviceID, "error", err)
		http.Error(w, "failed to forward command", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusAccepted, commandResponse{Status: "queued", DeviceID: dev.ID.String(), TransitionMs: req.TransitionMs})
}

func (s *Server) handleDeviceRefresh(w http.ResponseWriter, r *http.Request, deviceID string) {
	dev, err := s.repo.GetByID(r.Context(), deviceID)
	if err != nil {
		slog.Error("device refresh lookup failed", "device_id", deviceID, "error", err)
		http.Error(w, "device lookup failed", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	var req refreshRequest
	if r.Body != nil {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 8*1024))
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
		}
		defer r.Body.Close()
	}
	metadata := true
	state := true
	if req.Metadata != nil || req.State != nil {
		metadata = req.Metadata == nil || (req.Metadata != nil && *req.Metadata)
		state = req.State == nil || (req.State != nil && *req.State)
		if !metadata && !state {
			state = true
		}
	}
	props := make([]string, 0, len(req.Properties))
	for _, prop := range req.Properties {
		if trimmed := strings.TrimSpace(prop); trimmed != "" {
			props = append(props, trimmed)
		}
	}
	payload := map[string]any{
		"device_id":   dev.ID.String(),
		"protocol":    dev.Protocol,
		"external_id": dev.ExternalID,
		"metadata":    metadata,
		"state":       state,
	}
	if len(props) > 0 {
		payload["properties"] = props
	}
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "failed to encode refresh payload", http.StatusInternalServerError)
		return
	}
	if err := s.mqtt.Publish("homenavi/devicehub/commands/device.refresh", data); err != nil {
		slog.Error("refresh publish failed", "device_id", deviceID, "error", err)
		http.Error(w, "failed to enqueue refresh", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "device_id": dev.ID.String()})
}

var errInputNotFound = errors.New("input not found")

func (s *Server) applyInput(dev *model.Device, state map[string]any, inputID string, value any) error {
	if inputID == "" {
		return errors.New("input id is required")
	}
	var inputs []model.DeviceInput
	if len(dev.Inputs) > 0 {
		if err := json.Unmarshal(dev.Inputs, &inputs); err != nil {
			slog.Error("decode inputs failed", "device_id", dev.ID.String(), "error", err)
			return errors.New("device inputs unavailable")
		}
	}
	var matched *model.DeviceInput
	for idx := range inputs {
		in := &inputs[idx]
		if strings.EqualFold(in.ID, inputID) || strings.EqualFold(in.Property, inputID) {
			matched = in
			break
		}
	}
	if matched == nil {
		return errInputNotFound
	}
	key := matched.Property
	if key == "" {
		key = matched.CapabilityID
	}
	if key == "" {
		return errors.New("input missing property mapping")
	}
	switch matched.Type {
	case "toggle":
		b := toBool(value)
		state[normalizeToggleKey(key)] = b
	case "slider", "number":
		if num, ok := toNumber(value); ok {
			state[key] = num
		} else {
			return errors.New("invalid numeric value")
		}
	case "select":
		state[key] = value
	case "color":
		state[key] = normalizeColorValue(matched, value)
	default:
		state[key] = value
	}
	return nil
}

func normalizeToggleKey(key string) string {
	lk := strings.ToLower(key)
	if lk == "state" || lk == "power" {
		return "on"
	}
	return key
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		lowered := strings.TrimSpace(strings.ToLower(val))
		if lowered == "on" || lowered == "true" || lowered == "1" || lowered == "yes" {
			return true
		}
		return false
	case float64:
		return val != 0
	case float32:
		return val != 0
	case int:
		return val != 0
	case int64:
		return val != 0
	case uint64:
		return val != 0
	default:
		return false
	}
}

func toNumber(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func normalizeColorValue(input *model.DeviceInput, value any) any {
	if value == nil {
		return value
	}
	if m, ok := value.(map[string]any); ok {
		if hexRaw, found := m["hex"]; found {
			if hex, ok := hexRaw.(string); ok {
				return map[string]any{"hex": strings.TrimSpace(hex)}
			}
		}
		return value
	}
	if s, ok := value.(string); ok {
		return map[string]any{"hex": strings.TrimSpace(s)}
	}
	return value
}

func (s *Server) publishDeviceMetadata(dev *model.Device) {
	if dev == nil {
		return
	}
	s.handlePairingCandidate(dev)
	devJSON, err := json.Marshal(dev)
	if err != nil {
		slog.Error("encode device metadata failed", "device_id", dev.ID.String(), "error", err)
		return
	}
	if err := s.mqtt.Publish("homenavi/devicehub/events/device.upsert", devJSON); err != nil {
		slog.Warn("broadcast device upsert failed", "device_id", dev.ID.String(), "error", err)
	}
}

func (s *Server) publishDeviceRemoval(dev *model.Device, reason string) {
	if dev == nil {
		return
	}
	payload := map[string]any{
		"id":          dev.ID.String(),
		"device_id":   dev.ID.String(),
		"external_id": dev.ExternalID,
		"protocol":    dev.Protocol,
		"reason":      reason,
	}
	if data, err := json.Marshal(payload); err == nil {
		if err := s.mqtt.Publish(deviceRemovedTopic, data); err != nil {
			slog.Warn("device removal publish failed", "device_id", dev.ID, "error", err)
		}
	}
}

func (s *Server) handleIntegrations(w http.ResponseWriter, _ *http.Request) {
	if len(s.integrations) == 0 {
		s.writeEmptyArray(w)
		return
	}
	writeJSON(w, http.StatusOK, s.integrations)
}

type pairingStartRequest struct {
	Protocol string          `json:"protocol"`
	Timeout  int             `json:"timeout"`
	Metadata pairingMetadata `json:"metadata"`
}

func (s *Server) handlePairings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessions := s.snapshotPairings()
		writeJSON(w, http.StatusOK, sessions)
		return
	case http.MethodPost:
		defer r.Body.Close()
		var req pairingStartRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 8*1024)).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		protocol := normalizeProtocol(req.Protocol)
		if protocol == "" {
			http.Error(w, "protocol is required", http.StatusBadRequest)
			return
		}
		session, err := s.startPairing(protocol, req.Timeout, req.Metadata)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errPairingActive) {
				status = http.StatusConflict
			} else if errors.Is(err, errPairingUnsupported) {
				status = http.StatusNotImplemented
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusAccepted, session)
		return
	case http.MethodDelete:
		protocol := normalizeProtocol(r.URL.Query().Get("protocol"))
		if protocol == "" {
			http.Error(w, "protocol query parameter required", http.StatusBadRequest)
			return
		}
		session, err := s.stopPairing(protocol, "stopped")
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errPairingNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(err, errPairingUnsupported) {
				status = http.StatusNotImplemented
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusOK, session)
		return
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) snapshotPairings() []pairingSession {
	s.pairingMu.Lock()
	defer s.pairingMu.Unlock()
	result := make([]pairingSession, 0, len(s.pairings))
	for _, session := range s.pairings {
		clone := session.clone()
		result = append(result, clone)
	}
	return result
}

func (s *Server) startPairing(protocol string, timeout int, metadata pairingMetadata) (*pairingSession, error) {
	if err := ensurePairingSupported(protocol); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 60
	} else if timeout > 300 {
		timeout = 300
	}
	now := time.Now().UTC()
	meta := sanitizePairingMetadata(metadata)
	session := &pairingSession{
		ID:           uuid.NewString(),
		Protocol:     protocol,
		Status:       "starting",
		Active:       true,
		StartedAt:    now,
		ExpiresAt:    now.Add(time.Duration(timeout) * time.Second),
		Metadata:     meta,
		knownDevices: s.snapshotKnownDevices(),
	}
	s.pairingMu.Lock()
	if existing, ok := s.pairings[protocol]; ok && existing.Active {
		s.pairingMu.Unlock()
		return nil, errPairingActive
	}
	s.pairings[protocol] = session
	s.pairingMu.Unlock()
	if err := s.publishPairingCommand(protocol, "start", timeout); err != nil {
		s.pairingMu.Lock()
		delete(s.pairings, protocol)
		s.pairingMu.Unlock()
		return nil, fmt.Errorf("failed to start pairing: %w", err)
	}
	session.Status = "active"
	s.emitPairingEvent(session.clone())
	s.startPairingTimeout(protocol, session.ID, session.ExpiresAt)
	clone := session.clone()
	return &clone, nil
}

func (s *Server) stopPairing(protocol, status string) (*pairingSession, error) {
	if err := ensurePairingSupported(protocol); err != nil {
		return nil, err
	}
	s.pairingMu.Lock()
	session, ok := s.pairings[protocol]
	if !ok {
		s.pairingMu.Unlock()
		return nil, errPairingNotFound
	}
	if session.cancel != nil {
		session.cancel()
		session.cancel = nil
	}
	session.Active = false
	session.Status = status
	session.ExpiresAt = time.Now().UTC()
	clone := session.clone()
	s.pairingMu.Unlock()
	_ = s.publishPairingCommand(protocol, "stop", 0)
	s.emitPairingEvent(clone)
	return &clone, nil
}

func (s *Server) startPairingTimeout(protocol, sessionID string, expires time.Time) {
	duration := time.Until(expires)
	if duration <= 0 {
		duration = time.Second * 60
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.pairingMu.Lock()
	if session, ok := s.pairings[protocol]; ok && session.ID == sessionID {
		session.cancel = cancel
	} else {
		s.pairingMu.Unlock()
		cancel()
		return
	}
	s.pairingMu.Unlock()
	go func() {
		select {
		case <-time.After(duration):
			s.handlePairingTimeout(protocol, sessionID)
		case <-ctx.Done():
		}
	}()
}

func (s *Server) handlePairingTimeout(protocol, sessionID string) {
	s.pairingMu.Lock()
	session, ok := s.pairings[protocol]
	if !ok || session.ID != sessionID || !session.Active {
		s.pairingMu.Unlock()
		return
	}
	session.Active = false
	session.Status = "timeout"
	session.ExpiresAt = time.Now().UTC()
	if session.cancel != nil {
		session.cancel()
		session.cancel = nil
	}
	clone := session.clone()
	s.pairingMu.Unlock()
	_ = s.publishPairingCommand(protocol, "stop", 0)
	s.emitPairingEvent(clone)
}

func (s *Server) publishPairingCommand(protocol, action string, timeout int) error {
	payload := map[string]any{
		"protocol": protocol,
		"action":   action,
	}
	if timeout > 0 {
		payload["timeout"] = timeout
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.mqtt.Publish(pairingCommandTopic, data)
}

func (s *Server) emitPairingEvent(session pairingSession) {
	data, err := json.Marshal(session)
	if err != nil {
		slog.Warn("encode pairing event failed", "error", err)
		return
	}
	if err := s.mqtt.Publish(pairingEventTopic, data); err != nil {
		slog.Warn("pairing event publish failed", "error", err)
	}
}

func (s *Server) handlePairingProgressEvent(_ paho.Client, msg mqtt.Message) {
	if len(msg.Payload()) == 0 {
		return
	}
	var evt struct {
		Protocol   string `json:"protocol"`
		Stage      string `json:"stage"`
		Status     string `json:"status"`
		ExternalID string `json:"external_id"`
	}
	if err := json.Unmarshal(msg.Payload(), &evt); err != nil {
		slog.Debug("pairing progress decode failed", "error", err)
		return
	}
	protocol := normalizeProtocol(evt.Protocol)
	if protocol == "" {
		protocol = "zigbee"
	}
	if !protocolSupportsInterviewTracking(protocol) {
		return
	}
	s.pairingMu.Lock()
	session, ok := s.pairings[protocol]
	if !ok || !session.Active {
		s.pairingMu.Unlock()
		return
	}
	if session.candidateExternalID == "" && evt.ExternalID != "" {
		session.candidateExternalID = strings.ToLower(evt.ExternalID)
	}
	updated := false
	finalStatus := ""
	switch evt.Stage {
	case "device_joined", "device_announced":
		session.Status = "device_joined"
		session.awaitingInterview = true
		updated = true
	case "interview_started":
		session.Status = "interviewing"
		session.awaitingInterview = true
		updated = true
	case "interview_succeeded", "interview_complete":
		session.Status = "interview_complete"
		session.awaitingInterview = false
		updated = true
		finalStatus = "completed"
	case "interview_failed":
		session.Status = "failed"
		session.Active = false
		session.awaitingInterview = false
		updated = true
		finalStatus = "failed"
	default:
		s.pairingMu.Unlock()
		return
	}
	snapshot := session.clone()
	s.pairingMu.Unlock()
	if !updated {
		return
	}
	s.emitPairingEvent(snapshot)
	if finalStatus != "" {
		go func(proto, status string) {
			if _, err := s.stopPairing(proto, status); err != nil && !errors.Is(err, errPairingNotFound) {
				slog.Warn("pairing finalize failed", "protocol", proto, "status", status, "error", err)
			}
		}(protocol, finalStatus)
	}
}

func (s *Server) shouldAcceptPairingCandidate(session *pairingSession, dev *model.Device) bool {
	if session == nil || dev == nil {
		return false
	}
	if session.DeviceID != "" && session.DeviceID != dev.ID.String() {
		return false
	}
	if len(session.knownDevices) > 0 {
		if _, exists := session.knownDevices[dev.ID.String()]; exists {
			return false
		}
	}
	if session.candidateExternalID != "" && dev.ExternalID != "" {
		if !strings.EqualFold(session.candidateExternalID, dev.ExternalID) {
			return false
		}
	}
	if !session.StartedAt.IsZero() && !dev.CreatedAt.IsZero() {
		if dev.CreatedAt.Before(session.StartedAt.Add(-5 * time.Second)) {
			return false
		}
	}
	return true
}

func (s *Server) snapshotKnownDevices() map[string]struct{} {
	ctx := context.Background()
	devices, err := s.repo.List(ctx)
	if err != nil {
		slog.Debug("pairing snapshot failed", "error", err)
		return nil
	}
	if len(devices) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(devices))
	for _, dev := range devices {
		if dev.ID == uuid.Nil {
			continue
		}
		known[dev.ID.String()] = struct{}{}
	}
	return known
}

func sanitizePairingMetadata(meta pairingMetadata) pairingMetadata {
	trim := func(v string) string {
		return strings.TrimSpace(v)
	}
	return pairingMetadata{
		Name:         trim(meta.Name),
		Icon:         strings.ToLower(trim(meta.Icon)),
		Description:  trim(meta.Description),
		Type:         trim(meta.Type),
		Manufacturer: trim(meta.Manufacturer),
		Model:        trim(meta.Model),
	}
}

func normalizeProtocol(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func protocolSupportsInterviewTracking(proto string) bool {
	switch normalizeProtocol(proto) {
	case "zigbee":
		return true
	default:
		return false
	}
}

type unsupportedProtocolError struct {
	protocol string
}

func (e unsupportedProtocolError) Error() string {
	return fmt.Sprintf("protocol %s does not support this operation", e.protocol)
}

func (e unsupportedProtocolError) Is(target error) bool {
	return target == errPairingUnsupported
}

func ensurePairingSupported(protocol string) error {
	switch protocol {
	case "zigbee":
		return nil
	default:
		return unsupportedProtocolError{protocol: protocol}
	}
}

func (s *Server) requestProtocolRemoval(dev *model.Device) error {
	if dev == nil {
		return errors.New("device is required")
	}
	protocol := normalizeProtocol(dev.Protocol)
	switch protocol {
	case "zigbee":
		payload := map[string]string{
			"protocol":    protocol,
			"device_id":   dev.ID.String(),
			"external_id": dev.ExternalID,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return s.mqtt.Publish(deviceRemoveCommandTopic, data)
	default:
		return unsupportedProtocolError{protocol: protocol}
	}
}

func (s *Server) handlePairingCandidate(dev *model.Device) {
	if dev == nil || dev.ID == uuid.Nil {
		return
	}
	protocol := normalizeProtocol(dev.Protocol)
	if protocol == "" {
		return
	}
	deviceID := dev.ID.String()
	s.pairingMu.Lock()
	session, ok := s.pairings[protocol]
	if !ok || !session.Active {
		s.pairingMu.Unlock()
		return
	}
	if !s.shouldAcceptPairingCandidate(session, dev) {
		s.pairingMu.Unlock()
		return
	}
	meta := session.Metadata
	supportsInterview := protocolSupportsInterviewTracking(protocol)
	session.DeviceID = deviceID
	session.Status = "device_detected"
	if supportsInterview {
		session.awaitingInterview = true
	} else {
		session.Active = false
	}
	snapshot := session.clone()
	s.pairingMu.Unlock()
	s.emitPairingEvent(snapshot)
	go s.applyPairingMetadata(deviceID, meta)
	go func(proto string, deferCompletion bool) {
		if err := s.publishPairingCommand(proto, "stop", 0); err != nil {
			slog.Warn("pairing permit stop failed", "protocol", proto, "error", err)
		}
		if !deferCompletion {
			if _, err := s.stopPairing(proto, "completed"); err != nil && !errors.Is(err, errPairingNotFound) {
				slog.Warn("pairing stop failed", "protocol", proto, "error", err)
			}
		}
	}(protocol, supportsInterview)
}

func (s *Server) applyPairingMetadata(deviceID string, meta pairingMetadata) {
	trimmed := sanitizePairingMetadata(meta)
	if deviceID == "" || trimmed == (pairingMetadata{}) {
		return
	}
	ctx := context.Background()
	dev, err := s.repo.GetByID(ctx, deviceID)
	if err != nil || dev == nil {
		return
	}
	changed := false
	if trimmed.Name != "" && trimmed.Name != dev.Name {
		dev.Name = trimmed.Name
		changed = true
	}
	if trimmed.Description != "" && trimmed.Description != dev.Description {
		dev.Description = trimmed.Description
		changed = true
	}
	if trimmed.Type != "" && trimmed.Type != dev.Type {
		dev.Type = trimmed.Type
		changed = true
	}
	if trimmed.Manufacturer != "" && trimmed.Manufacturer != dev.Manufacturer {
		dev.Manufacturer = trimmed.Manufacturer
		changed = true
	}
	if trimmed.Model != "" && trimmed.Model != dev.Model {
		dev.Model = trimmed.Model
		changed = true
	}
	if trimmed.Icon != "" && trimmed.Icon != dev.Icon {
		dev.Icon = trimmed.Icon
		changed = true
	}
	if !changed {
		return
	}
	if err := s.repo.UpsertDevice(ctx, dev); err != nil {
		slog.Warn("pairing metadata update failed", "device_id", deviceID, "error", err)
		return
	}
	s.publishDeviceMetadata(dev)
}

func (s *Server) writeEmptyArray(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("[]"))
}

func (s *Server) buildDeviceItem(ctx context.Context, d *model.Device) (deviceListItem, error) {
	stateJSON, err := s.repo.GetDeviceState(ctx, d.ID.String())
	if err != nil {
		slog.Warn("device state lookup failed", "device_id", d.ID.String(), "error", err)
		stateJSON = nil
	}
	if len(stateJSON) == 0 {
		stateJSON = []byte(`{}`)
	}
	item := deviceListItem{
		ID:           d.ID,
		Protocol:     d.Protocol,
		ExternalID:   d.ExternalID,
		Name:         d.Name,
		Type:         d.Type,
		Manufacturer: d.Manufacturer,
		Model:        d.Model,
		Description:  d.Description,
		Firmware:     d.Firmware,
		Icon:         d.Icon,
		Online:       d.Online,
		LastSeen:     d.LastSeen,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
		State:        json.RawMessage(append([]byte(nil), stateJSON...)),
	}
	if len(d.Capabilities) > 0 {
		item.Capabilities = json.RawMessage(append([]byte(nil), d.Capabilities...))
	}
	if len(d.Inputs) > 0 {
		item.Inputs = json.RawMessage(append([]byte(nil), d.Inputs...))
	}
	return item, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}
