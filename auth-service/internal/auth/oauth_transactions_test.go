package auth

import (
	"context"
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
	if transaction.Request.Resource != "https://home.example/mcp" {
		t.Fatalf("resource = %q", transaction.Request.Resource)
	}
}
