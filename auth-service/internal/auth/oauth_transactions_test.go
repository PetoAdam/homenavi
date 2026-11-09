package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

type transactionTTLStore struct {
	*memoryRefreshStore
	lastTTL time.Duration
}

func (s *transactionTTLStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	s.lastTTL = ttl
	return s.memoryRefreshStore.Set(ctx, key, value, ttl)
}

func TestOAuthAuthorizationSessionRefreshesTransactionTTL(t *testing.T) {
	store := &transactionTTLStore{memoryRefreshStore: newMemoryRefreshStore()}
	service := &Service{cacheStore: store}
	transaction := OAuthAuthorizationTransaction{ID: "transaction-1"}
	if err := service.saveOAuthAuthorizationTransaction(context.Background(), transaction, time.Minute); err != nil {
		t.Fatalf("save transaction: %v", err)
	}
	if err := service.SetOAuthAuthorizationSession(context.Background(), transaction.ID, "user-1", "session-1"); err != nil {
		t.Fatalf("set authorization session: %v", err)
	}
	if store.lastTTL != oauthAuthorizationTransactionTTL {
		t.Fatalf("transaction TTL = %v, want %v", store.lastTTL, oauthAuthorizationTransactionTTL)
	}
}

func TestBeginOAuthAuthorizationDefaultsConfiguredMCPResource(t *testing.T) {
	service := newRefreshTestService(t, newMemoryRefreshStore())
	registry, err := NewOAuthClientRegistry([]OAuthClient{{
		ClientID:     "open-webui",
		DisplayName:  "Open WebUI",
		RedirectURIs: []string{"https://openwebui.example/oauth/callback"},
		Scopes:       []string{"home.devices.read"},
		Enabled:      true,
	}}, "https://home.example/mcp")
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	service.oauthClients = registry

	transaction, err := service.BeginOAuthAuthorization(context.Background(), OAuthAuthorizationRequest{
		ClientID:            "open-webui",
		RedirectURI:         "https://openwebui.example/oauth/callback",
		ResponseType:        "code",
		Scope:               "home.devices.read",
		State:               "state",
		CodeChallenge:       "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~",
		CodeChallengeMethod: "S256",
	})
	if err != nil {
		t.Fatalf("begin authorization: %v", err)
	}
	if transaction.Request.Resource != "https://home.example/mcp" || transaction.ClientName != "Open WebUI" {
		t.Fatalf("resource = %q, name = %q", transaction.Request.Resource, transaction.ClientName)
	}
}

func TestSelectedScopePersistsThroughAuthorizationAndRefresh(t *testing.T) {
	ctx := context.Background()
	service := newRefreshTestService(t, newMemoryRefreshStore())
	registry, err := NewOAuthClientRegistry([]OAuthClient{{ClientID: "client", DisplayName: "Client", RedirectURIs: []string{"https://client.example/callback"}, Scopes: []string{"home.devices.read", "home.devices.write"}, Enabled: true}}, "https://home.example/mcp")
	if err != nil {
		t.Fatal(err)
	}
	service.oauthClients = registry
	verifier := strings.Repeat("a", 43)
	digest := sha256.Sum256([]byte(verifier))
	request := OAuthAuthorizationRequest{ClientID: "client", RedirectURI: "https://client.example/callback", ResponseType: "code", Scope: "home.devices.read", State: "state", CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: "S256", Resource: "https://home.example/mcp"}
	if err := service.activateSession("session-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.GrantOAuthConsent(ctx, request, "user-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	code, err := service.AuthorizeOAuthForSession(ctx, request, "user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := service.ExchangeOAuthAuthorizationCode(ctx, code, OAuthAuthorizationCodeExchange{ClientID: request.ClientID, RedirectURI: request.RedirectURI, Resource: request.Resource, CodeVerifier: verifier})
	if err != nil || grant.Scope != request.Scope {
		t.Fatalf("grant=%#v err=%v", grant, err)
	}
	refresh, err := service.IssueOAuthRefreshToken(grant)
	if err != nil {
		t.Fatal(err)
	}
	renewed, _, err := service.RefreshOAuthAuthorizationGrant(refresh)
	if err != nil || renewed.Scope != request.Scope {
		t.Fatalf("renewed=%#v err=%v", renewed, err)
	}
}
