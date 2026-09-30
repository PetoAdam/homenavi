package dashboard

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/PetoAdam/homenavi/shared/mockdemo"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	demoGoodMorningWorkflowID       = "c358553a-7174-49b6-bcaa-44c30293ecf2"
	demoMovieNightWorkflowID        = "0f3fbc02-1e54-4b30-a40f-b871d39cff8f"
	demoLeaveHomeWorkflowID         = "8b40db6f-c7b3-4fe4-a716-95f57642d533"
	demoEveningLightsGroupID        = "f6127f16-9546-44a2-90a1-0fa0d7c26c33"
	demoWelcomeWidgetDefaultTitle   = "Start here"
	demoQuickLightsWidgetTitle      = "Quick Lights"
	demoEveningLightsWidgetTitle    = "Evening Lights"
	demoAirPurifierWidgetTitle      = "Air Quality"
	demoBedroomBlindWidgetTitle     = "Bedroom Blind"
	demoCoffeeMakerWidgetTitle      = "Coffee Maker"
	demoSofaLampWidgetTitle         = "Sofa Lamp"
	demoAirQualityTrendWidgetTitle  = "Air Quality Trend"
	demoGoodMorningWidgetTitle      = "Morning Scene"
	demoMovieNightWidgetTitle       = "Movie Night"
	demoLeaveHomeWidgetTitle        = "Leave Home"
)

// Service orchestrates dashboard use cases.
type Service struct {
	repo          Repository
	catalogSource WidgetCatalogSource
}

func NewService(repo Repository, catalogSource WidgetCatalogSource) *Service {
	return &Service{repo: repo, catalogSource: catalogSource}
}

func (s *Service) Catalog(ctx context.Context, auth AuthContext) []WidgetType {
	base := []WidgetType{
		{ID: "homenavi.demo.overview", DisplayName: "Welcome", Description: "Quick orientation for a fresh Homenavi dashboard.", Icon: "sparkles", DefaultSize: "sm", Verified: true, Source: "first_party"},
		{ID: "homenavi.weather", DisplayName: "Weather", Description: "Local weather overview.", Icon: "sun", DefaultSize: "md", Verified: true, Source: "first_party"},
		{ID: "homenavi.map", DisplayName: "Map", Description: "Rooms and placed devices.", Icon: "map", DefaultSize: "lg", Verified: true, Source: "first_party"},
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
					"controls":      map[string]any{"type": "array"},
					"fields":        map[string]any{"type": "array"},
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
					"hdp_device_id": map[string]any{"type": "string"},
					"metric_key":   map[string]any{"type": "string"},
					"range_preset": map[string]any{"type": "string"},
				},
			},
		},
		{
			ID:          "homenavi.device.multi",
			DisplayName: "Quick Controls",
			Description: "Toggle multiple curated devices and groups from one widget.",
			Icon:        "layer-group",
			DefaultSize: "md",
			Verified:    true,
			Source:      "first_party",
			SettingsSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"device_ids": map[string]any{"type": "array"},
					"group_ids":  map[string]any{"type": "array"},
				},
			},
		},
		{
			ID:          "homenavi.group.controls",
			DisplayName: "Group Controls",
			Description: "Control a curated device group together.",
			Icon:        "layer-group",
			DefaultSize: "sm",
			Verified:    true,
			Source:      "first_party",
			SettingsSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group_id":  map[string]any{"type": "string"},
					"group_ids": map[string]any{"type": "array"},
					"controls":  map[string]any{"type": "array"},
					"fields":    map[string]any{"type": "array"},
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

	merged := make([]WidgetType, 0, len(base)+8)
	byID := map[string]struct{}{}
	for _, widget := range base {
		byID[widget.ID] = struct{}{}
		merged = append(merged, widget)
	}
	if s.catalogSource == nil {
		return merged
	}
	widgets, err := s.catalogSource.Widgets(ctx, auth)
	if err != nil {
		return merged
	}
	for _, widget := range widgets {
		if strings.TrimSpace(widget.ID) == "" {
			continue
		}
		if _, ok := byID[widget.ID]; ok {
			continue
		}
		byID[widget.ID] = struct{}{}
		merged = append(merged, widget)
	}
	return merged
}

