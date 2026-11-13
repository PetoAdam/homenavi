package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PetoAdam/homenavi/api-gateway/internal/gateway"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestWebSocketProxyPropagatesHandshakeTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	headers := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Clone()
		upgrader := websocket.Upgrader{Subprotocols: []string{"mqtt"}}
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		kind, payload, err := connection.ReadMessage()
		if err == nil {
			_ = connection.WriteMessage(kind, payload)
		}
	}))
	defer upstream.Close()
	proxy := MakeWebSocketProxyHandler(gateway.RouteConfig{Path: "/mqtt", Upstream: "ws" + strings.TrimPrefix(upstream.URL, "http") + "/mqtt"})
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, parent := provider.Tracer("test").Start(r.Context(), "request")
		defer parent.End()
		proxy(w, r.WithContext(ctx))
	}))
	defer gatewayServer.Close()
	dialer := websocket.Dialer{Subprotocols: []string{"mqtt"}}
	connection, _, err := dialer.Dial("ws"+strings.TrimPrefix(gatewayServer.URL, "http")+"/mqtt", http.Header{"Baggage": []string{"secret=value"}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if connection.Subprotocol() != "mqtt" {
		t.Fatal("MQTT WebSocket subprotocol was not preserved")
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	kind, payload, err := connection.ReadMessage()
	if err != nil || kind != websocket.BinaryMessage || string(payload) != string([]byte{1, 2, 3}) {
		t.Fatalf("WebSocket payload changed: %v", err)
	}
	upstreamHeaders := <-headers
	remote := propagation.TraceContext{}.Extract(t.Context(), propagation.HeaderCarrier(upstreamHeaders))
	spans := recorder.Ended()
	if len(spans) == 0 || spans[0].Name() != "websocket.connect" || spans[0].SpanKind() != oteltrace.SpanKindClient || oteltrace.SpanContextFromContext(remote).SpanID() != spans[0].SpanContext().SpanID() || upstreamHeaders.Get("Baggage") != "" {
		t.Fatal("upstream handshake did not receive the bounded client span context")
	}
}

func TestMakeRestProxyHandlerPropagatesTraceContext(t *testing.T) {
	provider := trace.NewTracerProvider()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	var gotTraceparent string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceparent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	ctx, span := provider.Tracer("test").Start(t.Context(), "request")
	defer span.End()
	request := httptest.NewRequest(http.MethodGet, "/resource", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	MakeRestProxyHandler(gateway.RouteConfig{Path: "/resource", Upstream: upstream.URL + "/resource"}).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !strings.Contains(gotTraceparent, span.SpanContext().TraceID().String()) {
		t.Fatalf("status %d, traceparent %q", response.Code, gotTraceparent)
	}
}

func TestMakeRestProxyHandlerPreservesSingleTraceID(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Trace-ID", "downstream-trace")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	handler := MakeRestProxyHandler(gateway.RouteConfig{Path: "/resource", Upstream: upstream.URL})
	for _, test := range []struct{ gatewayTrace, want string }{{"gateway-trace", "gateway-trace"}, {"", "downstream-trace"}} {
		response := httptest.NewRecorder()
		if test.gatewayTrace != "" {
			response.Header().Set("Trace-ID", test.gatewayTrace)
		}
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/resource", nil))
		values := response.Header().Values("Trace-ID")
		if len(values) != 1 || values[0] != test.want {
			t.Fatalf("Trace-ID = %v, want [%s]", values, test.want)
		}
	}
}

func TestMakeRestProxyHandler_WildcardPreservesSuffix(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	route := gateway.RouteConfig{
		Path:     "/api/hdp/devices/*",
		Upstream: upstream.URL + "/api/hdp/devices/*",
		Methods:  []string{http.MethodPost},
		Type:     "rest",
		Access:   "public",
	}

	r := chi.NewRouter()
	r.Method(http.MethodPost, "/api/hdp/devices/*", MakeRestProxyHandler(route))

	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices/zigbee/0xa4c13867e32d96d4/commands", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	want := "/api/hdp/devices/zigbee/0xa4c13867e32d96d4/commands"
	if gotPath != want {
		t.Fatalf("expected upstream path %q got %q", want, gotPath)
	}
}

func TestMakeRestProxyHandlerPreservesPublicResourceHeaders(t *testing.T) {
	var gotHost, gotProto string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotProto = r.Header.Get("X-Forwarded-Proto")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	route := gateway.RouteConfig{Path: "/mcp", Upstream: upstream.URL + "/mcp", Methods: []string{http.MethodPost}, Type: "rest", Access: "public"}
	request := httptest.NewRequest(http.MethodPost, "http://api-gateway/mcp", nil)
	request.Host = "home.example.test"
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	MakeRestProxyHandler(route).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if gotHost != "home.example.test" || gotProto != "https" {
		t.Fatalf("public resource headers = host %q proto %q", gotHost, gotProto)
	}
}

func TestMakeRestProxyHandlerPreservesEncodedRouteParameters(t *testing.T) {
	var gotClientID string
	upstreamRouter := chi.NewRouter()
	upstreamRouter.Delete("/api/auth/oauth/consents/{clientID}", func(w http.ResponseWriter, r *http.Request) {
		gotClientID = chi.URLParam(r, "clientID")
		if r.URL.RawPath != "" {
			var err error
			gotClientID, err = url.PathUnescape(gotClientID)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	upstream := httptest.NewServer(upstreamRouter)
	defer upstream.Close()

	route := gateway.RouteConfig{Path: "/api/auth/oauth/consents/{clientID}", Upstream: upstream.URL + "/api/auth/oauth/consents/{clientID}"}
	router := chi.NewRouter()
	router.Delete(route.Path, MakeRestProxyHandler(route))
	for _, clientID := range []string{"https://client.example/app%1", "client%2Fname", "client+name", "simple-client"} {
		t.Run(clientID, func(t *testing.T) {
			gotClientID = ""
			request := httptest.NewRequest(http.MethodDelete, "/api/auth/oauth/consents/"+url.PathEscape(clientID), nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusNoContent || gotClientID != clientID {
				t.Fatalf("status = %d, client ID = %q, want %q", response.Code, gotClientID, clientID)
			}
		})
	}
}
