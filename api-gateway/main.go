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
	cfg, err := config.LoadConfig(cfgPath, routesDir)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Printf("Loaded config: %+v\n", cfg)

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

	log.Printf("API Gateway running on %s", cfg.ListenAddr)
	http.ListenAndServe(cfg.ListenAddr, mux)
}

func setupJWTKey() *rsa.PublicKey {
	pubKeyPath := os.Getenv("JWT_PUBLIC_KEY_PATH")
	if pubKeyPath == "" {
		log.Fatal("JWT_PUBLIC_KEY_PATH not set")
	}
	pubKey, err := apiMiddleware.LoadRSAPublicKey(pubKeyPath)
	if err != nil {
		log.Fatalf("Failed to load JWT public key: %v", err)
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
		log.Fatalf("Failed to connect to Redis: %v", err)
	} else {
		log.Printf("Connected to Redis: %s", pong)
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
		log.Printf("Not found: %s %s", r.Method, r.URL.Path)
		http.Error(w, "Not found", http.StatusNotFound)
	})

	return r
}
