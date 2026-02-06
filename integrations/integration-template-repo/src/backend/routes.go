package backend

import (
	"encoding/json"
	"net/http"
)

func RegisterAPIRoutes(mux *http.ServeMux, store *CounterStore) {
	mux.HandleFunc("/api/counter", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"count": store.Get()})
			return
		case http.MethodPost:
			var payload struct {
				Delta *int `json:"delta"`
				Count *int `json:"count"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid json")
				return
			}
			switch {
			case payload.Count != nil:
				_ = respondCount(w, store.Set(*payload.Count))
				return
			case payload.Delta != nil:
				_ = respondCount(w, store.Add(*payload.Delta))
				return
			default:
				writeJSONError(w, http.StatusBadRequest, "missing delta or count")
				return
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})
}

func respondCount(w http.ResponseWriter, count int) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(map[string]any{"count": count})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "code": status})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
