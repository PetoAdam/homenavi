package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

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

type Server struct {
	repo         *store.Repository
	mqtt         *mqtt.Client
	integrations []IntegrationDescriptor
}

func NewServer(repo *store.Repository, mqtt *mqtt.Client, integrations []IntegrationDescriptor) *Server {
	return &Server{repo: repo, mqtt: mqtt, integrations: integrations}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/devicehub/devices", s.handleDeviceCollection)
	mux.HandleFunc("/api/devicehub/devices/", s.handleDeviceRequest)
	mux.HandleFunc("/api/devicehub/integrations", s.handleIntegrations)
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

const deviceRemovedTopic = "homenavi/devicehub/events/device.removed"

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
