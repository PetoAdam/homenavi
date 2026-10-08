package oauth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
)

type fakeJWKSProvider struct {
	keySet authdomain.JSONWebKeySet
	err    error
}

func (f fakeJWKSProvider) JSONWebKeySet() (authdomain.JSONWebKeySet, error) {
	return f.keySet, f.err
}

func TestJWKSHandlerReturnsPublicKeySet(t *testing.T) {
	handler := NewJWKSHandler(fakeJWKSProvider{keySet: authdomain.JSONWebKeySet{Keys: []authdomain.JSONWebKey{{KeyID: "key-1"}}}})
	request := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/jwks.json", nil)
	response := httptest.NewRecorder()

	handler.HandleJWKS(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatalf("unexpected cache control %q", response.Header().Get("Cache-Control"))
	}
	if response.Body.String() == "" || response.Body.String()[0] != '{' {
		t.Fatalf("expected JSON body, got %q", response.Body.String())
	}
}

func TestJWKSHandlerReturnsServiceUnavailableWithoutKey(t *testing.T) {
	handler := NewJWKSHandler(fakeJWKSProvider{err: errors.New("no active key")})
	request := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/jwks.json", nil)
	response := httptest.NewRecorder()

	handler.HandleJWKS(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
