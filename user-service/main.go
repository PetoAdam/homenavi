package main

import (
	"crypto/rsa"
	"log/slog"
	"net/http"
	"os"

	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/observability"
	"github.com/PetoAdam/homenavi/user-service/db"
	"github.com/PetoAdam/homenavi/user-service/handlers"
	appmw "github.com/PetoAdam/homenavi/user-service/middleware"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
)

func main() {
	shutdownObs, promHandler, tracer := observability.SetupObservability("user-service")
	defer shutdownObs()
	port := envx.String("USER_SERVICE_PORT", "8001")

	db.MustInitDB()

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(observability.MetricsAndTracingMiddleware(tracer, "user-service"))

	r.Handle("/metrics", promHandler)

	// Public endpoints (no JWT)
	r.Post("/users", handlers.HandleUserCreate)
	r.Post("/users/validate", handlers.HandleUserValidate)

	// Protected endpoints: mount a subrouter with JWT verification
	r.Group(func(pr chi.Router) {
		pubKey := loadPublicKey()
		pr.Use(appmw.JWTAuthMiddleware(pubKey))
		// Basic auth: any valid token
		pr.Get("/users/{id}", handlers.HandleUserGet)
		pr.Get("/users", handlers.HandleUserGetByEmail) // both single fetch & list (list has role checks inside)
		pr.Post("/users/{id}/lockout", handlers.HandleLockout)
		pr.Patch("/users/{id}", handlers.HandleUserPatch)
		pr.Delete("/users/{id}", handlers.HandleUserDelete)
	})

	// Structured logger init (use LOG_FORMAT=json for JSON output)
	var handler slog.Handler = slog.NewTextHandler(os.Stdout, nil)
	if envx.String("LOG_FORMAT", "") == "json" {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	}
	slog.SetDefault(slog.New(handler))

	slog.Info("user service starting", "addr", ":"+port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		slog.Error("server stopped", "error", err)
	}
}

func loadPublicKey() *rsa.PublicKey {
	path := envx.String("JWT_PUBLIC_KEY_PATH", "")
	if path == "" {
		slog.Error("JWT_PUBLIC_KEY_PATH not set for user-service")
		os.Exit(1)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Error("failed reading public key", "error", err)
		os.Exit(1)
	}
	pub, err := jwt.ParseRSAPublicKeyFromPEM(data)
	if err != nil {
		slog.Error("failed parsing public key", "error", err)
		os.Exit(1)
	}
	return pub
}
