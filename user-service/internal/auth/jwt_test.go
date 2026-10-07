package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
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

func TestJWTAuthMiddlewareAcceptsOverlapKeysByKeyID(t *testing.T) {
	firstKey := newTestRSAKey(t)
	secondKey := newTestRSAKey(t)
	keySet, err := NewRSAPublicKeySet(&firstKey.PublicKey, &secondKey.PublicKey)
	if err != nil {
		t.Fatalf("NewRSAPublicKeySet() error = %v", err)
	}
	middleware := JWTAuthMiddleware(keySet, ValidationConfig{Issuer: "issuer", APIAudience: "api", ServiceAudience: "user-service"})
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, privateKey := range []*rsa.PrivateKey{firstKey, secondKey} {
		token := newTestAPIToken(t, privateKey, "issuer", "api")
		request := httptest.NewRequest(http.MethodGet, "/users/user-1", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("overlap key token status = %d, want %d", response.Code, http.StatusOK)
		}
	}
}

func TestJWTAuthMiddlewareRejectsUnknownKeyID(t *testing.T) {
	trustedKey := newTestRSAKey(t)
	untrustedKey := newTestRSAKey(t)
	keySet, err := NewRSAPublicKeySet(&trustedKey.PublicKey)
	if err != nil {
		t.Fatalf("NewRSAPublicKeySet() error = %v", err)
	}
	handler := JWTAuthMiddleware(keySet, ValidationConfig{Issuer: "issuer", APIAudience: "api"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/users/user-1", nil)
	request.Header.Set("Authorization", "Bearer "+newTestAPIToken(t, untrustedKey, "issuer", "api"))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unknown key token status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func newTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	return privateKey
}

func newTestAPIToken(t *testing.T, privateKey *rsa.PrivateKey, issuer, audience string) string {
	t.Helper()
	keyID, err := authx.KeyIDForRSAPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("KeyIDForRSAPublicKey() error = %v", err)
	}
	now := time.Now()
	claims := Claims{
		Role:      authx.RoleUser,
		Sub:       "user-1",
		SessionID: "session-1",
		TokenType: authx.TokenTypeAPI,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        "token-1",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return signedToken
}
