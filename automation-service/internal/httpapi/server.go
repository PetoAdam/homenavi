package httpapi

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"automation-service/internal/engine"
	"automation-service/internal/middleware"
	"automation-service/internal/store"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/datatypes"
)

type Server struct {
	repo   *store.Repo
	engine *engine.Engine
	pubKey *rsa.PublicKey

	httpClient     *http.Client
	userServiceURL string
}

func New(repo *store.Repo, eng *engine.Engine, pubKey *rsa.PublicKey, userServiceURL string, httpClient *http.Client) *Server {
	hc := httpClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Server{repo: repo, engine: eng, pubKey: pubKey, httpClient: hc, userServiceURL: strings.TrimRight(strings.TrimSpace(userServiceURL), "/")}
}

type userServiceUser struct {
	ID       string `json:"id"`
	UserName string `json:"user_name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type userServiceListResponse struct {
	Users      []userServiceUser `json:"users"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}

func getAuthToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}
	return ""
}

func (s *Server) fetchUser(ctx context.Context, token string, userID string) (*userServiceUser, error) {
	if s.userServiceURL == "" {
		return nil, errors.New("user service url not configured")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("user id required")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("missing auth token")
	}

	url := s.userServiceURL + "/users/" + userID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("user-service %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}

	var u userServiceUser
	if err := json.Unmarshal(b, &u); err != nil {
		return nil, errors.New("invalid user-service response")
	}
	u.Email = strings.TrimSpace(u.Email)
	u.UserName = strings.TrimSpace(u.UserName)
	if u.Email == "" {
		return nil, errors.New("user has no email")
	}
	if u.UserName == "" {
		u.UserName = "Homenavi user"
	}
	return &u, nil
}

func (s *Server) listAllUsers(ctx context.Context, token string) ([]userServiceUser, error) {
	if s.userServiceURL == "" {
		return nil, errors.New("user service url not configured")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("missing auth token")
	}

	all := make([]userServiceUser, 0, 256)
	page := 1
	pageSize := 200
	for {
		url := fmt.Sprintf("%s/users?page=%d&page_size=%d", s.userServiceURL, page, pageSize)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("user-service %s: %s", resp.Status, strings.TrimSpace(string(b)))
		}
		var lr userServiceListResponse
		if err := json.Unmarshal(b, &lr); err != nil {
			return nil, errors.New("invalid user-service list response")
		}
		all = append(all, lr.Users...)
		if lr.TotalPages <= 0 || page >= lr.TotalPages {
			break
		}
		page++
		if page > 1000 {
			break
		}
	}
	return all, nil
}

func (s *Server) enrichNotifyEmailRecipients(ctx context.Context, token string, defBytes []byte) ([]byte, error) {
	var d engine.Definition
	if err := json.Unmarshal(defBytes, &d); err != nil {
		return nil, errors.New("definition must be valid json")
	}

	for i := range d.Nodes {
		n := d.Nodes[i]
		if strings.ToLower(strings.TrimSpace(n.Kind)) != "action.notify_email" {
			continue
		}
		var a engine.ActionNotifyEmail
		if err := json.Unmarshal(n.Data, &a); err != nil {
			return nil, errors.New("action.notify_email data must be valid json object")
		}

		seen := map[string]struct{}{}
		recips := make([]engine.NotifyEmailRecipient, 0, len(a.UserIDs))

		// Expand role targets (admin-only in validation), using the caller's JWT.
		roleSet := map[string]struct{}{}
		for _, tr := range a.TargetRoles {
			r := strings.ToLower(strings.TrimSpace(tr))
			if r == "" {
				continue
			}
			roleSet[r] = struct{}{}
		}
		if len(roleSet) > 0 {
			users, err := s.listAllUsers(ctx, token)
			if err != nil {
				return nil, err
			}
			for _, u := range users {
				uid := strings.TrimSpace(u.ID)
				if uid == "" {
					continue
				}
				role := strings.ToLower(strings.TrimSpace(u.Role))
				if _, ok := roleSet[role]; !ok {
					continue
				}
				if _, ok := seen[uid]; ok {
					continue
				}
				seen[uid] = struct{}{}
				email := strings.TrimSpace(u.Email)
				name := strings.TrimSpace(u.UserName)
				if email == "" {
					continue
				}
				if name == "" {
					name = "Homenavi user"
				}
				recips = append(recips, engine.NotifyEmailRecipient{UserID: uid, Email: email, UserName: name})
			}
		}

		for _, rawID := range a.UserIDs {
			uid := strings.TrimSpace(rawID)
			if uid == "" {
				continue
			}
			if _, ok := seen[uid]; ok {
				continue
			}
			seen[uid] = struct{}{}

			u, err := s.fetchUser(ctx, token, uid)
			if err != nil {
				return nil, err
			}
			recips = append(recips, engine.NotifyEmailRecipient{UserID: uid, Email: u.Email, UserName: u.UserName})
		}
		a.Recipients = recips
		b, _ := json.Marshal(a)
		d.Nodes[i].Data = b
	}

	out, _ := json.Marshal(d)
	return out, nil
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	// NOTE: WebSocket routes are authenticated at the API gateway.
	// The gateway's WS reverse proxy does not forward Authorization/Cookies to upstream.
	// Therefore, the upstream WS handlers must NOT require JWT.
	r.Get("/api/automation/runs/{run_id}/ws", s.handleRunEventsWS)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	r.Route("/api/automation", func(r chi.Router) {
		if s.pubKey == nil {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					writeError(w, http.StatusInternalServerError, "jwt public key not configured")
				})
			})
			return
		}
		r.Use(middleware.JWTAuthMiddlewareRS256(s.pubKey))
		r.Use(middleware.RoleAtLeastMiddleware("resident"))

		r.Get("/nodes", s.handleNodes)

		r.Get("/workflows", s.handleListWorkflows)
		r.Post("/workflows", s.handleCreateWorkflow)
		r.Route("/workflows/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetWorkflow)
			r.Put("/", s.handleUpdateWorkflow)
			r.Post("/enable", s.handleEnableWorkflow(true))
			r.Post("/disable", s.handleEnableWorkflow(false))
			r.Post("/run", s.handleRunWorkflow)
			r.Get("/runs", s.handleListRuns)
			// delete restricted to admin
			r.With(middleware.RoleAtLeastMiddleware("admin")).Delete("/", s.handleDeleteWorkflow)
		})
		r.Get("/runs/{run_id}", s.handleGetRun)
	})

	return r
}

