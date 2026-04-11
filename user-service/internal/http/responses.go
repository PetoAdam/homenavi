package http

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, status int, message string, extra map[string]any) {
	payload := map[string]any{"error": message, "code": status}
	for k, v := range extra {
		if k != "error" && k != "code" {
			payload[k] = v
		}
	}
	writeJSON(w, status, payload)
}
