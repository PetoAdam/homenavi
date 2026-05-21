package http

import (
	"net/http"
	"strings"

	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"

	"github.com/google/uuid"
)

type groupCreateRequest struct {
	Name        string         `json:"name"`
	Slug        string         `json:"slug,omitempty"`
	Description string         `json:"description,omitempty"`
	DeviceIDs   []string       `json:"device_ids,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

type setGroupMembersRequest struct {
	DeviceIDs []string `json:"device_ids"`
}

func (s *Server) handleGroupsList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.repo.ListGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load groups")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleGroupsCreate(w http.ResponseWriter, r *http.Request) {
	var req groupCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(req.Name)
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(name)
	}
	group := &dbinfra.Group{Name: name, Slug: slug, Description: strings.TrimSpace(req.Description)}
	if req.Meta != nil {
		b, err := encodeJSONB(req.Meta)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid meta")
			return
		}
		group.Meta = b
	}
	if err := s.repo.CreateGroup(r.Context(), group); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.repo.SetGroupMembers(r.Context(), group.ID, parseUUIDs(req.DeviceIDs)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set group members")
		return
	}
	view, err := s.repo.GetGroupView(r.Context(), group.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load group")
		return
	}
	s.emit("ers.group.created", "group", group.ID)
	writeJSON(w, http.StatusCreated, view)
}

func (s *Server) handleGroupsGet(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "group_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	view, err := s.repo.GetGroupView(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGroupsPatch(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "group_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var patch map[string]any
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	allowed := map[string]struct{}{"name": {}, "slug": {}, "description": {}, "meta": {}}
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
			existing, err := s.repo.GetGroup(r.Context(), id)
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
	if name, ok := patch["name"].(string); ok {
		name = strings.TrimSpace(name)
		patch["name"] = name
		if _, hasSlug := patch["slug"]; !hasSlug {
			patch["slug"] = slugify(name)
		}
	}
	_, err = s.repo.UpdateGroup(r.Context(), id, patch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update group")
		return
	}
	view, err := s.repo.GetGroupView(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load group")
		return
	}
	s.emit("ers.group.updated", "group", id)
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGroupsDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "group_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.repo.DeleteGroup(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete group")
		return
	}
	s.emit("ers.group.deleted", "group", id)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) handleGroupsSetMembers(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "group_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req setGroupMembersRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.repo.SetGroupMembers(r.Context(), id, parseUUIDs(req.DeviceIDs)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update members")
		return
	}
	view, err := s.repo.GetGroupView(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load group")
		return
	}
	s.emit("ers.group.updated", "group", id)
	writeJSON(w, http.StatusOK, view)
}

func parseUUIDs(values []string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(values))
	for _, raw := range values {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}
