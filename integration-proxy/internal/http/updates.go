package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/PetoAdam/homenavi/integration-proxy/internal/config"
)

func (s *Server) initUpdateState(ic config.IntegrationConfig) {
	id := strings.TrimSpace(ic.ID)
	if id == "" {
		return
	}
	installed := strings.TrimSpace(ic.Version)
	if installed == "" {
		s.mu.RLock()
		if m, ok := s.manifests[id]; ok {
			installed = strings.TrimSpace(m.Version)
		}
		s.mu.RUnlock()
	}
	s.mu.Lock()
	state := s.updates[id]
	state.ID = id
	state.InstalledVersion = installed
	state.AutoUpdate = ic.AutoUpdate
	state.InProgress = s.updating[id]
	s.updates[id] = state
	s.mu.Unlock()
}

func (s *Server) copyUpdateStates() []integrationUpdateStatus {
	s.mu.RLock()
	ids := make([]string, 0, len(s.updates))
	for id := range s.updates {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	sort.Strings(ids)
	out := make([]integrationUpdateStatus, 0, len(ids))
	for _, id := range ids {
		s.mu.RLock()
		state := s.updates[id]
		s.mu.RUnlock()
		out = append(out, state)
	}
	return out
}

func (s *Server) setUpdateState(id string, apply func(*integrationUpdateStatus)) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	s.mu.Lock()
	state := s.updates[id]
	state.ID = id
	state.InProgress = s.updating[id]
	apply(&state)
	s.updates[id] = state
	s.mu.Unlock()
}

func (s *Server) startUpdate(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	s.mu.Lock()
	if s.updating[id] {
		s.mu.Unlock()
		return false
	}
	s.updating[id] = true
	state := s.updates[id]
	state.ID = id
	state.InProgress = true
	state.Error = ""
	s.updates[id] = state
	s.mu.Unlock()
	return true
}

func (s *Server) finishUpdate(id string, errMsg string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	delete(s.updating, id)
	state := s.updates[id]
	state.ID = id
	state.InProgress = false
	state.CheckedAt = &now
	state.Error = strings.TrimSpace(errMsg)
	s.updates[id] = state
	s.mu.Unlock()
}

func (s *Server) StartUpdateLoop(ctx context.Context, every time.Duration) {
	if every <= 0 {
		return
	}
	s.checkForUpdates(ctx, true)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.checkForUpdates(ctx, true)
		}
	}
}

type marketplaceIntegration struct {
	ID                  string                     `json:"id"`
	Version             string                     `json:"version"`
	ComposeFile         string                     `json:"compose_file"`
	DeploymentArtifacts marketplaceDeployArtifacts `json:"deployment_artifacts"`
}

type marketplaceDeployArtifacts struct {
	Compose struct {
		File string `json:"file"`
	} `json:"compose"`
	Helm struct {
		ChartRef string `json:"chart_ref"`
		Version  string `json:"version"`
	} `json:"helm"`
	K8sGenerated struct {
		Kind     string `json:"kind"`
		ChartRef string `json:"chart_ref"`
		Version  string `json:"version"`
	} `json:"k8s_generated"`
}

func (m marketplaceIntegration) composeArtifactFile() string {
	if file := strings.TrimSpace(m.DeploymentArtifacts.Compose.File); file != "" {
		return file
	}
	return strings.TrimSpace(m.ComposeFile)
}

func (m marketplaceIntegration) preferredHelmArtifact() (chartRef, version string) {
	chartRef = strings.TrimSpace(m.DeploymentArtifacts.Helm.ChartRef)
	version = strings.TrimSpace(m.DeploymentArtifacts.Helm.Version)
	if chartRef != "" {
		if version == "" {
			version = strings.TrimSpace(m.Version)
		}
		return chartRef, version
	}
	chartRef = strings.TrimSpace(m.DeploymentArtifacts.K8sGenerated.ChartRef)
	version = strings.TrimSpace(m.DeploymentArtifacts.K8sGenerated.Version)
	if chartRef == "" {
		return "", ""
	}
	if version == "" {
		version = strings.TrimSpace(m.Version)
	}
	return chartRef, version
}

