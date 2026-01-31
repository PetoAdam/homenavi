package models

import "time"

type Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Publisher     string `json:"publisher,omitempty"`
	Description   string `json:"description,omitempty"`
	Homepage      string `json:"homepage,omitempty"`
	Verified      bool   `json:"verified"`

	UI struct {
		Sidebar struct {
			Enabled bool   `json:"enabled"`
			Path    string `json:"path"`
			Label   string `json:"label"`
			Icon    string `json:"icon"`
		} `json:"sidebar"`
	} `json:"ui"`

	Widgets []struct {
		Type        string `json:"type"`
		DisplayName string `json:"display_name"`
		Description string `json:"description,omitempty"`
		Entry       struct {
			Kind string `json:"kind"`
			URL  string `json:"url"`
		} `json:"entry"`
	} `json:"widgets"`
}

type Registry struct {
	GeneratedAt  time.Time             `json:"generated_at"`
	Integrations []RegistryIntegration `json:"integrations"`
}

type RegistryIntegration struct {
	ID            string           `json:"id"`
	DisplayName   string           `json:"display_name"`
	Icon          string           `json:"icon,omitempty"`
	Route         string           `json:"route"`
	DefaultUIPath string           `json:"default_ui_path"`
	Widgets       []RegistryWidget `json:"widgets"`
}

type RegistryWidget struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	DefaultSize string `json:"default_size_hint,omitempty"`
	EntryURL    string `json:"entry_url,omitempty"`
	Verified    bool   `json:"verified"`
	Source      string `json:"source"`
}
