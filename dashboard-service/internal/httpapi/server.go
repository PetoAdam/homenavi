package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dashboard-service/internal/middleware"
	"dashboard-service/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Server struct {
	repo                *store.Repo
	integrationProxyURL string
	client              *http.Client
}

type ServerOptions struct {
	IntegrationProxyURL string
	HTTPClient          *http.Client
}

func NewServer(repo *store.Repo, opts ServerOptions) *Server {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &Server{
		repo:                repo,
		integrationProxyURL: strings.TrimSpace(opts.IntegrationProxyURL),
		client:              client,
	}
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

// DashboardDoc is the JSON document stored in Dashboard.Doc.
// This format is designed to align with react-grid-layout (Responsive).
//
// layouts: { breakpoint: [ {i,x,y,w,h}, ... ] }
// items: widget instances keyed by instance_id.
type DashboardDoc struct {
	Layouts map[string][]map[string]any `json:"layouts"`
	Items   []map[string]any            `json:"items"`
}

func defaultDashboardDoc() DashboardDoc {
	// Seed a default dashboard with core widgets.
	// NOTE: ids are stable only within the generated doc; they are re-generated per dashboard.
	w1 := uuid.New().String()
	w2 := uuid.New().String()
	w3 := uuid.New().String()
	w4 := uuid.New().String()

	layouts := map[string][]map[string]any{}
	// Base layout for lg, then reuse for others (frontend can further adjust per breakpoint).
	base := []map[string]any{
		{"i": w1, "x": 0, "y": 0, "w": 1, "h": 8},  // Weather
		{"i": w2, "x": 1, "y": 0, "w": 1, "h": 8},  // Device
		{"i": w3, "x": 2, "y": 0, "w": 1, "h": 8},  // Automation trigger
		{"i": w4, "x": 0, "y": 8, "w": 3, "h": 10}, // Map
	}
	for _, bp := range []string{"lg", "md", "sm", "xs", "xxs"} {
		layouts[bp] = base
	}

	items := []map[string]any{
		{"instance_id": w1, "widget_type": "homenavi.weather", "enabled": true, "settings": map[string]any{}},
		{"instance_id": w2, "widget_type": "homenavi.device", "enabled": true, "settings": map[string]any{}},
		{"instance_id": w3, "widget_type": "homenavi.automation.manual_trigger", "enabled": true, "settings": map[string]any{}},
		{"instance_id": w4, "widget_type": "homenavi.map", "enabled": true, "settings": map[string]any{}},
	}

	return DashboardDoc{Layouts: layouts, Items: items}
}

// RegisterRoutes mounts protected API endpoints under an already-authenticated router.
func (s *Server) RegisterRoutes(r chi.Router) {
	r.Get("/widgets/catalog", s.handleCatalog)
	r.Get("/widgets/weather", s.handleWeather)

	r.Route("/dashboard", func(r chi.Router) {
		r.Get("/me", s.handleGetMyDashboard)
		r.Put("/me", s.handlePutMyDashboard)

		// Admin-only endpoints (defense-in-depth; gateway should also enforce).
		r.Group(func(r chi.Router) {
			r.Use(middleware.RoleAtLeastMiddleware("admin"))
			r.Get("/default", s.handleGetDefaultDashboard)
			r.Put("/default", s.handlePutDefaultDashboard)
		})
	})
}

type weatherResponse struct {
	City    string `json:"city"`
	Current any    `json:"current"`
	Daily   any    `json:"daily"`
	Weekly  any    `json:"weekly"`
}

func (s *Server) handleWeather(w http.ResponseWriter, r *http.Request) {
	city := strings.TrimSpace(r.URL.Query().Get("city"))
	if city == "" {
		city = "Budapest"
	}

	// MVP: deterministic sample payload served by the platform.
	// A real implementation can be swapped in later (provider integration, caching, location).
	now := time.Now().UTC()
	_ = now

	resp := weatherResponse{
		City: city,
		Current: map[string]any{
			"temp_c": 22,
			"hi_c":   24,
			"lo_c":   15,
			"desc":   "Sunny",
			"icon":   "sun",
		},
		Daily: []map[string]any{
			{"hour": "09", "temp_c": 20, "icon": "sun"},
			{"hour": "12", "temp_c": 22, "icon": "cloud_sun"},
			{"hour": "15", "temp_c": 21, "icon": "cloud"},
			{"hour": "18", "temp_c": 18, "icon": "rain"},
			{"hour": "21", "temp_c": 16, "icon": "cloud"},
		},
		Weekly: []map[string]any{
			{"day": "Fri", "temp_c": 22, "icon": "sun"},
			{"day": "Sat", "temp_c": 21, "icon": "cloud_sun"},
			{"day": "Sun", "temp_c": 19, "icon": "cloud"},
			{"day": "Mon", "temp_c": 17, "icon": "rain"},
			{"day": "Tue", "temp_c": 18, "icon": "cloud"},
			{"day": "Wed", "temp_c": 20, "icon": "sun"},
			{"day": "Thu", "temp_c": 21, "icon": "cloud_sun"},
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	base := []store.WidgetType{
		{
			ID:          "homenavi.weather",
			DisplayName: "Weather",
			Description: "Local weather overview.",
			Icon:        "sun",
			DefaultSize: "md",
			Verified:    true,
			Source:      "first_party",
		},
		{
			ID:          "homenavi.map",
			DisplayName: "Map",
			Description: "Rooms and placed devices.",
			Icon:        "map",
			DefaultSize: "lg",
			Verified:    true,
			Source:      "first_party",
		},
		{
			ID:          "homenavi.device",
			DisplayName: "Device",
			Description: "A configurable device widget.",
			Icon:        "lightbulb",
			DefaultSize: "md",
			Verified:    true,
			Source:      "first_party",
			SettingsSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ers_device_id": map[string]any{"type": "string"},
					"hdp_device_id": map[string]any{"type": "string"},
					"field1":        map[string]any{"type": "string"},
					"field2":        map[string]any{"type": "string"},
				},
			},
		},
		{
			ID:          "homenavi.device.graph",
			DisplayName: "Device Graph",
			Description: "A time-series chart for a device metric.",
			Icon:        "chart",
			DefaultSize: "md",
			Verified:    true,
			Source:      "first_party",
			SettingsSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"device_id":    map[string]any{"type": "string"},
					"metric_key":   map[string]any{"type": "string"},
					"range_preset": map[string]any{"type": "string"},
				},
			},
		},
		{
			ID:          "homenavi.automation.manual_trigger",
			DisplayName: "Automation Trigger",
			Description: "Run a manual automation workflow.",
			Icon:        "bolt",
			DefaultSize: "sm",
			Verified:    true,
			Source:      "first_party",
			SettingsSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workflow_id": map[string]any{"type": "string"},
				},
			},
		},
	}

	// Merge in integration widgets discovered from integration-proxy.
	merged := make([]store.WidgetType, 0, len(base)+8)
	byID := map[string]struct{}{}
	for _, w := range base {
		if strings.TrimSpace(w.ID) == "" {
			continue
		}
		byID[w.ID] = struct{}{}
		merged = append(merged, w)
	}

	if strings.TrimSpace(s.integrationProxyURL) != "" {
		widgets, err := s.fetchIntegrationWidgets(r.Context(), r)
		if err == nil {
			for _, w := range widgets {
				if strings.TrimSpace(w.ID) == "" {
					continue
				}
				if _, ok := byID[w.ID]; ok {
					continue
				}
				byID[w.ID] = struct{}{}
				merged = append(merged, w)
			}
		}
	}

	writeJSON(w, http.StatusOK, merged)
}

