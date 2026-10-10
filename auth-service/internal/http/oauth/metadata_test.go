package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetadataHandlerDerivesIssuerFromForwardedRequest(t *testing.T) {
	handler := NewMetadataHandler("")
	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	request.Header.Set("X-Forwarded-Host", "home.example.test")
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	handler.HandleAuthorizationServerMetadata(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected metadata response: %d %s", response.Code, response.Body.String())
	}
	var metadata struct{ Issuer string `json:"issuer"` }
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata.Issuer != "https://home.example.test/api/auth" {
		t.Fatalf("issuer = %q", metadata.Issuer)
	}
}

func TestMetadataHandlerRejectsUnsafeForwardedScheme(t *testing.T) {
	handler := NewMetadataHandler("")
	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	request.Header.Set("X-Forwarded-Host", "home.example.test")
	request.Header.Set("X-Forwarded-Proto", "javascript")
	response := httptest.NewRecorder()

	handler.HandleAuthorizationServerMetadata(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected metadata response: %d %s", response.Code, response.Body.String())
	}
}

func TestAuthorizationServerMetadataUsesConfiguredIssuer(t *testing.T) {
	handler := NewMetadataHandler("https://home.example/api/auth/")
	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	response := httptest.NewRecorder()

	handler.HandleAuthorizationServerMetadata(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var metadata map[string]json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	var issuer, jwksURI, authorizationEndpoint, tokenEndpoint, registrationEndpoint string
	var pkceMethods, scopes []string
	for field, target := range map[string]any{
		"issuer":                 &issuer,
		"jwks_uri":               &jwksURI,
		"authorization_endpoint": &authorizationEndpoint,
		"token_endpoint":         &tokenEndpoint,
		"registration_endpoint":  &registrationEndpoint,
	} {
		if err := json.Unmarshal(metadata[field], target); err != nil {
			t.Fatalf("decode %s: %v", field, err)
		}
	}
	if err := json.Unmarshal(metadata["code_challenge_methods_supported"], &pkceMethods); err != nil {
		t.Fatalf("decode PKCE methods: %v", err)
	}
	if err := json.Unmarshal(metadata["scopes_supported"], &scopes); err != nil {
		t.Fatalf("decode supported scopes: %v", err)
	}
	if issuer != "https://home.example/api/auth" || jwksURI != "https://home.example/api/auth/oauth/jwks.json" || authorizationEndpoint != "https://home.example/api/auth/oauth/authorize" || tokenEndpoint != "https://home.example/api/auth/oauth/token" || registrationEndpoint != "https://home.example/api/auth/oauth/register" {
		t.Fatal("unexpected OAuth metadata")
	}
	if len(pkceMethods) != 1 || pkceMethods[0] != "S256" {
		t.Fatalf("unexpected PKCE methods: %#v", pkceMethods)
	}
	if len(scopes) != 7 || scopes[0] != "home.devices.read" || scopes[6] != "home.automation.execute" {
		t.Fatalf("unexpected supported scopes: %#v", scopes)
	}
}