func (s *Server) targetIntegrationForUpdate(ctx context.Context, ic config.IntegrationConfig) (*marketplaceIntegration, error) {
	id := strings.TrimSpace(ic.ID)
	if id == "" {
		return nil, fmt.Errorf("missing integration id")
	}
	marketItem, err := s.fetchMarketplaceIntegration(ctx, id)
	devLatest := strings.TrimSpace(ic.DevLatestVersion)
	devCompose := strings.TrimSpace(ic.DevComposeFile)
	devHelmChart := strings.TrimSpace(ic.DevHelmChartRef)
	devHelmVersion := strings.TrimSpace(ic.DevHelmVersion)
	if err != nil {
		if devLatest == "" && devCompose == "" && devHelmChart == "" {
			return nil, err
		}
		marketItem = &marketplaceIntegration{ID: id}
	}
	if marketItem == nil {
		marketItem = &marketplaceIntegration{ID: id}
	}
	marketItem.ID = id
	if devLatest != "" {
		marketItem.Version = devLatest
	}
	if devCompose != "" {
		marketItem.DeploymentArtifacts.Compose.File = devCompose
		marketItem.ComposeFile = devCompose
	}
	if devHelmChart != "" {
		marketItem.DeploymentArtifacts.Helm.ChartRef = devHelmChart
	}
	if devHelmVersion != "" {
		marketItem.DeploymentArtifacts.Helm.Version = devHelmVersion
	}
	return marketItem, nil
}

func (s *Server) fetchMarketplaceIntegration(ctx context.Context, id string) (*marketplaceIntegration, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("missing integration id")
	}
	base := s.marketplaceAPIBase()
	urlStr := fmt.Sprintf("%s/api/integrations/%s", base, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var item marketplaceIntegration
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&item); err != nil {
		return nil, err
	}
	item.ID = id
	return &item, nil
}

func normalizeSemver(version string) string {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return ""
	}
	return "v" + v
}

func isVersionNewer(installedVersion, latestVersion string) bool {
	installed := normalizeSemver(installedVersion)
	latest := normalizeSemver(latestVersion)
	if latest == "" || !semver.IsValid(latest) {
		return false
	}
	if installed == "" || !semver.IsValid(installed) {
		return true
	}
	return semver.Compare(latest, installed) > 0
}

func (s *Server) effectiveInstalledVersion(id string, cfgVersion string) string {
	ver := strings.TrimSpace(cfgVersion)
	if ver != "" {
		return ver
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.manifests[id]
	if !ok {
		return ""
	}
	return strings.TrimSpace(m.Version)
}

func (s *Server) loadIntegrationConfig(id string) (config.Config, int, error) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return config.Config{}, -1, err
	}
	for i, ic := range cfg.Integrations {
		if strings.TrimSpace(ic.ID) == id {
			return cfg, i, nil
		}
	}
	return cfg, -1, fmt.Errorf("integration not installed")
}

func (s *Server) updateInstalledIntegration(ctx context.Context, id string, target *marketplaceIntegration) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("missing id")
	}
	if target == nil {
		return fmt.Errorf("missing target integration")
	}
	if !s.startUpdate(id) {
		return fmt.Errorf("update already in progress")
	}
	return s.runInstalledIntegrationUpdate(ctx, id, target)
}

