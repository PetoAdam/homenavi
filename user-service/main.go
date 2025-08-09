package main

import (
	"log"
	"net/http"

	"user-service/db"
	"user-service/handlers"

	"github.com/go-chi/chi/v5"
)

func main() {
	db.MustInitDB()

	r := chi.NewRouter()

	r.Post("/users", handlers.HandleUserCreate)
	r.Get("/users/{id}", handlers.HandleUserGet)
	r.Get("/users", handlers.HandleUserGetByEmail)
	// Listing is handled by GET /users with query params (q,page,page_size)
	r.Patch("/users/{id}", handlers.HandleUserPatch)
	r.Post("/users/{id}/lockout", handlers.HandleLockout)
	r.Delete("/users/{id}", handlers.HandleUserDelete)
	r.Post("/users/validate", handlers.HandleUserValidate)

	log.Println("User service started on :8001")
	http.ListenAndServe(":8001", r)
}