func (s *Service) Weather(city string) WeatherResponse {
	city = strings.TrimSpace(city)
	if city == "" {
		city = "Budapest"
	}
	return WeatherResponse{
		City:    city,
		Current: map[string]any{"temp_c": 22, "hi_c": 24, "lo_c": 15, "desc": "Sunny", "icon": "sun"},
		Daily:   []map[string]any{{"hour": "09", "temp_c": 20, "icon": "sun"}, {"hour": "12", "temp_c": 22, "icon": "cloud_sun"}, {"hour": "15", "temp_c": 21, "icon": "cloud"}, {"hour": "18", "temp_c": 18, "icon": "rain"}, {"hour": "21", "temp_c": 16, "icon": "cloud"}},
		Weekly:  []map[string]any{{"day": "Fri", "temp_c": 22, "icon": "sun"}, {"day": "Sat", "temp_c": 21, "icon": "cloud_sun"}, {"day": "Sun", "temp_c": 19, "icon": "cloud"}, {"day": "Mon", "temp_c": 17, "icon": "rain"}, {"day": "Tue", "temp_c": 18, "icon": "cloud"}, {"day": "Wed", "temp_c": 20, "icon": "sun"}, {"day": "Thu", "temp_c": 21, "icon": "cloud_sun"}},
	}
}

func (s *Service) GetMyDashboard(ctx context.Context, userID uuid.UUID) (Dashboard, error) {
	ud, err := s.repo.GetUserDashboard(ctx, userID)
	if err != nil {
		return Dashboard{}, err
	}
	if ud != nil {
		if isDemoModeEnabled() {
			def, err := s.ensureDefault(ctx)
			if err != nil {
				return Dashboard{}, err
			}
			clonedDoc, err := cloneDashboardDoc(def.Doc)
			if err != nil {
				return Dashboard{}, err
			}
			updated, err := s.repo.UpdateUserDashboardDoc(ctx, userID, ud.LayoutVersion, clonedDoc)
			if err != nil {
				return Dashboard{}, err
			}
			if updated != nil {
				updated.Title = def.Title
				updated.LayoutEngine = def.LayoutEngine
				return *updated, nil
			}
			refreshed, err := s.repo.GetUserDashboard(ctx, userID)
			if err != nil {
				return Dashboard{}, err
			}
			if refreshed != nil {
				return *refreshed, nil
			}
		}
		return *ud, nil
	}

	def, err := s.ensureDefault(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	clonedDoc, err := cloneDashboardDoc(def.Doc)
	if err != nil {
		return Dashboard{}, err
	}

	d := &Dashboard{
		ID:            uuid.New(),
		Scope:         "user",
		OwnerUserID:   &userID,
		Title:         def.Title,
		LayoutEngine:  def.LayoutEngine,
		LayoutVersion: 1,
		Doc:           clonedDoc,
	}
	if err := s.repo.CreateDashboard(ctx, d); err != nil {
		return Dashboard{}, err
	}
	return *d, nil
}

func (s *Service) PutMyDashboard(ctx context.Context, userID uuid.UUID, layoutVersion int, doc json.RawMessage) (Dashboard, error) {
	updated, err := s.repo.UpdateUserDashboardDoc(ctx, userID, layoutVersion, datatypes.JSON(doc))
	if err != nil {
		return Dashboard{}, err
	}
	if updated == nil {
		return Dashboard{}, ErrConflict
	}
	return *updated, nil
}

func (s *Service) DeleteUserDashboard(ctx context.Context, userID uuid.UUID) error {
	return s.repo.DeleteUserDashboard(ctx, userID)
}

func (s *Service) GetDefaultDashboard(ctx context.Context) (Dashboard, error) {
	def, err := s.ensureDefault(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	return *def, nil
}

func (s *Service) PutDefaultDashboard(ctx context.Context, title string, doc json.RawMessage) (Dashboard, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Home"
	}
	var parsed any
	if err := json.Unmarshal(doc, &parsed); err != nil {
		return Dashboard{}, err
	}
	def, err := s.repo.UpsertDefaultDashboard(ctx, title, parsed)
	if err != nil {
		return Dashboard{}, err
	}
	return *def, nil
}

func (s *Service) ensureDefault(ctx context.Context) (*Dashboard, error) {
	if isDemoModeEnabled() {
		return s.repo.UpsertDefaultDashboard(ctx, "Home", defaultDashboardDocForMode(true))
	}
	def, err := s.repo.GetDefaultDashboard(ctx)
	if err != nil {
		return nil, err
	}
	if def != nil {
		return def, nil
	}
	return s.repo.UpsertDefaultDashboard(ctx, "Home", defaultDashboardDocForMode(false))
}

func isDemoModeEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("DEMO_MODE"))
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	if err == nil {
		return enabled
	}
	switch strings.ToLower(raw) {
	case "yes", "y", "on":
		return true
	default:
		return false
	}
}

