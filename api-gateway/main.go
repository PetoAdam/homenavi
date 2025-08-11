package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"api-gateway/internal/config"
	apiMiddleware "api-gateway/internal/middleware"
	"api-gateway/internal/observability"
	"api-gateway/internal/router"
	"crypto/rsa"

	"go.opentelemetry.io/otel/trace"
)

func main() {
	cfgPath := "config/gateway.yaml"
	routesDir := "config/routes"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	// Initialize structured logger (JSON if LOG_FORMAT=json)
	var handler slog.Handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	cfg, err := config.LoadConfig(cfgPath, routesDir)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	slog.Info("config loaded", "listen", cfg.ListenAddr, "routes", len(cfg.Routes))

	shutdown, promHandler, tracer := observability.SetupObservability()
	defer shutdown()

	pubKey := setupJWTKey()
	redisClient := setupRedisClient()
	defer redisClient.Close()

	wsRouter := setupWebSocketRouter(cfg, redisClient, pubKey)
	mainRouter := setupMainRouter(cfg, redisClient, pubKey, promHandler, tracer)

	mux := http.NewServeMux()
	mux.Handle("/ws/", wsRouter)
	mux.Handle("/", mainRouter)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	// Signal handling for graceful shutdown
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("api gateway starting", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server listen error", "error", err)
		}
	}()

	<-stopCh
	slog.Info("shutdown signal received")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	} else {
		slog.Info("server shut down gracefully")
	}
}

func setupJWTKey() *rsa.PublicKey {
	pubKeyPath := os.Getenv("JWT_PUBLIC_KEY_PATH")
	if pubKeyPath == "" {
		slog.Error("JWT_PUBLIC_KEY_PATH not set")
		os.Exit(1)
	}
	pubKey, err := apiMiddleware.LoadRSAPublicKey(pubKeyPath)
	if err != nil {
		slog.Error("failed to load JWT public key", "error", err)
		os.Exit(1)
	}
	return pubKey
}

func setupRedisClient() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	if pong, err := client.Ping(context.Background()).Result(); err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	} else {
		slog.Info("connected to redis", "pong", pong)
	}
	return client
}

func setupWebSocketRouter(cfg *config.GatewayConfig, redisClient *redis.Client, pubKey *rsa.PublicKey) http.Handler {
	r := chi.NewRouter()
	router.RegisterRoutes(r, cfg, redisClient, pubKey)
	return r
}

func setupMainRouter(cfg *config.GatewayConfig, redisClient *redis.Client, pubKey *rsa.PublicKey, promHandler http.Handler, tracer interface{}) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(observability.MetricsAndTracingMiddleware(tracer.(trace.Tracer)))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			corrID := r.Header.Get("X-Correlation-ID")
			if corrID == "" {
				corrID = uuid.New().String()
			}
			w.Header().Set("X-Correlation-ID", corrID)
			r = r.WithContext(context.WithValue(r.Context(), struct{ name string }{"correlation_id"}, corrID))
			next.ServeHTTP(w, r)
		})
	})

	r.Handle("/metrics", promHandler)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	router.RegisterRoutes(r, cfg, redisClient, pubKey)

	r.Get("/api/gateway/routes", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(cfg.Routes)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		slog.Warn("route not found", "method", r.Method, "path", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found", "code": http.StatusNotFound})
	})

	return r
}
