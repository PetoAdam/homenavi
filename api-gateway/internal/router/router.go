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
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(r chi.Router, cfg *config.GatewayConfig, redisClient *redis.Client, pubKey *rsa.PublicKey) {
	for _, route := range cfg.Routes {
		var h http.Handler
		switch route.Type {
		case "websocket":
			h = wrapWithAccessControl(pubKey, route.Access, true, proxy.MakeWebSocketProxyHandler(route))
		default: // "rest" or empty
			h = wrapWithAccessControl(pubKey, route.Access, false, proxy.MakeRestProxyHandler(route))
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
func wrapWithAccessControl(pubKey *rsa.PublicKey, access string, isWebSocket bool, next http.Handler) http.Handler {
	switch access {
	case "public":
		return next
	case "auth":
		if isWebSocket {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tokenStr := extractToken(r)
				if tokenStr == "" {
					http.Error(w, "Missing token", http.StatusUnauthorized)
					return
				}
				token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
					return pubKey, nil
				})
				if err != nil || !token.Valid {
					http.Error(w, "Invalid token", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
			})
		}
		// REST: use middleware
		return middleware.JWTAuthMiddlewareRS256(pubKey)(next)
	case "admin":
		if isWebSocket {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tokenStr := extractToken(r)
				if tokenStr == "" {
					http.Error(w, "Missing token", http.StatusUnauthorized)
					return
				}
				token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
					return pubKey, nil
				})
				if err != nil || !token.Valid {
					http.Error(w, "Invalid token", http.StatusUnauthorized)
					return
				}
				claims, ok := token.Claims.(jwt.MapClaims)
				if !ok || claims["role"] != "admin" {
					http.Error(w, "Admin only", http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
			})
		}
		// REST: use middleware
		return middleware.JWTAuthMiddlewareRS256(pubKey)(middleware.AdminOnlyMiddleware(next))
	default:
		return next
	}
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}
