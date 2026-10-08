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
