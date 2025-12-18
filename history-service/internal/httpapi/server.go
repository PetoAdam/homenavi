package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"history-service/internal/store"
)

type Server struct {
	repo *store.Repo
}

func New(repo *store.Repo) *Server {
	return &Server{repo: repo}
}

type statePointDTO struct {
	TS       time.Time       `json:"ts"`
	Payload  json.RawMessage `json:"payload"`
	Topic    string          `json:"topic"`
	Retained bool            `json:"retained"`
}

type listStateResponse struct {
	DeviceID   string          `json:"device_id"`
	From       *time.Time      `json:"from,omitempty"`
	To         *time.Time      `json:"to,omitempty"`
	Points     []statePointDTO `json:"points"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/history/health", s.handleHealth)
	mux.HandleFunc("/api/history/state", s.handleListState)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleListState(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	deviceID := strings.TrimSpace(q.Get("device_id"))
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	from, fromPtr, err := parseTimePtr(q.Get("from"))
	if err != nil {
		http.Error(w, "invalid from", http.StatusBadRequest)
		return
	}
	to, toPtr, err := parseTimePtr(q.Get("to"))
	if err != nil {
		http.Error(w, "invalid to", http.StatusBadRequest)
		return
	}

	limit := 1000
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	desc := false
	if strings.EqualFold(strings.TrimSpace(q.Get("order")), "desc") {
		desc = true
	}

	cursor, err := store.DecodeCursor(q.Get("cursor"))
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}

	page, err := s.repo.ListStatePoints(r.Context(), deviceID, from, to, limit, cursor, desc)
	if err != nil {
		slog.Error("history state query failed", "device_id", deviceID, "error", err)
		http.Error(w, "could not query history", http.StatusInternalServerError)
		return
	}

	points := make([]statePointDTO, 0, len(page.Points))
	for _, p := range page.Points {
		payload := json.RawMessage(append([]byte(nil), p.Payload...))
		points = append(points, statePointDTO{TS: p.TS, Payload: payload, Topic: p.Topic, Retained: p.Retained})
	}

	resp := listStateResponse{DeviceID: deviceID, Points: points, NextCursor: page.NextCursor}
	if fromPtr != nil {
		resp.From = fromPtr
	}
	if toPtr != nil {
		resp.To = toPtr
	}
	writeJSON(w, http.StatusOK, resp)
}

func parseTimePtr(v string) (time.Time, *time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		// accept RFC3339Nano too
		t, err = time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return time.Time{}, nil, err
		}
	}
	t = t.UTC()
	return t, &t, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
