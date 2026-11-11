package http

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/db"
)

const (
	maxDeviceStateBatch         = 100
	maxDeviceStateResponseBytes = 256 << 10
)

type batchDeviceState struct {
	DeviceID       string          `json:"device_id"`
	Status         string          `json:"status"`
	Online         *bool           `json:"online,omitempty"`
	LastSeen       *time.Time      `json:"last_seen,omitempty"`
	State          json.RawMessage `json:"state,omitempty"`
	StateUpdatedAt *time.Time      `json:"state_updated_at,omitempty"`
}

func deviceStateKeys(ids []string) ([]dbinfra.DeviceStateKey, bool) {
	if len(ids) == 0 || len(ids) > maxDeviceStateBatch {
		return nil, false
	}
	keys := make([]dbinfra.DeviceStateKey, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		protocol, external, ok := splitHDPDeviceID(id)
		if !ok || len(id) > 512 {
			return nil, false
		}
		external, err := normalizeExternalID(protocol, external)
		if err != nil || canonicalHDPDeviceID(protocol, external) != id || seen[id] {
			return nil, false
		}
		seen[id] = true
		keys = append(keys, dbinfra.DeviceStateKey{Protocol: protocol, ExternalID: external})
	}
	return keys, true
}

func batchDeviceStates(ids []string, rows []dbinfra.DeviceStateView) []batchDeviceState {
	byID := make(map[string]dbinfra.DeviceStateView, len(rows))
	for _, row := range rows {
		byID[canonicalHDPDeviceID(row.Protocol, row.ExternalID)] = row
	}
	results := make([]batchDeviceState, 0, len(ids))
	for _, id := range ids {
		result := batchDeviceState{DeviceID: id, Status: "not_found"}
		if row, ok := byID[id]; ok {
			result.Status = "ok"
			result.Online = &row.Online
			result.LastSeen = &row.LastSeen
			if row.StateUpdatedAt == nil || len(row.State) == 0 || !json.Valid(row.State) {
				result.Status = "state_unavailable"
			} else {
				result.State = row.State
				result.StateUpdatedAt = row.StateUpdatedAt
			}
		}
		results = append(results, result)
	}
	return results
}

func (s *Server) handleDeviceStatesBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		DeviceIDs []string `json:"device_ids"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid batch request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "invalid batch request", http.StatusBadRequest)
		return
	}
	keys, ok := deviceStateKeys(request.DeviceIDs)
	if !ok {
		http.Error(w, "device_ids must contain 1 to 100 unique canonical IDs", http.StatusBadRequest)
		return
	}
	if s.repo == nil {
		http.Error(w, "repository unavailable", http.StatusServiceUnavailable)
		return
	}
	rows, err := s.repo.GetDeviceStates(r.Context(), keys)
	if err != nil {
		http.Error(w, "state lookup unavailable", http.StatusServiceUnavailable)
		return
	}
	response, err := json.Marshal(map[string]any{"results": batchDeviceStates(request.DeviceIDs, rows), "read_at": time.Now().UTC()})
	if err != nil || len(response) > maxDeviceStateResponseBytes {
		http.Error(w, "batch response exceeds size limit", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)
}