func (s *Server) runInstalledIntegrationUpdate(ctx context.Context, id string, target *marketplaceIntegration) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("missing id")
	}
	if target == nil {
		return fmt.Errorf("missing target integration")
	}
	mode, modeErr := s.ensureMutableRuntime()
	if modeErr != nil {
		return modeErr
	}
	op := newIntegrationOperation(s, "update", id)
	op.set("checking", 15, "Checking update")
	defer func() {
		if r := recover(); r != nil {
			op.fail(fmt.Errorf("panic: %v", r))
			s.finishUpdate(id, fmt.Sprintf("panic: %v", r))
			panic(r)
		}
	}()

	cfg, index, err := s.loadIntegrationConfig(id)
	if err != nil {
		op.fail(err)
		s.finishUpdate(id, err.Error())
		return err
	}
	entry := cfg.Integrations[index]
	installedVersion := s.effectiveInstalledVersion(id, entry.Version)
	latestVersion := strings.TrimSpace(target.Version)
	s.logger.Printf("integration update id=%s installed=%q latest=%q", id, installedVersion, latestVersion)
	if !isVersionNewer(installedVersion, latestVersion) {
		s.setUpdateState(id, func(state *integrationUpdateStatus) {
			state.InstalledVersion = installedVersion
			state.LatestVersion = latestVersion
			state.UpdateAvailable = false
			state.AutoUpdate = entry.AutoUpdate
		})
		op.done("Already up to date")
		s.finishUpdate(id, "")
		return nil
	}

	op.set("preparing", 30, "Preparing update")
	if mode == runtimeCompose {
		composeInput := firstNonEmpty(target.composeArtifactFile(), strings.TrimSpace(entry.ComposeFile), s.defaultComposeFile(id))
		composeFile, err := s.resolveComposeFileForOperation(id, composeInput, strings.TrimSpace(target.Version))
		if err != nil {
			op.fail(err)
			s.finishUpdate(id, err.Error())
			return err
		}
		if strings.TrimSpace(composeFile) != "" {
			op.set("updating", 45, "Updating integration")
			if err := s.composeInstall(ctx, composeFile, id); err != nil {
				op.fail(fmt.Errorf("update failed: %w", err))
				s.finishUpdate(id, err.Error())
				return err
			}
			cfg.Integrations[index].ComposeFile = composeFile
		}
	}
	if mode == runtimeHelm {
		helmSpec, specErr := s.helmSpecFor(id, entry, target)
		if specErr != nil {
			op.fail(specErr)
			s.finishUpdate(id, specErr.Error())
			return specErr
		}
		op.set("updating", 45, "Updating integration")
		if err := s.helmInstallOrUpgrade(ctx, helmSpec, id); err != nil {
			op.fail(fmt.Errorf("update failed: %w", err))
			s.finishUpdate(id, err.Error())
			return err
		}
		cfg.Integrations[index].HelmReleaseName = helmSpec.ReleaseName
		cfg.Integrations[index].HelmNamespace = helmSpec.Namespace
		cfg.Integrations[index].HelmChartRef = helmSpec.ChartRef
		cfg.Integrations[index].HelmChartVersion = helmSpec.ChartVersion
		cfg.Integrations[index].HelmValuesFile = helmSpec.ValuesFile
		cfg.Integrations[index].Upstream = s.defaultHelmUpstream(helmSpec)
	}

	op.set("writing-config", 85, "Saving installed version")
	cfg.Integrations[index].Version = latestVersion
	if err := config.Save(s.configPath, cfg); err != nil {
		op.fail(err)
		s.finishUpdate(id, err.Error())
		return err
	}
	s.refreshManifestFromUpstream(ctx, id)
	updatedVersion := s.effectiveInstalledVersion(id, latestVersion)
	op.done("Updated")
	s.setUpdateState(id, func(state *integrationUpdateStatus) {
		state.InstalledVersion = updatedVersion
		state.LatestVersion = latestVersion
		state.UpdateAvailable = false
		state.AutoUpdate = cfg.Integrations[index].AutoUpdate
		state.Error = ""
	})
	s.finishUpdate(id, "")
	return nil
}

