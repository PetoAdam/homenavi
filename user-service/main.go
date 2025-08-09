package main

import (
	"crypto/rsa"
	"log"
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

	log.Println("User service started on :8001")
	http.ListenAndServe(":8001", r)
}

func loadPublicKey() *rsa.PublicKey {
	path := os.Getenv("JWT_PUBLIC_KEY_PATH")
	if path == "" {
		log.Fatal("JWT_PUBLIC_KEY_PATH not set for user-service")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("failed reading public key: %v", err)
	}
	pub, err := jwt.ParseRSAPublicKeyFromPEM(data)
	if err != nil {
		log.Fatalf("failed parsing public key: %v", err)
	}
	return pub
}
