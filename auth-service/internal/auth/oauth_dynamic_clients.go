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

type DynamicOAuthClientRegistration struct {
	Name                    string
	RedirectURIs            []string
	ApplicationType         string
	TokenEndpointAuthMethod string
}

func (s *Service) RegisterOAuthClient(ctx context.Context, name string, redirectURIs []string) (OAuthClient, error) {
	return s.RegisterDynamicOAuthClient(ctx, DynamicOAuthClientRegistration{
		Name:                    name,
		RedirectURIs:            redirectURIs,
		ApplicationType:         "native",
		TokenEndpointAuthMethod: "none",
	})
}

func (s *Service) RegisterDynamicOAuthClient(ctx context.Context, registration DynamicOAuthClientRegistration) (OAuthClient, error) {
	if s.cacheStore == nil || strings.TrimSpace(registration.Name) == "" {
		return OAuthClient{}, fmt.Errorf("invalid dynamic client registration")
	}
	if registration.ApplicationType == "" {
		registration.ApplicationType = "native"
	}
	if registration.TokenEndpointAuthMethod == "" {
		registration.TokenEndpointAuthMethod = "none"
	}
	if registration.ApplicationType != "native" && registration.ApplicationType != "web" || registration.TokenEndpointAuthMethod != "none" {
		return OAuthClient{}, fmt.Errorf("unsupported dynamic client metadata")
	}
	if err := validateDynamicOAuthClientRedirectURIs(registration.RedirectURIs, registration.ApplicationType); err != nil {
		return OAuthClient{}, err
	}
	clientID, err := newTokenID()
	if err != nil {
		return OAuthClient{}, err
	}
	client := OAuthClient{ClientID: "hn_" + clientID, DisplayName: strings.TrimSpace(registration.Name), RedirectURIs: registration.RedirectURIs, Scopes: dynamicClientScopes, ApplicationType: registration.ApplicationType, TokenEndpointAuthMethod: registration.TokenEndpointAuthMethod, Enabled: true}
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
	if client.ApplicationType == "" {
		client.ApplicationType = "native"
	}
	if client.TokenEndpointAuthMethod == "" {
		client.TokenEndpointAuthMethod = "none"
	}
	registry, err := NewOAuthClientRegistry([]OAuthClient{client}, s.config.OAuthMCPResource)
	if err != nil {
		return OAuthClient{}, err
	}
	return registry.ValidateAuthorizationRequest(request)
}

func validateDynamicOAuthClientRedirectURIs(redirectURIs []string, applicationType string) error {
	if len(redirectURIs) == 0 || len(redirectURIs) != len(unique(redirectURIs)) {
		return fmt.Errorf("at least one unique redirect URI is required")
	}
	for _, rawURI := range redirectURIs {
		uri, err := url.Parse(rawURI)
		if err != nil || uri.Fragment != "" || uri.User != nil || !isDynamicOAuthRedirectURI(uri, applicationType) {
			return fmt.Errorf("dynamic clients require a valid %s redirect URI", applicationType)
		}
	}
	return nil
}

func isDynamicOAuthRedirectURI(uri *url.URL, applicationType string) bool {
	if uri.Scheme == "http" {
		return applicationType == "native" && uri.Host != "" && isLoopbackHost(uri.Hostname())
	}
	if uri.Scheme == "https" {
		return uri.Host != ""
	}
	return applicationType == "native" && validNativeCustomOAuthRedirectURI(uri)
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
