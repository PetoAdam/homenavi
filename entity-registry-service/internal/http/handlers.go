package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/entity-registry-service/internal/realtime"
	"github.com/PetoAdam/homenavi/shared/cachex"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Server struct {
	repo         *dbinfra.Repository
	hub          *realtime.Hub
	cache        *cachex.JSONStore
	listCacheTTL time.Duration
}

type ServerOption func(*Server)

func WithCache(store *cachex.JSONStore, ttl time.Duration) ServerOption {
	return func(s *Server) {
		s.cache = store
		s.listCacheTTL = ttl
	}
}

func NewServer(repo *dbinfra.Repository, hub *realtime.Hub, opts ...ServerOption) *Server {
	server := &Server{repo: repo, hub: hub}
	for _, opt := range opts {
		if opt != nil {
			opt(server)
		}
	}
	return server
}

const (
	ersRoomsCacheKey   = "ers:list:rooms"
	ersTagsCacheKey    = "ers:list:tags"
	ersGroupsCacheKey  = "ers:list:groups"
	ersDevicesCacheKey = "ers:list:devices"
	ersHomeCacheKey    = "ers:list:home"
)

func (s *Server) cacheRead(ctx context.Context, key string, dst any) bool {
	if s.cache == nil || s.listCacheTTL <= 0 || key == "" {
		return false
	}
	if err := s.cache.Get(ctx, key, dst); err != nil {
		return false
	}
	return true
}

func (s *Server) cacheWrite(ctx context.Context, key string, value any) {
	if s.cache == nil || s.listCacheTTL <= 0 || key == "" {
		return
	}
	_ = s.cache.Set(ctx, key, value, s.listCacheTTL)
}

func (s *Server) invalidateListCaches(ctx context.Context, keys ...string) {
	if s.cache == nil || len(keys) == 0 {
		return
	}
	_ = s.cache.Delete(ctx, keys...)
}

func (s *Server) invalidateInventoryCaches(ctx context.Context) {
	s.invalidateListCaches(ctx, ersRoomsCacheKey, ersTagsCacheKey, ersGroupsCacheKey, ersDevicesCacheKey, ersHomeCacheKey)
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	if s.hub != nil {
		r.Get("/ws/ers", s.hub.ServeHTTP)
	}
	r.Get("/health", s.handleHealth)
	r.Get("/api/ers/home", s.handleHome)
	r.Route("/api/ers/rooms", func(r chi.Router) {
		r.Get("/", s.handleRoomsList)
		r.Post("/", s.handleRoomsCreate)
		r.Patch("/{room_id}", s.handleRoomsPatch)
		r.Delete("/{room_id}", s.handleRoomsDelete)
	})
	r.Route("/api/ers/tags", func(r chi.Router) {
		r.Get("/", s.handleTagsList)
		r.Post("/", s.handleTagsCreate)
		r.Delete("/{tag_id}", s.handleTagsDelete)
		r.Put("/{tag_id}/members", s.handleTagsSetMembers)
	})
	r.Route("/api/ers/groups", func(r chi.Router) {
		r.Get("/", s.handleGroupsList)
		r.Post("/", s.handleGroupsCreate)
		r.Get("/{group_id}", s.handleGroupsGet)
		r.Patch("/{group_id}", s.handleGroupsPatch)
		r.Delete("/{group_id}", s.handleGroupsDelete)
		r.Put("/{group_id}/members", s.handleGroupsSetMembers)
	})
	r.Route("/api/ers/devices", func(r chi.Router) {
		r.Get("/", s.handleDevicesList)
		r.Post("/", s.handleDevicesCreate)
		r.Get("/{device_id}", s.handleDevicesGet)
		r.Patch("/{device_id}", s.handleDevicesPatch)
		r.Delete("/{device_id}", s.handleDevicesDelete)
		r.Put("/{device_id}/tags", s.handleDevicesSetTags)
		r.Put("/{device_id}/bindings/hdp", s.handleDevicesSetHDPBindings)
	})
	r.Post("/api/ers/selectors/resolve", s.handleSelectorsResolve)
	return r
}

func (s *Server) emit(eventType, entity string, id any) {
	if s.hub == nil {
		return
	}
	strID := strings.TrimSpace(fmt.Sprint(id))
	s.hub.Broadcast(realtime.Event{Type: eventType, Entity: entity, ID: strID})
}

type jsonErr struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, jsonErr{Error: msg, Code: status})
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func parseUUIDParam(r *http.Request, key string) (uuid.UUID, error) {
	raw := strings.TrimSpace(chi.URLParam(r, key))
	if raw == "" {
		return uuid.Nil, errors.New("missing id")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.New("invalid id")
	}
	return id, nil
}

func decodeJSONBToMap(raw datatypes.JSON) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}, nil
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func mergeJSONMaps(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, v := range src {
		if vMap, ok := v.(map[string]any); ok {
			if existing, okExisting := dst[k].(map[string]any); okExisting {
				dst[k] = mergeJSONMaps(existing, vMap)
				continue
			}
			dst[k] = mergeJSONMaps(map[string]any{}, vMap)
			continue
		}
		dst[k] = v
	}
	return dst
}

func encodeJSONB(value any) (datatypes.JSON, error) {
	buf, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(buf), nil
}

func mergeMeta(existing datatypes.JSON, patch any) (datatypes.JSON, error) {
	patchObj, ok := patch.(map[string]any)
	if !ok {
		return encodeJSONB(patch)
	}
	existingObj, _ := decodeJSONBToMap(existing)
	merged := mergeJSONMaps(existingObj, patchObj)
	return encodeJSONB(merged)
}

type homeResponse struct {
	Rooms   int `json:"rooms"`
	Tags    int `json:"tags"`
	Groups  int `json:"groups"`
	Devices int `json:"devices"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.cache != nil && s.listCacheTTL > 0 {
		var cached homeResponse
		if err := s.cache.Get(ctx, ersHomeCacheKey, &cached); err == nil {
			writeJSON(w, http.StatusOK, cached)
			return
		}
	}
	rooms, err := s.repo.ListRooms(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load rooms")
		return
	}
	tags, err := s.repo.ListTags(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load tags")
		return
	}
	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load groups")
		return
	}
	devs, err := s.repo.ListDevices(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load devices")
		return
	}
	response := homeResponse{Rooms: len(rooms), Tags: len(tags), Groups: len(groups), Devices: len(devs)}
	s.cacheWrite(ctx, ersHomeCacheKey, response)
	writeJSON(w, http.StatusOK, response)
}

func slugify(value string) string {
	s := strings.TrimSpace(strings.ToLower(value))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range s {
		isAZ := r >= 'a' && r <= 'z'
		is09 := r >= '0' && r <= '9'
		if isAZ || is09 {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 60 {
		out = out[:60]
		out = strings.Trim(out, "-")
	}
	return out
}
