package proxy

import (
	"api-gateway/internal/config"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"encoding/json"

	"github.com/go-chi/chi/v5"
	"github.com/koding/websocketproxy"
)

func MakeRestProxyHandler(route config.RouteConfig) http.HandlerFunc {
	upstreamURL, err := url.Parse(route.Upstream)
	if err != nil {
		panic("Invalid upstream URL: " + route.Upstream)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("proxy request", "method", r.Method, "path", r.URL.Path, "upstream", upstreamURL.String())
		proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
		origDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			origDirector(req)
			ctx := chi.RouteContext(r.Context())
			path := upstreamURL.Path
			if ctx != nil {
				for i, key := range ctx.URLParams.Keys {
					val := ctx.URLParams.Values[i]
					path = strings.ReplaceAll(path, "{"+key+"}", val)
				}
			}
			req.URL.Path = path
			req.URL.RawQuery = r.URL.RawQuery
		}
		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			slog.Error("proxy error", "method", r.Method, "path", r.URL.Path, "upstream", upstreamURL.String(), "error", err)
			rw.Header().Set("Content-Type","application/json")
			rw.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(rw).Encode(map[string]any{"error":"upstream error","code":http.StatusBadGateway})
		}
		proxy.ServeHTTP(w, r)
	}
}

func MakeWebSocketProxyHandler(route config.RouteConfig) http.HandlerFunc {
	upstreamURL, err := url.Parse(route.Upstream)
	if err != nil {
		panic("Invalid upstream URL: " + route.Upstream)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("proxy websocket", "path", r.URL.Path, "upstream", upstreamURL.String())
		websocketproxy.ProxyHandler(upstreamURL).ServeHTTP(w, r)
	}
}
