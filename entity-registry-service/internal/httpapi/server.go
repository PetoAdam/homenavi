package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"entity-registry-service/internal/realtime"
	"entity-registry-service/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Server struct {
	repo *store.Repo
	hub  *realtime.Hub
}

func NewServer(repo *store.Repo, hub *realtime.Hub) *Server {
	return &Server{repo: repo, hub: hub}
}

func (s *Server) Register(mux *http.ServeMux) {
	r := chi.NewRouter()

	if s.hub != nil {
		r.Get("/ws/ers", s.hub.ServeHTTP)
	}

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

	mux.Handle("/", r)
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
		// If the stored value isn't an object (or is corrupted), treat it as empty.
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
	// If patch isn't an object, treat as replacement.
	patchObj, ok := patch.(map[string]any)
	if !ok {
		return encodeJSONB(patch)
	}
	existingObj, _ := decodeJSONBToMap(existing)
	merged := mergeJSONMaps(existingObj, patchObj)
	return encodeJSONB(merged)
}

// --- Handlers ---

type homeResponse struct {
	Rooms   int `json:"rooms"`
	Tags    int `json:"tags"`
	Devices int `json:"devices"`
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
	devs, err := s.repo.ListDevices(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load devices")
		return
	}
	writeJSON(w, http.StatusOK, homeResponse{Rooms: len(rooms), Tags: len(tags), Devices: len(devs)})
}

// Rooms

type roomCreateRequest struct {
	Name      string         `json:"name"`
	Slug      string         `json:"slug,omitempty"`
	SortOrder int            `json:"sort_order,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
}

func (s *Server) handleRoomsList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.repo.ListRooms(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load rooms")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleRoomsCreate(w http.ResponseWriter, r *http.Request) {
	var req roomCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(req.Name)
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(name)
	}
	room := &store.Room{Name: name, Slug: slug, SortOrder: req.SortOrder}
	if req.Meta != nil {
		b, err := encodeJSONB(req.Meta)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid meta")
			return
		}
		room.Meta = b
	}
	if err := s.repo.CreateRoom(r.Context(), room); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.emit("ers.room.created", "room", room.ID)
	writeJSON(w, http.StatusCreated, room)
}

func (s *Server) handleRoomsPatch(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "room_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var patch map[string]any
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Whitelist patchable fields to avoid accidental/unsafe column updates.
	allowed := map[string]struct{}{"name": {}, "slug": {}, "sort_order": {}, "meta": {}}
	for k := range patch {
		if _, ok := allowed[k]; !ok {
			writeError(w, http.StatusBadRequest, "unsupported field: "+k)
			return
		}
	}
	if rawMeta, ok := patch["meta"]; ok {
		if rawMeta == nil {
			patch["meta"] = nil
		} else {
			existing, err := s.repo.GetRoom(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			merged, err := mergeMeta(existing.Meta, rawMeta)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid meta")
				return
			}
			patch["meta"] = merged
		}
	}
	if v, ok := patch["name"].(string); ok {
		if strings.TrimSpace(v) != "" {
			patch["name"] = strings.TrimSpace(v)
			if _, hasSlug := patch["slug"]; !hasSlug {
				patch["slug"] = slugify(v)
			}
		}
	}
	row, err := s.repo.UpdateRoom(r.Context(), id, patch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update room")
		return
	}
	s.emit("ers.room.updated", "room", id)
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleRoomsDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "room_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.repo.DeleteRoom(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete room")
		return
	}
	s.emit("ers.room.deleted", "room", id)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// Tags

type tagCreateRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
}

func (s *Server) handleTagsList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.repo.ListTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load tags")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleTagsCreate(w http.ResponseWriter, r *http.Request) {
	var req tagCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(req.Name)
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(name)
	}
	tag := &store.Tag{Name: name, Slug: slug}
	if err := s.repo.CreateTag(r.Context(), tag); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.emit("ers.tag.created", "tag", tag.ID)
	writeJSON(w, http.StatusCreated, tag)
}

func (s *Server) handleTagsDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "tag_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.repo.DeleteTag(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete tag")
		return
	}
	s.emit("ers.tag.deleted", "tag", id)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

type setTagMembersRequest struct {
	DeviceIDs []string `json:"device_ids"`
}

func (s *Server) handleTagsSetMembers(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "tag_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req setTagMembersRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ids := make([]uuid.UUID, 0, len(req.DeviceIDs))
	for _, raw := range req.DeviceIDs {
		uid, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		ids = append(ids, uid)
	}
	if err := s.repo.SetTagMembers(r.Context(), id, ids); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update membership")
		return
	}
	s.emit("ers.tag.members_updated", "tag", id)
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

// Devices

type deviceCreateRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	RoomID      *string        `json:"room_id,omitempty"`
	TagIDs      []string       `json:"tag_ids,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

func (s *Server) handleDevicesList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.repo.ListDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load devices")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleDevicesCreate(w http.ResponseWriter, r *http.Request) {
	var req deviceCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var roomID *uuid.UUID
	if req.RoomID != nil && strings.TrimSpace(*req.RoomID) != "" {
		uid, err := uuid.Parse(strings.TrimSpace(*req.RoomID))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid room_id")
			return
		}
		roomID = &uid
	}
	dev := &store.Device{Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description), RoomID: roomID}
	if req.Meta != nil {
		b, err := encodeJSONB(req.Meta)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid meta")
			return
		}
		dev.Meta = b
	}
	if err := s.repo.CreateDevice(r.Context(), dev); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// tags
	if len(req.TagIDs) > 0 {
		ids := make([]uuid.UUID, 0, len(req.TagIDs))
		for _, raw := range req.TagIDs {
			uid, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil {
				continue
			}
			ids = append(ids, uid)
		}
		_ = s.repo.SetDeviceTags(r.Context(), dev.ID, ids)
	}

	view, err := s.repo.GetDeviceView(r.Context(), dev.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load device")
		return
	}
	s.emit("ers.device.created", "device", dev.ID)
	writeJSON(w, http.StatusCreated, view)
}

