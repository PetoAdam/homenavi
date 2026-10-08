package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const oauthDynamicClientPrefix = "oauth_dynamic_client:"

var dynamicClientScopes = []string{
	"home.devices.read",
	"home.devices.write",
	"home.inventory.read",
	"home.inventory.write",
	"home.history.read",
	"home.automation.read",
	"home.automation.execute",
}

func (s *Service) RegisterOAuthClient(ctx context.Context, name string, redirectURIs []string) (OAuthClient, error) {
	if s.cacheStore == nil || strings.TrimSpace(name) == "" {
		return OAuthClient{}, fmt.Errorf("invalid dynamic client registration")
	}
	if err := validateDynamicOAuthClientRedirectURIs(redirectURIs); err != nil {
		return OAuthClient{}, err
	}
	clientID, err := newTokenID()
	if err != nil {
		return OAuthClient{}, err
	}
	client := OAuthClient{ClientID: "hn_" + clientID, DisplayName: strings.TrimSpace(name), RedirectURIs: redirectURIs, Scopes: dynamicClientScopes, Enabled: true}
	if err := validateOAuthClient(client); err != nil {
		return OAuthClient{}, err
	}
	encoded, err := json.Marshal(client)
	if err != nil {
		return OAuthClient{}, err
	}
	if err := s.cacheStore.Set(ctx, oauthDynamicClientPrefix+client.ClientID, string(encoded), 30*24*time.Hour); err != nil {
		return OAuthClient{}, err
	}
	return client, nil
}

func (s *Service) validateOAuthRequest(ctx context.Context, request OAuthAuthorizationRequest) (OAuthClient, error) {
	if client, err := s.oauthClients.ValidateAuthorizationRequest(request); err == nil {
		return client, nil
	}
	encoded, err := s.cacheStore.Get(ctx, oauthDynamicClientPrefix+request.ClientID)
	if err != nil {
		return OAuthClient{}, fmt.Errorf("unknown or disabled OAuth client")
	}
	var client OAuthClient
	if json.Unmarshal([]byte(encoded), &client) != nil {
		return OAuthClient{}, fmt.Errorf("invalid OAuth client")
	}
	registry, err := NewOAuthClientRegistry([]OAuthClient{client}, s.config.OAuthMCPResource)
	if err != nil {
		return OAuthClient{}, err
	}
	return registry.ValidateAuthorizationRequest(request)
}

func validateDynamicOAuthClientRedirectURIs(redirectURIs []string) error {
	if len(redirectURIs) == 0 || len(redirectURIs) != len(unique(redirectURIs)) {
		return fmt.Errorf("at least one unique redirect URI is required")
	}
	for _, rawURI := range redirectURIs {
		uri, err := url.Parse(rawURI)
		if err != nil || uri.Fragment != "" || uri.RawQuery != "" || uri.User != nil || !isDynamicOAuthRedirectURI(uri) {
			return fmt.Errorf("dynamic clients require a loopback or approved VS Code redirect URI")
		}
	}
	return nil
}

func isDynamicOAuthRedirectURI(uri *url.URL) bool {
	if uri.Scheme == "http" {
		return isLoopbackHost(uri.Hostname())
	}
	return uri.Scheme == "https" && uri.Path == "/redirect" && (uri.Host == "vscode.dev" || uri.Host == "insiders.vscode.dev")
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
