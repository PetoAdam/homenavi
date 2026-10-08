package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"strings"
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
		JWTPrivateKey:   privateKey,
		JWTIssuer:       "https://auth.example.test",
		JWTAPIAudience:  "https://api.example.test",
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	}, newMemoryRefreshStore())

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
	if got := claims[authx.ClaimSessionID]; got == "" || got == nil {
		t.Fatalf("session ID = %v, want a non-empty value", got)
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

func TestIssueMCPAccessTokenUsesMCPAudienceAndType(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	service := NewService(Config{JWTPrivateKey: privateKey, JWTIssuer: "https://auth.example.test"}, nil)
	tokenString, err := service.IssueMCPAccessToken(&clientsinfra.User{ID: "user-1", Role: authx.RoleResident}, OAuthAuthorizationGrant{Subject: "user-1", SessionID: "session-1", ClientID: "mcp-cli", Scope: "home.devices.read", Resource: "https://home.example/mcp"})
	if err != nil {
		t.Fatalf("issue MCP token: %v", err)
	}
	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) { return &privateKey.PublicKey, nil })
	if err != nil {
		t.Fatalf("parse MCP token: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	audience, err := claims.GetAudience()
	if err != nil || !authx.HasAudience(audience, "https://home.example/mcp") || authx.HasAudience(audience, authx.AudienceAPI) {
		t.Fatalf("unexpected MCP audience: %v, %v", audience, err)
	}
	if claims[authx.ClaimTokenType] != authx.TokenTypeMCP || claims[authx.ClaimAuthorizedParty] != "mcp-cli" {
		t.Fatalf("unexpected MCP claims: %#v", claims)
	}
	if claims[authx.ClaimSessionID] != "session-1" {
		t.Fatalf("MCP session ID = %#v", claims[authx.ClaimSessionID])
	}
	issuedAt, issuedAtErr := claims.GetIssuedAt()
	expiresAt, expiresAtErr := claims.GetExpirationTime()
	if issuedAtErr != nil || expiresAtErr != nil || expiresAt.Time.Sub(issuedAt.Time) != MCPAccessTokenTTL {
		t.Fatalf("unexpected MCP token lifetime: issued_at=%v expires_at=%v errors=%v,%v", issuedAt, expiresAt, issuedAtErr, expiresAtErr)
	}
}

func TestIssueMCPAccessTokenRejectsNonResidentUser(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	service := NewService(Config{JWTPrivateKey: privateKey, JWTIssuer: "https://auth.example.test"}, nil)
	_, err = service.IssueMCPAccessToken(&clientsinfra.User{ID: "user-1", Role: authx.RoleUser}, OAuthAuthorizationGrant{Subject: "user-1", SessionID: "session-1", ClientID: "mcp-cli", Scope: "home.devices.read", Resource: "https://home.example/mcp"})
	if err == nil || !strings.Contains(err.Error(), "resident role") {
		t.Fatalf("expected resident role error, got %v", err)
	}
}

func TestExchangeMCPAccessTokenIssuesGatewayDelegatedToken(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	store := newMemoryRefreshStore()
	service := NewService(Config{
		JWTPrivateKey:                privateKey,
		JWTIssuer:                    "https://auth.example.test",
		JWTAPIAudience:               "https://gateway.example.test",
		MCPAuthorizationServerIssuer: "https://auth.example.test",
	}, store)
	const sessionID = "session-1"
	if err := store.Set(context.Background(), authx.SessionStatusKey(sessionID), authx.SessionStatusActive, time.Hour); err != nil {
		t.Fatalf("activate session: %v", err)
	}
	mcpToken, err := service.IssueMCPAccessToken(&clientsinfra.User{ID: "user-1", Role: authx.RoleResident}, OAuthAuthorizationGrant{Subject: "user-1", SessionID: sessionID, ClientID: "mcp-cli", Scope: "home.devices.write", Resource: "https://mcp.example.test/mcp"})
	if err != nil {
		t.Fatalf("issue MCP token: %v", err)
	}
	delegatedToken, err := service.ExchangeMCPAccessToken(context.Background(), mcpToken, "https://mcp.example.test/mcp", "home.devices.write")
	if err != nil {
		t.Fatalf("exchange MCP token: %v", err)
	}
	parsed, err := jwt.Parse(delegatedToken, func(token *jwt.Token) (any, error) { return &privateKey.PublicKey, nil })
	if err != nil {
		t.Fatalf("parse delegated token: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	audience, err := claims.GetAudience()
	if err != nil || !authx.HasAudience(audience, "https://gateway.example.test") {
		t.Fatalf("unexpected delegated audience: %v, %v", audience, err)
	}
	if claims[authx.ClaimTokenType] != authx.TokenTypeDelegated || claims[authx.ClaimScope] != "home.devices.write" || claims[authx.ClaimSessionID] != sessionID || claims[authx.ClaimAuthorizedParty] != "mcp-cli" || claims["role"] != authx.RoleResident {
		t.Fatalf("unexpected delegated claims: %#v", claims)
	}
}

func TestExchangeMCPAccessTokenAllowsDelegatedReadScopes(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	store := newMemoryRefreshStore()
	service := NewService(Config{JWTPrivateKey: privateKey, JWTIssuer: "https://auth.example.test", JWTAPIAudience: "https://gateway.example.test", MCPAuthorizationServerIssuer: "https://auth.example.test"}, store)
	const sessionID = "session-1"
	if err := store.Set(context.Background(), authx.SessionStatusKey(sessionID), authx.SessionStatusActive, time.Hour); err != nil {
		t.Fatalf("activate session: %v", err)
	}
	mcpToken, err := service.IssueMCPAccessToken(&clientsinfra.User{ID: "user-1", Role: authx.RoleResident}, OAuthAuthorizationGrant{Subject: "user-1", SessionID: sessionID, ClientID: "mcp-cli", Scope: "home.history.read", Resource: "https://mcp.example.test/mcp"})
	if err != nil {
		t.Fatalf("issue MCP token: %v", err)
	}
	delegatedToken, err := service.ExchangeMCPAccessToken(context.Background(), mcpToken, "https://mcp.example.test/mcp", "home.history.read")
	if err != nil {
		t.Fatalf("exchange MCP token: %v", err)
	}
	parsed, err := jwt.Parse(delegatedToken, func(token *jwt.Token) (any, error) { return &privateKey.PublicKey, nil })
	if err != nil {
		t.Fatalf("parse delegated token: %v", err)
	}
	if scope := parsed.Claims.(jwt.MapClaims)[authx.ClaimScope]; scope != "home.history.read" {
		t.Fatalf("delegated scope = %q", scope)
	}
}
