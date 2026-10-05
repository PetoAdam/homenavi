package middleware

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

func TestJWTAuthMiddlewareRS256Validation(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	validation := APIValidationConfig("https://auth.example.test", "https://api.example.test")

	tests := []struct {
		name          string
		issuer        string
		audience      []string
		tokenType     string
		signingMethod jwt.SigningMethod
		wantStatus    int
	}{
		{name: "valid API token", issuer: validation.Issuer, audience: []string{validation.Audience}, tokenType: authx.TokenTypeAPI, signingMethod: jwt.SigningMethodRS256, wantStatus: http.StatusNoContent},
		{name: "wrong issuer", issuer: "https://other-auth.example.test", audience: []string{validation.Audience}, tokenType: authx.TokenTypeAPI, signingMethod: jwt.SigningMethodRS256, wantStatus: http.StatusUnauthorized},
		{name: "wrong audience", issuer: validation.Issuer, audience: []string{"https://mcp.example.test/mcp"}, tokenType: authx.TokenTypeAPI, signingMethod: jwt.SigningMethodRS256, wantStatus: http.StatusUnauthorized},
		{name: "wrong token type", issuer: validation.Issuer, audience: []string{validation.Audience}, tokenType: authx.TokenTypeMCP, signingMethod: jwt.SigningMethodRS256, wantStatus: http.StatusUnauthorized},
		{name: "unexpected signing algorithm", issuer: validation.Issuer, audience: []string{validation.Audience}, tokenType: authx.TokenTypeAPI, signingMethod: jwt.SigningMethodHS256, wantStatus: http.StatusUnauthorized},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tokenString := signedTestToken(t, privateKey, test.issuer, test.audience, test.tokenType, test.signingMethod)
			handlerCalled := false
			handler := JWTAuthMiddlewareRS256(&privateKey.PublicKey, validation)(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				handlerCalled = true
				responseWriter.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+tokenString)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if handlerCalled != (test.wantStatus == http.StatusNoContent) {
				t.Fatalf("handler called = %t, want %t", handlerCalled, test.wantStatus == http.StatusNoContent)
			}
		})
	}
}

func signedTestToken(t *testing.T, privateKey *rsa.PrivateKey, issuer string, audience []string, tokenType string, signingMethod jwt.SigningMethod) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(signingMethod, Claims{
		Role:      authx.RoleResident,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   "user-1",
			Audience:  audience,
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	})

	var signingKey interface{} = privateKey
	if signingMethod.Alg() == jwt.SigningMethodHS256.Alg() {
		signingKey = []byte("test-signing-key")
	}
	tokenString, err := token.SignedString(signingKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tokenString
}
