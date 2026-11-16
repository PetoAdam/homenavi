package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

func (s *Server) handleDeviceInterfaceLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		DeviceID string `json:"device_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		http.Error(w, "invalid lookup request", http.StatusBadRequest)
		return
	}
	if _, ok := deviceStateKeys([]string{request.DeviceID}); !ok {
		http.Error(w, "canonical device ID required", http.StatusBadRequest)
		return
	}
	device, canonical, err := s.resolveDevice(r.Context(), request.DeviceID)
	if err != nil {
		http.Error(w, "device lookup unavailable", http.StatusServiceUnavailable)
		return
	}
	if device == nil || canonical != request.DeviceID {
		http.NotFound(w, r)
		return
	}
	capabilities, inputs := json.RawMessage(device.Capabilities), json.RawMessage(device.Inputs)
	if len(capabilities) > 128<<10 || !json.Valid(capabilities) {
		capabilities = json.RawMessage("null")
	}
	if len(inputs) > 128<<10 || !json.Valid(inputs) {
		inputs = json.RawMessage("null")
	}
	response, err := json.Marshal(map[string]any{"device_id": canonical, "capabilities": capabilities, "inputs": inputs})
	if err != nil || len(response) > 256<<10 {
		http.Error(w, "metadata exceeds size limit", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(response)
}

func (s *Server) handleDeviceMetadataPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			http.Error(w, "limit must be between 1 and 100", http.StatusBadRequest)
			return
		}
		limit = value
	}
	after := uuid.Nil
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		value, err := uuid.Parse(raw)
		if err != nil || value == uuid.Nil {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		after = value
	}
	if s.repo == nil {
		http.Error(w, "repository unavailable", http.StatusServiceUnavailable)
		return
	}
	revision, err := s.repo.MetadataRevision(r.Context())
	if err != nil {
		http.Error(w, "metadata unavailable", http.StatusServiceUnavailable)
		return
	}
	if raw := r.URL.Query().Get("revision"); raw != "" {
		expected, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || expected < 0 {
			http.Error(w, "invalid revision", http.StatusBadRequest)
			return
		}
		if expected != revision {
			http.Error(w, "metadata changed", http.StatusConflict)
			return
		}
	} else if after != uuid.Nil {
		http.Error(w, "revision required with cursor", http.StatusBadRequest)
		return
	}
	devices, err := s.repo.ListMetadataPage(r.Context(), after, limit+1)
	if err != nil {
		http.Error(w, "metadata unavailable", http.StatusServiceUnavailable)
		return
	}
	currentRevision, err := s.repo.MetadataRevision(r.Context())
	if err != nil {
		http.Error(w, "metadata unavailable", http.StatusServiceUnavailable)
		return
	}
	if currentRevision != revision {
		http.Error(w, "metadata changed", http.StatusConflict)
		return
	}
	next := ""
	if len(devices) > limit {
		devices = devices[:limit]
		next = devices[len(devices)-1].ID.String()
	}
	items := make([]map[string]any, 0, len(devices))
	for _, device := range devices {
		capabilities := json.RawMessage(device.Capabilities)
		id := canonicalHDPDeviceID(device.Protocol, device.ExternalID)
		keys, valid := deviceStateKeys([]string{id})
		if !json.Valid(capabilities) || !valid || keys[0].Protocol != device.Protocol || keys[0].ExternalID != device.ExternalID {
			http.Error(w, "invalid device metadata", http.StatusServiceUnavailable)
			return
		}
		items = append(items, map[string]any{"device_id": id, "source_id": device.ID, "device_type": device.Type, "capabilities": capabilities, "updated_at": device.UpdatedAt})
	}
	encoded, err := json.Marshal(map[string]any{"devices": items, "next_cursor": next, "revision": revision})
	if err != nil || len(encoded) > 1<<20 {
		http.Error(w, "metadata page exceeds size limit", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(encoded)
}
