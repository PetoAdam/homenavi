package http

import (
	"net/http"
	"strings"

	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"

	"github.com/google/uuid"
)

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
	tag := &dbinfra.Tag{Name: name, Slug: slug}
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
