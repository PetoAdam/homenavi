package main

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"

	httptransport "github.com/PetoAdam/homenavi/integration-proxy/internal/http"
)

func TestIntegrationProxyRoutesRequireResident(t *testing.T) {
	// This is a regression test: integration-proxy must be resident-gated even if
	// nginx is misconfigured, because it proxies arbitrary integration UIs/APIs.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	s := httptransport.New(nil, nil, &key.PublicKey, "", "")
	h := httptransport.NewRouter(s, &key.PublicKey)

	req := httptest.NewRequest(http.MethodGet, "/integrations/registry.json", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rw.Code)
	}
}
