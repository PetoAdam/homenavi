package http

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/automation-service/internal/auth"
	"github.com/PetoAdam/homenavi/automation-service/internal/engine"
	dbinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/db"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	repo   *dbinfra.Repository
	engine *engine.Engine
	pubKey *rsa.PublicKey

	httpClient          *http.Client
	userServiceURL      string
	integrationProxyURL string
}

func NewServer(repo *dbinfra.Repository, eng *engine.Engine, pubKey *rsa.PublicKey, userServiceURL string, integrationProxyURL string, httpClient *http.Client) *Server {
	hc := httpClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Server{
		repo:                repo,
		engine:              eng,
		pubKey:              pubKey,
		httpClient:          hc,
		userServiceURL:      strings.TrimRight(strings.TrimSpace(userServiceURL), "/"),
		integrationProxyURL: strings.TrimRight(strings.TrimSpace(integrationProxyURL), "/"),
	}
}

func getAuthToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > 7 && strings.HasPrefix(authHeader, "Bearer ") {
		return authHeader[7:]
	}
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}
	return ""
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	r.Get("/api/automation/runs/{run_id}/ws", s.handleRunEventsWS)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	r.Route("/api/automation", func(r chi.Router) {
		if s.pubKey == nil {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					writeError(w, http.StatusInternalServerError, "jwt public key not configured")
				})
			})
			return
		}
		r.Use(auth.JWTAuthMiddlewareRS256(s.pubKey))
		r.Use(auth.RoleAtLeastMiddleware("resident"))

		r.Get("/nodes", s.handleNodes)
		r.Get("/integration-steps", s.handleIntegrationSteps)
		r.Get("/workflows", s.handleListWorkflows)
		r.Post("/workflows", s.handleCreateWorkflow)
		r.Route("/workflows/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetWorkflow)
			r.Put("/", s.handleUpdateWorkflow)
			r.Post("/enable", s.handleEnableWorkflow(true))
			r.Post("/disable", s.handleEnableWorkflow(false))
			r.Post("/run", s.handleRunWorkflow)
			r.Get("/runs", s.handleListRuns)
			r.With(auth.RoleAtLeastMiddleware("admin")).Delete("/", s.handleDeleteWorkflow)
		})
		r.Get("/runs/{run_id}", s.handleGetRun)
	})

	return r
}

func parsePositiveInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty")
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int(r-'0')
		if n > 1_000_000 {
			return 0, errors.New("too large")
		}
	}
	if n <= 0 {
		return 0, errors.New("must be > 0")
	}
	return n, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg, "code": status})
}
