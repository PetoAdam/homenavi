package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	oauthAuthorizationTransactionPrefix = "oauth_authorization_transaction:"
	oauthAuthorizationTransactionTTL    = 15 * time.Minute
)

// OAuthAuthorizationTransaction binds a browser authorization request to an
// authenticated session without placing bearer credentials in a browser cookie.
type OAuthAuthorizationTransaction struct {
	ID            string                    `json:"-"`
	Request       OAuthAuthorizationRequest `json:"request"`
	Subject       string                    `json:"subject,omitempty"`
	SessionID     string                    `json:"session_id,omitempty"`
	PendingUserID string                    `json:"pending_user_id,omitempty"`
}

func (s *Service) BeginOAuthAuthorization(ctx context.Context, request OAuthAuthorizationRequest) (OAuthAuthorizationTransaction, error) {
	if s.cacheStore == nil {
		return OAuthAuthorizationTransaction{}, fmt.Errorf("authorization transactions are unavailable")
	}
	if _, err := s.validateOAuthRequest(ctx, request); err != nil {
		return OAuthAuthorizationTransaction{}, err
	}
	id, err := newTokenID()
	if err != nil {
		return OAuthAuthorizationTransaction{}, fmt.Errorf("generate authorization transaction ID: %w", err)
	}
	transaction := OAuthAuthorizationTransaction{ID: id, Request: request}
	if err := s.saveOAuthAuthorizationTransaction(ctx, transaction, oauthAuthorizationTransactionTTL); err != nil {
		return OAuthAuthorizationTransaction{}, err
	}
	return transaction, nil
}

func (s *Service) GetOAuthAuthorizationTransaction(ctx context.Context, id string) (OAuthAuthorizationTransaction, error) {
	if s.cacheStore == nil || id == "" {
		return OAuthAuthorizationTransaction{}, fmt.Errorf("authorization transaction not found")
	}
	encoded, err := s.cacheStore.Get(ctx, oauthAuthorizationTransactionPrefix+id)
	if err != nil {
		return OAuthAuthorizationTransaction{}, fmt.Errorf("authorization transaction not found")
	}
	var transaction OAuthAuthorizationTransaction
	if err := json.Unmarshal([]byte(encoded), &transaction); err != nil {
		return OAuthAuthorizationTransaction{}, fmt.Errorf("invalid authorization transaction")
	}
	transaction.ID = id
	return transaction, nil
}

func (s *Service) SetOAuthAuthorizationSession(ctx context.Context, id, subject, sessionID string) error {
	transaction, err := s.GetOAuthAuthorizationTransaction(ctx, id)
	if err != nil || subject == "" || sessionID == "" {
		return fmt.Errorf("authorization transaction not found")
	}
	transaction.Subject = subject
	transaction.SessionID = sessionID
	transaction.PendingUserID = ""
	return s.saveOAuthAuthorizationTransaction(ctx, transaction, oauthAuthorizationTransactionTTL)
}

func (s *Service) SetOAuthAuthorizationPendingUser(ctx context.Context, id, userID string) error {
	transaction, err := s.GetOAuthAuthorizationTransaction(ctx, id)
	if err != nil || userID == "" {
		return fmt.Errorf("authorization transaction not found")
	}
	transaction.PendingUserID = userID
	return s.saveOAuthAuthorizationTransaction(ctx, transaction, oauthAuthorizationTransactionTTL)
}

func (s *Service) DeleteOAuthAuthorizationTransaction(ctx context.Context, id string) error {
	if s.cacheStore == nil || id == "" {
		return nil
	}
	return s.cacheStore.Delete(ctx, oauthAuthorizationTransactionPrefix+id)
}

func (s *Service) saveOAuthAuthorizationTransaction(ctx context.Context, transaction OAuthAuthorizationTransaction, ttl time.Duration) error {
	if ttl == 0 {
		remaining, err := s.cacheStore.TTL(ctx, oauthAuthorizationTransactionPrefix+transaction.ID)
		if err != nil || remaining <= 0 {
			return fmt.Errorf("authorization transaction not found")
		}
		ttl = remaining
	}
	id := transaction.ID
	transaction.ID = ""
	encoded, err := json.Marshal(transaction)
	if err != nil {
		return err
	}
	return s.cacheStore.Set(ctx, oauthAuthorizationTransactionPrefix+id, string(encoded), ttl)
}
