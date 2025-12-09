package router

import (
	"api-gateway/internal/config"
	"api-gateway/internal/middleware"
	"api-gateway/internal/proxy"
	"api-gateway/internal/ratelimit"
	"crypto/rsa"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(r chi.Router, cfg *config.GatewayConfig, redisClient *redis.Client, pubKey *rsa.PublicKey, _ any) {
	for _, route := range cfg.Routes {
		var h http.Handler
		switch route.Type {
		case "websocket":
			h = wrapWithAccessControl(pubKey, route.Access, proxy.MakeWebSocketProxyHandler(route))
		case "websocket-mqtt":
			// Treat identical to generic websocket reverse proxy; upstream must be a native MQTT WS listener.
			h = wrapWithAccessControl(pubKey, route.Access, proxy.MakeWebSocketProxyHandler(route))
		default: // "rest" or empty
			h = wrapWithAccessControl(pubKey, route.Access, proxy.MakeRestProxyHandler(route))
		}

		// Apply rate limiting to both REST and WebSocket routes
		if route.RateLimit != nil {
			limiter := ratelimit.New(redisClient, route.Path, ratelimit.LimiterConfig{RPS: route.RateLimit.RPS, Burst: route.RateLimit.Burst})
			h = limiter.Middleware(ratelimit.KeyByUserOrIP)(h)
		} else if cfg.RateLimit.Enabled {
			limiter := ratelimit.New(redisClient, "global", ratelimit.LimiterConfig{RPS: cfg.RateLimit.RPS, Burst: cfg.RateLimit.Burst})
			h = limiter.Middleware(ratelimit.KeyByUserOrIP)(h)
		}

		// Always remove trailing slash before registering route
		path := route.Path
		if len(path) > 1 && strings.HasSuffix(path, "/") {
			path = strings.TrimRight(path, "/")
		}
		for _, method := range route.Methods {
			r.Method(method, path, h)
		}
	}
}

// wrapWithAccessControl applies access control for both REST and WebSocket routes
func wrapWithAccessControl(pubKey *rsa.PublicKey, access string, next http.Handler) http.Handler {
	switch access {
	case "public":
		return next
	case "auth":
		return middleware.JWTAuthMiddlewareRS256(pubKey)(next)
	case "resident":
		return middleware.JWTAuthMiddlewareRS256(pubKey)(middleware.RoleAtLeastMiddleware("resident")(next))
	case "admin":
		return middleware.JWTAuthMiddlewareRS256(pubKey)(middleware.RoleAtLeastMiddleware("admin")(next))
	default:
		return next
	}
}
