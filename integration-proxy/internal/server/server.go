package server

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"homenavi/integration-proxy/internal/auth"
	"homenavi/integration-proxy/internal/config"
	"homenavi/integration-proxy/internal/models"
)

type Server struct {
	logger      *log.Logger
	proxies     map[string]*httputil.ReverseProxy
	upstreams   map[string]*url.URL
	manifests   map[string]models.Manifest
	manifestErr map[string]string
	installStat map[string]installStatus
	mu          sync.RWMutex
	client      *http.Client
	validator   *jsonschema.Schema
	pubKey      *rsa.PublicKey
	schemaPath  string
	configPath  string
}

type installStatus struct {
	ID       string    `json:"id"`
	Stage    string    `json:"stage"`
	Progress int       `json:"progress"`
	Message  string    `json:"message,omitempty"`
	Updated  time.Time `json:"updated_at"`
}

func New(logger *log.Logger, validator *jsonschema.Schema, pubKey *rsa.PublicKey, schemaPath, configPath string) *Server {
	s := &Server{
		logger:      logger,
		proxies:     make(map[string]*httputil.ReverseProxy),
		upstreams:   make(map[string]*url.URL),
		manifests:   make(map[string]models.Manifest),
		manifestErr: make(map[string]string),
		installStat: make(map[string]installStatus),
		client:      &http.Client{Timeout: 8 * time.Second},
		validator:   validator,
		pubKey:      pubKey,
		schemaPath:  schemaPath,
		configPath:  configPath,
	}
	return s
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/integrations/registry.json", s.handleRegistry)
	mux.HandleFunc("/registry.json", s.handleRegistry)
	mux.HandleFunc("/integrations/marketplace.json", s.handleMarketplace)
	mux.HandleFunc("/marketplace.json", s.handleMarketplace)
	mux.HandleFunc("/integrations/reload", s.handleReload)
	mux.HandleFunc("/reload", s.handleReload)
	mux.HandleFunc("/integrations/restart-all", s.handleRestartAll)
	mux.HandleFunc("/integrations/restart/", s.handleRestartIntegration)
	mux.HandleFunc("/integrations/install", s.handleInstall)
	mux.HandleFunc("/integrations/uninstall", s.handleUninstall)
	mux.HandleFunc("/integrations/install-status/", s.handleInstallStatus)
	mux.HandleFunc("/integrations/", s.handleProxy)
	mux.HandleFunc("/", s.handleProxy)
	return mux
}

