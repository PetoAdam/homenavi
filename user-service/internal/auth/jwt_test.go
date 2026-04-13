package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer abc123")
	if got := ExtractToken(req); got != "abc123" {
		t.Fatalf("expected token, got %q", got)
	}
}

func TestRoleAtLeastMiddlewareRejectsMissingClaims(t *testing.T) {
	h := RoleAtLeastMiddleware("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
