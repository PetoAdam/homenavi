package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
)

const oauthAuthorizationCodeTTL = 5 * time.Minute

type OAuthAuthorizationCodeInput struct {
	Subject       string
	SessionID     string
	ClientID      string
	RedirectURI   string
	Scope         string
	Resource      string
	CodeChallenge string
}

type OAuthAuthorizationCodeExchange struct {
	ClientID     string
	RedirectURI  string
	Resource     string
	CodeVerifier string
}

type OAuthAuthorizationGrant struct {
	Subject   string
	SessionID string
	ClientID  string
	Scope     string
	Resource  string
}

type oauthAuthorizationCodeRecord struct {
	OAuthAuthorizationCodeInput
}

type OAuthAuthorizationCodeManager struct {
	store cacheinfra.Store
}

func NewOAuthAuthorizationCodeManager(store cacheinfra.Store) *OAuthAuthorizationCodeManager {
	return &OAuthAuthorizationCodeManager{store: store}
}

func (m *OAuthAuthorizationCodeManager) Issue(ctx context.Context, input OAuthAuthorizationCodeInput) (string, error) {
	if input.Subject == "" || input.SessionID == "" || input.ClientID == "" || input.RedirectURI == "" || input.Resource == "" || !pkceVerifierPattern.MatchString(input.CodeChallenge) {
		return "", fmt.Errorf("invalid authorization code input")
	}
	codeBytes := make([]byte, 32)
	if _, err := rand.Read(codeBytes); err != nil {
		return "", fmt.Errorf("generate authorization code: %w", err)
	}
	code := base64.RawURLEncoding.EncodeToString(codeBytes)
	record, err := json.Marshal(oauthAuthorizationCodeRecord{OAuthAuthorizationCodeInput: input})
	if err != nil {
		return "", fmt.Errorf("encode authorization code: %w", err)
	}
	if err := m.store.Set(ctx, oauthAuthorizationCodeKey(code), string(record), oauthAuthorizationCodeTTL); err != nil {
		return "", fmt.Errorf("store authorization code: %w", err)
	}
	return code, nil
}

func (m *OAuthAuthorizationCodeManager) Consume(ctx context.Context, code string, exchange OAuthAuthorizationCodeExchange) (OAuthAuthorizationGrant, error) {
	if code == "" || !pkceVerifierPattern.MatchString(exchange.CodeVerifier) {
		return OAuthAuthorizationGrant{}, fmt.Errorf("invalid authorization code exchange")
	}
	recordJSON, err := m.store.GetDelete(ctx, oauthAuthorizationCodeKey(code))
	if err != nil {
		return OAuthAuthorizationGrant{}, fmt.Errorf("authorization code is invalid or expired")
	}
	var record oauthAuthorizationCodeRecord
	if err := json.Unmarshal([]byte(recordJSON), &record); err != nil {
		return OAuthAuthorizationGrant{}, fmt.Errorf("decode authorization code: %w", err)
	}
	if exchange.Resource == "" {
		exchange.Resource = record.Resource
	}
	if record.ClientID != exchange.ClientID || record.RedirectURI != exchange.RedirectURI || record.Resource != exchange.Resource {
		return OAuthAuthorizationGrant{}, fmt.Errorf("authorization code binding does not match")
	}
	verifierDigest := sha256.Sum256([]byte(exchange.CodeVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(verifierDigest[:])
	if subtle.ConstantTimeCompare([]byte(record.CodeChallenge), []byte(challenge)) != 1 {
		return OAuthAuthorizationGrant{}, fmt.Errorf("PKCE verification failed")
	}
	return OAuthAuthorizationGrant{Subject: record.Subject, SessionID: record.SessionID, ClientID: record.ClientID, Scope: record.Scope, Resource: record.Resource}, nil
}

func oauthAuthorizationCodeKey(code string) string {
	digest := sha256.Sum256([]byte(code))
	return "oauth_authorization_code:" + hex.EncodeToString(digest[:])
}
