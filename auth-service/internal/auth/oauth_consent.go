package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
)

const (
	oauthConsentPrefix = "oauth_consent:"
	oauthConsentTTL    = 30 * 24 * time.Hour
)

type OAuthConsent struct {
	ClientID  string   `json:"client_id"`
	Resource  string   `json:"resource"`
	Scopes    []string `json:"scopes"`
	GrantedAt int64    `json:"granted_at"`
}

type OAuthConsentManager struct {
	store cacheinfra.Store
}

func NewOAuthConsentManager(store cacheinfra.Store) *OAuthConsentManager {
	return &OAuthConsentManager{store: store}
}

func (m *OAuthConsentManager) Grant(ctx context.Context, subject string, request OAuthAuthorizationRequest) error {
	if m == nil || m.store == nil || strings.TrimSpace(subject) == "" {
		return fmt.Errorf("OAuth consent is not configured")
	}
	consent := OAuthConsent{ClientID: request.ClientID, Resource: request.Resource, Scopes: strings.Fields(request.Scope), GrantedAt: time.Now().UTC().Unix()}
	encoded, err := json.Marshal(consent)
	if err != nil {
		return fmt.Errorf("encode OAuth consent: %w", err)
	}
	return m.store.Set(ctx, oauthConsentKey(subject, request.ClientID), string(encoded), oauthConsentTTL)
}

func (m *OAuthConsentManager) Has(ctx context.Context, subject string, request OAuthAuthorizationRequest) (bool, error) {
	if m == nil || m.store == nil || strings.TrimSpace(subject) == "" {
		return false, fmt.Errorf("OAuth consent is not configured")
	}
	encoded, err := m.store.Get(ctx, oauthConsentKey(subject, request.ClientID))
	if err == cacheinfra.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load OAuth consent: %w", err)
	}
	var consent OAuthConsent
	if err := json.Unmarshal([]byte(encoded), &consent); err != nil {
		return false, fmt.Errorf("decode OAuth consent: %w", err)
	}
	if consent.ClientID != request.ClientID || consent.Resource != request.Resource {
		return false, nil
	}
	granted := make(map[string]struct{}, len(consent.Scopes))
	for _, scope := range consent.Scopes {
		granted[scope] = struct{}{}
	}
	for _, scope := range strings.Fields(request.Scope) {
		if _, ok := granted[scope]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func (m *OAuthConsentManager) Revoke(ctx context.Context, subject, clientID string) error {
	if m == nil || m.store == nil || strings.TrimSpace(subject) == "" || strings.TrimSpace(clientID) == "" {
		return fmt.Errorf("invalid OAuth consent revocation")
	}
	return m.store.Delete(ctx, oauthConsentKey(subject, clientID))
}

func oauthConsentKey(subject, clientID string) string {
	return oauthConsentPrefix + subject + ":" + clientID
}
