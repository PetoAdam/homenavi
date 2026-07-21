package http

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter_AllowsDeclaredIntegrationPublicPathWithoutResidentToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	s := New(nil, nil, &key.PublicKey, "", "")
	s.mu.Lock()
	s.manifests["spotify"] = Manifest{
		ID: "spotify",
		Auth: ManifestAuth{PublicPaths: []string{"/api/admin/auth/callback"}},
	}
	s.mu.Unlock()

	h := NewRouter(s, &key.PublicKey)

	publicReq := httptest.NewRequest(http.MethodGet, "/integrations/spotify/api/admin/auth/callback?code=abc", nil)
	publicRW := httptest.NewRecorder()
	h.ServeHTTP(publicRW, publicReq)
	if publicRW.Code != http.StatusNotFound {
		t.Fatalf("expected callback request to pass auth and reach proxy handler (404 without upstream), got %d", publicRW.Code)
	}

	protectedReq := httptest.NewRequest(http.MethodGet, "/integrations/spotify/api/admin/auth/status", nil)
	protectedRW := httptest.NewRecorder()
	h.ServeHTTP(protectedRW, protectedReq)
	if protectedRW.Code != http.StatusUnauthorized {
		t.Fatalf("expected non-public integration auth endpoint to require resident token, got %d", protectedRW.Code)
	}
}

func TestRouter_PublicPathMatchingNormalizesTrailingSlash(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	s := New(nil, nil, &key.PublicKey, "", "")
	s.mu.Lock()
	s.manifests["spotify"] = Manifest{
		ID: "spotify",
		Auth: ManifestAuth{PublicPaths: normalizePublicPaths([]string{"/api/admin/auth/callback/", " /api/admin/auth/callback "})},
	}
	s.mu.Unlock()

	h := NewRouter(s, &key.PublicKey)

	req := httptest.NewRequest(http.MethodGet, "/integrations/spotify/api/admin/auth/callback/", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusNotFound {
		t.Fatalf("expected normalized callback path to pass auth and reach proxy handler (404 without upstream), got %d", rw.Code)
	}
}

func TestRouter_HealthPathRemainsPublic(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	s := New(nil, nil, &key.PublicKey, "", "")
	h := NewRouter(s, &key.PublicKey)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected health endpoint to stay public, got %d", rw.Code)
	}
}