func (s *Server) handleRunEventsWS(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "run_id"))
	if err != nil {
		http.Error(w, "invalid run id", http.StatusBadRequest)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Subscribe to run events (includes a small replay buffer).
	ch, cancel := s.engine.SubscribeRunEvents(runID)
	defer cancel()

	// Read pump just to detect disconnects.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = conn.SetReadDeadline(time.Time{})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Periodic ping to keep intermediaries alive.
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-done:
			return
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(2*time.Second)); err != nil {
				return
			}
		case evt, ok := <-ch:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(evt); err != nil {
				slog.Debug("ws write failed", "error", err)
				return
			}
		}
	}
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	// Static catalog; UI is data-driven.
	catalog := []map[string]any{
		{"kind": "trigger.manual", "label": "Manual", "fields": []map[string]any{}},
		{"kind": "trigger.device_state", "label": "Device state", "fields": []map[string]any{
			{"name": "device_id", "type": "string", "required": true},
			{"name": "key", "type": "string", "required": false, "help": "State key (e.g. motion, temperature). Empty matches any state frame."},
			{"name": "op", "type": "string", "required": false, "enum": []string{"exists", "eq", "neq", "gt", "gte", "lt", "lte"}},
			{"name": "value", "type": "json", "required": false},
			{"name": "ignore_retained", "type": "bool", "required": false, "default": true},
			{"name": "cooldown_sec", "type": "int", "required": false, "default": 2},
		}},
		{"kind": "trigger.schedule", "label": "Schedule (cron)", "fields": []map[string]any{
			{"name": "cron", "type": "string", "required": true, "help": "Cron with seconds: e.g. '0 */5 * * * *' (every 5 minutes)"},
			{"name": "cooldown_sec", "type": "int", "required": false, "default": 1},
		}},
		{"kind": "logic.sleep", "label": "Sleep", "fields": []map[string]any{
			{"name": "duration_sec", "type": "int", "required": true, "default": 5},
		}},
		{"kind": "logic.if", "label": "If", "fields": []map[string]any{
			{"name": "path", "type": "string", "required": true, "help": "Dot-path in trigger event, e.g. state.motion"},
			{"name": "op", "type": "string", "required": false, "enum": []string{"exists", "eq", "neq", "gt", "gte", "lt", "lte"}},
			{"name": "value", "type": "json", "required": false},
		}},
		{"kind": "logic.for", "label": "For loop", "fields": []map[string]any{
			{"name": "count", "type": "int", "required": true, "default": 3},
		}},
		{"kind": "action.send_command", "label": "Send device command", "fields": []map[string]any{
			{"name": "device_id", "type": "string", "required": true},
			{"name": "command", "type": "string", "required": true, "default": "set_state"},
			{"name": "args", "type": "json", "required": false, "help": "Command args map. For set_state: {state:'ON', brightness:80}."},
			{"name": "wait_for_result", "type": "bool", "required": false, "default": false},
			{"name": "result_timeout_sec", "type": "int", "required": false, "default": 15},
		}},
		{"kind": "action.notify_email", "label": "Notify email", "fields": []map[string]any{
			{"name": "user_ids", "type": "json", "required": true, "help": "Array of user IDs."},
			{"name": "subject", "type": "string", "required": true},
			{"name": "message", "type": "string", "required": true},
		}},
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": catalog, "version": "automation"})
}

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
	claims := middleware.GetClaims(r)
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
	wf := &store.Workflow{Name: name, Enabled: false, Definition: datatypes.JSON(def), CreatedBy: createdBy}
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
		claims := middleware.GetClaims(r)
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

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workflow id")
		return
	}
	limit := 5
	if ls := strings.TrimSpace(r.URL.Query().Get("limit")); ls != "" {
		if n, err := parsePositiveInt(ls); err == nil {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	runs, err := s.repo.ListRuns(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "run_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	run, steps, err := s.repo.GetRunWithSteps(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "steps": steps})
}

func validateDefinition(raw json.RawMessage, claims *middleware.Claims) ([]byte, error) {
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

	// Additional authorization rules that depend on caller identity.
	// - Residents can only set notify_email recipients to themselves.
	// - Admins can set any recipients.
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

func parsePositiveInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty")
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int(r-'0')
		if n > 1_000_000 {
			return 0, errors.New("too large")
		}
	}
	if n <= 0 {
		return 0, errors.New("must be > 0")
	}
	return n, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg, "code": status})
}