func (s *Server) checkForUpdates(ctx context.Context, autoApply bool) {
	mode, modeErr := s.runtimeMode()
	if modeErr != nil {
		return
	}
	if mode == runtimeGitOps {
		autoApply = false
	}
	if strings.TrimSpace(s.configPath) == "" {
		return
	}
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return
	}
	for _, ic := range cfg.Integrations {
		id := strings.TrimSpace(ic.ID)
		if id == "" {
			continue
		}
		installed := s.effectiveInstalledVersion(id, ic.Version)
		marketItem, fetchErr := s.targetIntegrationForUpdate(ctx, ic)
		now := time.Now().UTC()
		if fetchErr != nil {
			s.logger.Printf("integration update-check id=%s failed err=%v", id, fetchErr)
			s.setUpdateState(id, func(state *integrationUpdateStatus) {
				state.InstalledVersion = installed
				state.AutoUpdate = ic.AutoUpdate
				state.CheckedAt = &now
				state.Error = fetchErr.Error()
			})
			continue
		}
		latest := strings.TrimSpace(marketItem.Version)
		available := isVersionNewer(installed, latest)
		s.logger.Printf("integration update-check id=%s installed=%q latest=%q available=%t auto_update=%t", id, installed, latest, available, ic.AutoUpdate)
		s.setUpdateState(id, func(state *integrationUpdateStatus) {
			state.InstalledVersion = installed
			state.LatestVersion = latest
			state.UpdateAvailable = available
			state.AutoUpdate = ic.AutoUpdate
			state.CheckedAt = &now
			state.Error = ""
		})
		if autoApply && ic.AutoUpdate && available {
			s.logger.Printf("integration auto-update queued id=%s", id)
			if err := s.updateInstalledIntegration(ctx, id, marketItem); err != nil {
				s.setUpdateState(id, func(state *integrationUpdateStatus) {
					state.Error = err.Error()
				})
				s.logger.Printf("integration auto-update failed id=%s err=%v", id, err)
			}
		}
	}
}

func (s *Server) handleUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("refresh")), "true") {
		s.checkForUpdates(r.Context(), false)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"management_mode": s.managementModeLabel(), "updates": s.copyUpdateStates()})
}

func (s *Server) handleUpdateIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json", "code": http.StatusBadRequest})
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing id", "code": http.StatusBadRequest})
		return
	}
	op := newIntegrationOperation(s, "update", id)
	op.set("queued", 10, "Queued")
	cfg, index, err := s.loadIntegrationConfig(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "integration not installed", "code": http.StatusNotFound})
		op.fail(err)
		return
	}
	item, err := s.targetIntegrationForUpdate(r.Context(), cfg.Integrations[index])
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to fetch marketplace integration", "code": http.StatusBadGateway, "detail": err.Error()})
		op.fail(err)
		return
	}
	s.logger.Printf("integration update requested id=%s target_version=%q", id, strings.TrimSpace(item.Version))
	if err := s.updateInstalledIntegration(r.Context(), id, item); err != nil {
		statusCode := http.StatusInternalServerError
		errMsg := strings.TrimSpace(err.Error())
		if strings.Contains(strings.ToLower(errMsg), "already in progress") {
			statusCode = http.StatusConflict
		}
		if strings.Contains(strings.ToLower(errMsg), "gitops mode") {
			statusCode = http.StatusConflict
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to update integration", "code": statusCode, "detail": errMsg})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "updated", "id": id})
}

func (s *Server) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	var req struct {
		ID         string `json:"id"`
		AutoUpdate bool   `json:"auto_update"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json", "code": http.StatusBadRequest})
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing id", "code": http.StatusBadRequest})
		return
	}
	cfg, index, err := s.loadIntegrationConfig(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "integration not installed", "code": http.StatusNotFound})
		return
	}
	cfg.Integrations[index].AutoUpdate = req.AutoUpdate
	if err := config.Save(s.configPath, cfg); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to save update policy", "code": http.StatusInternalServerError, "detail": err.Error()})
		return
	}
	s.setUpdateState(id, func(state *integrationUpdateStatus) {
		state.AutoUpdate = req.AutoUpdate
	})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "id": id, "auto_update": req.AutoUpdate})
}
