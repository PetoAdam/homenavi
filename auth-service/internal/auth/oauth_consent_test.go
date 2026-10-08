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
	if store.lastTTL != oauthConsentTTL {
		t.Fatalf("consent TTL = %v, want %v", store.lastTTL, oauthConsentTTL)
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
