package http

import (
	"crypto/rsa"
	"net/http"
	"path"
	"strings"

	proxyauth "github.com/PetoAdam/homenavi/integration-proxy/internal/auth"
)

func NewRouter(server *Server, pubKey *rsa.PublicKey) http.Handler {
	return proxyauth.RequireResident(pubKey, server.allowUnauthenticated)(server.Routes())
}

func (s *Server) allowUnauthenticated(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}

	id, innerPath, ok := integrationPathFromRequestPath(r.URL.Path)
	if !ok {
		return false
	}

	s.mu.RLock()
	m, ok := s.manifests[id]
	s.mu.RUnlock()
	if !ok || len(m.Auth.PublicPaths) == 0 {
		return false
	}

	normalizedPath := normalizeManifestPath(innerPath)
	for _, candidate := range m.Auth.PublicPaths {
		if normalizedPath == candidate {
			return true
		}
	}
	return false
}

func integrationPathFromRequestPath(p string) (string, string, bool) {
	clean := strings.TrimSpace(p)
	if clean == "" || clean == "/" {
		return "", "", false
	}

	rest := strings.TrimPrefix(clean, "/")
	if strings.HasPrefix(rest, "integrations/") {
		rest = strings.TrimPrefix(rest, "integrations/")
	}
	if rest == "" {
		return "", "", false
	}

	parts := strings.SplitN(rest, "/", 2)
	id := strings.TrimSpace(parts[0])
	if id == "" {
		return "", "", false
	}

	innerPath := "/"
	if len(parts) == 2 {
		innerPath = "/" + parts[1]
	}

	return id, innerPath, true
}

func normalizeManifestPath(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return path.Clean(trimmed)
}