func (s *Server) handleDevicesGet(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "device_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	view, err := s.repo.GetDeviceView(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleDevicesPatch(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "device_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var patch map[string]any
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Whitelist patchable fields to avoid accidental/unsafe column updates.
	allowed := map[string]struct{}{"name": {}, "description": {}, "room_id": {}, "meta": {}}
	for k := range patch {
		if _, ok := allowed[k]; !ok {
			writeError(w, http.StatusBadRequest, "unsupported field: "+k)
			return
		}
	}
	if rawMeta, ok := patch["meta"]; ok {
		if rawMeta == nil {
			patch["meta"] = nil
		} else {
			existing, err := s.repo.GetDevice(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			merged, err := mergeMeta(existing.Meta, rawMeta)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid meta")
				return
			}
			patch["meta"] = merged
		}
	}
	if raw, ok := patch["room_id"]; ok {
		if raw == nil {
			patch["room_id"] = nil
		} else if s, okS := raw.(string); okS {
			s = strings.TrimSpace(s)
			if s == "" {
				patch["room_id"] = nil
			} else {
				uid, err := uuid.Parse(s)
				if err != nil {
					writeError(w, http.StatusBadRequest, "invalid room_id")
					return
				}
				patch["room_id"] = uid
			}
		}
	}
	_, err = s.repo.UpdateDevice(r.Context(), id, patch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update device")
		return
	}
	view, err := s.repo.GetDeviceView(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load device")
		return
	}
	s.emit("ers.device.updated", "device", id)
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleDevicesDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "device_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.repo.DeleteDevice(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete device")
		return
	}
	s.emit("ers.device.deleted", "device", id)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

type setDeviceTagsRequest struct {
	TagIDs []string `json:"tag_ids"`
}

func (s *Server) handleDevicesSetTags(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "device_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req setDeviceTagsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ids := make([]uuid.UUID, 0, len(req.TagIDs))
	for _, raw := range req.TagIDs {
		uid, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		ids = append(ids, uid)
	}
	if err := s.repo.SetDeviceTags(r.Context(), id, ids); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update tags")
		return
	}
	view, err := s.repo.GetDeviceView(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load device")
		return
	}
	s.emit("ers.device.updated", "device", id)
	writeJSON(w, http.StatusOK, view)
}

type setHDPBindingsRequest struct {
	HDPExternalID  string   `json:"hdp_external_id,omitempty"`
	HDPExternalIDs []string `json:"hdp_external_ids,omitempty"`
}

func (s *Server) handleDevicesSetHDPBindings(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "device_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req setHDPBindingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ids := req.HDPExternalIDs
	if strings.TrimSpace(req.HDPExternalID) != "" {
		ids = append(ids, req.HDPExternalID)
	}
	// Deduplicate to avoid unique-index violations.
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(ids))
	for _, raw := range ids {
		x := strings.TrimSpace(raw)
		if x == "" {
			continue
		}
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		unique = append(unique, x)
	}
	if err := s.repo.SetDeviceHDPBindings(r.Context(), id, unique); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set bindings")
		return
	}
	view, err := s.repo.GetDeviceView(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load device")
		return
	}
	s.emit("ers.device.bindings_updated", "device", id)
	writeJSON(w, http.StatusOK, view)
}

// Selectors

type selectorResolveRequest struct {
	Selector string `json:"selector"`
}

type selectorResolveResponse struct {
	Selector       string   `json:"selector"`
	HDPExternalIDs []string `json:"hdp_external_ids"`
	DeviceIDs      []string `json:"device_ids"`
}

func (s *Server) handleSelectorsResolve(w http.ResponseWriter, r *http.Request) {
	var req selectorResolveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ids, devIDs, err := s.repo.ResolveSelectorToHDP(r.Context(), req.Selector)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	outDev := make([]string, 0, len(devIDs))
	for _, id := range devIDs {
		outDev = append(outDev, id.String())
	}
	writeJSON(w, http.StatusOK, selectorResolveResponse{Selector: strings.TrimSpace(req.Selector), HDPExternalIDs: ids, DeviceIDs: outDev})
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
