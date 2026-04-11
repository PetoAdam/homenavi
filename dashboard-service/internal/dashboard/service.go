package dashboard

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/datatypes"
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
	def, err := s.repo.GetDefaultDashboard(ctx)
	if err != nil {
		return nil, err
	}
	if def != nil {
		return def, nil
	}
	return s.repo.UpsertDefaultDashboard(ctx, "Home", defaultDashboardDoc())
}

func defaultDashboardDoc() DashboardDoc {
	w1 := uuid.New().String()
	w2 := uuid.New().String()
	w3 := uuid.New().String()
	w4 := uuid.New().String()
	base := []map[string]any{{"i": w1, "x": 0, "y": 0, "w": 1, "h": 8}, {"i": w2, "x": 1, "y": 0, "w": 1, "h": 8}, {"i": w3, "x": 2, "y": 0, "w": 1, "h": 8}, {"i": w4, "x": 0, "y": 8, "w": 3, "h": 10}}
	layouts := map[string][]map[string]any{}
	for _, bp := range []string{"lg", "md", "sm", "xs", "xxs"} {
		layouts[bp] = base
	}
	items := []map[string]any{{"instance_id": w1, "widget_type": "homenavi.weather", "enabled": true, "settings": map[string]any{}}, {"instance_id": w2, "widget_type": "homenavi.device", "enabled": true, "settings": map[string]any{}}, {"instance_id": w3, "widget_type": "homenavi.automation.manual_trigger", "enabled": true, "settings": map[string]any{}}, {"instance_id": w4, "widget_type": "homenavi.map", "enabled": true, "settings": map[string]any{}}}
	return DashboardDoc{Layouts: layouts, Items: items}
}

func cloneDashboardDoc(raw datatypes.JSON) (datatypes.JSON, error) {
	var defDoc DashboardDoc
	if err := json.Unmarshal(raw, &defDoc); err != nil {
		return nil, err
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
	for bp, items := range defDoc.Layouts {
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
	cloned := DashboardDoc{Layouts: newLayouts, Items: newItems}
	buf, err := json.Marshal(cloned)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(buf), nil
}