func (s *Server) AddIntegration(ic config.IntegrationConfig) error {
	id := strings.TrimSpace(ic.ID)
	if id == "" {
		return fmt.Errorf("missing id")
	}
	u, err := url.Parse(strings.TrimSpace(ic.Upstream))
	if err != nil {
		return fmt.Errorf("parse upstream: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid upstream %q", ic.Upstream)
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	origDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		origDirector(r)
		// director sets r.URL.Path based on incoming path; we'll rewrite later in handler
		// ensure forwarded headers
		if r.Header.Get("X-Forwarded-Proto") == "" {
			if r.TLS != nil {
				r.Header.Set("X-Forwarded-Proto", "https")
			} else {
				r.Header.Set("X-Forwarded-Proto", "http")
			}
		}
		r.Header.Set("X-Forwarded-Host", r.Host)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.logger.Printf("proxy error id=%s path=%s err=%v", id, r.URL.Path, err)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}

	// Adjust responses from upstream to account for the proxy prefix.
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Rewrite Location headers for redirects that use absolute or root-relative paths.
		loc := resp.Header.Get("Location")
		if loc != "" {
			// If Location is an absolute-path (starts with '/') rewrite to include /integrations/<id>
			if strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "/integrations/") {
				// Prefer an absolute URL using the original host so nginx won't re-rewrite it.
				host := resp.Request.Header.Get("X-Forwarded-Host")
				proto := resp.Request.Header.Get("X-Forwarded-Proto")
				if host == "" {
					host = resp.Request.Host
				}
				if proto == "" {
					proto = "http"
					if resp.Request.TLS != nil {
						proto = "https"
					}
				}
				newLoc := fmt.Sprintf("%s://%s/integrations/%s%s", proto, host, id, loc)
				resp.Header.Set("Location", newLoc)
			} else if strings.HasPrefix(loc, u.Scheme+"://"+u.Host) {
				// If upstream returned an absolute URL pointing to itself, convert to proxy absolute URL.
				parsed, err := url.Parse(loc)
				if err == nil {
					host := resp.Request.Header.Get("X-Forwarded-Host")
					proto := resp.Request.Header.Get("X-Forwarded-Proto")
					if host == "" {
						host = resp.Request.Host
					}
					if proto == "" {
						proto = "http"
						if resp.Request.TLS != nil {
							proto = "https"
						}
					}
					newLoc := fmt.Sprintf("%s://%s/integrations/%s%s", proto, host, id, parsed.Path)
					if parsed.RawQuery != "" {
						newLoc += "?" + parsed.RawQuery
					}
					resp.Header.Set("Location", newLoc)
				}
			}
		}

		// Adjust Set-Cookie Path attributes so cookies are scoped under the integration prefix.
		if sc := resp.Header.Values("Set-Cookie"); len(sc) > 0 {
			newVals := make([]string, 0, len(sc))
			for _, c := range sc {
				// naive Path= replacement: replace "Path=/" with "Path=/integrations/<id>/"
				repl := strings.ReplaceAll(c, "Path=/;", fmt.Sprintf("Path=/integrations/%s/;", id))
				repl = strings.ReplaceAll(repl, "Path=/,", fmt.Sprintf("Path=/integrations/%s/;", id))
				// also handle Path=/ at end
				if strings.HasSuffix(repl, "Path=/") {
					repl = strings.TrimSuffix(repl, "Path=/") + fmt.Sprintf("Path=/integrations/%s/", id)
				}
				newVals = append(newVals, repl)
			}
			resp.Header.Del("Set-Cookie")
			for _, v := range newVals {
				resp.Header.Add("Set-Cookie", v)
			}
		}

		return nil
	}

	s.mu.Lock()
	s.proxies[id] = proxy
	s.upstreams[id] = u
	s.mu.Unlock()

	// initial manifest fetch (non-fatal)
	_ = s.refreshManifest(context.Background(), id, u)
	return nil
}

func (s *Server) StartRefreshLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refreshAll(ctx)
		}
	}
}

func (s *Server) ReloadFromConfig() error {
	if strings.TrimSpace(s.configPath) == "" {
		return fmt.Errorf("config path is empty")
	}
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.proxies = make(map[string]*httputil.ReverseProxy)
	s.upstreams = make(map[string]*url.URL)
	s.manifests = make(map[string]models.Manifest)
	s.manifestErr = make(map[string]string)
	s.installStat = make(map[string]installStatus)
	s.mu.Unlock()

	for _, ic := range cfg.Integrations {
		if err := s.AddIntegration(ic); err != nil {
			return err
		}
	}

	return nil
}

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
	go func() {
		ctx := context.Background()
		if err := s.ReloadFromConfig(); err != nil {
			s.logger.Printf("restart-all reload failed: %v", err)
		}
		cfg, cfgErr := config.Load(s.configPath)
		marketplace, mpErr := config.LoadMarketplace(s.marketplacePath())
		if cfgErr == nil && mpErr == nil && s.composeEnabled() {
			for _, ic := range cfg.Integrations {
				id := strings.TrimSpace(ic.ID)
				if id == "" {
					continue
				}
				composeFile := ""
				for _, entry := range marketplace.Integrations {
					if strings.TrimSpace(entry.ID) == id {
						if filePath, err := s.resolveComposeFile(id, entry); err == nil {
							composeFile = filePath
						} else {
							s.logger.Printf("restart-all resolve compose failed id=%s err=%v", id, err)
						}
						break
					}
				}
				if strings.TrimSpace(composeFile) == "" {
					continue
				}
				composeFile = s.expandComposePath(composeFile)
				if err := s.runCompose(ctx, composeFile, "restart", id); err != nil {
					s.logger.Printf("restart-all failed id=%s err=%v", id, err)
				}
			}
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
		composeFile := ""
		if s.composeEnabled() {
			if marketplace, err := config.LoadMarketplace(s.marketplacePath()); err == nil {
				for _, entry := range marketplace.Integrations {
					if strings.TrimSpace(entry.ID) == id {
						if filePath, err := s.resolveComposeFile(id, entry); err == nil {
							composeFile = filePath
						} else {
							s.logger.Printf("restart %s resolve compose failed: %v", id, err)
						}
						break
					}
				}
			}
		}
		if strings.TrimSpace(composeFile) != "" {
			composeFile = s.expandComposePath(composeFile)
			if err := s.runCompose(ctx, composeFile, "restart", id); err != nil {
				s.logger.Printf("restart %s failed: %v", id, err)
				s.setInstallStatus(id, "error", 100, "Restart failed")
			} else {
				s.setInstallStatus(id, "restarted", 100, "Restarted")
			}
		}
		s.refreshManifestFromUpstream(ctx, id)
	}()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "queued"})
}

