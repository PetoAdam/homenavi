package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"api-gateway/internal/config"
	"api-gateway/internal/ratelimit"
)

func main() {
	cfgPath := "config/gateway.yaml"
	routesDir := "config/routes"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg, err := config.LoadConfig(cfgPath, routesDir)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Printf("Loaded config: %+v\n", cfg)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Initialize Redis client for rate limiting
	redisClient := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"), // e.g. "redis:6379"
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	defer redisClient.Close()

	// Ping Redis to confirm connection
	if pong, err := redisClient.Ping(context.Background()).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	} else {
		log.Printf("Connected to Redis: %s", pong)
	}

	for _, route := range cfg.Routes {
		registerRoute(r, route, cfg, redisClient)
	}

	for _, route := range cfg.Routes {
		for _, method := range route.Methods {
			log.Printf("Registered route: %s %s (access: %s)", method, route.Path, route.Access)
		}
	}

	// Not found handler
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Not found: %s %s", r.Method, r.URL.Path)
		http.Error(w, "Not found", http.StatusNotFound)
	})

	r.Get("/api/gateway/routes", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(cfg.Routes)
	})

	log.Printf("API Gateway running on %s", cfg.ListenAddr)
	http.ListenAndServe(cfg.ListenAddr, r)
}

func registerRoute(r chi.Router, route config.RouteConfig, cfg *config.GatewayConfig, redisClient *redis.Client) {
	handler := makeProxyHandler(route)
	var h http.Handler = handler
	// Wrap with access control
	switch route.Access {
	case "public":
		// No auth
	case "auth":
		h = jwtAuthMiddleware(cfg.JWTSecret)(h)
	case "admin":
		h = jwtAuthMiddleware(cfg.JWTSecret)(adminOnlyMiddleware(h))
	}
	// Add per-route or global rate limiting
	if route.RateLimit != nil {
		limiter := ratelimit.New(redisClient, route.Path, ratelimit.LimiterConfig{RPS: route.RateLimit.RPS, Burst: route.RateLimit.Burst})
		h = limiter.Middleware(ratelimit.KeyByUserOrIP)(h)
	} else if cfg.RateLimit.Enabled {
		limiter := ratelimit.New(redisClient, "global", ratelimit.LimiterConfig{RPS: cfg.RateLimit.RPS, Burst: cfg.RateLimit.Burst})
		h = limiter.Middleware(ratelimit.KeyByUserOrIP)(h)
	}
	// Register both with and without trailing slash
	for _, method := range route.Methods {
		r.Method(method, route.Path, h)
		if !strings.HasSuffix(route.Path, "/") {
			r.Method(method, route.Path+"/", h)
		}
	}
}

func makeProxyHandler(route config.RouteConfig) http.HandlerFunc {
	upstreamURL, err := url.Parse(route.Upstream)
	if err != nil {
		panic("Invalid upstream URL: " + route.Upstream)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Proxying %s %s to upstream %s", r.Method, r.URL.Path, upstreamURL)
		proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
		origDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			origDirector(req)
			// Replace chi URL params in upstream path (e.g., {id})
			ctx := chi.RouteContext(r.Context())
			path := upstreamURL.Path
			if ctx != nil {
				for i, key := range ctx.URLParams.Keys {
					val := ctx.URLParams.Values[i]
					path = strings.ReplaceAll(path, "{"+key+"}", val)
				}
			}
			req.URL.Path = path
			req.URL.RawQuery = r.URL.RawQuery
		}
		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			log.Printf("Proxy error for %s %s to %s: %v", r.Method, r.URL.Path, upstreamURL, err)
			http.Error(rw, "Upstream error: "+err.Error(), http.StatusBadGateway)
		}
		proxy.ServeHTTP(w, r)
	}
}

type Claims struct {
	Role string `json:"role"`
	Name string `json:"name"`
	jwt.RegisteredClaims
}

func jwtAuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				http.Error(w, "Missing token", http.StatusUnauthorized)
				return
			}
			token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			claims, ok := token.Claims.(*Claims)
			if !ok {
				http.Error(w, "Invalid claims", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), "claims", claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func adminOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value("claims").(*Claims)
		if !ok || claims.Role != "admin" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Admin only"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
