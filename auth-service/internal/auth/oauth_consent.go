package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
)

const (
	oauthConsentPrefix = "oauth_consent:"
	oauthConsentTTL    = 0
)

type OAuthConsent struct {
	ClientID   string   `json:"client_id"`
	ClientName string   `json:"client_name,omitempty"`
	Resource   string   `json:"resource"`
	Scopes     []string `json:"scopes"`
	GrantedAt  int64    `json:"granted_at"`
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
	if err := m.store.Set(ctx, oauthConsentKey(subject, request.ClientID), encoded, oauthConsentTTL); err != nil {
		return false, fmt.Errorf("persist OAuth consent: %w", err)
	}
	return true, nil
}

func (m *OAuthConsentManager) Revoke(ctx context.Context, subject, clientID string) error {
	if m == nil || m.store == nil || strings.TrimSpace(subject) == "" || strings.TrimSpace(clientID) == "" {
		return fmt.Errorf("invalid OAuth consent revocation")
	}
	return m.store.Delete(ctx, oauthConsentKey(subject, clientID))
}

func (m *OAuthConsentManager) List(ctx context.Context, subject string) ([]OAuthConsent, error) {
	if m == nil || m.store == nil || strings.TrimSpace(subject) == "" {
		return nil, fmt.Errorf("OAuth consent is not configured")
	}
	keys, err := m.store.Scan(ctx, oauthConsentPrefix+subject+":*", 100)
	if err != nil {
		return nil, fmt.Errorf("list OAuth consents: %w", err)
	}
	consents := make([]OAuthConsent, 0, len(keys))
	for _, key := range keys {
		encoded, err := m.store.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("load OAuth consent: %w", err)
		}
		var consent OAuthConsent
		if err := json.Unmarshal([]byte(encoded), &consent); err != nil || consent.ClientID == "" {
			return nil, fmt.Errorf("decode OAuth consent")
		}
		consents = append(consents, consent)
	}
	sort.Slice(consents, func(left, right int) bool { return consents[left].GrantedAt > consents[right].GrantedAt })
	return consents, nil
}

func oauthConsentKey(subject, clientID string) string {
	return oauthConsentPrefix + subject + ":" + clientID
}