func defaultDashboardDocForMode(demoMode bool) DashboardDoc {
	if demoMode {
		return demoDefaultDashboardDoc()
	}
	return defaultDashboardDoc()
}

func defaultDashboardDoc() DashboardDoc {
	w1 := uuid.New().String()
	w2 := uuid.New().String()
	w3 := uuid.New().String()
	w4 := uuid.New().String()
	layoutsByCols := map[string][]map[string]any{
		"4": {
			{"i": w1, "x": 0, "y": 0, "w": 1, "h": 8},
			{"i": w2, "x": 1, "y": 0, "w": 1, "h": 8},
			{"i": w3, "x": 2, "y": 0, "w": 1, "h": 8},
			{"i": w4, "x": 3, "y": 0, "w": 1, "h": 10},
		},
		"3": {
			{"i": w1, "x": 0, "y": 0, "w": 1, "h": 8},
			{"i": w2, "x": 1, "y": 0, "w": 1, "h": 8},
			{"i": w3, "x": 2, "y": 0, "w": 1, "h": 8},
			{"i": w4, "x": 0, "y": 8, "w": 2, "h": 10},
		},
		"2": {
			{"i": w1, "x": 0, "y": 0, "w": 1, "h": 8},
			{"i": w2, "x": 1, "y": 0, "w": 1, "h": 8},
			{"i": w3, "x": 0, "y": 8, "w": 1, "h": 8},
			{"i": w4, "x": 1, "y": 8, "w": 1, "h": 10},
		},
		"1": {
			{"i": w1, "x": 0, "y": 0, "w": 1, "h": 8},
			{"i": w2, "x": 0, "y": 8, "w": 1, "h": 8},
			{"i": w3, "x": 0, "y": 16, "w": 1, "h": 8},
			{"i": w4, "x": 0, "y": 24, "w": 1, "h": 10},
		},
	}
	items := []map[string]any{{"instance_id": w1, "widget_type": "homenavi.weather", "enabled": true, "settings": map[string]any{}}, {"instance_id": w2, "widget_type": "homenavi.device", "enabled": true, "settings": map[string]any{}}, {"instance_id": w3, "widget_type": "homenavi.automation.manual_trigger", "enabled": true, "settings": map[string]any{}}, {"instance_id": w4, "widget_type": "homenavi.map", "enabled": true, "settings": map[string]any{}}}
	return DashboardDoc{LayoutsByCols: layoutsByCols, Items: items}
}

