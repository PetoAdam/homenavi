package http

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/api-gateway/internal/gateway"
	"github.com/PetoAdam/homenavi/shared/observability"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestMainRouterHealth(t *testing.T) {
	router := NewMainRouter(gateway.Config{}, nil, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), otel.Tracer("test"), "")
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

func TestNormalizeBrowserTraceIdentity(t *testing.T) {
	t.Setenv("HOMENAVI_TRACE_INGRESS_NEW_ROOT", "true")
	jsonBody := []byte(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"auth-service"}}]},"scopeSpans":[{"scope":{"name":"backend"},"spans":[{"kind":2,"traceId":"11111111111111111111111111111111","links":[{}]}]}]}]}`)
	clean, err := normalizeBrowserTraces(jsonBody, "application/json")
	if err != nil || strings.Contains(string(clean), "auth-service") || strings.Contains(string(clean), "11111111111111111111111111111111") || strings.Contains(string(clean), `"links"`) || strings.Contains(string(clean), `"kind":2`) || !strings.Contains(string(clean), `"kind":3`) {
		t.Fatalf("unexpected normalized JSON: %s (%v)", clean, err)
	}
	request := &collectortracev1.ExportTraceServiceRequest{ResourceSpans: []*tracev1.ResourceSpans{{
		Resource:   &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "auth-service"}}}}},
		ScopeSpans: []*tracev1.ScopeSpans{{Spans: []*tracev1.Span{{Kind: tracev1.Span_SPAN_KIND_SERVER, TraceId: bytes.Repeat([]byte{0x11}, 16), Links: []*tracev1.Span_Link{{}}}}}},
	}}}
	encoded, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	clean, err = normalizeBrowserTraces(encoded, "application/x-protobuf")
	if err != nil {
		t.Fatal(err)
	}
	var result collectortracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(clean, &result); err != nil {
		t.Fatal(err)
	}
	resource := result.ResourceSpans[0]
	if resource.Resource.Attributes[0].Value.GetStringValue() != "homenavi-frontend" || resource.ScopeSpans[0].Spans[0].Kind != tracev1.Span_SPAN_KIND_CLIENT || bytes.Equal(resource.ScopeSpans[0].Spans[0].TraceId, bytes.Repeat([]byte{0x11}, 16)) || len(resource.ScopeSpans[0].Spans[0].Links) != 0 {
		t.Fatalf("unexpected normalized protobuf: %v", &result)
	}
	incomingID, _ := oteltrace.TraceIDFromHex("11111111111111111111111111111111")
	expected := observability.NamespaceIngressTraceID(incomingID)
	if !strings.Contains(string(cleanJSONForTest(t, jsonBody)), expected.String()) || !bytes.Equal(resource.ScopeSpans[0].Spans[0].TraceId, expected[:]) {
		t.Fatal("JSON and protobuf IDs do not match gateway link namespace")
	}
	t.Setenv("HOMENAVI_TRACE_INGRESS_NEW_ROOT", "false")
	trusted, err := normalizeBrowserTraces(jsonBody, "application/json")
	if err != nil || !strings.Contains(string(trusted), incomingID.String()) {
		t.Fatalf("trusted parentage changed: %s %v", trusted, err)
	}
}

func cleanJSONForTest(t *testing.T, body []byte) []byte {
	t.Helper()
	clean, err := normalizeBrowserTraces(body, "application/json")
	if err != nil {
		t.Fatal(err)
	}
	return clean
}

func TestNormalizeBrowserTraceProtoJSONNames(t *testing.T) {
	t.Setenv("HOMENAVI_TRACE_INGRESS_NEW_ROOT", "true")
	payload := []byte(`{"resource_spans":[{"schema_url":"resource-schema","resource":{"attributes":[{"key":"service.name","value":{"stringValue":"auth-service"}}]},"scope_spans":[{"schema_url":"scope-schema","spans":[{"trace_id":"11111111111111111111111111111111","kind":2}]}]}]}`)
	clean, err := normalizeBrowserTraces(payload, "application/json")
	if err != nil || !strings.Contains(string(clean), `"resourceSpans"`) || !strings.Contains(string(clean), `"scopeSpans"`) || !strings.Contains(string(clean), `"kind":3`) || strings.Contains(string(clean), "auth-service") {
		t.Fatalf("unexpected normalized payload: %s (%v)", clean, err)
	}
	if !strings.Contains(string(clean), `"schemaUrl":"resource-schema"`) || !strings.Contains(string(clean), `"schemaUrl":"scope-schema"`) || strings.Contains(string(clean), `"schema_url"`) {
		t.Fatalf("schema URLs were not preserved: %s", clean)
	}
	for _, invalid := range []string{
		`{"resourceSpans":[],"resource_spans":[]}`,
		`{"resourceSpans":[{"scopeSpans":[],"scope_spans":[]}]}`,
		`{"resourceSpans":[{"schemaUrl":"a","schema_url":"b"}]}`,
		`{"resourceSpans":[{"scopeSpans":[{"schemaUrl":"a","schema_url":"b"}]}]}`,
		`{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"11111111111111111111111111111111","trace_id":"22222222222222222222222222222222"}]}]}]}`,
	} {
		if _, err := normalizeBrowserTraces([]byte(invalid), "application/json"); err == nil {
			t.Fatalf("accepted ambiguous payload: %s", invalid)
		}
	}
}

func TestBrowserTraceHandlerBoundaries(t *testing.T) {
	var forwarded http.Header
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	handler := browserTraceHandler(collector.URL)
	tests := []struct {
		name, contentType string
		body              []byte
		status            int
	}{
		{"valid", "application/json", []byte(`{}`), http.StatusOK},
		{"wrong content type", "text/plain", []byte(`{}`), http.StatusUnsupportedMediaType},
		{"oversized", "application/json", bytes.Repeat([]byte("x"), browserTraceMaxBytes+1), http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/telemetry/traces", bytes.NewReader(test.body))
			req.Header.Set("Content-Type", test.contentType)
			req.Header.Set("Authorization", "Bearer secret")
			req.Header.Set("Cookie", "auth_token=secret")
			req.Header.Set("traceparent", "secret")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != test.status {
				t.Fatalf("expected %d, got %d: %s", test.status, rr.Code, rr.Body.String())
			}
		})
	}
	if forwarded.Get("Authorization") != "" || forwarded.Get("Cookie") != "" || forwarded.Get("traceparent") != "" {
		t.Fatalf("sensitive headers forwarded: %#v", forwarded)
	}
	if !strings.HasPrefix(forwarded.Get("Content-Type"), "application/json") {
		t.Fatalf("unexpected forwarded content type: %q", forwarded.Get("Content-Type"))
	}
}

func TestBrowserTraceCollectorHeaders(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=Bearer%20generic")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "Authorization=Bearer%20collector,X-Tenant=tenant")
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer collector" || r.Header.Get("X-Tenant") != "tenant" || r.Header.Get("Cookie") != "" {
			t.Errorf("unexpected collector headers: %v", r.Header)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/telemetry/traces", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer browser")
	request.Header.Set("Cookie", "secret")
	response := httptest.NewRecorder()
	browserTraceHandler(collector.URL).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}

func TestBrowserTraceCollectorTLS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	clientCA, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	clientRoots := x509.NewCertPool()
	clientRoots.AddCert(clientCA)
	collector := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	collector.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientRoots, MinVersion: tls.VersionTLS12}
	collector.StartTLS()
	defer collector.Close()
	dir := t.TempDir()
	caPath, certPath, keyPath := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "client.pem"), filepath.Join(dir, "key.pem")
	for path, data := range map[string][]byte{
		caPath:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: collector.Certificate().Raw}),
		certPath: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPath:  pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
	} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", filepath.Join(dir, "missing-ca.pem"))
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", filepath.Join(dir, "missing-client.pem"))
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_KEY", filepath.Join(dir, "missing-key.pem"))
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", caPath)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE", certPath)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY", keyPath)
	request := httptest.NewRequest(http.MethodPost, "/api/telemetry/traces", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	browserTraceHandler(collector.URL).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("mTLS export failed: %d %s", response.Code, response.Body.String())
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY", "")
	response = httptest.NewRecorder()
	browserTraceHandler(collector.URL).ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("incomplete mTLS configuration: %d", response.Code)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY", keyPath)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", filepath.Join(dir, "missing-ca.pem"))
	if _, err := browserCollectorClient(); err == nil {
		t.Fatal("missing CA accepted")
	}
	invalidCA := filepath.Join(dir, "invalid-ca.pem")
	if err := os.WriteFile(invalidCA, []byte("not a certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", invalidCA)
	if _, err := browserCollectorClient(); err == nil {
		t.Fatal("invalid CA accepted")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", caPath)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY", invalidCA)
	if _, err := browserCollectorClient(); err == nil {
		t.Fatal("invalid client key accepted")
	}
	for _, name := range []string{"OTEL_EXPORTER_OTLP_CERTIFICATE", "OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", "OTEL_EXPORTER_OTLP_CLIENT_KEY", "OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", "OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE", "OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY"} {
		t.Setenv(name, "")
	}
	request = httptest.NewRequest(http.MethodPost, "/api/telemetry/traces", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	browserTraceHandler(collector.URL).ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("default TLS accepted untrusted collector: %d", response.Code)
	}
}

func TestRoutedWebSocketIngressTracing(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); _ = provider.Shutdown(t.Context()) })
	headers := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Clone()
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			_ = connection.Close()
		}
	}))
	defer upstream.Close()
	cfg := gateway.Config{Routes: []gateway.RouteConfig{{Path: "/ws/test", Upstream: "ws" + strings.TrimPrefix(upstream.URL, "http"), Methods: []string{http.MethodGet}, Type: "websocket", Access: "public"}}}
	server := httptest.NewServer(NewRootRouter(NewWebSocketRouter(cfg, nil, nil, provider.Tracer("gateway")), http.NotFoundHandler()))
	defer server.Close()
	for _, test := range []struct {
		setting   string
		continued bool
	}{{"false", true}, {"true", false}} {
		t.Setenv("HOMENAVI_TRACE_INGRESS_NEW_ROOT", test.setting)
		connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/test", http.Header{"Traceparent": []string{"00-11111111111111111111111111111111-2222222222222222-01"}})
		if err != nil {
			t.Fatal(err)
		}
		_ = connection.Close()
		remote := propagation.TraceContext{}.Extract(t.Context(), propagation.HeaderCarrier(<-headers))
		spanContext := oteltrace.SpanContextFromContext(remote)
		if !spanContext.IsValid() || (spanContext.TraceID().String() == "11111111111111111111111111111111") != test.continued {
			t.Fatalf("setting %s: unexpected upstream context %v", test.setting, spanContext)
		}
	}
}

func TestWebSocketHandshakeSpanEndsWhileConnected(t *testing.T) {
	before := gatewayRequestCount(t, "/ws/held", "101")
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer provider.Shutdown(t.Context())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			defer connection.Close()
			_, _, _ = connection.ReadMessage()
		}
	}))
	defer upstream.Close()
	cfg := gateway.Config{Routes: []gateway.RouteConfig{{Path: "/ws/held", Upstream: "ws" + strings.TrimPrefix(upstream.URL, "http"), Methods: []string{http.MethodGet}, Type: "websocket", Access: "public"}}}
	server := httptest.NewServer(NewRootRouter(NewWebSocketRouter(cfg, nil, nil, provider.Tracer("gateway")), http.NotFoundHandler()))
	defer server.Close()
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/held", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		for _, span := range recorder.Ended() {
			if span.SpanKind() != oteltrace.SpanKindServer {
				continue
			}
			for _, attr := range span.Attributes() {
				if string(attr.Key) == "http.status_code" && attr.Value.AsInt64() == http.StatusSwitchingProtocols {
					if got := gatewayRequestCount(t, "/ws/held", "101"); got != before+1 {
						t.Fatalf("handshake metric: got %v, want %v", got, before+1)
					}
					return
				}
			}
			t.Fatalf("server span did not record 101: %v", span.Attributes())
		}
		select {
		case <-deadline:
			t.Fatal("handshake span remained open while connection was active")
		case <-ticker.C:
		}
	}
}

func TestWebSocketFailedHandshakeMetrics(t *testing.T) {
	before := gatewayRequestCount(t, "/ws/failure", "503")
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer provider.Shutdown(t.Context())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	cfg := gateway.Config{Routes: []gateway.RouteConfig{{Path: "/ws/failure", Upstream: "ws" + strings.TrimPrefix(upstream.URL, "http"), Methods: []string{http.MethodGet}, Type: "websocket", Access: "public"}}}
	server := httptest.NewServer(NewRootRouter(NewWebSocketRouter(cfg, nil, nil, provider.Tracer("gateway")), http.NotFoundHandler()))
	defer server.Close()
	response, err := http.Get(server.URL + "/ws/failure")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected HTTP status: %d", response.StatusCode)
	}
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		for _, span := range recorder.Ended() {
			if span.SpanKind() != oteltrace.SpanKindServer {
				continue
			}
			for _, attr := range span.Attributes() {
				if string(attr.Key) == "http.status_code" && attr.Value.AsInt64() == http.StatusServiceUnavailable {
					if got := gatewayRequestCount(t, "/ws/failure", "503"); got != before+1 {
						t.Fatalf("failed handshake metric: got %v, want %v", got, before+1)
					}
					return
				}
			}
			t.Fatalf("server span did not record failure: %v", span.Attributes())
		}
		select {
		case <-deadline:
			t.Fatal("failed handshake span did not end")
		case <-ticker.C:
		}
	}
}

func gatewayRequestCount(t *testing.T, path, status string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "http_requests_total" {
			continue
		}
		for _, metric := range family.Metric {
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["service"] == "api-gateway" && labels["endpoint"] == path && labels["method"] == "GET" && labels["status"] == status {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func TestBrowserTraceEndpointRequiresAuthentication(t *testing.T) {
	t.Setenv("HOMENAVI_BROWSER_OTEL_ENABLED", "true")
	router := NewMainRouter(gateway.Config{}, nil, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), otel.Tracer("test"), "")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/telemetry/traces", strings.NewReader("{}")))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", rr.Code)
	}
}

func TestBrowserTraceRuntimeConfig(t *testing.T) {
	t.Setenv("HOMENAVI_BROWSER_OTEL_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	router := NewMainRouter(gateway.Config{}, nil, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), otel.Tracer("test"), "")
	for _, test := range []struct{ endpoint, expected string }{
		{"", "{\"enabled\":false}\n"},
		{"http://collector:4318/v1/traces", "{\"enabled\":true}\n"},
	} {
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", test.endpoint)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/telemetry/config", nil))
		if response.Code != http.StatusOK || response.Body.String() != test.expected || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected runtime config: %d %q", response.Code, response.Body.String())
		}
	}
}

func TestMainRouterGatewayHealth(t *testing.T) {
	router := NewMainRouter(gateway.Config{}, nil, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), otel.Tracer("test"), "")
	req := httptest.NewRequest(http.MethodGet, "/api/gateway/health", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Fatalf("expected ok body, got %q", rr.Body.String())
	}
}

func TestMainRouterRoutesEndpoint(t *testing.T) {
	cfg := gateway.Config{Routes: []gateway.RouteConfig{{Path: "/api/test", Upstream: "http://example.com", Methods: []string{http.MethodGet}, Access: "public"}}}
	router := NewMainRouter(cfg, nil, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), otel.Tracer("test"), "")
	req := httptest.NewRequest(http.MethodGet, "/api/gateway/routes", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var routes []gateway.RouteConfig
	if err := json.NewDecoder(rr.Body).Decode(&routes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(routes) != 1 || routes[0].Path != "/api/test" {
		t.Fatalf("unexpected routes payload: %#v", routes)
	}
}

func TestConfiguredPublicRouteProxies(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxied")
	}))
	defer upstream.Close()

	cfg := gateway.Config{Routes: []gateway.RouteConfig{{Path: "/api/test", Upstream: upstream.URL, Methods: []string{http.MethodGet}, Access: "public"}}}
	router := NewMainRouter(cfg, nil, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), otel.Tracer("test"), "")
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "proxied" {
		t.Fatalf("expected proxied body, got %q", rr.Body.String())
	}
}
