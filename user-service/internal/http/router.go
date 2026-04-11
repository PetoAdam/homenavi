package http

import (
	"crypto/rsa"
	"net/http"

	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/PetoAdam/homenavi/user-service/internal/auth"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func NewRouter(handler *UsersHandler, promHandler http.Handler, tracer oteltrace.Tracer, pubKey *rsa.PublicKey) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(sharedobs.MetricsAndTracingMiddleware(tracer, "user-service"))

	r.Handle("/metrics", promHandler)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/users", handler.HandleCreate)
	r.Post("/users/validate", handler.HandleValidate)

	r.Group(func(pr chi.Router) {
		pr.Use(auth.JWTAuthMiddleware(pubKey))
		pr.Get("/users/{id}", handler.HandleGet)
		pr.Get("/users", handler.HandleQuery)
		pr.Post("/users/{id}/lockout", handler.HandleLockout)
		pr.Patch("/users/{id}", handler.HandlePatch)
		pr.Delete("/users/{id}", handler.HandleDelete)
	})

	return r
}
