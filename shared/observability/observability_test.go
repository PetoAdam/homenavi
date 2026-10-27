package observability

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"
)

type hijackableRecorder struct {
	*httptest.ResponseRecorder
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

func TestWrapHandlerSetsTraceID(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(t.Context()) }()

	h := WrapHandler(tp.Tracer("test"), "test-service", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rr.Code)
	}
	if rr.Header().Get("Trace-ID") == "" {
		t.Fatal("expected Trace-ID header to be set")
	}
}

func TestWrapHandlerSetsTraceIDBeforeBodyWrite(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(t.Context()) }()

	h := WrapHandler(tp.Tracer("test"), "test-service", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Header().Get("Trace-ID") == "" {
		t.Fatal("expected Trace-ID header to be set before body write")
	}
}

func TestWrapHandlerPreservesHijacker(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(t.Context()) }()

	h := WrapHandler(tp.Tracer("test"), "test-service", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Hijacker); !ok {
			t.Fatal("expected wrapped response writer to implement http.Hijacker")
		}
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))

	rr := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ws", nil))
	if rr.Code != http.StatusSwitchingProtocols {
		t.Fatalf("expected status %d, got %d", http.StatusSwitchingProtocols, rr.Code)
	}
}

func TestWithMetricsEndpointExposesMetricsAndWrapsOtherRoutes(t *testing.T) {
	tp := trace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(t.Context()) }()

	metricsCalled := false
	metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metricsCalled = true
		_, _ = io.WriteString(w, "metrics-ok")
	})

	wrapped := WithMetricsEndpoint(metricsHandler, tp.Tracer("test"), "test-service", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))

	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !metricsCalled {
		t.Fatal("expected metrics handler to be called")
	}
	if rr.Header().Get("Trace-ID") != "" {
		t.Fatal("expected /metrics to bypass tracing middleware")
	}
	if rr.Body.String() != "metrics-ok" {
		t.Fatalf("expected metrics body, got %q", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	wrapped.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Header().Get("Trace-ID") == "" {
		t.Fatal("expected traced route to set Trace-ID")
	}
}
