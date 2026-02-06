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
	"path"
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
	mu          sync.RWMutex
	client      *http.Client
	validator   *jsonschema.Schema
	pubKey      *rsa.PublicKey
	schemaPath  string
	configPath  string
}

func New(logger *log.Logger, validator *jsonschema.Schema, pubKey *rsa.PublicKey, schemaPath, configPath string) *Server {
	s := &Server{
		logger:      logger,
		proxies:     make(map[string]*httputil.ReverseProxy),
		upstreams:   make(map[string]*url.URL),
		manifests:   make(map[string]models.Manifest),
		manifestErr: make(map[string]string),
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
	mux.HandleFunc("/integrations/reload", s.handleReload)
	mux.HandleFunc("/reload", s.handleReload)
	mux.HandleFunc("/integrations/restart-all", s.handleRestartAll)
	mux.HandleFunc("/integrations/restart/", s.handleRestartIntegration)
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
		if err := s.ReloadFromConfig(); err != nil {
			s.logger.Printf("restart-all reload failed: %v", err)
		}
		s.refreshAll(context.Background())
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
	go s.refreshManifestFromUpstream(context.Background(), id)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "queued"})
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