func (s *Server) handleMarketplace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	marketplace, err := config.LoadMarketplace(s.marketplacePath())
	if err != nil {
		s.logger.Printf("marketplace load failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	installed := map[string]bool{}
	if strings.TrimSpace(s.configPath) != "" {
		if cfg, err := config.Load(s.configPath); err == nil {
			for _, ic := range cfg.Integrations {
				installed[ic.ID] = true
			}
		} else {
			s.logger.Printf("marketplace installed load failed: %v", err)
		}
	}
	items := make([]models.MarketplaceIntegration, 0, len(marketplace.Integrations))
	for _, entry := range marketplace.Integrations {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		items = append(items, models.MarketplaceIntegration{
			ID:          id,
			DisplayName: entry.DisplayName,
			Description: entry.Description,
			Icon:        entry.Icon,
			Version:     entry.Version,
			Publisher:   entry.Publisher,
			Homepage:    entry.Homepage,
			Installed:   installed[id],
		})
	}
	resp := models.Marketplace{GeneratedAt: time.Now().UTC(), Integrations: items}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	var req struct {
		ID       string `json:"id"`
		Upstream string `json:"upstream"`
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
	for _, ic := range cfg.Integrations {
		if ic.ID == id {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "integration already installed", "code": http.StatusConflict})
			return
		}
	}
	upstream := strings.TrimSpace(req.Upstream)
	composeFile := ""
	marketplace, err := config.LoadMarketplace(s.marketplacePath())
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to load marketplace", "code": http.StatusInternalServerError})
		return
	}
	for _, entry := range marketplace.Integrations {
		if strings.TrimSpace(entry.ID) == id {
			if upstream == "" {
				upstream = strings.TrimSpace(entry.Upstream)
			}
			composeFile, err = s.resolveComposeFile(id, entry)
			if err != nil {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to prepare compose file", "code": http.StatusInternalServerError, "detail": err.Error()})
				return
			}
			break
		}
	}
	composeFile = s.expandComposePath(composeFile)
	if upstream == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing upstream", "code": http.StatusBadRequest})
		return
	}
	if strings.TrimSpace(composeFile) != "" {
		if err := s.ensureSecretsFile(id); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to prepare secrets file", "code": http.StatusInternalServerError, "detail": err.Error()})
			s.setInstallStatus(id, "error", 100, err.Error())
			return
		}
	}
	s.setInstallStatus(id, "writing-config", 20, "Updating installed integrations")
	cfg.Integrations = append(cfg.Integrations, config.IntegrationConfig{ID: id, Upstream: upstream})
	if err := config.Save(s.configPath, cfg); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to write config", "code": http.StatusInternalServerError, "detail": err.Error()})
		s.setInstallStatus(id, "error", 100, err.Error())
		return
	}
	s.setInstallStatus(id, "starting", 45, "Starting integration")
	if strings.TrimSpace(composeFile) != "" {
		if err := s.composeInstall(r.Context(), composeFile, id); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to start integration", "code": http.StatusInternalServerError, "detail": err.Error()})
			s.setInstallStatus(id, "error", 100, err.Error())
			return
		}
	}
	s.setInstallStatus(id, "reloading", 70, "Reloading integration proxy")
	if err := s.ReloadFromConfig(); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to reload", "code": http.StatusInternalServerError})
		s.setInstallStatus(id, "error", 100, err.Error())
		return
	}
	s.setInstallStatus(id, "ready", 100, "Installed")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "installed", "id": id})
}

