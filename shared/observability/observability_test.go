package observability

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

func TestMetricsUseBoundedRoutePatterns(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	router := chi.NewRouter()
	router.Get("/users/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := WrapHandler(provider.Tracer("test"), "bounded-route-test", router)
	for _, path := range []string{"/users/one", "/users/two", "/missing/one", "/missing/two"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if got := testutil.ToFloat64(requestCounter.WithLabelValues("bounded-route-test", "/users/{id}", "GET", "204")); got != 2 {
		t.Fatalf("matched requests = %v", got)
	}
	if got := testutil.ToFloat64(requestCounter.WithLabelValues("bounded-route-test", "unmatched", "GET", "404")); got != 2 {
		t.Fatalf("unmatched requests = %v", got)
	}
	for _, span := range recorder.Ended() {
		if strings.Contains(span.Name(), "/one") || strings.Contains(span.Name(), "/two") {
			t.Fatalf("span contains a raw path: %q", span.Name())
		}
	}
}

func TestTracingDoesNotRecordUntrustedRequestData(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	request := httptest.NewRequest(http.MethodGet, "/private-value", nil)
	request.Header.Set("X-Request-ID", "private-value")
	WrapHandler(provider.Tracer("test"), "redaction-test", http.NotFoundHandler()).ServeHTTP(httptest.NewRecorder(), request)
	for _, span := range recorder.Ended() {
		if strings.Contains(span.Name(), "private-value") {
			t.Fatalf("untrusted path recorded in span name: %q", span.Name())
		}
		for _, attribute := range span.Attributes() {
			if strings.Contains(attribute.Value.Emit(), "private-value") {
				t.Fatal("untrusted request data recorded in span attributes")
			}
		}
	}
}

func TestInvalidCollectorConfigDisablesExportWithoutFailingStartup(t *testing.T) {
	requests := make(chan struct{}, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	previousProvider, previousMeter, previousPropagator := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetMeterProvider(previousMeter)
		otel.SetTextMapPropagator(previousPropagator)
	})
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", collector.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", "/missing/collector-ca.pem")
	shutdown, _, tracer, err := SetupObservability("invalid-collector-test")
	if err != nil {
		t.Fatal(err)
	}
	_, span := tracer.Start(t.Context(), "request")
	span.End()
	shutdown()
	select {
	case <-requests:
		t.Fatal("exported with invalid collector credentials")
	default:
	}
	if err := validateCollectorConfig("https://user:secret@example.com/traces"); err == nil {
		t.Fatal("accepted collector credentials in URL")
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

func TestUpgradeCompletesSpanOnce(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	handler := WrapHandler(provider.Tracer("test"), "upgrade-test", http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		FinishHTTPUpgrade(request.Context())
		FinishHTTPUpgrade(request.Context())
		if len(recorder.Ended()) != 1 {
			t.Fatal("upgrade did not finish its span before returning")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ws", nil))
	if len(recorder.Ended()) != 1 {
		t.Fatal("upgrade completed its span more than once")
	}
}

func TestBackgroundOperationUsesNewRootAndReportsErrors(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	}()
	ctx, parent := provider.Tracer("test").Start(context.Background(), "parent")
	_, operation := StartBackgroundOperation(ctx, "background")
	FinishBackgroundOperation(operation, errors.New("failed"))
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 2 || spans[0].Parent().IsValid() || spans[0].Status().Code != codes.Error {
		t.Fatalf("unexpected background spans: %#v", spans)
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
