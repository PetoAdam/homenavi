package http

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
		Setup struct {
			Enabled bool   `json:"enabled"`
			Path    string `json:"path"`
			Label   string `json:"label"`
			Icon    string `json:"icon"`
		} `json:"setup"`
	} `json:"ui"`

	Widgets []struct {
		Type        string `json:"type"`
		DisplayName string `json:"display_name"`
		Description string `json:"description,omitempty"`
		Icon        string `json:"icon,omitempty"`
		DefaultSize string `json:"default_size_hint,omitempty"`
		Entry       struct {
			Kind string `json:"kind"`
			URL  string `json:"url"`
		} `json:"entry"`
	} `json:"widgets"`

	DeviceExtension     DeviceExtensionManifest     `json:"device_extension,omitempty"`
	AutomationExtension AutomationExtensionManifest `json:"automation_extension,omitempty"`
}

type DeviceExtensionManifest struct {
	Enabled             bool   `json:"enabled"`
	ProviderID          string `json:"provider_id,omitempty"`
	Protocol            string `json:"protocol,omitempty"`
	DiscoveryMode       string `json:"discovery_mode,omitempty"`
	SupportsPairing     bool   `json:"supports_pairing"`
	CapabilitySchemaURL string `json:"capability_schema_url,omitempty"`
}

type AutomationExtensionManifest struct {
	Enabled         bool   `json:"enabled"`
	Scope           string `json:"scope,omitempty"`
	StepsCatalogURL string `json:"steps_catalog_url,omitempty"`
	ExecuteEndpoint string `json:"execute_endpoint,omitempty"`
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
	ID                  string                       `json:"id"`
	DisplayName         string                       `json:"display_name"`
	Description         string                       `json:"description,omitempty"`
	Icon                string                       `json:"icon,omitempty"`
	Route               string                       `json:"route"`
	DefaultUIPath       string                       `json:"default_ui_path"`
	SetupUIPath         string                       `json:"setup_ui_path,omitempty"`
	InstalledVersion    string                       `json:"installed_version,omitempty"`
	LatestVersion       string                       `json:"latest_version,omitempty"`
	UpdateAvailable     bool                         `json:"update_available,omitempty"`
	AutoUpdate          bool                         `json:"auto_update,omitempty"`
	UpdateCheckedAt     *time.Time                   `json:"update_checked_at,omitempty"`
	UpdateError         string                       `json:"update_error,omitempty"`
	UpdateInProgress    bool                         `json:"update_in_progress,omitempty"`
	Widgets             []RegistryWidget             `json:"widgets"`
	Secrets             []SecretSpec                 `json:"secrets,omitempty"`
	DeviceExtension     *RegistryDeviceExtension     `json:"device_extension,omitempty"`
	AutomationExtension *RegistryAutomationExtension `json:"automation_extension,omitempty"`
}

type RegistryDeviceExtension struct {
	ProviderID          string `json:"provider_id,omitempty"`
	Protocol            string `json:"protocol,omitempty"`
	DiscoveryMode       string `json:"discovery_mode,omitempty"`
	SupportsPairing     bool   `json:"supports_pairing"`
	CapabilitySchemaURL string `json:"capability_schema_url,omitempty"`
}

type RegistryAutomationExtension struct {
	Scope           string `json:"scope,omitempty"`
	StepsCatalogURL string `json:"steps_catalog_url,omitempty"`
	ExecuteEndpoint string `json:"execute_endpoint,omitempty"`
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
	GeneratedAt  time.Time                `json:"generated_at"`
	Integrations []MarketplaceIntegration `json:"integrations"`
}

type AutomationStepsCatalog struct {
	GeneratedAt time.Time              `json:"generated_at"`
	Actions     []AutomationStepRecord `json:"actions,omitempty"`
	Triggers    []AutomationStepRecord `json:"triggers,omitempty"`
	Conditions  []AutomationStepRecord `json:"conditions,omitempty"`
}

type AutomationStepRecord struct {
	IntegrationID string         `json:"integration_id"`
	Scope         string         `json:"scope,omitempty"`
	Step          map[string]any `json:"step"`
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
