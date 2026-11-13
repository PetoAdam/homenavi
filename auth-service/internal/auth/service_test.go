package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/oauth2"
)

type oauthTestTransport func(*http.Request) (*http.Response, error)

func (transport oauthTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestGoogleOAuthUsesRequestTraceWithoutExternalPropagation(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	service := &Service{googleOAuthConfig: &oauth2.Config{ClientID: "client", ClientSecret: "secret", Endpoint: oauth2.Endpoint{TokenURL: "https://oauth.example/token", AuthStyle: oauth2.AuthStyleInParams}}}
	transport := oauthTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("traceparent") != "" || request.Header.Get("baggage") != "" {
			t.Error("OAuth request leaked internal tracing headers")
		}
		body := `{"id":"google-id","email":"user@example.test"}`
		if request.URL.Path == "/token" {
			body = `{"access_token":"secret-token","token_type":"Bearer"}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Transport: transport})
	user, err := service.ExchangeGoogleOAuthCode(ctx, "secret-code", "")
	parent.End()
	if err != nil || user.ID != "google-id" {
		t.Fatalf("OAuth exchange failed: %v", err)
	}
	spans := recorder.Ended()
	if len(spans) != 3 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() || spans[1].Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("OAuth HTTP spans did not follow the request")
	}
}

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
	tokenString, err := service.IssueMCPAccessToken(&clientsinfra.User{ID: "user-1", Role: authx.RoleResident}, OAuthAuthorizationGrant{Subject: "user-1", SessionID: "session-1", ClientID: "mcp-cli", Scope: "home.devices.read", Resource: "https://home.example/mcp", GrantID: "grant-1"})
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
	if claims[authx.ClaimTokenType] != authx.TokenTypeMCP || claims[authx.ClaimAuthorizedParty] != "mcp-cli" || claims[authx.ClaimMCPGrantID] != "grant-1" {
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
	_, err = service.IssueMCPAccessToken(&clientsinfra.User{ID: "user-1", Role: authx.RoleUser}, OAuthAuthorizationGrant{Subject: "user-1", SessionID: "session-1", ClientID: "mcp-cli", Scope: "home.devices.read", Resource: "https://home.example/mcp", GrantID: "grant-1"})
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
	if err := store.Set(context.Background(), authx.MCPGrantStatusKey("grant-1"), authx.SessionStatusActive, 0); err != nil {
		t.Fatalf("activate grant: %v", err)
	}
	mcpToken, err := service.IssueMCPAccessToken(&clientsinfra.User{ID: "user-1", Role: authx.RoleResident}, OAuthAuthorizationGrant{Subject: "user-1", SessionID: sessionID, ClientID: "mcp-cli", Scope: "home.devices.write", Resource: "https://mcp.example.test/mcp", GrantID: "grant-1"})
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
	if claims[authx.ClaimTokenType] != authx.TokenTypeDelegated || claims[authx.ClaimScope] != "home.devices.write" || claims[authx.ClaimSessionID] != sessionID || claims[authx.ClaimAuthorizedParty] != "mcp-cli" || claims[authx.ClaimMCPGrantID] != "grant-1" || claims["role"] != authx.RoleResident {
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
	if err := store.Set(context.Background(), authx.MCPGrantStatusKey("grant-1"), authx.SessionStatusActive, 0); err != nil {
		t.Fatalf("activate grant: %v", err)
	}
	mcpToken, err := service.IssueMCPAccessToken(&clientsinfra.User{ID: "user-1", Role: authx.RoleResident}, OAuthAuthorizationGrant{Subject: "user-1", SessionID: sessionID, ClientID: "mcp-cli", Scope: "home.history.read", Resource: "https://mcp.example.test/mcp", GrantID: "grant-1"})
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

func TestExchangeMCPAccessTokenAllowsOnlyDiscoveryScopePair(t *testing.T) {
	store := newMemoryRefreshStore()
	service := newRefreshTestService(t, store)
	if err := service.activateSession("session-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(context.Background(), authx.MCPGrantStatusKey("grant-1"), authx.SessionStatusActive, 0); err != nil {
		t.Fatal(err)
	}
	resource := "https://home.example/mcp"
	token, err := service.IssueMCPAccessToken(&clientsinfra.User{ID: "user-1", Role: authx.RoleResident}, OAuthAuthorizationGrant{Subject: "user-1", SessionID: "session-1", ClientID: "client", Scope: "home.devices.read home.inventory.read home.devices.write", Resource: resource, GrantID: "grant-1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		scope string
		valid bool
	}{
		{"home.devices.read home.inventory.read", true},
		{"home.inventory.read home.devices.read", true},
		{"home.devices.read home.devices.write", false},
		{"home.devices.read home.devices.read", false},
		{"home.devices.read unknown", false},
		{"", false},
	} {
		delegated, err := service.ExchangeMCPAccessToken(context.Background(), token, resource, test.scope)
		if (err == nil) != test.valid {
			t.Errorf("scope=%q err=%v", test.scope, err)
		}
		if err == nil {
			parsed, err := jwt.Parse(delegated, func(token *jwt.Token) (any, error) {
				keyID, _ := token.Header["kid"].(string)
				return service.keyRing.PublicKeyForID(keyID)
			})
			if err != nil || parsed.Claims.(jwt.MapClaims)[authx.ClaimScope] != test.scope {
				t.Fatalf("scope=%q delegated claims invalid: %v", test.scope, err)
			}
		}
	}
}

func TestExchangeMCPAccessTokenRejectsRevokedGrant(t *testing.T) {
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
	if err := store.Set(context.Background(), authx.MCPGrantStatusKey("grant-1"), authx.SessionStatusActive, 0); err != nil {
		t.Fatalf("activate grant: %v", err)
	}
	mcpToken, err := service.IssueMCPAccessToken(&clientsinfra.User{ID: "user-1", Role: authx.RoleResident}, OAuthAuthorizationGrant{Subject: "user-1", SessionID: sessionID, ClientID: "mcp-cli", Scope: "home.devices.read", Resource: "https://mcp.example.test/mcp", GrantID: "grant-1"})
	if err != nil {
		t.Fatalf("issue MCP token: %v", err)
	}
	if err := store.Delete(context.Background(), authx.MCPGrantStatusKey("grant-1")); err != nil {
		t.Fatalf("revoke grant: %v", err)
	}
	if _, err := service.ExchangeMCPAccessToken(context.Background(), mcpToken, "https://mcp.example.test/mcp", "home.devices.read"); err == nil {
		t.Fatal("expected revoked grant to reject token exchange")
	}
}

func TestVerifySecondFactorAcceptsStoredEmailCodeAndRejectsInvalidCode(t *testing.T) {
	store := newMemoryRefreshStore()
	service := newRefreshTestService(t, store)
	user := &clientsinfra.User{ID: "user-1", TwoFactorEnabled: true, TwoFactorType: "email"}
	if err := service.StoreVerificationCode("2fa_email", user.ID, "123456"); err != nil {
		t.Fatalf("store email code: %v", err)
	}
	if err := service.VerifySecondFactor(user, "000000"); err == nil || err.Code != 401 {
		t.Fatalf("invalid code error = %#v, want unauthorized", err)
	}
	if err := service.VerifySecondFactor(user, "123456"); err != nil {
		t.Fatalf("valid code rejected: %v", err)
	}
}

func TestCancelVerificationCodeDeletesPendingCode(t *testing.T) {
	store := newMemoryRefreshStore()
	service := newRefreshTestService(t, store)
	if err := service.StoreVerificationCode("2fa_email", "user-1", "123456"); err != nil {
		t.Fatalf("store email code: %v", err)
	}
	if err := service.CancelVerificationCode("2fa_email", "user-1"); err != nil {
		t.Fatalf("cancel verification code: %v", err)
	}
	if err := service.ValidateVerificationCode("2fa_email", "user-1", "123456"); err == nil {
		t.Fatal("cancelled email code was accepted")
	}
}
