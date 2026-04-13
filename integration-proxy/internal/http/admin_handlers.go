package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/PetoAdam/homenavi/integration-proxy/internal/auth"
	"github.com/PetoAdam/homenavi/integration-proxy/internal/config"
)

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	go func() {
		if err := s.ReloadFromConfig(); err != nil {
			s.logger.Printf("reload failed: %v", err)
		}
	}()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "queued"})
}

func (s *Server) handleRestartAll(w http.ResponseWriter, r *http.Request) {
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
	go func() {
		ctx := context.Background()
		if err := s.ReloadFromConfig(); err != nil {
			s.logger.Printf("restart-all reload failed: %v", err)
		}
		cfg, cfgErr := config.Load(s.configPath)
		if cfgErr == nil && mode == runtimeCompose {
			for _, ic := range cfg.Integrations {
				id := strings.TrimSpace(ic.ID)
				if id == "" {
					continue
				}
				s.setInstallStatus(id, "restarting", 60, "Restarting integration")
				composeFile := s.defaultComposeFile(id)
				if strings.TrimSpace(composeFile) == "" {
					s.setInstallStatus(id, "restarted", 100, "Restarted")
					continue
				}
				composeFile = s.expandComposePath(composeFile)
				if err := s.runCompose(ctx, composeFile, "restart", id); err != nil {
					s.logger.Printf("restart-all failed id=%s err=%v", id, err)
					s.setInstallStatus(id, "error", 100, "Restart failed")
				} else {
					s.setInstallStatus(id, "restarted", 100, "Restarted")
				}
			}
		} else if cfgErr == nil && mode == runtimeHelm {
			for _, ic := range cfg.Integrations {
				id := strings.TrimSpace(ic.ID)
				if id == "" {
					continue
				}
				s.setInstallStatus(id, "restarting", 60, "Restarting integration")
				releaseName := firstNonEmpty(strings.TrimSpace(ic.HelmReleaseName), s.defaultHelmReleaseName(id))
				namespace := firstNonEmpty(strings.TrimSpace(ic.HelmNamespace), s.defaultHelmNamespace())
				if err := s.helmRestart(ctx, releaseName, namespace); err != nil {
					s.logger.Printf("restart-all helm failed id=%s err=%v", id, err)
					s.setInstallStatus(id, "error", 100, "Restart failed")
				} else {
					s.setInstallStatus(id, "restarted", 100, "Restarted")
				}
			}
		} else if cfgErr == nil {
			for _, ic := range cfg.Integrations {
				id := strings.TrimSpace(ic.ID)
				if id == "" {
					continue
				}
				s.setInstallStatus(id, "restarting", 60, "Restarting integration")
				s.setInstallStatus(id, "restarted", 100, "Restarted")
			}
		} else {
			s.logger.Printf("restart-all config load failed: %v", cfgErr)
		}
		s.refreshAll(ctx)
	}()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "queued"})
}

func (s *Server) handleRestartIntegration(w http.ResponseWriter, r *http.Request) {
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
	id := strings.TrimPrefix(r.URL.Path, "/integrations/restart/")
	id = strings.TrimSpace(id)
	if id == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing id", "code": http.StatusBadRequest})
		return
	}
	s.mu.RLock()
	_, ok := s.upstreams[id]
	s.mu.RUnlock()
	if !ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "unknown integration", "code": http.StatusNotFound})
		return
	}
	go func() {
		ctx := context.Background()
		s.setInstallStatus(id, "restarting", 60, "Restarting integration")
		if mode == runtimeCompose {
			composeFile := s.defaultComposeFile(id)
			if strings.TrimSpace(composeFile) != "" {
				composeFile = s.expandComposePath(composeFile)
				if err := s.runCompose(ctx, composeFile, "restart", id); err != nil {
					s.logger.Printf("restart %s failed: %v", id, err)
					s.setInstallStatus(id, "error", 100, "Restart failed")
				} else {
					s.setInstallStatus(id, "restarted", 100, "Restarted")
				}
			} else {
				s.setInstallStatus(id, "restarted", 100, "Restarted")
			}
		} else if mode == runtimeHelm {
			cfgData, index, cfgErr := s.loadIntegrationConfig(id)
			if cfgErr != nil {
				s.logger.Printf("restart %s config load failed: %v", id, cfgErr)
				s.setInstallStatus(id, "error", 100, "Restart failed")
			} else {
				ic := cfgData.Integrations[index]
				releaseName := firstNonEmpty(strings.TrimSpace(ic.HelmReleaseName), s.defaultHelmReleaseName(id))
				namespace := firstNonEmpty(strings.TrimSpace(ic.HelmNamespace), s.defaultHelmNamespace())
				if err := s.helmRestart(ctx, releaseName, namespace); err != nil {
					s.logger.Printf("restart %s failed: %v", id, err)
					s.setInstallStatus(id, "error", 100, "Restart failed")
				} else {
					s.setInstallStatus(id, "restarted", 100, "Restarted")
				}
			}
		} else {
			s.setInstallStatus(id, "restarted", 100, "Restarted")
		}
		s.refreshManifestFromUpstream(ctx, id)
	}()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "queued"})
}

func (s *Server) handleInstallStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/integrations/install-status/")
	id = strings.TrimSpace(id)
	if id == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing id", "code": http.StatusBadRequest})
		return
	}
	status, ok := s.getInstallStatus(id)
	if !ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "status not found", "code": http.StatusNotFound})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	payload := map[string]any{
		"management_mode": s.managementModeLabel(),
		"id":              status.ID,
		"stage":           status.Stage,
		"progress":        status.Progress,
		"message":         status.Message,
		"updated_at":      status.Updated,
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.pubKey == nil {
		return true
	}
	role, err := auth.RoleFromRequest(s.pubKey, r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid token", "code": http.StatusUnauthorized})
		return false
	}
	if !auth.RoleAtLeast("admin", role) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "forbidden", "code": http.StatusForbidden})
		return false
	}
	return true
}
