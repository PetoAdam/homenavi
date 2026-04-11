package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/PetoAdam/homenavi/automation-service/internal/auth"
	"github.com/PetoAdam/homenavi/automation-service/internal/engine"
	dbinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/db"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type workflowPayload struct {
	Name       string          `json:"name"`
	Enabled    *bool           `json:"enabled,omitempty"`
	Definition json.RawMessage `json:"definition"`
}

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	rows, err := s.repo.ListWorkflows(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workflows")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": rows})
}

func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workflow id")
		return
	}
	wf, err := s.repo.GetWorkflow(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (s *Server) handleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var p workflowPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	claims := auth.GetClaims(r)
	def, err := validateDefinition(p.Definition, claims)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	def, err = s.enrichNotifyEmailRecipients(r.Context(), getAuthToken(r), def)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	createdBy := ""
	if claims != nil {
		createdBy = claims.Sub
	}
	wf := &dbinfra.Workflow{Name: name, Enabled: false, Definition: datatypes.JSON(def), CreatedBy: createdBy}
	if err := s.repo.CreateWorkflow(r.Context(), wf); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workflow")
		return
	}
	_ = s.engine.ReloadNow(r.Context())
	writeJSON(w, http.StatusCreated, wf)
}

func (s *Server) handleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workflow id")
		return
	}
	wf, err := s.repo.GetWorkflow(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	var p workflowPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(p.Name) != "" {
		wf.Name = strings.TrimSpace(p.Name)
	}
	if p.Enabled != nil {
		wf.Enabled = *p.Enabled
	}
	if len(p.Definition) > 0 {
		claims := auth.GetClaims(r)
		def, err := validateDefinition(p.Definition, claims)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		def, err = s.enrichNotifyEmailRecipients(r.Context(), getAuthToken(r), def)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		wf.Definition = datatypes.JSON(def)
	}
	if err := s.repo.UpdateWorkflow(r.Context(), wf); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update workflow")
		return
	}
	_ = s.engine.ReloadNow(r.Context())
	writeJSON(w, http.StatusOK, wf)
}

func (s *Server) handleDeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workflow id")
		return
	}
	if err := s.repo.DeleteWorkflow(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	_ = s.engine.ReloadNow(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) handleEnableWorkflow(enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid workflow id")
			return
		}
		if err := s.repo.SetWorkflowEnabled(r.Context(), id, enabled); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update workflow")
			return
		}
		_ = s.engine.ReloadNow(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled})
	}
}

func (s *Server) handleRunWorkflow(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workflow id")
		return
	}
	runID, err := s.engine.RunWorkflowNow(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queued": true, "run_id": runID.String()})
}

func validateDefinition(raw json.RawMessage, claims *auth.Claims) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("definition is required")
	}
	var d engine.Definition
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, errors.New("definition must be valid json")
	}
	if err := d.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	if claims != nil {
		role := strings.ToLower(strings.TrimSpace(claims.Role))
		sub := strings.TrimSpace(claims.Sub)
		if role == "resident" && sub != "" {
			for _, n := range d.Nodes {
				if strings.ToLower(strings.TrimSpace(n.Kind)) != "action.notify_email" {
					continue
				}
				var a engine.ActionNotifyEmail
				if err := json.Unmarshal(n.Data, &a); err != nil {
					return nil, errors.New("action.notify_email data must be valid json object")
				}
				for _, tr := range a.TargetRoles {
					if strings.TrimSpace(tr) != "" {
						return nil, errors.New("residents cannot target user groups")
					}
				}
				for _, uid := range a.UserIDs {
					if strings.TrimSpace(uid) != sub {
						return nil, errors.New("residents can only notify themselves")
					}
				}
			}
		}
	}
	b, _ := json.Marshal(d)
	return b, nil
}
