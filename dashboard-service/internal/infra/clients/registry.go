package clients

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/dashboard-service/internal/dashboard"
)

type RegistryClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewRegistryClient(baseURL string, httpClient *http.Client) *RegistryClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}
	return &RegistryClient{baseURL: strings.TrimSpace(baseURL), httpClient: httpClient}
}

func (c *RegistryClient) Widgets(ctx context.Context, auth dashboard.AuthContext) ([]dashboard.WidgetType, error) {
	base := strings.TrimRight(strings.TrimSpace(c.baseURL), "/")
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
	if auth.Authorization != "" {
		req.Header.Set("Authorization", auth.Authorization)
	}
	if auth.AuthToken != "" {
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: auth.AuthToken})
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("integration registry fetch failed")
	}

	var reg struct {
		Integrations []struct {
			Icon    string `json:"icon,omitempty"`
			Widgets []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
				Description string `json:"description,omitempty"`
				Icon        string `json:"icon,omitempty"`
				DefaultSize string `json:"default_size_hint,omitempty"`
				EntryURL    string `json:"entry_url,omitempty"`
				Verified    bool   `json:"verified"`
			} `json:"widgets"`
		} `json:"integrations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, err
	}

	out := make([]dashboard.WidgetType, 0, 16)
	for _, integration := range reg.Integrations {
		fallbackIcon := strings.TrimSpace(integration.Icon)
		for _, widget := range integration.Widgets {
			icon := strings.TrimSpace(widget.Icon)
			if icon == "" {
				icon = fallbackIcon
			}
			entryURL := strings.TrimSpace(widget.EntryURL)
			var entry *dashboard.WidgetEntry
			if entryURL != "" {
				entry = &dashboard.WidgetEntry{Kind: "iframe", URL: entryURL}
			}
			out = append(out, dashboard.WidgetType{ID: widget.ID, DisplayName: widget.DisplayName, Description: widget.Description, Icon: icon, DefaultSize: widget.DefaultSize, EntryURL: entryURL, Entry: entry, Verified: widget.Verified, Source: "integration"})
		}
	}
	return out, nil
}

var _ dashboard.WidgetCatalogSource = (*RegistryClient)(nil)
