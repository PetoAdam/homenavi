package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Dashboard struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey" json:"dashboard_id"`
	Scope         string         `gorm:"type:varchar(16);index" json:"scope"`
	OwnerUserID   *uuid.UUID     `gorm:"type:uuid;index" json:"owner_user_id,omitempty"`
	Title         string         `gorm:"type:varchar(128)" json:"title"`
	LayoutEngine  string         `gorm:"type:varchar(32)" json:"layout_engine"`
	LayoutVersion int            `gorm:"not null;default:1" json:"layout_version"`
	Doc           datatypes.JSON `gorm:"type:jsonb" json:"doc"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// Widget catalog entry served from dashboard-service.
// In v1 this is static for core widgets.
// Later it will merge integration-registry discovered widgets.
//
// NOTE: Keep fields minimal so the frontend can render an “Add widget” list.
// The settings schema is intentionally a loose JSON blob (schema-ish).
//
// This is not persisted (yet).
//
// `verified` is used for future marketplace integrations.
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

// WidgetEntry is a structured widget entry point.
// For iframe widgets, URL should be an absolute path (e.g. /integrations/<id>/widgets/foo/).
type WidgetEntry struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}
