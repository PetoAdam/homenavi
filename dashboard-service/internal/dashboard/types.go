package dashboard

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Dashboard is the persisted dashboard aggregate.
type Dashboard struct {
	ID            uuid.UUID      `json:"dashboard_id"`
	Scope         string         `json:"scope"`
	OwnerUserID   *uuid.UUID     `json:"owner_user_id,omitempty"`
	Title         string         `json:"title"`
	LayoutEngine  string         `json:"layout_engine"`
	LayoutVersion int            `json:"layout_version"`
	Doc           datatypes.JSON `json:"doc"`
	CreatedAt     time.Time      `json:"created_at,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at,omitempty"`
}

type WidgetType struct {
	ID             string         `json:"id"`
	DisplayName    string         `json:"display_name"`
	Description    string         `json:"description"`
	Icon           string         `json:"icon,omitempty"`
	DefaultSize    string         `json:"default_size_hint,omitempty"`
	EntryURL       string         `json:"entry_url,omitempty"`
	Entry          *WidgetEntry   `json:"entry,omitempty"`
	SettingsSchema map[string]any `json:"settings_schema,omitempty"`
	Verified       bool           `json:"verified"`
	Source         string         `json:"source"`
}

type WidgetEntry struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// DashboardDoc is the JSON document stored in Dashboard.Doc.
type DashboardDoc struct {
	Layouts map[string][]map[string]any `json:"layouts"`
	Items   []map[string]any            `json:"items"`
}

type WeatherResponse struct {
	City    string `json:"city"`
	Current any    `json:"current"`
	Daily   any    `json:"daily"`
	Weekly  any    `json:"weekly"`
}

type AuthContext struct {
	Authorization string
	AuthToken     string
}
