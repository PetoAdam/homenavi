package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/dashboard-service/internal/auth"
	"github.com/PetoAdam/homenavi/dashboard-service/internal/dashboard"
	"github.com/PetoAdam/homenavi/shared/cachex"
	"github.com/google/uuid"
)

type DashboardService interface {
	Catalog(context.Context, dashboard.AuthContext) []dashboard.WidgetType
	Weather(string) dashboard.WeatherResponse
	GetMyDashboard(context.Context, uuid.UUID) (dashboard.Dashboard, error)
	PutMyDashboard(context.Context, uuid.UUID, int, json.RawMessage) (dashboard.Dashboard, error)
	GetDefaultDashboard(context.Context) (dashboard.Dashboard, error)
	PutDefaultDashboard(context.Context, string, json.RawMessage) (dashboard.Dashboard, error)
}

type Handler struct {
	service DashboardService
	cache   *cachex.JSONStore
	ttl     time.Duration
}

type HandlerOption func(*Handler)

func WithCache(store *cachex.JSONStore, ttl time.Duration) HandlerOption {
	return func(h *Handler) {
		h.cache = store
		h.ttl = ttl
	}
}

func NewHandler(service DashboardService, opts ...HandlerOption) *Handler {
	handler := &Handler{service: service}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

func (h *Handler) cacheKeyForUserDashboard(userID uuid.UUID) string {
	return "dashboard:user:" + userID.String()
}

func (h *Handler) cacheKeyForDefaultDashboard() string {
	return "dashboard:default"
}

func (h *Handler) cacheRead(ctx context.Context, key string, dst any) bool {
	if h.cache == nil || h.ttl <= 0 || key == "" {
		return false
	}
	return h.cache.Get(ctx, key, dst) == nil
}

func (h *Handler) cacheWrite(ctx context.Context, key string, value any) {
	if h.cache == nil || h.ttl <= 0 || key == "" {
		return
	}
	_ = h.cache.Set(ctx, key, value, h.ttl)
}

func (h *Handler) cacheDelete(ctx context.Context, keys ...string) {
	if h.cache == nil || len(keys) == 0 {
		return
	}
	_ = h.cache.Delete(ctx, keys...)
}

func (h *Handler) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	authCtx := dashboard.AuthContext{Authorization: strings.TrimSpace(r.Header.Get("Authorization"))}
	if cookie, err := r.Cookie("auth_token"); err == nil {
		authCtx.AuthToken = strings.TrimSpace(cookie.Value)
	}
	writeJSON(w, http.StatusOK, h.service.Catalog(r.Context(), authCtx))
}

func (h *Handler) HandleWeather(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.service.Weather(r.URL.Query().Get("city")))
}

func (h *Handler) HandleGetMyDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUserIDFromClaims(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var cached dashboard.Dashboard
	if h.cacheRead(r.Context(), h.cacheKeyForUserDashboard(userID), &cached) {
		writeJSON(w, http.StatusOK, cached)
		return
	}
	result, err := h.service.GetMyDashboard(r.Context(), userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load dashboard")
		return
	}
	h.cacheWrite(r.Context(), h.cacheKeyForUserDashboard(userID), result)
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) HandlePutMyDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUserIDFromClaims(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		LayoutVersion int             `json:"layout_version"`
		Doc           json.RawMessage `json:"doc"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.LayoutVersion <= 0 {
		writeJSONError(w, http.StatusBadRequest, "layout_version required")
		return
	}
	if len(req.Doc) == 0 {
		writeJSONError(w, http.StatusBadRequest, "doc required")
		return
	}
	result, err := h.service.PutMyDashboard(r.Context(), userID, req.LayoutVersion, req.Doc)
	if err != nil {
		if errors.Is(err, dashboard.ErrConflict) {
			writeJSONError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to save dashboard")
		return
	}
	h.cacheDelete(r.Context(), h.cacheKeyForUserDashboard(userID))
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) HandleGetDefaultDashboard(w http.ResponseWriter, r *http.Request) {
	var cached dashboard.Dashboard
	if h.cacheRead(r.Context(), h.cacheKeyForDefaultDashboard(), &cached) {
		writeJSON(w, http.StatusOK, cached)
		return
	}
	result, err := h.service.GetDefaultDashboard(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load default dashboard")
		return
	}
	h.cacheWrite(r.Context(), h.cacheKeyForDefaultDashboard(), result)
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) HandlePutDefaultDashboard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string          `json:"title"`
		Doc   json.RawMessage `json:"doc"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Doc) == 0 {
		writeJSONError(w, http.StatusBadRequest, "doc required")
		return
	}
	result, err := h.service.PutDefaultDashboard(r.Context(), req.Title, req.Doc)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid doc")
		return
	}
	h.cacheDelete(r.Context(), h.cacheKeyForDefaultDashboard())
	writeJSON(w, http.StatusOK, result)
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func parseUserIDFromClaims(r *http.Request) (uuid.UUID, bool) {
	claims := auth.GetClaims(r)
	if claims == nil || strings.TrimSpace(claims.Subject) == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(claims.Subject))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}
