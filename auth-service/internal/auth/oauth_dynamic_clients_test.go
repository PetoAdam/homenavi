package auth

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterOAuthClientAllowsHTTPSAndLoopbackRedirects(t *testing.T) {
	service := newRefreshTestService(t, newMemoryRefreshStore())
	redirectURIs := []string{"http://127.0.0.1/", "http://127.0.0.1:33418/", "http://[::1]:33418/callback", "https://openwebui.example/oauth/callback", "https://client.example/callback?connection=homenavi"}
	client, err := service.RegisterOAuthClient(context.Background(), "MCP client", redirectURIs)
	if err != nil {
		t.Fatalf("register dynamic client: %v", err)
	}
	if !strings.HasPrefix(client.ClientID, "hn_") || client.Scopes[0] != "home.devices.read" || !contains(client.Scopes, "home.devices.write") || !contains(client.Scopes, "home.automation.execute") {
		t.Fatalf("unexpected dynamic client: %#v", client)
	}

	request := OAuthAuthorizationRequest{
		ClientID: client.ClientID, RedirectURI: "http://127.0.0.1:33418/", ResponseType: "code", State: "state", Scope: "home.devices.read", CodeChallenge: strings.Repeat("a", 43), CodeChallengeMethod: "S256", Resource: "http://localhost:8080/mcp",
	}
	if _, err := service.validateOAuthRequest(context.Background(), request); err != nil {
		t.Fatalf("validate registered client: %v", err)
	}
	request.RedirectURI = "https://openwebui.example/oauth/callback"
	if _, err := service.validateOAuthRequest(context.Background(), request); err != nil {
		t.Fatalf("validate registered HTTPS client: %v", err)
	}
	request.Scope = "home.devices.write home.automation.execute"
	if _, err := service.validateOAuthRequest(context.Background(), request); err != nil {
		t.Fatalf("validate registered controlled scopes: %v", err)
	}
}

func TestRegisterOAuthClientRejectsUnsafeRedirects(t *testing.T) {
	service := newRefreshTestService(t, newMemoryRefreshStore())
	for _, redirectURI := range []string{"http://client.example/callback", "ftp://client.example/callback", "https://user@client.example/callback", "https://client.example/callback#fragment", "/callback", "http://localhost:33418/callback#fragment"} {
		if _, err := service.RegisterOAuthClient(context.Background(), "Unsafe client", []string{redirectURI}); err == nil {
			t.Fatalf("expected redirect URI %q to be rejected", redirectURI)
		}
	}
}

func TestRegisterDynamicOAuthClientAllowsNativeCustomSchemeAndPreservesMetadata(t *testing.T) {
	service := newRefreshTestService(t, newMemoryRefreshStore())
	client, err := service.RegisterDynamicOAuthClient(context.Background(), DynamicOAuthClientRegistration{
		Name:                    "Homenavi Mobile",
		RedirectURIs:            []string{"mcp-agent://callback"},
		ApplicationType:         "native",
		TokenEndpointAuthMethod: "none",
	})
	if err != nil {
		t.Fatalf("register native custom-scheme client: %v", err)
	}
	if client.ApplicationType != "native" || client.TokenEndpointAuthMethod != "none" {
		t.Fatalf("unexpected client metadata: %#v", client)
	}
	request := OAuthAuthorizationRequest{ClientID: client.ClientID, RedirectURI: "mcp-agent://callback", ResponseType: "code", State: "state", Scope: "home.devices.write", CodeChallenge: strings.Repeat("a", 43), CodeChallengeMethod: "S256", Resource: "http://localhost:8080/mcp"}
	if _, err := service.validateOAuthRequest(context.Background(), request); err != nil {
		t.Fatalf("validate native custom-scheme client: %v", err)
	}
}

func TestRegisterDynamicOAuthClientRejectsUnsafePublicClientMetadata(t *testing.T) {
	service := newRefreshTestService(t, newMemoryRefreshStore())
	for _, registration := range []DynamicOAuthClientRegistration{
		{Name: "Web custom scheme", RedirectURIs: []string{"mcp-agent://callback"}, ApplicationType: "web", TokenEndpointAuthMethod: "none"},
		{Name: "Native secret", RedirectURIs: []string{"mcp-agent://callback"}, ApplicationType: "native", TokenEndpointAuthMethod: "client_secret_post"},
		{Name: "Native opaque URI", RedirectURIs: []string{"mcp-agent:callback"}, ApplicationType: "native", TokenEndpointAuthMethod: "none"},
	} {
		if _, err := service.RegisterDynamicOAuthClient(context.Background(), registration); err == nil {
			t.Fatalf("expected registration to be rejected: %#v", registration)
		}
	}
}
