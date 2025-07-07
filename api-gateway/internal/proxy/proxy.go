package proxy

import (
	"api-gateway/internal/config"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

func MakeProxyHandler(route config.RouteConfig) http.HandlerFunc {
	upstreamURL, err := url.Parse(route.Upstream)
	if err != nil {
		panic("Invalid upstream URL: " + route.Upstream)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Proxying %s %s to upstream %s", r.Method, r.URL.Path, upstreamURL)
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
			log.Printf("Proxy error for %s %s to %s: %v", r.Method, r.URL.Path, upstreamURL, err)
			http.Error(rw, "Upstream error: "+err.Error(), http.StatusBadGateway)
		}
		proxy.ServeHTTP(w, r)
	}
}
