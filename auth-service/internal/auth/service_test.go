package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
)

func TestIssueAccessTokenIncludesAPIClaims(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	service := NewService(Config{
		JWTPrivateKey:  privateKey,
		JWTIssuer:      "https://auth.example.test",
		JWTAPIAudience: "https://api.example.test",
		AccessTokenTTL: time.Minute,
	}, nil)

	tokenString, err := service.IssueAccessToken(&clientsinfra.User{
		ID:        "user-1",
		FirstName: "Ada",
		LastName:  "Lovelace",
		Role:      authx.RoleResident,
	})
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}

	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return &privateKey.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	if parsed.Method.Alg() != jwt.SigningMethodRS256.Alg() {
		t.Fatalf("expected RS256, got %q", parsed.Method.Alg())
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("unexpected claims type %T", parsed.Claims)
	}
	if issuer, err := claims.GetIssuer(); err != nil || issuer != "https://auth.example.test" {
		t.Fatalf("issuer = %q, error = %v", issuer, err)
	}
	audience, err := claims.GetAudience()
	if err != nil || !authx.HasAudience(audience, "https://api.example.test") {
		t.Fatalf("audience = %v, error = %v", audience, err)
	}
	if got := claims[authx.ClaimTokenType]; got != authx.TokenTypeAPI {
		t.Fatalf("token type = %v, want %q", got, authx.TokenTypeAPI)
	}
	if got := claims["jti"]; got == "" || got == nil {
		t.Fatalf("jti = %v, want a non-empty value", got)
	}
	if got := claims["nbf"]; got == nil {
		t.Fatal("expected not-before claim")
	}
}

func TestIssueAccessTokenRejectsNilUser(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	service := NewService(Config{JWTPrivateKey: privateKey}, nil)

	if _, err := service.IssueAccessToken(nil); err == nil {
		t.Fatal("expected nil user to be rejected")
	}
}
