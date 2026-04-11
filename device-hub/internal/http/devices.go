package http

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/device-hub/internal/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
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
		http.Error(w, "could not load devices", http.StatusInternalServerError)
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
			DeviceID:     canonicalHDPDeviceID(d.Protocol, d.ExternalID),
			Protocol:     d.Protocol,
			ExternalID:   d.ExternalID,
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
	protocol := normalizeProtocol(req.Protocol)
	external, err := normalizeExternalID(protocol, req.ExternalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	existing, err := s.repo.GetByExternal(r.Context(), protocol, external)
	if err != nil {
		slog.Error("device lookup failed", "protocol", protocol, "external", external, "error", err)
		http.Error(w, "could not create device", http.StatusInternalServerError)
		return
	}
	if existing != nil {
		http.Error(w, "device already exists", http.StatusConflict)
		return
	}
	dev := &model.Device{
		Protocol:     protocol,
		ExternalID:   external,
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
		http.Error(w, "could not create device", http.StatusInternalServerError)
		return
	}
	s.publishDeviceMetadata(dev)
	item, err := s.buildDeviceItem(r.Context(), dev)
	if err != nil {
		http.Error(w, "could not encode device", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleDeviceRequest(w http.ResponseWriter, r *http.Request) {
	deviceID, action, ok := parseHDPDeviceRequestPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if deviceID == "" {
		s.handleDeviceList(w, r)
		return
	}
	switch action {
	case "":
		s.handleDevice(w, r, deviceID)
	case "commands":
		s.handleDeviceCommand(w, r, deviceID)
	case "refresh":
		s.handleDeviceRefresh(w, r, deviceID)
	default:
		http.NotFound(w, r)
	}
}

func parseHDPDeviceRequestPath(path string) (deviceID string, action string, ok bool) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", "", false
	}
	prefix := "/api/hdp/devices/"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", "", false
	}
	suffix := strings.Trim(strings.TrimPrefix(trimmed, prefix), "/")
	if suffix == "" {
		return "", "", true
	}
	parts := strings.Split(suffix, "/")
	if len(parts) == 0 {
		return "", "", true
	}
	last := parts[len(parts)-1]
	if last == "commands" || last == "refresh" {
		id := strings.Join(parts[:len(parts)-1], "/")
		if strings.TrimSpace(id) == "" {
			return "", "", false
		}
		return id, last, true
	}
	return suffix, "", true
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
	if req.Icon == nil {
		http.Error(w, "no updatable fields provided", http.StatusBadRequest)
		return
	}
	dev, hdpID, err := s.resolveDevice(r.Context(), deviceID)
	if err != nil {
		slog.Error("device update lookup failed", "device_id", deviceID, "error", err)
		http.Error(w, "could not load device", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	updated := false
	if req.Icon != nil {
		icon := strings.TrimSpace(*req.Icon)
		if dev.Icon != icon {
			dev.Icon = icon
			updated = true
		}
	}
	if !updated {
		if hdpID == "" {
			hdpID = canonicalHDPDeviceID(dev.Protocol, dev.ExternalID)
		}
		writeJSON(w, http.StatusOK, deviceUpdateResponse{Status: "unchanged", DeviceID: hdpID, Icon: dev.Icon})
		return
	}
	if err := s.repo.UpsertDevice(r.Context(), dev); err != nil {
		slog.Error("device update failed", "device_id", deviceID, "error", err)
		http.Error(w, "could not update device", http.StatusInternalServerError)
		return
	}
	s.publishDeviceMetadata(dev)
	if hdpID == "" {
		hdpID = canonicalHDPDeviceID(dev.Protocol, dev.ExternalID)
	}
	writeJSON(w, http.StatusOK, deviceUpdateResponse{Status: "updated", DeviceID: hdpID, Icon: dev.Icon})
}

func (s *Server) handleDeviceGet(w http.ResponseWriter, r *http.Request, deviceID string) {
	dev, _, err := s.resolveDevice(r.Context(), deviceID)
	if err != nil {
		slog.Error("device get failed", "device_id", deviceID, "error", err)
		http.Error(w, "could not load device", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	item, err := s.buildDeviceItem(r.Context(), dev)
	if err != nil {
		http.Error(w, "could not encode device", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleDeviceDelete(w http.ResponseWriter, r *http.Request, deviceID string) {
	dev, hdpID, err := s.resolveDevice(r.Context(), deviceID)
	if err != nil {
		slog.Error("device delete lookup failed", "device_id", deviceID, "error", err)
		http.Error(w, "could not load device", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	if hdpID == "" {
		hdpID = canonicalHDPDeviceID(dev.Protocol, dev.ExternalID)
	}
	force := queryBool(r.URL.Query().Get("force"))
	if !force {
		if err := s.requestProtocolRemoval(dev); err != nil {
			slog.Error("protocol removal failed", "device_id", dev.ID, "protocol", normalizeProtocol(dev.Protocol), "error", err)
			http.Error(w, "could not request protocol removal", http.StatusBadGateway)
			return
		}
	}
	if err := s.repo.DeleteDeviceAndState(r.Context(), dev.ID.String()); err != nil {
		slog.Error("device delete failed", "device_id", deviceID, "error", err)
		http.Error(w, "could not delete device", http.StatusInternalServerError)
		return
	}
	reason := "api-delete"
	if force {
		reason = "api-delete-force"
	}
	s.publishDeviceRemoval(dev, reason)
	w.WriteHeader(http.StatusNoContent)
}

func queryBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *Server) handleDeviceCommand(w http.ResponseWriter, r *http.Request, deviceID string) {
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		w.Header().Set("Allow", "POST, PATCH")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slog.Info("device command received", "device_id", deviceID, "method", r.Method)
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
	if len(req.State) == 0 {
		http.Error(w, "state is required", http.StatusBadRequest)
		return
	}
	dev, hdpID, err := s.resolveDevice(r.Context(), deviceID)
	if err != nil {
		slog.Error("command lookup failed", "device_id", deviceID, "error", err)
		http.Error(w, "could not load device", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	if hdpID == "" {
		hdpID = canonicalHDPDeviceID(dev.Protocol, dev.ExternalID)
	}
	targetExternal := hdpID
	if targetExternal == "" {
		slog.Error("command missing external id", "device_id", deviceID, "protocol", dev.Protocol)
		http.Error(w, "device external id missing", http.StatusInternalServerError)
		return
	}
	statePatch := make(map[string]any)
	for k, v := range req.State {
		if k != "" && v != nil {
			statePatch[k] = v
		}
	}
	if len(statePatch) == 0 {
		http.Error(w, "resolved state is empty", http.StatusBadRequest)
		return
	}
	corr := strings.TrimSpace(req.CorrelationID)
	if corr == "" {
		corr = uuid.NewString()
	}
	if req.TransitionMs != nil {
		statePatch["transition_ms"] = *req.TransitionMs
	}
	baselineState := s.loadBaselineState(r.Context(), dev.ID.String())
	s.beginCommandLifecycle(targetExternal, corr, statePatch, baselineState)
	s.publishCommandLifecycle(targetExternal, corr, commandStatusAccepted, "", nil)
	hdpCommand := map[string]any{
		"schema":    hdpSchema,
		"type":      "command",
		"device_id": targetExternal,
		"command":   "set_state",
		"args":      statePatch,
		"ts":        time.Now().UnixMilli(),
		"corr":      corr,
	}
	slog.Info("device command dispatch", "device_id", deviceID, "external", targetExternal, "corr", corr)
	if err := s.publishHDPCommand(targetExternal, hdpCommand); err != nil {
		s.removePendingCommand(targetExternal, corr)
		s.publishCommandLifecycle(targetExternal, corr, commandStatusFailed, err.Error(), nil)
		http.Error(w, "could not dispatch command", http.StatusBadGateway)
		return
	}
	s.publishCommandLifecycle(targetExternal, corr, commandStatusQueued, "", nil)
	writeJSON(w, http.StatusAccepted, commandResponse{Status: "queued", DeviceID: targetExternal, TransitionMs: req.TransitionMs, CorrelationID: corr})
}

func (s *Server) handleDeviceRefresh(w http.ResponseWriter, r *http.Request, deviceID string) {
	dev, hdpID, err := s.resolveDevice(r.Context(), deviceID)
	if err != nil {
		slog.Error("device refresh lookup failed", "device_id", deviceID, "error", err)
		http.Error(w, "could not load device", http.StatusInternalServerError)
		return
	}
	if dev == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	if hdpID == "" {
		hdpID = canonicalHDPDeviceID(dev.Protocol, dev.ExternalID)
	}
	slog.Info("device refresh requested", "device_id", hdpID, "protocol", dev.Protocol)
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
	cmd := map[string]any{
		"command":    "refresh",
		"metadata":   metadata,
		"state":      state,
		"properties": props,
	}
	if err := s.publishHDPCommand(hdpID, cmd); err != nil {
		http.Error(w, "could not queue refresh", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "device_id": hdpID})
}
