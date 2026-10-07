package auth

import "testing"

func TestOAuthClientRegistryValidatesAuthorizationRequest(t *testing.T) {
	registry, err := NewOAuthClientRegistry([]OAuthClient{{
		ClientID:     "mcp-cli",
		DisplayName:  "MCP CLI",
		RedirectURIs: []string{"https://client.example/callback"},
		Scopes:       []string{"home.devices.read"},
		Enabled:      true,
	}}, "https://home.example/mcp")
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	valid := OAuthAuthorizationRequest{ClientID: "mcp-cli", RedirectURI: "https://client.example/callback", ResponseType: "code", Scope: "home.devices.read", State: "state", CodeChallenge: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~", CodeChallengeMethod: "S256", Resource: "https://home.example/mcp"}
	if _, err := registry.ValidateAuthorizationRequest(valid); err != nil {
		t.Fatalf("validate request: %v", err)
	}
	for name, mutate := range map[string]func(*OAuthAuthorizationRequest){
		"unregistered redirect": func(request *OAuthAuthorizationRequest) { request.RedirectURI = "https://client.example/other" },
		"missing state":         func(request *OAuthAuthorizationRequest) { request.State = "" },
		"plain PKCE":            func(request *OAuthAuthorizationRequest) { request.CodeChallengeMethod = "plain" },
		"wrong resource":        func(request *OAuthAuthorizationRequest) { request.Resource = "https://home.example/api" },
		"duplicate scope":       func(request *OAuthAuthorizationRequest) { request.Scope += " home.devices.read" },
	} {
		t.Run(name, func(t *testing.T) {
			request := valid
			mutate(&request)
			if _, err := registry.ValidateAuthorizationRequest(request); err == nil {
				t.Fatal("expected authorization request to be rejected")
			}
		})
	}
}

func TestOAuthClientRegistryRejectsUnsafeClients(t *testing.T) {
	_, err := NewOAuthClientRegistry([]OAuthClient{{ClientID: "client", DisplayName: "Client", RedirectURIs: []string{"https://client.example/callback#fragment"}, Scopes: []string{"read"}}}, "https://home.example/mcp")
	if err == nil {
		t.Fatal("expected invalid redirect URI to be rejected")
	}
}

func TestOAuthClientRegistryAllowsLoopbackRedirectsOnly(t *testing.T) {
	for _, redirectURI := range []string{"http://127.0.0.1:33418/", "http://localhost:33418/callback", "https://vscode.dev/redirect"} {
		if _, err := NewOAuthClientRegistry([]OAuthClient{{ClientID: "vscode", DisplayName: "VS Code", RedirectURIs: []string{redirectURI}, Scopes: []string{"home.devices.read"}}}, "https://home.example/mcp"); err != nil {
			t.Fatalf("redirect %q rejected: %v", redirectURI, err)
		}
	}
	if _, err := NewOAuthClientRegistry([]OAuthClient{{ClientID: "unsafe", DisplayName: "Unsafe", RedirectURIs: []string{"http://client.example/callback"}, Scopes: []string{"home.devices.read"}}}, "https://home.example/mcp"); err == nil {
		t.Fatal("non-loopback HTTP redirect was accepted")
	}
}

func TestOAuthClientRegistryAllowsNativeLoopbackCallbackPortChanges(t *testing.T) {
	registry, err := NewOAuthClientRegistry([]OAuthClient{{
		ClientID:     "vscode",
		DisplayName:  "VS Code",
		RedirectURIs: []string{"http://127.0.0.1:33418/"},
		Scopes:       []string{"home.devices.read"},
		Enabled:      true,
	}}, "http://localhost:8080/mcp")
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	request := OAuthAuthorizationRequest{ClientID: "vscode", RedirectURI: "http://127.0.0.1:61887/", ResponseType: "code", Scope: "home.devices.read", State: "state", CodeChallenge: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~", CodeChallengeMethod: "S256", Resource: "http://localhost:8080/mcp"}
	if _, err := registry.ValidateAuthorizationRequest(request); err != nil {
		t.Fatalf("validate loopback redirect with a new port: %v", err)
	}
	request.RedirectURI = "http://127.0.0.1:61887/callback"
	if _, err := registry.ValidateAuthorizationRequest(request); err == nil {
		t.Fatal("expected different loopback path to be rejected")
	}
}

func TestOAuthClientRegistryAllowsValidatedDynamicMCPResource(t *testing.T) {
	registry, err := NewOAuthClientRegistry([]OAuthClient{{ClientID: "mcp-cli", DisplayName: "MCP CLI", RedirectURIs: []string{"https://client.example/callback"}, Scopes: []string{"home.devices.read"}, Enabled: true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	request := OAuthAuthorizationRequest{ClientID: "mcp-cli", RedirectURI: "https://client.example/callback", ResponseType: "code", Scope: "home.devices.read", State: "state", CodeChallenge: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~", CodeChallengeMethod: "S256", Resource: "https://home.example/mcp"}
	if _, err := registry.ValidateAuthorizationRequest(request); err != nil {
		t.Fatalf("dynamic resource rejected: %v", err)
	}
	request.Resource = "https://home.example/api"
	if _, err := registry.ValidateAuthorizationRequest(request); err == nil {
		t.Fatal("non-MCP resource was accepted")
	}
}
