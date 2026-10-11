package auth

import (
	"context"
	"testing"
	"time"
)

type consentTTLStore struct {
	*memoryRefreshStore
	lastTTL time.Duration
}

func (s *consentTTLStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	s.lastTTL = ttl
	return s.memoryRefreshStore.Set(ctx, key, value, ttl)
}

func TestOAuthConsentIsScopeBoundAndRevocable(t *testing.T) {
	store := &consentTTLStore{memoryRefreshStore: newMemoryRefreshStore()}
	manager := NewOAuthConsentManager(store)
	request := OAuthAuthorizationRequest{ClientID: "mcp-cli", Resource: "https://home.example/mcp", Scope: "home.devices.read home.history.read"}
	if err := manager.Grant(context.Background(), "user-1", request); err != nil {
		t.Fatalf("grant consent: %v", err)
	}
	if store.lastTTL != 0 {
		t.Fatalf("consent TTL = %v, want persistent storage", store.lastTTL)
	}
	granted, err := manager.Has(context.Background(), "user-1", request)
	if err != nil || !granted {
		t.Fatalf("consent = %t, %v", granted, err)
	}
	granted, err = manager.Has(context.Background(), "user-1", OAuthAuthorizationRequest{ClientID: "mcp-cli", Resource: request.Resource, Scope: "home.devices.read home.inventory.read"})
	if err != nil || granted {
		t.Fatalf("expanded scopes consent = %t, %v", granted, err)
	}
	if err := manager.Revoke(context.Background(), "user-1", "mcp-cli"); err != nil {
		t.Fatalf("revoke consent: %v", err)
	}
	granted, err = manager.Has(context.Background(), "user-1", request)
	if err != nil || granted {
		t.Fatalf("revoked consent = %t, %v", granted, err)
	}
}

func TestOAuthConsentListIsSubjectScopedAndNewestFirst(t *testing.T) {
	store := newMemoryRefreshStore()
	manager := NewOAuthConsentManager(store)
	context := context.Background()
	if err := manager.Grant(context, "user-1", OAuthAuthorizationRequest{ClientID: "older-client", Resource: "https://home.example/mcp", Scope: "home.devices.read"}); err != nil {
		t.Fatalf("grant older consent: %v", err)
	}
	if err := manager.Grant(context, "user-2", OAuthAuthorizationRequest{ClientID: "other-user-client", Resource: "https://home.example/mcp", Scope: "home.devices.read"}); err != nil {
		t.Fatalf("grant other user consent: %v", err)
	}
	store.values[oauthConsentKey("user-1", "newer-client")] = `{"client_id":"newer-client","resource":"https://home.example/mcp","scopes":["home.history.read"],"granted_at":2}`
	store.values[oauthConsentKey("user-1", "older-client")] = `{"client_id":"older-client","resource":"https://home.example/mcp","scopes":["home.devices.read"],"granted_at":1}`

	consents, err := manager.List(context, "user-1")
	if err != nil {
		t.Fatalf("list consents: %v", err)
	}
	if len(consents) != 2 || consents[0].ClientID != "newer-client" || consents[1].ClientID != "older-client" {
		t.Fatalf("unexpected consents: %#v", consents)
	}
}
