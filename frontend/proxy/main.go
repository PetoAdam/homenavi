package main

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func main() {
	logger := log.New(os.Stdout, "frontend-proxy ", log.LstdFlags|log.LUTC)
	addr := envOrDefault("FRONTEND_LISTEN_ADDR", ":80")
	staticRoot := envOrDefault("FRONTEND_STATIC_ROOT", "/app/dist")
	apiProxy := mustProxy(envOrDefault("API_GATEWAY_URL", "http://api-gateway:8080"))
	integrationProxy := mustProxy(envOrDefault("INTEGRATION_PROXY_URL", "http://integration-proxy:8099"))

	mux := http.NewServeMux()
	mux.Handle("/api/", apiProxy)
	mux.Handle("/integrations/", integrationProxy)
	mux.Handle("/ws/", apiProxy)
	mux.HandleFunc("/", spaHandler(staticRoot))

	logger.Printf("serving frontend on %s with static root %s", addr, staticRoot)
	if err := http.ListenAndServe(addr, logRequests(logger, mux)); err != nil {
		logger.Fatalf("server failed: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func mustProxy(raw string) http.Handler {
	target, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("invalid proxy target %q: %v", raw, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		originalDirector(r)
		r.Header.Set("X-Forwarded-Proto", forwardedProto(r))
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
			r.Header.Set("X-Real-IP", host)
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	return proxy
}

func forwardedProto(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	return "http"
}

func spaHandler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cleanPath := path.Clean("/" + r.URL.Path)
		resolved := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(cleanPath, "/")))
		if info, err := os.Stat(resolved); err == nil && !info.IsDir() {
			applyCacheHeaders(w, cleanPath)
			http.ServeFile(w, r, resolved)
			return
		}

		indexPath := filepath.Join(root, "index.html")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, indexPath)
	}
}

func applyCacheHeaders(w http.ResponseWriter, requestPath string) {
	switch {
	case strings.HasPrefix(requestPath, "/assets/"):
		w.Header().Set("Cache-Control", "public, immutable")
	case strings.HasPrefix(requestPath, "/icons/"):
		w.Header().Set("Cache-Control", "public, max-age=2592000")
	case requestPath == "/manifest.webmanifest":
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
}

func logRequests(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
