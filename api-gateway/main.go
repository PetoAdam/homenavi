package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"api-gateway/internal/config"
	"api-gateway/internal/observability"
	"api-gateway/internal/router"
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

	// Observability setup
	shutdown, promHandler, tracer := observability.SetupObservability()
	defer shutdown()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// Per-endpoint metrics and tracing middleware
	r.Use(observability.MetricsAndTracingMiddleware(tracer))
	// Correlation ID middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			corrID := r.Header.Get("X-Correlation-ID")
			if corrID == "" {
				corrID = uuid.New().String()
			}
			w.Header().Set("X-Correlation-ID", corrID)
			r = r.WithContext(context.WithValue(r.Context(), "correlation_id", corrID))
			next.ServeHTTP(w, r)
		})
	})

	// Prometheus metrics endpoint
	r.Handle("/metrics", promHandler)
	// Healthcheck endpoint
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	redisClient := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	defer redisClient.Close()

	if pong, err := redisClient.Ping(context.Background()).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	} else {
		log.Printf("Connected to Redis: %s", pong)
	}

	router.RegisterRoutes(r, cfg, redisClient)

	for _, route := range cfg.Routes {
		for _, method := range route.Methods {
			log.Printf("Registered route: %s %s (access: %s)", method, route.Path, route.Access)
		}
	}

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
