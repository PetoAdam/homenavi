package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PetoAdam/homenavi/api-gateway/internal/gateway"
	"go.opentelemetry.io/otel"
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
