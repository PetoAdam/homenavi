package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/integration-proxy/internal/config"
	"github.com/PetoAdam/homenavi/shared/envx"
)

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	var req struct {
		ID          string `json:"id"`
		Upstream    string `json:"upstream"`
		ComposeFile string `json:"compose_file"`
		Version     string `json:"version"`
		AutoUpdate  *bool  `json:"auto_update"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
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
	op := newIntegrationOperation(s, "install", id)
	op.set("queued", 5, "Queued")
	mode, modeErr := s.ensureMutableRuntime()
	if modeErr != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(strings.ToLower(modeErr.Error()), "gitops mode") {
			statusCode = http.StatusConflict
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "runtime unavailable", "code": statusCode, "detail": modeErr.Error()})
		op.fail(modeErr)
		return
	}
	if strings.TrimSpace(s.configPath) == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "config path unavailable", "code": http.StatusInternalServerError})
		op.fail(fmt.Errorf("config path unavailable"))
		return
	}
	cfg, err := config.Load(s.configPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to load config", "code": http.StatusInternalServerError})
		op.fail(fmt.Errorf("failed to load config: %w", err))
		return
	}
	for _, ic := range cfg.Integrations {
		if ic.ID == id {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "integration already installed", "code": http.StatusConflict})
			return
		}
	}
	upstream := strings.TrimSpace(req.Upstream)
	if upstream == "" {
		upstream = defaultUpstreamForID(id)
	}
	op.set("preparing", 15, "Preparing installation")
	composeFile := ""
	helmSpec := helmDeploymentSpec{}
	marketTarget, _ := s.fetchMarketplaceIntegration(r.Context(), id)
	marketVersion := ""
	if marketTarget != nil {
		marketVersion = strings.TrimSpace(marketTarget.Version)
	}
	if mode == runtimeCompose {
		composeInput := strings.TrimSpace(req.ComposeFile)
		if composeInput == "" && marketTarget != nil {
			composeInput = marketTarget.composeArtifactFile()
		}
		composeFile, err = s.resolveComposeFileForOperation(id, composeInput, marketVersion)
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to prepare compose file", "code": http.StatusInternalServerError, "detail": err.Error()})
			op.fail(fmt.Errorf("failed to prepare compose file: %w", err))
			return
		}
		if strings.TrimSpace(composeFile) != "" {
			service, svcErr := s.composeServiceForID(composeFile, id)
			if svcErr != nil {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid compose file", "code": http.StatusBadRequest, "detail": svcErr.Error()})
				op.fail(fmt.Errorf("invalid compose file: %w", svcErr))
				return
			}
			if service != "" && (upstream == "" || upstream == defaultUpstreamForID(id)) {
				upstream = fmt.Sprintf("http://%s:8099", service)
			}
		}
	}
	if mode == runtimeHelm {
		entryForHelm := config.IntegrationConfig{ID: id}
		helmSpec, err = s.helmSpecFor(id, entryForHelm, marketTarget)
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to resolve helm artifact", "code": http.StatusBadRequest, "detail": err.Error()})
			op.fail(err)
			return
		}
		if upstream == "" || upstream == defaultUpstreamForID(id) {
			upstream = s.defaultHelmUpstream(helmSpec)
		}
	}
	if upstream == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing upstream", "code": http.StatusBadRequest})
		op.fail(fmt.Errorf("missing upstream"))
		return
	}
	if mode == runtimeCompose && strings.TrimSpace(composeFile) != "" {
		if err := s.ensureSecretsFile(id); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to prepare secrets file", "code": http.StatusInternalServerError, "detail": err.Error()})
			op.fail(fmt.Errorf("failed to prepare secrets file: %w", err))
			return
		}
		if err := s.ensureSetupFile(id); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to prepare setup file", "code": http.StatusInternalServerError, "detail": err.Error()})
			op.fail(fmt.Errorf("failed to prepare setup file: %w", err))
			return
		}
	}
	startedByCompose := false
	startedByHelm := false
	op.set("starting", 45, "Starting integration")
	opCtx, cancelOp := context.WithTimeout(context.Background(), s.operationTimeout())
	defer cancelOp()
	if mode == runtimeCompose && strings.TrimSpace(composeFile) != "" {
		if err := s.composeInstall(opCtx, composeFile, id); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to start integration", "code": http.StatusInternalServerError, "detail": err.Error()})
			op.fail(fmt.Errorf("failed to start integration: %w", err))
			return
		}
		startedByCompose = true
	}
	if mode == runtimeHelm {
		if err := s.helmInstallOrUpgrade(opCtx, helmSpec, id); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to deploy integration with helm", "code": http.StatusInternalServerError, "detail": err.Error()})
			op.fail(fmt.Errorf("failed to deploy integration with helm: %w", err))
			return
		}
		startedByHelm = true
	}
	op.set("writing-config", 70, "Updating installed integrations")
	entry := config.IntegrationConfig{ID: id, Upstream: upstream, Version: strings.TrimSpace(req.Version)}
	if entry.Version == "" && marketTarget != nil {
		entry.Version = marketVersion
	}
	if req.AutoUpdate != nil {
		entry.AutoUpdate = *req.AutoUpdate
	}
	if mode == runtimeCompose {
		entry.ComposeFile = strings.TrimSpace(composeFile)
	}
	if mode == runtimeHelm {
		entry.HelmReleaseName = helmSpec.ReleaseName
		entry.HelmNamespace = helmSpec.Namespace
		entry.HelmChartRef = helmSpec.ChartRef
		entry.HelmChartVersion = helmSpec.ChartVersion
		entry.HelmValuesFile = helmSpec.ValuesFile
		entry.Upstream = s.defaultHelmUpstream(helmSpec)
	}
	nextCfg := cfg
	nextCfg.Integrations = append(nextCfg.Integrations, entry)
	if err := config.Save(s.configPath, nextCfg); err != nil {
		if startedByCompose {
			if stopErr := s.composeUninstall(opCtx, composeFile, id); stopErr != nil {
				s.logger.Printf("failed to rollback compose install id=%s err=%v", id, stopErr)
			}
		}
		if startedByHelm {
			if stopErr := s.helmUninstall(opCtx, helmSpec.ReleaseName, helmSpec.Namespace); stopErr != nil {
				s.logger.Printf("failed to rollback helm install id=%s err=%v", id, stopErr)
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to write config", "code": http.StatusInternalServerError, "detail": err.Error()})
		op.fail(fmt.Errorf("failed to write config: %w", err))
		return
	}
	op.set("reloading", 85, "Reloading integration proxy")
	if err := s.ReloadFromConfig(); err != nil {
		if saveErr := config.Save(s.configPath, cfg); saveErr != nil {
			s.logger.Printf("failed to rollback config after reload error id=%s err=%v", id, saveErr)
		}
		if startedByCompose {
			if stopErr := s.composeUninstall(opCtx, composeFile, id); stopErr != nil {
				s.logger.Printf("failed to rollback compose install after reload error id=%s err=%v", id, stopErr)
			}
		}
		if startedByHelm {
			if stopErr := s.helmUninstall(opCtx, helmSpec.ReleaseName, helmSpec.Namespace); stopErr != nil {
				s.logger.Printf("failed to rollback helm install after reload error id=%s err=%v", id, stopErr)
			}
		}
		if reloadErr := s.ReloadFromConfig(); reloadErr != nil {
			s.logger.Printf("failed to reload proxy after rollback id=%s err=%v", id, reloadErr)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to reload", "code": http.StatusInternalServerError})
		op.fail(fmt.Errorf("failed to reload: %w", err))
		return
	}
	op.done("Installed")
	s.checkForUpdates(r.Context(), false)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "installed", "id": id})
}

func (s *Server) ensureSecretsFile(id string) error {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid integration id")
	}
	root := envx.String("INTEGRATIONS_ROOT", "")
	if root == "" {
		return nil
	}
	secretsDir := filepath.Join(root, "integrations", "secrets")
	if err := os.MkdirAll(secretsDir, 0o755); err != nil {
		return err
	}
	secretsPath := filepath.Join(secretsDir, fmt.Sprintf("%s.secrets.json", id))
	info, err := os.Stat(secretsPath)
	if err == nil {
		if info.IsDir() {
			if err := os.RemoveAll(secretsPath); err != nil {
				return err
			}
			return os.WriteFile(secretsPath, []byte("{}\n"), 0o644)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(secretsPath, []byte("{}\n"), 0o644)
}

func (s *Server) ensureSetupFile(id string) error {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid integration id")
	}
	root := envx.String("INTEGRATIONS_ROOT", "")
	if root == "" {
		return nil
	}
	setupDir := filepath.Join(root, "integrations", "setup")
	if err := os.MkdirAll(setupDir, 0o755); err != nil {
		return err
	}
	setupPath := filepath.Join(setupDir, fmt.Sprintf("%s.setup.json", id))
	info, err := os.Stat(setupPath)
	if err == nil {
		if info.IsDir() {
			if err := os.RemoveAll(setupPath); err != nil {
				return err
			}
			return os.WriteFile(setupPath, []byte("{}\n"), 0o644)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(setupPath, []byte("{}\n"), 0o644)
}

func (s *Server) handleUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	mode, modeErr := s.ensureMutableRuntime()
	if modeErr != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(strings.ToLower(modeErr.Error()), "gitops mode") {
			statusCode = http.StatusConflict
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "runtime unavailable", "code": statusCode, "detail": modeErr.Error()})
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	if strings.TrimSpace(s.configPath) == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "config path unavailable", "code": http.StatusInternalServerError})
		return
	}
	cfg, err := config.Load(s.configPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to load config", "code": http.StatusInternalServerError})
		return
	}
	updated := make([]config.IntegrationConfig, 0, len(cfg.Integrations))
	var removed config.IntegrationConfig
	found := false
	for _, ic := range cfg.Integrations {
		if ic.ID == id {
			found = true
			removed = ic
			continue
		}
		updated = append(updated, ic)
	}
	if !found {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "integration not installed", "code": http.StatusNotFound})
		return
	}
	composeFile := firstNonEmpty(strings.TrimSpace(removed.ComposeFile), s.defaultComposeFile(id))
	composeFile = s.expandComposePath(composeFile)
	releaseName := firstNonEmpty(strings.TrimSpace(removed.HelmReleaseName), s.defaultHelmReleaseName(id))
	namespace := firstNonEmpty(strings.TrimSpace(removed.HelmNamespace), s.defaultHelmNamespace())
	opCtx, cancelOp := context.WithTimeout(context.Background(), s.operationTimeout())
	defer cancelOp()
	cfg.Integrations = updated
	if err := config.Save(s.configPath, cfg); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to write config", "code": http.StatusInternalServerError, "detail": err.Error()})
		return
	}
	if mode == runtimeCompose && strings.TrimSpace(composeFile) != "" {
		if err := s.composeUninstall(opCtx, composeFile, id); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to stop integration", "code": http.StatusInternalServerError, "detail": err.Error()})
			return
		}
	}
	if mode == runtimeHelm {
		if err := s.helmUninstall(opCtx, releaseName, namespace); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to stop integration", "code": http.StatusInternalServerError, "detail": err.Error()})
			return
		}
	}
	if err := s.ReloadFromConfig(); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to reload", "code": http.StatusInternalServerError})
		return
	}
	s.mu.Lock()
	delete(s.updates, id)
	delete(s.updating, id)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "uninstalled", "id": id})
}

func (s *Server) refreshAll(ctx context.Context) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.upstreams))
	for id := range s.upstreams {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	for _, id := range ids {
		s.refreshManifestFromUpstream(ctx, id)
	}
}

func (s *Server) marketplaceAPIBase() string {
	base := envx.String("INTEGRATIONS_MARKETPLACE_API_BASE", "")
	if base == "" {
		base = "https://marketplace.homenavi.org"
	}
	return strings.TrimRight(base, "/")
}

func (s *Server) resolveComposeFileFromPayload(id, composeFile string) (string, error) {
	composeFile = strings.TrimSpace(composeFile)
	if composeFile == "" {
		return "", nil
	}
	if strings.HasPrefix(composeFile, "http://") || strings.HasPrefix(composeFile, "https://") {
		if strings.TrimSpace(s.configPath) == "" {
			return "", fmt.Errorf("config path unavailable")
		}
		resp, err := s.client.Get(composeFile)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("compose file fetch failed")
		}
		const maxComposeSize = 512 * 1024
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxComposeSize))
		if err != nil {
			return "", err
		}
		composeDir := filepath.Join(filepath.Dir(s.configPath), "compose")
		if err := os.MkdirAll(composeDir, 0o755); err != nil {
			return "", err
		}
		filePath := filepath.Join(composeDir, fmt.Sprintf("%s.yml", id))
		if err := os.WriteFile(filePath, append(body, '\n'), 0o644); err != nil {
			return "", err
		}
		return filePath, nil
	}
	return composeFile, nil
}

func (s *Server) defaultComposeFile(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.TrimSpace(s.configPath) == "" {
		return ""
	}
	baseDir := filepath.Dir(s.configPath)
	local := filepath.Clean(filepath.Join(baseDir, "..", "compose", fmt.Sprintf("%s.yml", id)))
	if info, err := os.Stat(local); err == nil && !info.IsDir() {
		return local
	}
	root := envx.String("INTEGRATIONS_ROOT", "")
	if root != "" {
		rootFile := filepath.Join(root, "integrations", "compose", fmt.Sprintf("%s.yml", id))
		if info, err := os.Stat(rootFile); err == nil && !info.IsDir() {
			return rootFile
		}
	}
	return ""
}

func (s *Server) setInstallStatus(id, stage string, progress int, message string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	s.mu.Lock()
	s.installStat[id] = installStatus{ID: id, Stage: stage, Progress: progress, Message: message, Updated: time.Now().UTC()}
	s.mu.Unlock()
}

func (s *Server) getInstallStatus(id string) (installStatus, bool) {
	s.mu.RLock()
	status, ok := s.installStat[id]
	s.mu.RUnlock()
	return status, ok
}