func (s *Server) ensureSecretsFile(id string) error {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid integration id")
	}
	root := strings.TrimSpace(os.Getenv("INTEGRATIONS_ROOT"))
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

func (s *Server) handleUninstall(w http.ResponseWriter, r *http.Request) {
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
	found := false
	for _, ic := range cfg.Integrations {
		if ic.ID == id {
			found = true
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
	composeFile := ""
	if marketplace, err := config.LoadMarketplace(s.marketplacePath()); err == nil {
		for _, entry := range marketplace.Integrations {
			if strings.TrimSpace(entry.ID) == id {
				composeFile, err = s.resolveComposeFile(id, entry)
				if err != nil {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to prepare compose file", "code": http.StatusInternalServerError, "detail": err.Error()})
					return
				}
				break
			}
		}
	}
	composeFile = s.expandComposePath(composeFile)
	cfg.Integrations = updated
	if err := config.Save(s.configPath, cfg); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to write config", "code": http.StatusInternalServerError, "detail": err.Error()})
		return
	}
	if strings.TrimSpace(composeFile) != "" {
		if err := s.composeUninstall(r.Context(), composeFile, id); err != nil {
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "uninstalled", "id": id})
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
	_ = json.NewEncoder(w).Encode(status)
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

func (s *Server) marketplacePath() string {
	if strings.TrimSpace(s.configPath) == "" {
		return ""
	}
	dir := path.Dir(s.configPath)
	return path.Join(dir, "marketplace.yaml")
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

func (s *Server) composeEnabled() bool {
	val := strings.TrimSpace(os.Getenv("INTEGRATIONS_COMPOSE_ENABLED"))
	if val != "" {
		val = strings.ToLower(val)
		return val == "1" || val == "true" || val == "yes"
	}
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		return true
	}
	return false
}

func (s *Server) composeBaseArgs(fileOverride string) ([]string, error) {
	file := strings.TrimSpace(fileOverride)
	if file == "" {
		file = strings.TrimSpace(os.Getenv("INTEGRATIONS_COMPOSE_FILE"))
	}
	if file == "" {
		file = strings.TrimSpace(os.Getenv("DOCKER_COMPOSE_FILE"))
	}
	project := strings.TrimSpace(os.Getenv("INTEGRATIONS_COMPOSE_PROJECT"))
	if file == "" {
		return nil, fmt.Errorf("compose file not configured")
	}
	args := []string{"compose", "-f", file}
	if envFile := strings.TrimSpace(os.Getenv("INTEGRATIONS_COMPOSE_ENV_FILE")); envFile != "" {
		args = append(args, "--env-file", envFile)
	}
	if project != "" {
		args = append(args, "-p", project)
	}
	return args, nil
}

func (s *Server) resolveComposeFile(id string, entry config.MarketplaceIntegration) (string, error) {
	composeFile := strings.TrimSpace(entry.ComposeFile)
	if composeFile != "" {
		return composeFile, nil
	}
	composeYAML := strings.TrimSpace(entry.ComposeYAML)
	if composeYAML == "" {
		return "", nil
	}
	if strings.TrimSpace(s.configPath) == "" {
		return "", fmt.Errorf("config path unavailable")
	}
	baseDir := filepath.Dir(s.configPath)
	composeDir := filepath.Join(baseDir, "compose")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		return "", err
	}
	filePath := filepath.Join(composeDir, fmt.Sprintf("%s.yml", id))
	if err := os.WriteFile(filePath, []byte(composeYAML+"\n"), 0o644); err != nil {
		return "", err
	}
	return filePath, nil
}

func (s *Server) expandComposePath(pathRaw string) string {
	pathRaw = strings.TrimSpace(pathRaw)
	if pathRaw == "" {
		return ""
	}
	root := strings.TrimSpace(os.Getenv("INTEGRATIONS_ROOT"))
	if root == "" {
		return pathRaw
	}
	replaced := strings.ReplaceAll(pathRaw, "${INTEGRATIONS_ROOT}", root)
	if replaced == pathRaw {
		replaced = strings.ReplaceAll(pathRaw, "$INTEGRATIONS_ROOT", root)
	}
	return replaced
}

func (s *Server) runCompose(ctx context.Context, composeFile string, args ...string) error {
	if !s.composeEnabled() {
		return nil
	}
	base, err := s.composeBaseArgs(composeFile)
	if err != nil {
		return err
	}
	bin := strings.TrimSpace(os.Getenv("INTEGRATIONS_COMPOSE_BIN"))
	if bin == "" {
		bin = "docker"
	}
	cmd := exec.CommandContext(ctx, bin, append(base, args...)...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *Server) composeInstall(ctx context.Context, composeFile, id string) error {
	if !s.composeEnabled() {
		return nil
	}
	s.setInstallStatus(id, "pulling", 35, "Pulling container image")
	if err := s.runCompose(ctx, composeFile, "pull", id); err != nil {
		return err
	}
	s.setInstallStatus(id, "starting", 55, "Starting container")
	return s.runCompose(ctx, composeFile, "up", "-d", id)
}

func (s *Server) composeUninstall(ctx context.Context, composeFile, id string) error {
	if !s.composeEnabled() {
		return nil
	}
	if err := s.runCompose(ctx, composeFile, "stop", id); err != nil {
		return err
	}
	return s.runCompose(ctx, composeFile, "rm", "-f", id)
}

func (s *Server) handleRegistry(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.proxies))
	for id := range s.proxies {
		ids = append(ids, id)
	}
	s.mu.RUnlock()

	// Manifest refresh happens in the background loop or via explicit reload.

	reg := models.Registry{GeneratedAt: time.Now().UTC(), Integrations: make([]models.RegistryIntegration, 0)}
	for _, id := range ids {
		s.mu.RLock()
		m, ok := s.manifests[id]
		errMsg := s.manifestErr[id]
		s.mu.RUnlock()
		if !ok {
			if errMsg != "" {
				s.logger.Printf("manifest unavailable id=%s err=%s", id, errMsg)
			}
			reg.Integrations = append(reg.Integrations, models.RegistryIntegration{
				ID:            id,
				DisplayName:   id,
				Route:         "/apps/" + id,
				DefaultUIPath: "/ui/",
				Widgets:       []models.RegistryWidget{},
			})
			continue
		}

		defPath := strings.TrimSpace(m.UI.Sidebar.Path)
		if defPath == "" {
			defPath = "/ui/"
		}

		icon := strings.TrimSpace(m.UI.Sidebar.Icon)
		iconLower := strings.ToLower(icon)
		if strings.HasPrefix(iconLower, "http://") || strings.HasPrefix(iconLower, "https://") {
			// Only allow bundled/same-origin icons or FA tokens; drop remote URLs.
			icon = ""
		}
		if strings.HasPrefix(icon, "/") && !strings.HasPrefix(icon, "/integrations/") {
			icon = "/integrations/" + id + icon
		}
		// Host route (inside the Homenavi app shell) is always /apps/<id>.
		// Integration content is served separately under /integrations/<id>/... via the proxy.
		regInt := models.RegistryIntegration{
			ID:            id,
			DisplayName:   firstNonEmpty(strings.TrimSpace(m.UI.Sidebar.Label), strings.TrimSpace(m.Name), id),
			Description:   strings.TrimSpace(m.Description),
			Icon:          icon,
			Route:         "/apps/" + id,
			DefaultUIPath: defPath,
			Secrets:       append([]models.SecretSpec{}, m.Secrets...),
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
			regInt.Widgets = append(regInt.Widgets, models.RegistryWidget{
				ID:          wid,
				DisplayName: firstNonEmpty(strings.TrimSpace(wdg.DisplayName), wid),
				Description: strings.TrimSpace(wdg.Description),
				Icon:        icon,
				EntryURL:    entryURL,
				Verified:    false,
				Source:      "integration",
			})
		}
		reg.Integrations = append(reg.Integrations, regInt)
	}

	// Optional query filtering + pagination
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if q != "" {
		filtered := make([]models.RegistryIntegration, 0, len(reg.Integrations))
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
		reg.Integrations = []models.RegistryIntegration{}
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

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if strings.HasPrefix(p, "/integrations/") {
		p = strings.TrimPrefix(p, "/integrations/")
	} else if strings.HasPrefix(p, "/integrations") {
		p = strings.TrimPrefix(p, "/integrations")
		p = strings.TrimPrefix(p, "/")
	}
	if p == "" || p == "/" {
		http.NotFound(w, r)
		return
	}

	parts := strings.SplitN(p, "/", 2)
	id := parts[0]
	rest := ""
	if len(parts) == 2 {
		rest = parts[1]
	}

	s.mu.RLock()
	proxy := s.proxies[id]
	s.mu.RUnlock()
	if proxy == nil {
		http.NotFound(w, r)
		return
	}

	// clone request to avoid mutating shared state
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/" + rest
	if strings.HasSuffix(r.URL.Path, "/") && !strings.HasSuffix(r2.URL.Path, "/") {
		r2.URL.Path += "/"
	}
	// preserve raw path
	r2.URL.RawPath = ""
	// propagate original host
	r2.Host = r.Host

	proxy.ServeHTTP(w, r2)
}

func (s *Server) refreshManifestFromUpstream(ctx context.Context, id string) {
	s.mu.RLock()
	up := s.upstreams[id]
	s.mu.RUnlock()
	if up == nil {
		return
	}
	_ = s.refreshManifest(ctx, id, up)
}

func (s *Server) refreshManifest(ctx context.Context, id string, upstream *url.URL) error {
	mURL := *upstream
	mURL.Path = path.Join(mURL.Path, "/.well-known/homenavi-integration.json")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, mURL.String(), nil)
	resp, err := s.client.Do(req)
	if err != nil {
		s.setManifestErr(id, err.Error())
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		s.setManifestErr(id, fmt.Sprintf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(b))))
		return fmt.Errorf("bad status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		s.setManifestErr(id, err.Error())
		return err
	}

	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		s.setManifestErr(id, "invalid json")
		return err
	}
	if s.validator != nil {
		if err := s.validator.Validate(raw); err != nil {
			s.setManifestErr(id, "schema validation failed")
			return err
		}
	}

	var m models.Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		s.setManifestErr(id, "invalid manifest")
		return err
	}
	// normalize id
	m.ID = id

	s.mu.Lock()
	s.manifests[id] = m
	delete(s.manifestErr, id)
	s.mu.Unlock()
	return nil
}

func (s *Server) setManifestErr(id, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifestErr[id] = msg
}

func LoadSchema(schemaPath string) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.LoadURL = func(u string) (io.ReadCloser, error) {
		// Only allow local file reads
		u = strings.TrimPrefix(u, "file://")
		f, err := os.Open(u)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	if err := compiler.AddResource("schema.json", strings.NewReader(mustRead(schemaPath))); err != nil {
		return nil, err
	}
	return compiler.Compile("schema.json")
}

func mustRead(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
