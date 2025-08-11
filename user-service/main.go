package main

import (
	"crypto/rsa"
	"log/slog"
	"net/http"
	"os"

	"user-service/db"
	"user-service/handlers"
	"user-service/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

func main() {
	db.MustInitDB()

	r := chi.NewRouter()

	// Public endpoints (no JWT)
	r.Post("/users", handlers.HandleUserCreate)
	r.Post("/users/validate", handlers.HandleUserValidate)

	// Protected endpoints: mount a subrouter with JWT verification
	r.Group(func(pr chi.Router) {
		pubKey := loadPublicKey()
		pr.Use(middleware.JWTAuthMiddleware(pubKey))
		// Basic auth: any valid token
		pr.Get("/users/{id}", handlers.HandleUserGet)
		pr.Get("/users", handlers.HandleUserGetByEmail) // both single fetch & list (list has role checks inside)
		pr.Patch("/users/{id}", handlers.HandleUserPatch)
		pr.Post("/users/{id}/lockout", handlers.HandleLockout)
		pr.Delete("/users/{id}", handlers.HandleUserDelete)
	})

	// Structured logger init (use LOG_FORMAT=json for JSON output)
	var handler slog.Handler = slog.NewTextHandler(os.Stdout, nil)
	if os.Getenv("LOG_FORMAT") == "json" { handler = slog.NewJSONHandler(os.Stdout, nil) }
	slog.SetDefault(slog.New(handler))

	slog.Info("user service starting", "addr", ":8001")
	if err := http.ListenAndServe(":8001", r); err != nil {
		slog.Error("server stopped", "error", err)
	}
}

func loadPublicKey() *rsa.PublicKey {
	path := os.Getenv("JWT_PUBLIC_KEY_PATH")
	if path == "" { slog.Error("JWT_PUBLIC_KEY_PATH not set for user-service"); os.Exit(1) }
	data, err := os.ReadFile(path)
	if err != nil { slog.Error("failed reading public key", "error", err); os.Exit(1) }
	pub, err := jwt.ParseRSAPublicKeyFromPEM(data)
	if err != nil { slog.Error("failed parsing public key", "error", err); os.Exit(1) }
	return pub
}
