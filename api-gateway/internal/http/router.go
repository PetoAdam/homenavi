package http

import (
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/PetoAdam/homenavi/api-gateway/internal/gateway"
	apiMiddleware "github.com/PetoAdam/homenavi/api-gateway/internal/middleware"
	"github.com/PetoAdam/homenavi/api-gateway/internal/proxy"
	"github.com/PetoAdam/homenavi/api-gateway/internal/ratelimit"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func NewRootRouter(wsRouter, mainRouter http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/ws/", wsRouter)
	mux.Handle("/", mainRouter)
	return mux
}

func NewWebSocketRouter(cfg gateway.Config, redisClient redis.UniversalClient, pubKey *rsa.PublicKey) http.Handler {
	r := chi.NewRouter()
	registerConfiguredRoutes(r, cfg, redisClient, pubKey)
	return r
}

func NewMainRouter(cfg gateway.Config, redisClient redis.UniversalClient, pubKey *rsa.PublicKey, promHandler http.Handler, tracer oteltrace.Tracer, corsAllowOrigins string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(CORS(corsAllowOrigins))
	r.Use(sharedobs.MetricsAndTracingMiddleware(tracer, "api-gateway"))
	r.Use(CorrelationID())

	r.Handle("/metrics", promHandler)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	registerConfiguredRoutes(r, cfg, redisClient, pubKey)

	r.Get("/api/gateway/routes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(cfg.Routes)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		slog.Warn("route not found", "method", r.Method, "path", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found", "code": http.StatusNotFound})
	})

	return r
}

func registerConfiguredRoutes(r chi.Router, cfg gateway.Config, redisClient redis.UniversalClient, pubKey *rsa.PublicKey) {
	for _, route := range cfg.Routes {
		var h http.Handler
		switch route.Type {
		case "websocket", "websocket-mqtt":
			h = wrapWithAccessControl(pubKey, route.Access, proxy.MakeWebSocketProxyHandler(route))
		default:
			h = wrapWithAccessControl(pubKey, route.Access, proxy.MakeRestProxyHandler(route))
		}

		if route.RateLimit != nil {
			limiter := ratelimit.New(redisClient, route.Path, ratelimit.LimiterConfig{RPS: route.RateLimit.RPS, Burst: route.RateLimit.Burst})
			h = limiter.Middleware(ratelimit.KeyByUserOrIP)(h)
		} else if cfg.RateLimit.Enabled {
			limiter := ratelimit.New(redisClient, "global", ratelimit.LimiterConfig{RPS: cfg.RateLimit.RPS, Burst: cfg.RateLimit.Burst})
			h = limiter.Middleware(ratelimit.KeyByUserOrIP)(h)
		}

		path := route.Path
		if len(path) > 1 && strings.HasSuffix(path, "/") {
			path = strings.TrimRight(path, "/")
		}
		for _, method := range route.Methods {
			r.Method(method, path, h)
		}
	}
}

func wrapWithAccessControl(pubKey *rsa.PublicKey, access string, next http.Handler) http.Handler {
	switch access {
	case "public":
		return next
	case "auth":
		return apiMiddleware.JWTAuthMiddlewareRS256(pubKey)(next)
	case "resident":
		return apiMiddleware.JWTAuthMiddlewareRS256(pubKey)(apiMiddleware.RoleAtLeastMiddleware("resident")(next))
	case "admin":
		return apiMiddleware.JWTAuthMiddlewareRS256(pubKey)(apiMiddleware.RoleAtLeastMiddleware("admin")(next))
	default:
		return next
	}
}
