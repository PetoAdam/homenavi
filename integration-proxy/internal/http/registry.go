package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/integration-proxy/internal/config"
)

func (s *Server) handleRegistry(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.proxies))
	for id := range s.proxies {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	sort.Strings(ids)

	cfgByID := map[string]config.IntegrationConfig{}
	if strings.TrimSpace(s.configPath) != "" {
		if cfg, err := config.Load(s.configPath); err == nil {
			for _, ic := range cfg.Integrations {
				trimmedID := strings.TrimSpace(ic.ID)
				if trimmedID == "" {
					continue
				}
				cfgByID[trimmedID] = ic
			}
		}
	}

	reg := Registry{GeneratedAt: time.Now().UTC(), Integrations: make([]RegistryIntegration, 0)}
	for _, id := range ids {
		s.mu.RLock()
		m, ok := s.manifests[id]
		errMsg := s.manifestErr[id]
		s.mu.RUnlock()
		if !ok {
			if errMsg != "" {
				s.logger.Printf("manifest unavailable id=%s err=%s", id, errMsg)
			}
			entry := cfgByID[id]
			s.mu.RLock()
			updateState := s.updates[id]
			s.mu.RUnlock()
			reg.Integrations = append(reg.Integrations, RegistryIntegration{
				ID:               id,
				DisplayName:      id,
				Route:            "/apps/" + id,
				DefaultUIPath:    "/ui/",
				InstalledVersion: firstNonEmpty(strings.TrimSpace(entry.Version), updateState.InstalledVersion),
				LatestVersion:    updateState.LatestVersion,
				UpdateAvailable:  updateState.UpdateAvailable,
				AutoUpdate:       entry.AutoUpdate || updateState.AutoUpdate,
				UpdateCheckedAt:  updateState.CheckedAt,
				UpdateError:      updateState.Error,
				UpdateInProgress: updateState.InProgress,
				Widgets:          []RegistryWidget{},
			})
			continue
		}

		defPath := strings.TrimSpace(m.UI.Sidebar.Path)
		if defPath == "" {
			defPath = "/ui/"
		}
		setupPath := ""
		if m.UI.Setup.Enabled {
			setupPath = strings.TrimSpace(m.UI.Setup.Path)
			if setupPath == "" {
				setupPath = "/ui/"
			}
		}

		icon := strings.TrimSpace(m.UI.Sidebar.Icon)
		iconLower := strings.ToLower(icon)
		if strings.HasPrefix(iconLower, "http://") || strings.HasPrefix(iconLower, "https://") {
			icon = ""
		}
		if strings.HasPrefix(icon, "/") && !strings.HasPrefix(icon, "/integrations/") {
			icon = "/integrations/" + id + icon
		}
		regInt := RegistryIntegration{
			ID:               id,
			DisplayName:      firstNonEmpty(strings.TrimSpace(m.UI.Sidebar.Label), strings.TrimSpace(m.Name), id),
			Description:      strings.TrimSpace(m.Description),
			Icon:             icon,
			Route:            "/apps/" + id,
			DefaultUIPath:    defPath,
			SetupUIPath:      setupPath,
			InstalledVersion: strings.TrimSpace(m.Version),
			Secrets:          append([]SecretSpec{}, m.Secrets...),
		}

		if entry, ok := cfgByID[id]; ok {
			regInt.AutoUpdate = entry.AutoUpdate
			if strings.TrimSpace(entry.Version) != "" {
				regInt.InstalledVersion = strings.TrimSpace(entry.Version)
			}
		}
		s.mu.RLock()
		updateState, hasUpdateState := s.updates[id]
		s.mu.RUnlock()
		if hasUpdateState {
			if updateState.InstalledVersion != "" {
				regInt.InstalledVersion = updateState.InstalledVersion
			}
			regInt.LatestVersion = updateState.LatestVersion
			regInt.UpdateAvailable = updateState.UpdateAvailable
			regInt.UpdateCheckedAt = updateState.CheckedAt
			regInt.UpdateError = updateState.Error
			regInt.UpdateInProgress = updateState.InProgress
			if updateState.AutoUpdate {
				regInt.AutoUpdate = true
			}
		}

		if m.DeviceExtension.Enabled {
			regInt.DeviceExtension = &RegistryDeviceExtension{
				ProviderID:          strings.TrimSpace(m.DeviceExtension.ProviderID),
				Protocol:            strings.TrimSpace(m.DeviceExtension.Protocol),
				DiscoveryMode:       strings.TrimSpace(m.DeviceExtension.DiscoveryMode),
				SupportsPairing:     m.DeviceExtension.SupportsPairing,
				CapabilitySchemaURL: strings.TrimSpace(m.DeviceExtension.CapabilitySchemaURL),
			}
		}

		if m.AutomationExtension.Enabled {
			regInt.AutomationExtension = &RegistryAutomationExtension{
				Scope:           strings.TrimSpace(m.AutomationExtension.Scope),
				StepsCatalogURL: strings.TrimSpace(m.AutomationExtension.StepsCatalogURL),
				ExecuteEndpoint: strings.TrimSpace(m.AutomationExtension.ExecuteEndpoint),
			}
		}

		for _, wdg := range m.Widgets {
			wid := strings.TrimSpace(wdg.Type)
			if wid == "" || strings.TrimSpace(wdg.Entry.URL) == "" {
				continue
			}
			entryURL := strings.TrimSpace(wdg.Entry.URL)
			if strings.HasPrefix(entryURL, "/") && !strings.HasPrefix(entryURL, "/integrations/") {
				entryURL = "/integrations/" + id + entryURL
			}
			wdgIcon := strings.TrimSpace(wdg.Icon)
			wdgIconLower := strings.ToLower(wdgIcon)
			if strings.HasPrefix(wdgIconLower, "http://") || strings.HasPrefix(wdgIconLower, "https://") {
				wdgIcon = ""
			}
			if strings.HasPrefix(wdgIcon, "/") && !strings.HasPrefix(wdgIcon, "/integrations/") {
				wdgIcon = "/integrations/" + id + wdgIcon
			}
			if wdgIcon == "" {
				wdgIcon = icon
			}
			regInt.Widgets = append(regInt.Widgets, RegistryWidget{
				ID:          wid,
				DisplayName: firstNonEmpty(strings.TrimSpace(wdg.DisplayName), wid),
				Description: strings.TrimSpace(wdg.Description),
				Icon:        wdgIcon,
				DefaultSize: strings.TrimSpace(wdg.DefaultSize),
				EntryURL:    entryURL,
				Verified:    false,
				Source:      "integration",
			})
		}
		reg.Integrations = append(reg.Integrations, regInt)
	}

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if q != "" {
		filtered := make([]RegistryIntegration, 0, len(reg.Integrations))
		for _, it := range reg.Integrations {
			if strings.Contains(strings.ToLower(it.DisplayName), q) ||
				strings.Contains(strings.ToLower(it.ID), q) ||
				strings.Contains(strings.ToLower(it.Route), q) {
				filtered = append(filtered, it)
			}
		}
		reg.Integrations = filtered
	}

	page := clampInt(queryInt(r, "page", 1), 1, 1000000)
	pageSize := clampInt(queryInt(r, "page_size", 50), 1, 200)
	reg.Total = len(reg.Integrations)
	reg.Page = page
	reg.PageSize = pageSize
	if reg.Total == 0 {
		reg.TotalPages = 0
	} else {
		reg.TotalPages = (reg.Total + pageSize - 1) / pageSize
	}
	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	end := start + pageSize
	if start >= reg.Total {
		reg.Integrations = []RegistryIntegration{}
	} else {
		if end > reg.Total {
			end = reg.Total
		}
		reg.Integrations = reg.Integrations[start:end]
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(reg)
}

type integrationSnapshot struct {
	id       string
	upstream *url.URL
	manifest Manifest
}

func (s *Server) collectIntegrationSnapshots() []integrationSnapshot {
	s.mu.RLock()
	ids := make([]string, 0, len(s.manifests))
	for id := range s.manifests {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	sort.Strings(ids)

	out := make([]integrationSnapshot, 0, len(ids))
	for _, id := range ids {
		s.mu.RLock()
		up := s.upstreams[id]
		m, ok := s.manifests[id]
		s.mu.RUnlock()
		if !ok || up == nil {
			continue
		}
		upCopy := *up
		out = append(out, integrationSnapshot{id: id, upstream: &upCopy, manifest: m})
	}
	return out
}

func (s *Server) handleAutomationStepsCatalog(w http.ResponseWriter, r *http.Request) {
	steps := AutomationStepsCatalog{GeneratedAt: time.Now().UTC()}

	for _, it := range s.collectIntegrationSnapshots() {
		a := it.manifest.AutomationExtension
		if !a.Enabled {
			continue
		}
		scope := strings.ToLower(strings.TrimSpace(a.Scope))
		if scope != "integration_only" {
			continue
		}
		catalogPath := strings.TrimSpace(a.StepsCatalogURL)
		if catalogPath == "" {
			continue
		}
		if !strings.HasPrefix(catalogPath, "/") {
			catalogPath = "/" + catalogPath
		}

		catalogURL := *it.upstream
		catalogURL.Path = path.Join(catalogURL.Path, catalogPath)

		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, catalogURL.String(), nil)
		resp, err := s.client.Do(req)
		if err != nil {
			s.logger.Printf("automation steps fetch failed id=%s err=%v", it.id, err)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			s.logger.Printf("automation steps fetch failed id=%s status=%d", it.id, resp.StatusCode)
			_ = resp.Body.Close()
			continue
		}

		var payload struct {
			Actions    []map[string]any `json:"actions"`
			Triggers   []map[string]any `json:"triggers"`
			Conditions []map[string]any `json:"conditions"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload)
		_ = resp.Body.Close()
		if decodeErr != nil {
			s.logger.Printf("automation steps parse failed id=%s err=%v", it.id, decodeErr)
			continue
		}

		for _, action := range payload.Actions {
			steps.Actions = append(steps.Actions, AutomationStepRecord{IntegrationID: it.id, Scope: scope, Step: action})
		}
		for _, trigger := range payload.Triggers {
			steps.Triggers = append(steps.Triggers, AutomationStepRecord{IntegrationID: it.id, Scope: scope, Step: trigger})
		}
		for _, condition := range payload.Conditions {
			steps.Conditions = append(steps.Conditions, AutomationStepRecord{IntegrationID: it.id, Scope: scope, Step: condition})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(steps)
}

func queryInt(r *http.Request, key string, def int) int {
	if r == nil {
		return def
	}
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return val
}

func clampInt(val, minVal, maxVal int) int {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}