func demoDefaultDashboardDoc() DashboardDoc {
	w1 := uuid.New().String()
	w2 := uuid.New().String()
	w3 := uuid.New().String()
	w4 := uuid.New().String()
	w5 := uuid.New().String()
	w6 := uuid.New().String()
	w7 := uuid.New().String()
	w8 := uuid.New().String()
	w9 := uuid.New().String()
	w10 := uuid.New().String()
	w11 := uuid.New().String()
	w12 := uuid.New().String()
	w13 := uuid.New().String()
	w14 := uuid.New().String()
	layoutsByCols := map[string][]map[string]any{
		"4": {
			{"i": w2, "x": 0, "y": 0, "w": 1, "h": 5},
			{"i": w11, "x": 1, "y": 0, "w": 1, "h": 3},
			{"i": w1, "x": 2, "y": 0, "w": 2, "h": 3},
			{"i": w14, "x": 1, "y": 3, "w": 1, "h": 4},
			{"i": w10, "x": 2, "y": 3, "w": 2, "h": 8},
			{"i": w3, "x": 0, "y": 5, "w": 1, "h": 5},
			{"i": w13, "x": 1, "y": 7, "w": 1, "h": 3},
			{"i": w6, "x": 0, "y": 11, "w": 1, "h": 5},
			{"i": w4, "x": 1, "y": 11, "w": 1, "h": 6},
			{"i": w7, "x": 2, "y": 11, "w": 1, "h": 5},
			{"i": w5, "x": 3, "y": 11, "w": 1, "h": 5},
			{"i": w12, "x": 0, "y": 16, "w": 1, "h": 3},
			{"i": w8, "x": 3, "y": 16, "w": 1, "h": 5},
			{"i": w9, "x": 1, "y": 17, "w": 2, "h": 6},
		},
		"3": {
			{"i": w2, "x": 0, "y": 0, "w": 1, "h": 5},
			{"i": w1, "x": 1, "y": 0, "w": 2, "h": 3},
			{"i": w11, "x": 0, "y": 5, "w": 1, "h": 3},
			{"i": w10, "x": 1, "y": 3, "w": 2, "h": 8},
			{"i": w14, "x": 0, "y": 8, "w": 1, "h": 4},
			{"i": w3, "x": 0, "y": 12, "w": 1, "h": 5},
			{"i": w13, "x": 0, "y": 17, "w": 1, "h": 3},
			{"i": w7, "x": 1, "y": 11, "w": 1, "h": 5},
			{"i": w5, "x": 2, "y": 11, "w": 1, "h": 5},
			{"i": w4, "x": 1, "y": 16, "w": 1, "h": 6},
			{"i": w6, "x": 2, "y": 16, "w": 1, "h": 5},
			{"i": w12, "x": 0, "y": 20, "w": 1, "h": 3},
			{"i": w8, "x": 2, "y": 21, "w": 1, "h": 5},
			{"i": w9, "x": 0, "y": 23, "w": 2, "h": 6},
		},
		"2": {
			{"i": w2, "x": 0, "y": 0, "w": 1, "h": 5},
			{"i": w1, "x": 1, "y": 0, "w": 1, "h": 3},
			{"i": w11, "x": 0, "y": 5, "w": 1, "h": 3},
			{"i": w10, "x": 1, "y": 3, "w": 1, "h": 8},
			{"i": w14, "x": 0, "y": 8, "w": 1, "h": 4},
			{"i": w3, "x": 0, "y": 12, "w": 1, "h": 5},
			{"i": w7, "x": 1, "y": 11, "w": 1, "h": 5},
			{"i": w13, "x": 0, "y": 17, "w": 1, "h": 3},
			{"i": w6, "x": 1, "y": 16, "w": 1, "h": 5},
			{"i": w4, "x": 0, "y": 20, "w": 1, "h": 6},
			{"i": w12, "x": 1, "y": 21, "w": 1, "h": 3},
			{"i": w8, "x": 1, "y": 24, "w": 1, "h": 5},
			{"i": w5, "x": 0, "y": 26, "w": 1, "h": 5},
			{"i": w9, "x": 0, "y": 31, "w": 2, "h": 6},
		},
		"1": {
			{"i": w2, "x": 0, "y": 0, "w": 1, "h": 5},
			{"i": w11, "x": 0, "y": 5, "w": 1, "h": 3},
			{"i": w1, "x": 0, "y": 8, "w": 1, "h": 3},
			{"i": w14, "x": 0, "y": 11, "w": 1, "h": 4},
			{"i": w3, "x": 0, "y": 15, "w": 1, "h": 5},
			{"i": w13, "x": 0, "y": 20, "w": 1, "h": 3},
			{"i": w10, "x": 0, "y": 23, "w": 1, "h": 8},
			{"i": w7, "x": 0, "y": 31, "w": 1, "h": 5},
			{"i": w4, "x": 0, "y": 36, "w": 1, "h": 6},
			{"i": w5, "x": 0, "y": 42, "w": 1, "h": 5},
			{"i": w6, "x": 0, "y": 47, "w": 1, "h": 5},
			{"i": w8, "x": 0, "y": 52, "w": 1, "h": 5},
			{"i": w9, "x": 0, "y": 57, "w": 1, "h": 6},
			{"i": w12, "x": 0, "y": 63, "w": 1, "h": 3},
		},
	}
	items := []map[string]any{
		{"instance_id": w1, "widget_type": "homenavi.demo.overview", "enabled": true, "settings": map[string]any{"title": demoWelcomeWidgetDefaultTitle}},
		{"instance_id": w2, "widget_type": "homenavi.weather", "enabled": true, "settings": map[string]any{"city": "Budapest", "location_name": "Budapest, Hungary"}},
		{"instance_id": w3, "widget_type": "homenavi.device.multi", "enabled": true, "settings": map[string]any{"title": demoQuickLightsWidgetTitle, "device_ids": []string{mockdemo.SofaLampHDPDeviceID, mockdemo.TVBacklightHDPDeviceID, mockdemo.KitchenPendantHDPDeviceID, mockdemo.DeskLampHDPDeviceID}, "group_ids": []string{demoEveningLightsGroupID}}},
		{"instance_id": w4, "widget_type": "homenavi.group.controls", "enabled": true, "settings": map[string]any{"title": demoEveningLightsWidgetTitle, "group_id": demoEveningLightsGroupID, "controls": []string{"on", "brightness"}}},
		{"instance_id": w5, "widget_type": "homenavi.device", "enabled": true, "settings": map[string]any{"hdp_device_id": mockdemo.AirPurifierHDPDeviceID, "title": demoAirPurifierWidgetTitle, "controls": []string{"on", "fan_speed"}, "fields": []string{"air_quality"}}},
		{"instance_id": w6, "widget_type": "homenavi.device", "enabled": true, "settings": map[string]any{"hdp_device_id": mockdemo.BedroomBlindHDPDeviceID, "title": demoBedroomBlindWidgetTitle, "controls": []string{"position"}}},
		{"instance_id": w7, "widget_type": "homenavi.device", "enabled": true, "settings": map[string]any{"hdp_device_id": mockdemo.CoffeeMakerHDPDeviceID, "title": demoCoffeeMakerWidgetTitle, "controls": []string{"power"}, "fields": []string{"power_draw"}}},
		{"instance_id": w8, "widget_type": "homenavi.device", "enabled": true, "settings": map[string]any{"hdp_device_id": mockdemo.SofaLampHDPDeviceID, "title": demoSofaLampWidgetTitle, "controls": []string{"on", "brightness"}}},
		{"instance_id": w9, "widget_type": "homenavi.device.graph", "enabled": true, "settings": map[string]any{"hdp_device_id": mockdemo.AirPurifierHDPDeviceID, "metric_key": "air_quality", "range_preset": "1h", "title": demoAirQualityTrendWidgetTitle}},
		{"instance_id": w10, "widget_type": "homenavi.map", "enabled": true, "settings": map[string]any{}},
		{"instance_id": w11, "widget_type": "homenavi.automation.manual_trigger", "enabled": true, "settings": map[string]any{"workflow_id": demoGoodMorningWorkflowID, "title": demoGoodMorningWidgetTitle}},
		{"instance_id": w12, "widget_type": "homenavi.automation.manual_trigger", "enabled": true, "settings": map[string]any{"workflow_id": demoMovieNightWorkflowID, "title": demoMovieNightWidgetTitle}},
		{"instance_id": w13, "widget_type": "homenavi.automation.manual_trigger", "enabled": true, "settings": map[string]any{"workflow_id": demoLeaveHomeWorkflowID, "title": demoLeaveHomeWidgetTitle}},
		{"instance_id": w14, "widget_type": "integration.spotify.player", "enabled": true, "settings": map[string]any{}},
	}
	return DashboardDoc{LayoutsByCols: layoutsByCols, Items: items}
}

