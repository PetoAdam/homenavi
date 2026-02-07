package models

import (
	"encoding/json"
	"strings"
	"time"
)

type Manifest struct {
	SchemaVersion int          `json:"schema_version"`
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Version       string       `json:"version"`
	Publisher     string       `json:"publisher,omitempty"`
	Description   string       `json:"description,omitempty"`
	Homepage      string       `json:"homepage,omitempty"`
	Verified      bool         `json:"verified"`
	Secrets       []SecretSpec `json:"secrets,omitempty"`

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
	Page         int                   `json:"page,omitempty"`
	PageSize     int                   `json:"page_size,omitempty"`
	Total        int                   `json:"total,omitempty"`
	TotalPages   int                   `json:"total_pages,omitempty"`
}

type RegistryIntegration struct {
	ID            string           `json:"id"`
	DisplayName   string           `json:"display_name"`
	Description   string           `json:"description,omitempty"`
	Icon          string           `json:"icon,omitempty"`
	Route         string           `json:"route"`
	DefaultUIPath string           `json:"default_ui_path"`
	Widgets       []RegistryWidget `json:"widgets"`
	Secrets       []SecretSpec     `json:"secrets,omitempty"`
}

type SecretSpec struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
}

func (s *SecretSpec) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err == nil {
		s.Key = strings.TrimSpace(raw)
		return nil
	}
	var obj struct {
		Key         string `json:"key"`
		Name        string `json:"name"`
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	key := strings.TrimSpace(obj.Key)
	if key == "" {
		key = strings.TrimSpace(obj.Name)
	}
	if key == "" {
		key = strings.TrimSpace(obj.ID)
	}
	s.Key = key
	s.Description = strings.TrimSpace(obj.Description)
	return nil
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

type Marketplace struct {
	GeneratedAt  time.Time               `json:"generated_at"`
	Integrations []MarketplaceIntegration `json:"integrations"`
}

type MarketplaceIntegration struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Version     string `json:"version,omitempty"`
	Publisher   string `json:"publisher,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	Installed   bool   `json:"installed"`
}
