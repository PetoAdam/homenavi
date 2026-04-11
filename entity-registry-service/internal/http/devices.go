package http

import (
	"net/http"
	"strings"

	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"

	"github.com/google/uuid"
)

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
	dev := &dbinfra.Device{Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description), RoomID: roomID}
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
