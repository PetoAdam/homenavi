//go:build ignore
// +build ignore

package main

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

type Claims struct {
	Role string `json:"role"`
	Name string `json:"name"`
	jwt.RegisteredClaims
}

func loadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return jwt.ParseRSAPublicKeyFromPEM(keyData)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "code": status})
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}
	return ""
}

func roleAtLeast(required, actual string) bool {
	roleRank := map[string]int{
		"public":   0,
		"user":     1,
		"resident": 2,
		"admin":    3,
		"service":  4,
	}
	return roleRank[actual] >= roleRank[required]
}

func requireResident(pubKey *rsa.PublicKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL != nil && r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			tokenStr := extractToken(r)
			if tokenStr == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing token")
				return
			}
			token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
				return pubKey, nil
			})
			if err != nil || !token.Valid {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			claims, ok := token.Claims.(*Claims)
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "invalid claims")
				return
			}
			if !roleAtLeast("resident", strings.TrimSpace(claims.Role)) {
				writeJSONError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type Config struct {
	Integrations []IntegrationConfig `yaml:"integrations"`
}

type IntegrationConfig struct {
	ID       string `yaml:"id"`
	Upstream string `yaml:"upstream"`
}

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
}

type RegistryIntegration struct {
	ID            string           `json:"id"`
	DisplayName   string           `json:"display_name"`
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

type Server struct {
	logger      *log.Logger
	proxies     map[string]*httputil.ReverseProxy
	upstreams   map[string]*url.URL
	manifests   map[string]Manifest
	manifestErr map[string]string
	mu          sync.RWMutex
	client      *http.Client
	validator   *jsonschema.Schema
	schemaPath  string
}

func main() {
	var (
		listenAddr = flag.String("listen", ":8099", "listen address")
		configPath = flag.String("config", getenv("INTEGRATIONS_CONFIG_PATH", "/config/integrations.yaml"), "path to integrations yaml")
		schemaPath = flag.String("schema", getenv("INTEGRATIONS_SCHEMA_PATH", "/config/homenavi-integration.schema.json"), "path to integration manifest jsonschema")
		refresh    = flag.Duration("refresh", 30*time.Second, "manifest refresh interval")
	)
	flag.Parse()

	logger := log.New(os.Stdout, "integration-proxy ", log.LstdFlags|log.LUTC)
	pubKeyPath := strings.TrimSpace(os.Getenv("JWT_PUBLIC_KEY_PATH"))
	if pubKeyPath == "" {
		logger.Fatalf("JWT_PUBLIC_KEY_PATH is required to protect /integrations/*")
	}
	pubKey, err := loadRSAPublicKey(pubKeyPath)
	if err != nil {
		logger.Fatalf("load JWT public key: %v", err)
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	validator, err := loadSchema(*schemaPath)
	if err != nil {
		logger.Fatalf("load schema: %v", err)
	}

	s := &Server{
		logger:      logger,
		proxies:     make(map[string]*httputil.ReverseProxy),
		upstreams:   make(map[string]*url.URL),
		manifests:   make(map[string]Manifest),
		manifestErr: make(map[string]string),
		client:      &http.Client{Timeout: 8 * time.Second},
		validator:   validator,
		schemaPath:  *schemaPath,
	}

	for _, ic := range cfg.Integrations {
		if err := s.addIntegration(ic); err != nil {
			logger.Fatalf("add integration %q: %v", ic.ID, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.refreshLoop(ctx, *refresh)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/integrations/registry.json", s.handleRegistry)
	mux.HandleFunc("/registry.json", s.handleRegistry)
	mux.HandleFunc("/integrations/", s.handleProxy)
	mux.HandleFunc("/", s.handleProxy)

	h := requireResident(pubKey)(mux)
	srv := &http.Server{Addr: *listenAddr, Handler: h}
	logger.Printf("listening on %s", *listenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("server error: %v", err)
	}
}

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func loadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadSchema(schemaPath string) (*jsonschema.Schema, error) {
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

func (s *Server) addIntegration(ic IntegrationConfig) error {
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
			new := make([]string, 0, len(sc))
			for _, c := range sc {
				// naive Path= replacement: replace "Path=/" with "Path=/integrations/<id>/"
				repl := strings.ReplaceAll(c, "Path=/;", fmt.Sprintf("Path=/integrations/%s/;", id))
				repl = strings.ReplaceAll(repl, "Path=/,", fmt.Sprintf("Path=/integrations/%s/;", id))
				// also handle Path=/ at end
				if strings.HasSuffix(repl, "Path=/") {
					repl = strings.TrimSuffix(repl, "Path=/") + fmt.Sprintf("Path=/integrations/%s/", id)
				}
				new = append(new, repl)
			}
			resp.Header.Del("Set-Cookie")
			for _, v := range new {
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

func (s *Server) refreshLoop(ctx context.Context, every time.Duration) {
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

	// refresh manifests on-demand
	for _, id := range ids {
		s.refreshManifestFromUpstream(r.Context(), id)
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
		regInt := RegistryIntegration{
			ID:            id,
			DisplayName:   firstNonEmpty(strings.TrimSpace(m.UI.Sidebar.Label), strings.TrimSpace(m.Name), id),
			Icon:          icon,
			Route:         "/apps/" + id,
			DefaultUIPath: defPath,
			Secrets:       append([]SecretSpec{}, m.Secrets...),
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
			regInt.Widgets = append(regInt.Widgets, RegistryWidget{
				ID:          wid,
				DisplayName: firstNonEmpty(strings.TrimSpace(wdg.DisplayName), wid),
				Description: strings.TrimSpace(wdg.Description),
				Icon:        strings.TrimSpace(m.UI.Sidebar.Icon),
				EntryURL:    entryURL,
				Verified:    false,
				Source:      "integration",
			})
		}
		reg.Integrations = append(reg.Integrations, regInt)
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(reg)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
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
	if err := s.validator.Validate(raw); err != nil {
		s.setManifestErr(id, "schema validation failed")
		return err
	}

	var m Manifest
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
