package http

import (
	"net/http"
	"strings"

	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"
)

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
	room := &dbinfra.Room{Name: name, Slug: slug, SortOrder: req.SortOrder}
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
