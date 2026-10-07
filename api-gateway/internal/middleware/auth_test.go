package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
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

func TestJWTAuthMiddlewareRS256RejectsMissingRequiredClaims(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	validation := APIValidationConfig("https://auth.example.test", "https://api.example.test")
	now := time.Now()

	tests := []struct {
		name   string
		claims Claims
	}{
		{name: "missing expiration", claims: requiredTestClaims(validation, now)},
		{name: "missing not before", claims: requiredTestClaims(validation, now)},
		{name: "missing issued at", claims: requiredTestClaims(validation, now)},
		{name: "missing token ID", claims: requiredTestClaims(validation, now)},
		{name: "missing subject", claims: requiredTestClaims(validation, now)},
		{name: "missing role", claims: requiredTestClaims(validation, now)},
		{name: "missing session ID", claims: requiredTestClaims(validation, now)},
	}
	tests[0].claims.ExpiresAt = nil
	tests[1].claims.NotBefore = nil
	tests[2].claims.IssuedAt = nil
	tests[3].claims.ID = ""
	tests[4].claims.Subject = ""
	tests[5].claims.Role = ""
	tests[6].claims.SessionID = ""

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, test.claims)
			tokenString, err := token.SignedString(privateKey)
			if err != nil {
				t.Fatalf("sign token: %v", err)
			}
			handler := JWTAuthMiddlewareRS256(&privateKey.PublicKey, validation)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("handler must not be called")
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+tokenString)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestJWTAuthMiddlewareRS256KeySetSelectsKnownKeyIDs(t *testing.T) {
	activeKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate active key: %v", err)
	}
	overlapKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate overlap key: %v", err)
	}
	activeKeyID, err := authx.KeyIDForRSAPublicKey(&activeKey.PublicKey)
	if err != nil {
		t.Fatalf("derive active key ID: %v", err)
	}
	overlapKeyID, err := authx.KeyIDForRSAPublicKey(&overlapKey.PublicKey)
	if err != nil {
		t.Fatalf("derive overlap key ID: %v", err)
	}
	validation := APIValidationConfig("https://auth.example.test", "https://api.example.test")
	keySet := RSAPublicKeySet{activeKeyID: &activeKey.PublicKey, overlapKeyID: &overlapKey.PublicKey}

	tests := []struct {
		name   string
		key    *rsa.PrivateKey
		keyID  string
		status int
	}{
		{name: "active key", key: activeKey, keyID: activeKeyID, status: http.StatusNoContent},
		{name: "overlap key", key: overlapKey, keyID: overlapKeyID, status: http.StatusNoContent},
		{name: "missing key ID", key: activeKey, status: http.StatusUnauthorized},
		{name: "unknown key ID", key: activeKey, keyID: "unknown", status: http.StatusUnauthorized},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, requiredTestClaims(validation, time.Now()))
			if test.keyID != "" {
				token.Header["kid"] = test.keyID
			}
			tokenString, err := token.SignedString(test.key)
			if err != nil {
				t.Fatalf("sign token: %v", err)
			}
			handler := JWTAuthMiddlewareRS256KeySet(keySet, validation)(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				responseWriter.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+tokenString)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
		})
	}
}

func TestJWTAuthMiddlewareRS256ValidatesSessionState(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	baseValidation := APIValidationConfig("https://auth.example.test", "https://api.example.test")
	tests := []struct {
		name       string
		validator  SessionValidator
		wantStatus int
	}{
		{name: "active session", validator: func(context.Context, string) (bool, error) { return true, nil }, wantStatus: http.StatusNoContent},
		{name: "revoked session", validator: func(context.Context, string) (bool, error) { return false, nil }, wantStatus: http.StatusUnauthorized},
		{name: "store unavailable", validator: func(context.Context, string) (bool, error) { return false, errors.New("redis unavailable") }, wantStatus: http.StatusServiceUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validation := baseValidation
			validation.SessionValidator = test.validator
			handler := JWTAuthMiddlewareRS256(&privateKey.PublicKey, validation)(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				responseWriter.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+signedTestToken(t, privateKey, validation.Issuer, []string{validation.Audience}, authx.TokenTypeAPI, jwt.SigningMethodRS256))
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func requiredTestClaims(validation ValidationConfig, now time.Time) Claims {
	return Claims{
		Role:      authx.RoleResident,
		SessionID: "session-1",
		TokenType: authx.TokenTypeAPI,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    validation.Issuer,
			Subject:   "user-1",
			Audience:  []string{validation.Audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        "token-1",
		},
	}
}

func signedTestToken(t *testing.T, privateKey *rsa.PrivateKey, issuer string, audience []string, tokenType string, signingMethod jwt.SigningMethod) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(signingMethod, Claims{
		Role:      authx.RoleResident,
		SessionID: "session-1",
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   "user-1",
			Audience:  audience,
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        "token-1",
		},
	})
	keyID, err := authx.KeyIDForRSAPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("derive key ID: %v", err)
	}
	token.Header["kid"] = keyID

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
