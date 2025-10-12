package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"device-hub/internal/model"
	"device-hub/internal/mqtt"
	"device-hub/internal/store"
)

type Server struct {
	repo *store.Repository
	mqtt *mqtt.Client
}

func NewServer(repo *store.Repository, mqtt *mqtt.Client) *Server {
	return &Server{repo: repo, mqtt: mqtt}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/devicehub/devices", s.handleDeviceCollection)
	mux.HandleFunc("/api/devicehub/devices/", s.handleDeviceRequest)
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
	Name string `json:"name"`
}

type renameResponse struct {
	Status   string `json:"status"`
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
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
	Capabilities json.RawMessage `json:"capabilities,omitempty"`
	Inputs       json.RawMessage `json:"inputs,omitempty"`
	Online       bool            `json:"online"`
	LastSeen     time.Time       `json:"last_seen"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	State        json.RawMessage `json:"state"`
}

func (s *Server) handleDeviceCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleDeviceList(w, r)
	default:
		w.Header().Set("Allow", "GET")
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
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request, deviceID string) {
	switch r.Method {
	case http.MethodPatch:
		s.handleDevicePatch(w, r, deviceID)
	default:
		w.Header().Set("Allow", "PATCH")
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
	var req renameRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	dev, err := s.repo.GetByID(r.Context(), deviceID)
	if err != nil {
		slog.Error("rename lookup failed", "device_id", deviceID, "error", err)
		http.Error(w, "device lookup failed", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	dev.Name = name
	if err := s.repo.UpsertDevice(r.Context(), dev); err != nil {
		slog.Error("rename update failed", "device_id", deviceID, "error", err)
		http.Error(w, "failed to update device", http.StatusInternalServerError)
		return
	}
	s.publishDeviceMetadata(dev)
	writeJSON(w, http.StatusOK, renameResponse{Status: "updated", DeviceID: dev.ID.String(), Name: dev.Name})
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}
