package http

import (
	"net/http"

	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func NewRouter(handler *Handler, promHandler http.Handler, tracer oteltrace.Tracer) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(sharedobs.MetricsAndTracingMiddleware(tracer, "email-service"))
	handler.Register(r, promHandler)
	return r
}