type legacyDashboardDoc struct {
	Layouts map[string][]map[string]any `json:"layouts"`
	Items   []map[string]any            `json:"items"`
}

func normalizeLegacyLayoutsByCols(layouts map[string][]map[string]any) map[string][]map[string]any {
	next := map[string][]map[string]any{
		"4": {},
		"3": {},
		"2": {},
		"1": {},
	}
	if len(layouts) == 0 {
		return next
	}

	copyLayout := func(items []map[string]any) []map[string]any {
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			clone := map[string]any{}
			for key, value := range item {
				clone[key] = value
			}
			out = append(out, clone)
		}
		return out
	}

	getMaxRight := func(items []map[string]any) int {
		maxRight := 0
		for _, item := range items {
			x, _ := toInt(item["x"])
			w, _ := toInt(item["w"])
			if x+w > maxRight {
				maxRight = x + w
			}
		}
		return maxRight
	}

	lg := copyLayout(layouts["lg"])
	md := copyLayout(layouts["md"])
	sm := copyLayout(layouts["sm"])
	xxs := copyLayout(layouts["xxs"])
	xl := copyLayout(layouts["xl"])
	xs := copyLayout(layouts["xs"])

	if len(xl) > 0 {
		next["4"] = xl
	} else if len(lg) > 0 {
		next["4"] = lg
	} else if len(md) > 0 {
		next["4"] = md
	}

	if len(md) > 0 && getMaxRight(md) <= 3 {
		next["3"] = md
	} else if len(lg) > 0 && getMaxRight(lg) <= 3 {
		next["3"] = lg
	}

	if len(sm) > 0 {
		next["2"] = sm
	}
	if len(xxs) > 0 {
		next["1"] = xxs
	} else if len(xs) > 0 {
		next["1"] = xs
	}

	return next
}

func toInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func cloneDashboardDoc(raw datatypes.JSON) (datatypes.JSON, error) {
	var defDoc DashboardDoc
	if err := json.Unmarshal(raw, &defDoc); err != nil {
		return nil, err
	}
	if len(defDoc.LayoutsByCols) == 0 {
		var legacy legacyDashboardDoc
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return nil, err
		}
		defDoc.LayoutsByCols = normalizeLegacyLayoutsByCols(legacy.Layouts)
		if len(defDoc.Items) == 0 {
			defDoc.Items = legacy.Items
		}
	}
	newLayouts := map[string][]map[string]any{}
	newItems := []map[string]any{}
	idMap := map[string]string{}
	for _, item := range defDoc.Items {
		oldID, _ := item["instance_id"].(string)
		widgetType, _ := item["widget_type"].(string)
		if strings.TrimSpace(widgetType) == "" {
			continue
		}
		newID := uuid.New().String()
		idMap[oldID] = newID
		settings := map[string]any{}
		if rawSettings, ok := item["settings"].(map[string]any); ok {
			settings = rawSettings
		}
		enabled := true
		if rawEnabled, ok := item["enabled"].(bool); ok {
			enabled = rawEnabled
		}
		newItems = append(newItems, map[string]any{"instance_id": newID, "widget_type": widgetType, "enabled": enabled, "settings": settings})
	}
	for bp, items := range defDoc.LayoutsByCols {
		next := make([]map[string]any, 0, len(items))
		for _, layoutItem := range items {
			oldID, _ := layoutItem["i"].(string)
			mapped := idMap[oldID]
			if mapped == "" {
				continue
			}
			copyItem := map[string]any{}
			for key, value := range layoutItem {
				copyItem[key] = value
			}
			copyItem["i"] = mapped
			next = append(next, copyItem)
		}
		newLayouts[bp] = next
	}
	cloned := DashboardDoc{LayoutsByCols: newLayouts, Items: newItems}
	buf, err := json.Marshal(cloned)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(buf), nil
}