type integrationRegistry struct {
	Integrations []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Icon        string `json:"icon,omitempty"`
		Widgets     []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			Description string `json:"description,omitempty"`
			Icon        string `json:"icon,omitempty"`
			DefaultSize string `json:"default_size_hint,omitempty"`
			EntryURL    string `json:"entry_url,omitempty"`
			Verified    bool   `json:"verified"`
			Source      string `json:"source"`
		} `json:"widgets"`
	} `json:"integrations"`
}

func (s *Server) fetchIntegrationWidgets(ctx context.Context, in *http.Request) ([]store.WidgetType, error) {
	base := strings.TrimRight(strings.TrimSpace(s.integrationProxyURL), "/")
	if base == "" {
		return nil, errors.New("integration proxy url empty")
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/integrations/registry.json"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	// integration-proxy now requires resident/admin for all integration paths.
	// Forward the caller's auth so the catalog merge works for authenticated users.
	if in != nil {
		if auth := strings.TrimSpace(in.Header.Get("Authorization")); auth != "" {
			req.Header.Set("Authorization", auth)
		}
		if c, err := in.Cookie("auth_token"); err == nil && strings.TrimSpace(c.Value) != "" {
			req.AddCookie(c)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("integration registry fetch failed")
	}

	var reg integrationRegistry
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, err
	}

	out := make([]store.WidgetType, 0, 16)
	for _, integ := range reg.Integrations {
		fallbackIcon := strings.TrimSpace(integ.Icon)
		for _, w := range integ.Widgets {
			icon := strings.TrimSpace(w.Icon)
			if icon == "" {
				icon = fallbackIcon
			}
			entryURL := strings.TrimSpace(w.EntryURL)
			var entry *store.WidgetEntry
			if entryURL != "" {
				entry = &store.WidgetEntry{Kind: "iframe", URL: entryURL}
			}
			out = append(out, store.WidgetType{
				ID:          w.ID,
				DisplayName: w.DisplayName,
				Description: w.Description,
				Icon:        icon,
				DefaultSize: w.DefaultSize,
				EntryURL:    entryURL,
				Entry:       entry,
				Verified:    w.Verified,
				Source:      "integration",
			})
		}
	}
	return out, nil
}

func parseUserIDFromClaims(r *http.Request) (uuid.UUID, bool) {
	claims := middleware.GetClaims(r)
	if claims == nil {
		return uuid.Nil, false
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(claims.Subject))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

type dashboardResponse struct {
	DashboardID   uuid.UUID      `json:"dashboard_id"`
	Scope         string         `json:"scope"`
	OwnerUserID   *uuid.UUID     `json:"owner_user_id,omitempty"`
	Title         string         `json:"title"`
	LayoutEngine  string         `json:"layout_engine"`
	LayoutVersion int            `json:"layout_version"`
	Doc           datatypes.JSON `json:"doc"`
	CreatedAt     any            `json:"created_at,omitempty"`
	UpdatedAt     any            `json:"updated_at,omitempty"`
}

func toDashboardResponse(d *store.Dashboard) dashboardResponse {
	return dashboardResponse{
		DashboardID:   d.ID,
		Scope:         d.Scope,
		OwnerUserID:   d.OwnerUserID,
		Title:         d.Title,
		LayoutEngine:  d.LayoutEngine,
		LayoutVersion: d.LayoutVersion,
		Doc:           d.Doc,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
}

func (s *Server) ensureDefault(ctx context.Context) (*store.Dashboard, error) {
	def, err := s.repo.GetDefaultDashboard(ctx)
	if err != nil {
		return nil, err
	}
	if def != nil {
		return def, nil
	}
	doc := defaultDashboardDoc()
	return s.repo.UpsertDefaultDashboard(ctx, "Home", doc)
}

func (s *Server) handleGetMyDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUserIDFromClaims(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ctx := r.Context()
	ud, err := s.repo.GetUserDashboard(ctx, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load dashboard")
		return
	}
	if ud != nil {
		writeJSON(w, http.StatusOK, toDashboardResponse(ud))
		return
	}

	// Create default if missing, then clone.
	def, err := s.repo.GetDefaultDashboard(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load default dashboard")
		return
	}
	if def == nil {
		doc := defaultDashboardDoc()
		def, err = s.repo.UpsertDefaultDashboard(ctx, "Home", doc)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create default dashboard")
			return
		}
	}

	// Clone default doc but regenerate instance IDs so each user has independent instances.
	var defDoc DashboardDoc
	_ = json.Unmarshal(def.Doc, &defDoc)

	// Rebuild doc with new instance ids but same widget types.
	newLayouts := map[string][]map[string]any{}
	newItems := []map[string]any{}
	idMap := map[string]string{}

	for _, it := range defDoc.Items {
		oldID, _ := it["instance_id"].(string)
		wt, _ := it["widget_type"].(string)
		if strings.TrimSpace(wt) == "" {
			continue
		}
		newID := uuid.New().String()
		idMap[oldID] = newID
		settings := map[string]any{}
		if sRaw, ok := it["settings"].(map[string]any); ok {
			settings = sRaw
		}
		enabled := true
		if e, ok := it["enabled"].(bool); ok {
			enabled = e
		}
		newItems = append(newItems, map[string]any{"instance_id": newID, "widget_type": wt, "enabled": enabled, "settings": settings})
	}
	for bp, arr := range defDoc.Layouts {
		var next []map[string]any
		for _, li := range arr {
			i, _ := li["i"].(string)
			mapped := idMap[i]
			if mapped == "" {
				continue
			}
			copy := map[string]any{}
			for k, v := range li {
				copy[k] = v
			}
			copy["i"] = mapped
			next = append(next, copy)
		}
		newLayouts[bp] = next
	}

	newDoc := DashboardDoc{Layouts: newLayouts, Items: newItems}
	buf, err := json.Marshal(newDoc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create dashboard")
		return
	}

	d := &store.Dashboard{
		ID:            uuid.New(),
		Scope:         "user",
		OwnerUserID:   &userID,
		Title:         def.Title,
		LayoutEngine:  def.LayoutEngine,
		LayoutVersion: 1,
		Doc:           datatypes.JSON(buf),
	}
	if err := s.repo.CreateDashboard(ctx, d); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create dashboard")
		return
	}
	writeJSON(w, http.StatusOK, toDashboardResponse(d))
}

type putDashboardRequest struct {
	LayoutVersion int             `json:"layout_version"`
	Doc           json.RawMessage `json:"doc"`
}

func (s *Server) handlePutMyDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUserIDFromClaims(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req putDashboardRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.LayoutVersion <= 0 {
		writeError(w, http.StatusBadRequest, "layout_version required")
		return
	}
	if len(req.Doc) == 0 {
		writeError(w, http.StatusBadRequest, "doc required")
		return
	}
	updated, err := s.repo.UpdateUserDashboardDoc(r.Context(), userID, req.LayoutVersion, datatypes.JSON(req.Doc))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save dashboard")
		return
	}
	if updated == nil {
		writeError(w, http.StatusConflict, "dashboard version conflict")
		return
	}
	writeJSON(w, http.StatusOK, toDashboardResponse(updated))
}

func (s *Server) handleGetDefaultDashboard(w http.ResponseWriter, r *http.Request) {
	// Admin-only in routing.
	ctx := r.Context()
	def, err := s.repo.GetDefaultDashboard(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load default dashboard")
		return
	}
	if def == nil {
		doc := defaultDashboardDoc()
		def, err = s.repo.UpsertDefaultDashboard(ctx, "Home", doc)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create default dashboard")
			return
		}
	}
	writeJSON(w, http.StatusOK, toDashboardResponse(def))
}

type putDefaultDashboardRequest struct {
	Title string          `json:"title"`
	Doc   json.RawMessage `json:"doc"`
}

func (s *Server) handlePutDefaultDashboard(w http.ResponseWriter, r *http.Request) {
	// Admin-only in routing.
	var req putDefaultDashboardRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Home"
	}
	if len(req.Doc) == 0 {
		writeError(w, http.StatusBadRequest, "doc required")
		return
	}
	var doc any
	if err := json.Unmarshal(req.Doc, &doc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid doc")
		return
	}
	def, err := s.repo.UpsertDefaultDashboard(r.Context(), title, doc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save default dashboard")
		return
	}
	writeJSON(w, http.StatusOK, toDashboardResponse(def))
}
