package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"
)

func TestRouterHealth(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(t.Context()) }()
	router := NewRouter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), tp.Tracer("test"))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Fatalf("expected ok body, got %q", rr.Body.String())
	}
}
