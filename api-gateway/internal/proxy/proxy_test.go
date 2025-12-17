package proxy

import (
	"api-gateway/internal/config"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMakeRestProxyHandler_WildcardPreservesSuffix(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	route := config.RouteConfig{
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
