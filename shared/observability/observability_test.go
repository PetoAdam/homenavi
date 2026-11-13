package observability

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

type hijackableRecorder struct {
	*httptest.ResponseRecorder
}

func TestCollectorConfigurationFailsClosed(t *testing.T) {
	for _, endpoint := range []string{"htps://collector.example/v1/traces", "https://user:secret@collector.example/v1/traces", "https://collector.example/%zz"} {
		if err := validateCollectorConfig(endpoint); err == nil {
			t.Errorf("accepted invalid endpoint %q", endpoint)
		}
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", "/missing/collector-ca.pem")
	if err := validateCollectorConfig("https://collector.example/v1/traces"); err == nil {
		t.Fatal("accepted missing collector CA")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE", "/missing/client.pem")
	if err := validateCollectorConfig("https://collector.example/v1/traces"); err == nil {
		t.Fatal("accepted incomplete client credentials")
	}
}

func TestInvalidCollectorConfigurationDoesNotExport(t *testing.T) {
	requests := make(chan struct{}, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
		writer.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	previous := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	for _, test := range []struct{ endpoint, ca string }{
		{endpoint: strings.Replace(collector.URL, "http://", "htps://", 1)},
		{endpoint: collector.URL, ca: "/missing/collector-ca.pem"},
	} {
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", test.endpoint)
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", test.ca)
		shutdown, _, tracer, err := SetupObservability("invalid-collector-test")
		if err != nil {
			t.Fatal(err)
		}
		_, span := tracer.Start(t.Context(), "request")
		span.End()
		shutdown()
		select {
		case <-requests:
			t.Fatal("exported with invalid collector configuration")
		default:
		}
	}
}

func TestUnresponsiveCollectorDoesNotBlockRequestsOrShutdown(t *testing.T) {
	release := make(chan struct{})
	collector := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	defer collector.Close()
	defer close(release)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", collector.URL+"/v1/traces")
	previous := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	shutdown, _, tracer, err := SetupObservability("unresponsive-collector-test")
	if err != nil {
		t.Fatal(err)
	}
	handler := WrapHandler(tracer, "unresponsive-collector-test", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("request failed with collector unavailable: %d", response.Code)
	}
	done := make(chan struct{})
	go func() { shutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("shutdown blocked on collector")
	}
}

func TestBackgroundOperationUsesIndependentRootAndRedactedError(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	ctx, cancel := context.WithCancel(t.Context())
	ctx, parent := provider.Tracer("test").Start(ctx, "request")
	operationCtx, span := StartBackgroundOperation(ctx, "registry.reconcile")
	cancel()
	if operationCtx.Err() != context.Canceled {
		t.Fatal("background operation lost cancellation")
	}
	FinishBackgroundOperation(span, errors.New("secret payload"))
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 2 || spans[0].Parent().IsValid() || spans[0].SpanContext().TraceID() == parent.SpanContext().TraceID() || spans[0].Status().Code != codes.Error || strings.Contains(spans[0].Status().Description, "secret") {
		t.Fatal("background span did not use an independent root and bounded error")
	}
}

func TestExternalHTTPClientTracesWithoutPropagatingHeadersOrSecrets(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		for _, header := range []string{"traceparent", "tracestate", "baggage"} {
			if request.Header.Get(header) != "" {
				t.Errorf("external request leaked %s", header)
			}
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/?apikey=secret-value", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("baggage", "secret=value")
	client := InstrumentExternalHTTPClient(InstrumentHTTPClient(server.Client()))
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 2 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() || strings.Contains(fmt.Sprint(spans[0].Attributes()), "secret") {
		t.Fatal("external client span is disconnected or exposes credentials")
	}
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

func TestWrapHandlerCreatesServerSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer func() { _ = tp.Shutdown(t.Context()) }()

	h := WrapHandler(tp.Tracer("test"), "test-service", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].SpanKind() != oteltrace.SpanKindServer {
		t.Fatalf("expected one server span, got %v", spans)
	}
}

func TestGatewayCanResetUntrustedTraceContext(t *testing.T) {
	t.Setenv("HOMENAVI_TRACE_INGRESS_NEW_ROOT", "")
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder), trace.WithSampler(trace.ParentBased(trace.AlwaysSample())))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	handler := WrapHandler(provider.Tracer("test"), "api-gateway", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-00")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	spans := recorder.Ended()
	incomingID, _ := oteltrace.TraceIDFromHex("11111111111111111111111111111111")
	if len(spans) != 1 || spans[0].Parent().IsValid() || spans[0].SpanContext().TraceID() == incomingID || len(spans[0].Links()) != 2 || spans[0].Links()[0].SpanContext.TraceID() != incomingID || spans[0].Links()[1].SpanContext.TraceID() != NamespaceIngressTraceID(incomingID) || spans[0].Links()[0].SpanContext.SpanID().String() != "2222222222222222" || spans[0].Links()[1].SpanContext.SpanID().String() != "2222222222222222" || response.Header().Get("Trace-ID") != spans[0].SpanContext().TraceID().String() {
		t.Fatal("gateway did not link and independently sample the untrusted incoming trace")
	}
}

func TestServerSpanUsesRouteTemplate(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	router := chi.NewRouter()
	router.Get("/devices/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	WrapHandler(provider.Tracer("test"), "test-service", router).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/devices/secret", nil))
	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Name() != "GET /devices/{id}" || strings.Contains(fmt.Sprint(spans[0].Attributes()), "secret") {
		t.Fatalf("unexpected route span: %v", spans)
	}
}

func TestHTTPMetricsUseBoundedRouteLabels(t *testing.T) {
	provider := trace.NewTracerProvider()
	defer func() { _ = provider.Shutdown(t.Context()) }()
	t.Cleanup(func() {
		requestCounter.DeleteLabelValues("route-label-test", "/runs/{id}", http.MethodGet, "204")
		requestCounter.DeleteLabelValues("route-label-test", "unmatched", http.MethodGet, "404")
		requestCounter.DeleteLabelValues("route-label-test", "unmatched", "_OTHER", "405")
	})
	router := chi.NewRouter()
	router.Get("/runs/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := WrapHandler(provider.Tracer("test"), "route-label-test", router)
	for _, path := range []string{"/runs/first", "/runs/second", "/unknown/first", "/unknown/second"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	for _, method := range []string{"CUSTOM_ONE", "CUSTOM_TWO"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/unknown", nil))
	}
	for _, test := range []struct{ endpoint, status string }{{"/runs/{id}", "204"}, {"unmatched", "404"}} {
		counter, err := requestCounter.GetMetricWithLabelValues("route-label-test", test.endpoint, http.MethodGet, test.status)
		if err != nil {
			t.Fatal(err)
		}
		metric := &dto.Metric{}
		if err := counter.Write(metric); err != nil || metric.GetCounter().GetValue() != 2 {
			t.Fatalf("unexpected %s metric: %v, %v", test.endpoint, metric, err)
		}
	}
	counter, err := requestCounter.GetMetricWithLabelValues("route-label-test", "unmatched", "_OTHER", "405")
	if err != nil {
		t.Fatal(err)
	}
	metric := &dto.Metric{}
	if err := counter.Write(metric); err != nil || metric.GetCounter().GetValue() != 2 {
		t.Fatalf("unexpected custom method metric: %v, %v", metric, err)
	}
}

func TestWebSocketUpgradeMetricsUseRouteTemplate(t *testing.T) {
	provider := trace.NewTracerProvider()
	defer func() { _ = provider.Shutdown(t.Context()) }()
	t.Cleanup(func() {
		requestCounter.DeleteLabelValues("websocket-label-test", "/ws/runs/{id}", http.MethodGet, "101")
	})
	router := chi.NewRouter()
	router.Use(MetricsAndTracingMiddleware(provider.Tracer("test"), "websocket-label-test"))
	router.Get("/ws/runs/{id}", func(_ http.ResponseWriter, request *http.Request) {
		FinishHTTPUpgrade(request.Context())
	})
	for _, path := range []string{"/ws/runs/first", "/ws/runs/second"} {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	counter, err := requestCounter.GetMetricWithLabelValues("websocket-label-test", "/ws/runs/{id}", http.MethodGet, "101")
	if err != nil {
		t.Fatal(err)
	}
	metric := &dto.Metric{}
	if err := counter.Write(metric); err != nil || metric.GetCounter().GetValue() != 2 {
		t.Fatalf("unexpected WebSocket metric: %v, %v", metric, err)
	}
}

func TestServerSpanUsesStandardMuxPattern(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	router := http.NewServeMux()
	router.HandleFunc("GET /devices/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	WrapHandler(provider.Tracer("test"), "test-service", router).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/devices/secret", nil))
	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Name() != "GET /devices/{id}" || strings.Contains(fmt.Sprint(spans[0].Attributes()), "secret") {
		t.Fatalf("unexpected route span: %v", spans)
	}
}

func TestWrapHandlerDoesNotRecordUntrustedRequestData(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer func() { _ = tp.Shutdown(t.Context()) }()
	h := WrapHandler(tp.Tracer("test"), "test-service", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/secret-value", nil)
	request.Header.Set("X-Request-ID", "secret-value")
	h.ServeHTTP(httptest.NewRecorder(), request)
	spans := recorder.Ended()
	if len(spans) != 1 || strings.Contains(spans[0].Name(), "secret-value") {
		t.Fatal("request path appeared in the span name")
	}
	for _, attr := range spans[0].Attributes() {
		if strings.Contains(attr.Value.Emit(), "secret-value") {
			t.Fatal("untrusted request data appeared in span attributes")
		}
	}
}

func TestSetupObservabilityContinuesWhenExporterFails(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://collector.invalid/v1/traces")
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	previousMeter := otel.GetMeterProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		otel.SetMeterProvider(previousMeter)
	})
	shutdown, metrics, tracer, err := setupObservability("availability-test", func(context.Context, string) (trace.SpanExporter, error) {
		return nil, errors.New("collector credentials must not be logged")
	})
	if err != nil || shutdown == nil || metrics == nil || tracer == nil {
		t.Fatalf("telemetry failure prevented startup: %v", err)
	}
	defer shutdown()
	handler := WrapHandler(tracer, "availability-test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("request failed after exporter error: %d", response.Code)
	}
}

func TestSetupObservabilityExportsConnectedOTLPSpans(t *testing.T) {
	requests := make(chan *collectortrace.ExportTraceServiceRequest, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("unexpected OTLP path %q", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var request collectortrace.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- &request
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", collector.URL+"/v1/traces")
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	shutdown, _, tracer, err := SetupObservability("otlp-test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, root := tracer.Start(t.Context(), "root")
	_, child := tracer.Start(ctx, "child")
	child.End()
	root.End()
	shutdown()
	traceID := root.SpanContext().TraceID()
	spanID := root.SpanContext().SpanID()
	select {
	case request := <-requests:
		var spansFound int
		for _, resource := range request.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				for _, span := range scope.Spans {
					if span.Name == "child" && string(span.TraceId) == string(traceID[:]) && string(span.ParentSpanId) == string(spanID[:]) {
						spansFound++
					}
				}
			}
		}
		if spansFound != 1 {
			t.Fatal("OTLP export did not preserve the child trace and parent IDs")
		}
	default:
		t.Fatal("collector received no OTLP traces")
	}
}

func TestJaegerQueriesConnectedMQTTTrace(t *testing.T) {
	collectorURL := os.Getenv("HOMENAVI_TEST_JAEGER_OTLP")
	queryURL := os.Getenv("HOMENAVI_TEST_JAEGER_QUERY")
	if collectorURL == "" || queryURL == "" {
		t.Skip("Jaeger integration endpoints are not configured")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", collectorURL+"/v1/traces")
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	shutdown, _, tracer, err := SetupObservability("homenavi-jaeger-test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, root := tracer.Start(t.Context(), "command")
	producerCtx, producer := StartMessageProducer(ctx)
	envelope := map[string]any{"ts": time.Now().UnixMilli(), "corr": "test-command"}
	InjectMessageContext(producerCtx, envelope)
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	producer.End()
	_, consumer := StartMessageConsumer(t.Context(), payload, false, time.Now())
	consumer.End()
	root.End()
	shutdown()

	type reference struct {
		RefType string `json:"refType"`
		SpanID  string `json:"spanID"`
	}
	type span struct {
		TraceID    string      `json:"traceID"`
		SpanID     string      `json:"spanID"`
		References []reference `json:"references"`
	}
	var result struct {
		Data []struct {
			Spans []span `json:"spans"`
		} `json:"data"`
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}
	for {
		response, queryErr := client.Get(queryURL + "/api/traces/" + root.SpanContext().TraceID().String())
		if queryErr == nil {
			decodeErr := json.NewDecoder(response.Body).Decode(&result)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && len(result.Data) > 0 && len(result.Data[0].Spans) >= 3 {
				break
			}
		}
		select {
		case <-deadline.C:
			t.Fatal("Jaeger did not return the exported trace")
		case <-ticker.C:
		}
	}
	parents := make(map[string]string)
	for _, item := range result.Data[0].Spans {
		if item.TraceID != root.SpanContext().TraceID().String() {
			t.Fatal("Jaeger returned a span with the wrong trace ID")
		}
		for _, ref := range item.References {
			if ref.RefType == "CHILD_OF" {
				parents[item.SpanID] = ref.SpanID
			}
		}
	}
	if parents[producer.SpanContext().SpanID().String()] != root.SpanContext().SpanID().String() || parents[consumer.SpanContext().SpanID().String()] != producer.SpanContext().SpanID().String() {
		t.Fatal("Jaeger did not preserve the MQTT producer and consumer parent chain")
	}
}

func TestInstrumentHTTPTransportConnectsServerSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = tp.Shutdown(t.Context())
	})

	downstream := httptest.NewServer(WrapHandler(tp.Tracer("downstream"), "downstream", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("baggage") != "" {
			t.Error("baggage must not be forwarded by default")
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	defer downstream.Close()
	client := &http.Client{Transport: InstrumentHTTPTransport(nil)}
	upstream := WrapHandler(tp.Tracer("upstream"), "upstream", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, downstream.URL, nil)
		if err != nil {
			t.Error(err)
			return
		}
		request.Header.Set("baggage", "secret=value")
		response, err := client.Do(request)
		if err != nil {
			t.Error(err)
			return
		}
		_ = response.Body.Close()
	}))
	upstream.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	spans := recorder.Ended()
	if len(spans) != 3 {
		t.Fatalf("expected three spans, got %d", len(spans))
	}
	downstreamSpan, clientSpan, upstreamSpan := spans[0], spans[1], spans[2]
	if clientSpan.SpanKind() != oteltrace.SpanKindClient || downstreamSpan.Parent().SpanID() != clientSpan.SpanContext().SpanID() || clientSpan.Parent().SpanID() != upstreamSpan.SpanContext().SpanID() || downstreamSpan.SpanContext().TraceID() != upstreamSpan.SpanContext().TraceID() {
		t.Fatal("HTTP spans do not form the expected parent chain")
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
