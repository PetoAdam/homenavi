package router

import (
	"api-gateway/internal/config"
	"api-gateway/internal/middleware"
	"api-gateway/internal/proxy"
	"api-gateway/internal/ratelimit"
	"net/http"
	"strings"

	"crypto/rsa"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

func RegisterRoutes(r chi.Router, cfg *config.GatewayConfig, redisClient *redis.Client, pubKey *rsa.PublicKey) {
	for _, route := range cfg.Routes {
		handler := proxy.MakeProxyHandler(route)
		var h http.Handler = handler
		switch route.Access {
		case "public":
			// No auth
		case "auth":
			h = middleware.JWTAuthMiddlewareRS256(pubKey)(h)
		case "admin":
			h = middleware.JWTAuthMiddlewareRS256(pubKey)(middleware.AdminOnlyMiddleware(h))
		}
		if route.RateLimit != nil {
			limiter := ratelimit.New(redisClient, route.Path, ratelimit.LimiterConfig{RPS: route.RateLimit.RPS, Burst: route.RateLimit.Burst})
			h = limiter.Middleware(ratelimit.KeyByUserOrIP)(h)
		} else if cfg.RateLimit.Enabled {
			limiter := ratelimit.New(redisClient, "global", ratelimit.LimiterConfig{RPS: cfg.RateLimit.RPS, Burst: cfg.RateLimit.Burst})
			h = limiter.Middleware(ratelimit.KeyByUserOrIP)(h)
		}
		for _, method := range route.Methods {
			r.Method(method, route.Path, h)
			if !strings.HasSuffix(route.Path, "/") {
				r.Method(method, route.Path+"/", h)
			}
		}
	}
}
