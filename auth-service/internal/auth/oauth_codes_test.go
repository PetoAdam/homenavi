package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestOAuthAuthorizationCodeConsumesOnceWithBoundPKCE(t *testing.T) {
	store := newMemoryRefreshStore()
	manager := NewOAuthAuthorizationCodeManager(store)
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	input := OAuthAuthorizationCodeInput{Subject: "user-1", SessionID: "session-1", ClientID: "mcp-cli", RedirectURI: "https://client.example/callback", Scope: "home.devices.read", Resource: "https://home.example/mcp", CodeChallenge: challenge}
	code, err := manager.Issue(context.Background(), input)
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	grant, err := manager.Consume(context.Background(), code, OAuthAuthorizationCodeExchange{ClientID: input.ClientID, RedirectURI: input.RedirectURI, Resource: input.Resource, CodeVerifier: verifier})
	if err != nil || grant.Subject != input.Subject || grant.SessionID != input.SessionID || grant.Scope != input.Scope {
		t.Fatalf("consume code = %#v, %v", grant, err)
	}
	if _, err := manager.Consume(context.Background(), code, OAuthAuthorizationCodeExchange{ClientID: input.ClientID, RedirectURI: input.RedirectURI, Resource: input.Resource, CodeVerifier: verifier}); err == nil {
		t.Fatal("expected reused code to fail")
	}
}

func TestOAuthAuthorizationCodeRejectsWrongVerifierAndConsumesCode(t *testing.T) {
	store := newMemoryRefreshStore()
	manager := NewOAuthAuthorizationCodeManager(store)
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	digest := sha256.Sum256([]byte(verifier))
	code, err := manager.Issue(context.Background(), OAuthAuthorizationCodeInput{Subject: "user-1", SessionID: "session-1", ClientID: "mcp-cli", RedirectURI: "https://client.example/callback", Scope: "read", Resource: "https://home.example/mcp", CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:])})
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	exchange := OAuthAuthorizationCodeExchange{ClientID: "mcp-cli", RedirectURI: "https://client.example/callback", Resource: "https://home.example/mcp", CodeVerifier: "012345678901234567890123456789012345678901234567890"}
	if _, err := manager.Consume(context.Background(), code, exchange); err == nil {
		t.Fatal("expected wrong verifier to fail")
	}
	if _, err := manager.Consume(context.Background(), code, exchange); err == nil {
		t.Fatal("expected failed exchange to consume authorization code")
	}
}

func TestOAuthAuthorizationCodeDefaultsOmittedResource(t *testing.T) {
	store := newMemoryRefreshStore()
	manager := NewOAuthAuthorizationCodeManager(store)
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	digest := sha256.Sum256([]byte(verifier))
	input := OAuthAuthorizationCodeInput{Subject: "user-1", SessionID: "session-1", ClientID: "open-webui", RedirectURI: "https://openwebui.example/oauth/callback", Scope: "home.devices.read", Resource: "https://home.example/mcp", CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:])}
	code, err := manager.Issue(context.Background(), input)
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	grant, err := manager.Consume(context.Background(), code, OAuthAuthorizationCodeExchange{ClientID: input.ClientID, RedirectURI: input.RedirectURI, CodeVerifier: verifier})
	if err != nil {
		t.Fatalf("consume code without resource: %v", err)
	}
	if grant.Resource != input.Resource {
		t.Fatalf("resource = %q, want %q", grant.Resource, input.Resource)
	}
}
