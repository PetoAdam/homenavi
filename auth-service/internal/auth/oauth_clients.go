package auth

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var pkceVerifierPattern = regexp.MustCompile(`^[A-Za-z0-9\-._~]{43,128}$`)

// OAuthClient is an administratively configured OAuth client.
type OAuthClient struct {
	ClientID                string   `json:"client_id"`
	DisplayName             string   `json:"display_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	Scopes                  []string `json:"scopes"`
	ApplicationType         string   `json:"application_type,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	Enabled                 bool     `json:"enabled"`
}

type OAuthAuthorizationRequest struct {
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	ResponseType        string `json:"response_type"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	Resource            string `json:"resource"`
}

type OAuthClientRegistry struct {
	clients  map[string]OAuthClient
	resource string
}

func NewOAuthClientRegistry(clients []OAuthClient, resource string) (*OAuthClientRegistry, error) {
	registry := &OAuthClientRegistry{clients: make(map[string]OAuthClient, len(clients)), resource: strings.TrimSpace(resource)}
	for _, client := range clients {
		if err := validateOAuthClient(client); err != nil {
			return nil, err
		}
		if _, exists := registry.clients[client.ClientID]; exists {
			return nil, fmt.Errorf("duplicate OAuth client ID %q", client.ClientID)
		}
		registry.clients[client.ClientID] = client
	}
	return registry, nil
}

func (r *OAuthClientRegistry) normalizeAuthorizationRequest(request OAuthAuthorizationRequest) OAuthAuthorizationRequest {
	if request.Resource == "" && r.resource != "" {
		request.Resource = r.resource
	}
	return request
}

func (r *OAuthClientRegistry) ValidateAuthorizationRequest(request OAuthAuthorizationRequest) (OAuthClient, error) {
	client, exists := r.clients[request.ClientID]
	if !exists || !client.Enabled {
		return OAuthClient{}, fmt.Errorf("unknown or disabled OAuth client")
	}
	if request.ResponseType != "code" || strings.TrimSpace(request.State) == "" {
		return OAuthClient{}, fmt.Errorf("authorization code response type and state are required")
	}
	if request.CodeChallengeMethod != "S256" || !pkceVerifierPattern.MatchString(request.CodeChallenge) {
		return OAuthClient{}, fmt.Errorf("PKCE S256 code challenge is required")
	}
	if r.resource != "" && request.Resource != r.resource {
		return OAuthClient{}, fmt.Errorf("invalid resource indicator")
	}
	if r.resource == "" && !validMCPResource(request.Resource) {
		return OAuthClient{}, fmt.Errorf("invalid resource indicator")
	}
	if !matchesRegisteredOAuthRedirectURI(client.RedirectURIs, request.RedirectURI) {
		return OAuthClient{}, fmt.Errorf("redirect URI is not registered")
	}
	requestedScopes := strings.Fields(request.Scope)
	if len(requestedScopes) == 0 || len(requestedScopes) != len(unique(requestedScopes)) {
		return OAuthClient{}, fmt.Errorf("at least one unique scope is required")
	}
	for _, scope := range requestedScopes {
		if !contains(client.Scopes, scope) {
			return OAuthClient{}, fmt.Errorf("scope is not allowed for this client")
		}
	}
	return client, nil
}

func validMCPResource(value string) bool {
	resource, err := url.Parse(strings.TrimSpace(value))
	if err != nil || resource.Host == "" || resource.Path != "/mcp" || resource.RawQuery != "" || resource.Fragment != "" {
		return false
	}
	if resource.Scheme == "https" {
		return true
	}
	return resource.Scheme == "http" && (resource.Hostname() == "localhost" || resource.Hostname() == "127.0.0.1" || resource.Hostname() == "::1")
}

func validateOAuthClient(client OAuthClient) error {
	if strings.TrimSpace(client.ClientID) == "" || strings.TrimSpace(client.DisplayName) == "" || len(client.RedirectURIs) == 0 {
		return fmt.Errorf("OAuth client ID, display name, and redirect URI are required")
	}
	for _, rawURI := range client.RedirectURIs {
		uri, err := url.Parse(rawURI)
		if err != nil || uri.Fragment != "" || uri.User != nil || !validOAuthClientRedirectURI(client, uri) {
			return fmt.Errorf("OAuth client %q has invalid redirect URI", client.ClientID)
		}
	}
	if len(client.Scopes) == 0 || len(client.Scopes) != len(unique(client.Scopes)) {
		return fmt.Errorf("OAuth client %q must have unique allowed scopes", client.ClientID)
	}
	return nil
}

func validOAuthClientRedirectURI(client OAuthClient, uri *url.URL) bool {
	if uri.Scheme == "https" {
		return uri.Host != ""
	}
	if uri.Scheme == "http" {
		return uri.Host != "" && isLoopbackHost(uri.Hostname())
	}
	return client.ApplicationType == "native" && validNativeCustomOAuthRedirectURI(uri)
}

func validNativeCustomOAuthRedirectURI(uri *url.URL) bool {
	if uri.Scheme == "" || uri.Host == "" || uri.Opaque != "" {
		return false
	}
	switch strings.ToLower(uri.Scheme) {
	case "about", "data", "file", "ftp", "ftps", "javascript", "mailto", "tel", "ws", "wss":
		return false
	default:
		return true
	}
}

func matchesRegisteredOAuthRedirectURI(registeredURIs []string, requestedURI string) bool {
	if contains(registeredURIs, requestedURI) {
		return true
	}
	requested, err := url.Parse(requestedURI)
	if err != nil || requested.Scheme != "http" || !isLoopbackHost(requested.Hostname()) || requested.Path == "" || requested.RawQuery != "" || requested.Fragment != "" || requested.User != nil {
		return false
	}
	for _, registeredURI := range registeredURIs {
		registered, err := url.Parse(registeredURI)
		if err != nil || registered.Scheme != "http" || !isLoopbackHost(registered.Hostname()) {
			continue
		}
		if strings.EqualFold(registered.Hostname(), requested.Hostname()) && registered.Path == requested.Path && registered.RawQuery == "" && registered.Fragment == "" && registered.User == nil {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func unique(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
