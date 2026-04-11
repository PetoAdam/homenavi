package http

import (
	"context"
	"crypto/rsa"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/PetoAdam/homenavi/integration-proxy/internal/config"
)

type Server struct {
	logger      *log.Logger
	proxies     map[string]*httputil.ReverseProxy
	upstreams   map[string]*url.URL
	manifests   map[string]Manifest
	manifestErr map[string]string
	installStat map[string]installStatus
	updates     map[string]integrationUpdateStatus
	updating    map[string]bool
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

type integrationUpdateStatus struct {
	ID               string     `json:"id"`
	InstalledVersion string     `json:"installed_version,omitempty"`
	LatestVersion    string     `json:"latest_version,omitempty"`
	UpdateAvailable  bool       `json:"update_available"`
	AutoUpdate       bool       `json:"auto_update"`
	CheckedAt        *time.Time `json:"checked_at,omitempty"`
	Error            string     `json:"error,omitempty"`
	InProgress       bool       `json:"in_progress"`
}

func New(logger *log.Logger, validator *jsonschema.Schema, pubKey *rsa.PublicKey, schemaPath, configPath string) *Server {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Server{
		logger:      logger,
		proxies:     make(map[string]*httputil.ReverseProxy),
		upstreams:   make(map[string]*url.URL),
		manifests:   make(map[string]Manifest),
		manifestErr: make(map[string]string),
		installStat: make(map[string]installStatus),
		updates:     make(map[string]integrationUpdateStatus),
		updating:    make(map[string]bool),
		client:      &http.Client{Timeout: 8 * time.Second},
		validator:   validator,
		pubKey:      pubKey,
		schemaPath:  schemaPath,
		configPath:  configPath,
	}
}

func LoadSchema(schemaPath string) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.LoadURL = func(u string) (io.ReadCloser, error) {
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

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/integrations/registry.json", s.handleRegistry)
	mux.HandleFunc("/registry.json", s.handleRegistry)
	mux.HandleFunc("/integrations/marketplace.json", s.handleMarketplace)
	mux.HandleFunc("/marketplace.json", s.handleMarketplace)
	mux.HandleFunc("/integrations/automation-steps.json", s.handleAutomationStepsCatalog)
	mux.HandleFunc("/automation-steps.json", s.handleAutomationStepsCatalog)
	mux.HandleFunc("/integrations/reload", s.handleReload)
	mux.HandleFunc("/reload", s.handleReload)
	mux.HandleFunc("/integrations/restart-all", s.handleRestartAll)
	mux.HandleFunc("/integrations/restart/", s.handleRestartIntegration)
	mux.HandleFunc("/integrations/install", s.handleInstall)
	mux.HandleFunc("/integrations/uninstall", s.handleUninstall)
	mux.HandleFunc("/integrations/install-status/", s.handleInstallStatus)
	mux.HandleFunc("/integrations/updates", s.handleUpdates)
	mux.HandleFunc("/integrations/update", s.handleUpdateIntegration)
	mux.HandleFunc("/integrations/update-policy", s.handleUpdatePolicy)
	mux.HandleFunc("/integrations/", s.handleProxy)
	mux.HandleFunc("/", s.handleProxy)
	return mux
}

func (s *Server) AddIntegration(ic config.IntegrationConfig) error {
	id := strings.TrimSpace(ic.ID)
	if id == "" {
		return fmt.Errorf("missing id")
	}
	upstream := s.normalizeIntegrationUpstream(ic)
	u, err := url.Parse(upstream)
	if err != nil {
		return fmt.Errorf("parse upstream: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid upstream %q", upstream)
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	origDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		origDirector(r)
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
	proxy.ModifyResponse = func(resp *http.Response) error {
		loc := resp.Header.Get("Location")
		if loc != "" {
			if strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "/integrations/") {
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
				resp.Header.Set("Location", fmt.Sprintf("%s://%s/integrations/%s%s", proto, host, id, loc))
			} else if strings.HasPrefix(loc, u.Scheme+"://"+u.Host) {
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
		if sc := resp.Header.Values("Set-Cookie"); len(sc) > 0 {
			newVals := make([]string, 0, len(sc))
			for _, c := range sc {
				repl := strings.ReplaceAll(c, "Path=/;", fmt.Sprintf("Path=/integrations/%s/;", id))
				repl = strings.ReplaceAll(repl, "Path=/,", fmt.Sprintf("Path=/integrations/%s/;", id))
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

	_ = s.refreshManifest(context.Background(), id, u)
	s.initUpdateState(ic)
	return nil
}

func (s *Server) normalizeIntegrationUpstream(ic config.IntegrationConfig) string {
	id := strings.TrimSpace(ic.ID)
	upstream := strings.TrimSpace(ic.Upstream)
	if id == "" {
		return upstream
	}
	mode, err := s.runtimeMode()
	if err != nil || mode != runtimeHelm {
		return upstream
	}
	defaultByID := defaultUpstreamForID(id)
	releaseName := firstNonEmpty(strings.TrimSpace(ic.HelmReleaseName), s.defaultHelmReleaseName(id))
	namespace := firstNonEmpty(strings.TrimSpace(ic.HelmNamespace), s.defaultHelmNamespace())
	if strings.TrimSpace(releaseName) == "" || strings.TrimSpace(namespace) == "" {
		return upstream
	}
	chartRef := firstNonEmpty(strings.TrimSpace(ic.HelmChartRef), strings.TrimSpace(ic.DevHelmChartRef))
	expectedUpstream := s.defaultHelmUpstream(helmDeploymentSpec{ReleaseName: releaseName, Namespace: namespace, ChartRef: chartRef})
	legacyReleaseUpstream := fmt.Sprintf("http://%s.%s:8099", releaseName, namespace)
	if upstream == "" || upstream == defaultByID || upstream == legacyReleaseUpstream || upstream == expectedUpstream {
		return expectedUpstream
	}
	return upstream
}

func (s *Server) defaultHelmUpstream(spec helmDeploymentSpec) string {
	releaseName := strings.TrimSpace(spec.ReleaseName)
	if releaseName == "" {
		return ""
	}
	namespace := strings.TrimSpace(spec.Namespace)
	if namespace == "" {
		namespace = s.defaultHelmNamespace()
	}
	host := helmServiceName(releaseName, spec.ChartRef)
	if host == "" {
		host = releaseName
	}
	return fmt.Sprintf("http://%s.%s:8099", host, namespace)
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
	s.manifests = make(map[string]Manifest)
	s.manifestErr = make(map[string]string)
	s.installStat = make(map[string]installStatus)
	s.updates = make(map[string]integrationUpdateStatus)
	s.updating = make(map[string]bool)
	s.mu.Unlock()

	for _, ic := range cfg.Integrations {
		if err := s.AddIntegration(ic); err != nil {
			return err
		}
	}

	return nil
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
