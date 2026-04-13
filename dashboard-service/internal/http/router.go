package http

import (
	"crypto/rsa"
	"net/http"

	"github.com/PetoAdam/homenavi/dashboard-service/internal/auth"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func NewRouter(handler *Handler, promHandler http.Handler, tracer oteltrace.Tracer, pubKey *rsa.PublicKey) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(sharedobs.MetricsAndTracingMiddleware(tracer, "dashboard-service"))

	r.Handle("/metrics", promHandler)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(api chi.Router) {
		api.Use(auth.JWTAuthMiddleware(pubKey))
		api.Use(auth.RoleAtLeastMiddleware("resident"))
		api.Get("/widgets/catalog", handler.HandleCatalog)
		api.Get("/widgets/weather", handler.HandleWeather)
		api.Route("/dashboard", func(dr chi.Router) {
			dr.Get("/me", handler.HandleGetMyDashboard)
			dr.Put("/me", handler.HandlePutMyDashboard)
			dr.Group(func(admin chi.Router) {
				admin.Use(auth.RoleAtLeastMiddleware("admin"))
				admin.Get("/default", handler.HandleGetDefaultDashboard)
				admin.Put("/default", handler.HandlePutDefaultDashboard)
			})
		})
	})

	return r
}
