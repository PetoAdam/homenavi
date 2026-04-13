package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type correlationIDKey struct{}

func CorrelationID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			corrID := r.Header.Get("X-Correlation-ID")
			if corrID == "" {
				corrID = uuid.New().String()
			}
			w.Header().Set("X-Correlation-ID", corrID)
			r = r.WithContext(context.WithValue(r.Context(), correlationIDKey{}, corrID))
			next.ServeHTTP(w, r)
		})
	}
}

func CORS(allowOrigins string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range strings.Split(strings.TrimSpace(allowOrigins), ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = true
		}
	}

	const allowMethods = "GET,POST,PUT,PATCH,DELETE,OPTIONS"
	const allowHeaders = "Authorization,Content-Type,X-Requested-With,X-Correlation-ID"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (len(allowed) == 0 || allowed[origin]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
