package http

import (
	"net/http"

	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func NewRouter(promHandler http.Handler, tracer oteltrace.Tracer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	return sharedobs.WrapHandler(tracer, "mock-adapter", mux)
}
