package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
)

func TestMCPRejectsNonMCPTokensAndUnexpectedOrigins(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	config := Config{Enabled: true, Issuer: "https://auth.example.test", Resource: "https://mcp.example.test/mcp", PublicKey: &privateKey.PublicKey, AllowedOrigins: map[string]struct{}{"https://client.example.test": {}}}
	handler := New(config)

	for _, test := range []struct {
		name   string
		token  string
		origin string
		status int
	}{
		{name: "valid MCP token", token: signedToken(t, privateKey, config, authx.TokenTypeMCP, config.Resource), status: http.StatusOK},
		{name: "API token", token: signedToken(t, privateKey, config, authx.TokenTypeAPI, config.Resource), status: http.StatusUnauthorized},
		{name: "wrong audience", token: signedToken(t, privateKey, config, authx.TokenTypeMCP, "homenavi-api"), status: http.StatusUnauthorized},
		{name: "disallowed origin", token: signedToken(t, privateKey, config, authx.TokenTypeMCP, config.Resource), origin: "https://evil.example.test", status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`))
			request.Header.Set("Authorization", "Bearer "+test.token)
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
		})
	}
}

func TestMCPRejectsNonResidentTokens(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	config := Config{Enabled: true, Issuer: "https://auth.example.test", Resource: "https://mcp.example.test/mcp", PublicKey: &privateKey.PublicKey}
	token := signedTokenWithClaimsAndRole(t, privateKey, config, authx.TokenTypeMCP, config.Resource, "home.devices.write", authx.RoleUser)
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	New(config).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestMCPDisabledDoesNotExposeMetadataOrTransport(t *testing.T) {
	handler := New(Config{})
	for _, path := range []string{"/mcp", "/.well-known/oauth-protected-resource"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}
}

func TestMCPChallengeUsesResourceMetadataEndpoint(t *testing.T) {
	response := httptest.NewRecorder()
	New(Config{Enabled: true, Resource: "https://mcp.example.test/mcp"}).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if got, want := response.Header().Get("WWW-Authenticate"), `Bearer resource_metadata="https://mcp.example.test/.well-known/oauth-protected-resource", scope="home.devices.read home.devices.write home.inventory.read home.inventory.write home.history.read home.automation.read home.automation.execute"`; got != want {
		t.Fatalf("WWW-Authenticate = %q, want %q", got, want)
	}
}

func TestMCPUsesConfiguredResource(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	resource := "https://home.example.test/mcp"
	config := Config{Enabled: true, Resource: resource, PublicKey: &privateKey.PublicKey}
	handler := New(config)

	metadataRequest := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	metadataResponse := httptest.NewRecorder()
	handler.ServeHTTP(metadataResponse, metadataRequest)
	if metadataResponse.Code != http.StatusOK || !strings.Contains(metadataResponse.Body.String(), resource) {
		t.Fatalf("unexpected metadata response: %d %s", metadataResponse.Code, metadataResponse.Body.String())
	}

	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`))
	request.Header.Set("Authorization", "Bearer "+signedToken(t, privateKey, Config{Issuer: "https://home.example.test/api/auth"}, authx.TokenTypeMCP, resource))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
}

func TestMCPDerivesResourceFromForwardedRequest(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	request.Host = "ignored.internal"
	request.Header.Set("X-Forwarded-Host", "home.example.test")
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	New(Config{Enabled: true}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var metadata struct {
		Resource string `json:"resource"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Resource != "https://home.example.test/mcp" {
		t.Fatalf("resource = %q", metadata.Resource)
	}
}

func TestMCPToolDiscoveryIsScoped(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	config := Config{Enabled: true, Issuer: "https://auth.example.test", Resource: "https://mcp.example.test/mcp", PublicKey: &privateKey.PublicKey}
	handler := New(config)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	request.Header.Set("Authorization", "Bearer "+signedTokenWithScopes(t, privateKey, config, "home.devices.read"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2025-03-26")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range result.Result.Tools {
		names[tool.Name] = true
	}
	if !names["list_devices"] || names["list_rooms"] || names["query_state_history"] {
		t.Fatalf("unexpected scoped tools: %#v", names)
	}
}

func TestMCPGroupToolsRequireAllScopes(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	config := Config{Enabled: true, Issuer: "https://auth.example.test", Resource: "https://mcp.example.test/mcp", PublicKey: &privateKey.PublicKey}
	for _, test := range []struct {
		name                     string
		scopes                   string
		groupState, groupCommand bool
	}{
		{name: "inventory read only", scopes: "home.inventory.read"},
		{name: "device read only", scopes: "home.devices.read"},
		{name: "device write only", scopes: "home.devices.write"},
		{name: "inventory and device read", scopes: "home.inventory.read home.devices.read", groupState: true},
		{name: "inventory read and device write", scopes: "home.inventory.read home.devices.write", groupCommand: true},
		{name: "all group tool scopes", scopes: "home.inventory.read home.devices.read home.devices.write", groupState: true, groupCommand: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
			request.Header.Set("Authorization", "Bearer "+signedTokenWithScopes(t, privateKey, config, test.scopes))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			request.Header.Set("MCP-Protocol-Version", "2025-03-26")
			response := httptest.NewRecorder()
			New(config).ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			var result struct {
				Result struct {
					Tools []struct {
						Name string `json:"name"`
					} `json:"tools"`
				} `json:"result"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			names := map[string]bool{}
			for _, tool := range result.Result.Tools {
				names[tool.Name] = true
			}
			if names["get_device_group_state"] != test.groupState || names["send_device_group_command"] != test.groupCommand {
				t.Fatalf("unexpected composite group tools: %#v", names)
			}
		})
	}
}

func TestMCPRejectsRevokedSession(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	config := Config{Enabled: true, Issuer: "https://auth.example.test", Resource: "https://mcp.example.test/mcp", PublicKey: &privateKey.PublicKey, SessionValidator: func(_ context.Context, sessionID string) (bool, error) {
		return sessionID != "session-1", nil
	}}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`))
	request.Header.Set("Authorization", "Bearer "+signedToken(t, privateKey, config, authx.TokenTypeMCP, config.Resource))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	New(config).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestMCPEnforcesRateLimitAndToolFlags(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	config := Config{Enabled: true, Issuer: "https://auth.example.test", Resource: "https://mcp.example.test/mcp", PublicKey: &privateKey.PublicKey, RateLimitPerMinute: 1, EnabledTools: map[string]struct{}{"get_device": {}}}
	handler := New(config)
	request := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`))
		r.Header.Set("Authorization", "Bearer "+signedToken(t, privateKey, config, authx.TokenTypeMCP, config.Resource))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		return r
	}
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, request())
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d", first.Code)
	}
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, request())
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}

func TestValidateWritePolicy(t *testing.T) {
	for _, test := range []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "empty key", wantErr: true},
		{name: "whitespace key", key: "        ", wantErr: true},
		{name: "short key", key: "1234567", wantErr: true},
		{name: "valid key", key: "command-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateWritePolicy(controlledWrite{IdempotencyKey: test.key})
			if (err != nil) != test.wantErr {
				t.Fatalf("validateWritePolicy() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func signedToken(t *testing.T, privateKey *rsa.PrivateKey, config Config, tokenType, audience string) string {
	return signedTokenWithClaims(t, privateKey, config, tokenType, audience, "home.devices.read")
}

func signedTokenWithScopes(t *testing.T, privateKey *rsa.PrivateKey, config Config, scopes string) string {
	return signedTokenWithClaims(t, privateKey, config, authx.TokenTypeMCP, config.Resource, scopes)
}

func signedTokenWithClaims(t *testing.T, privateKey *rsa.PrivateKey, config Config, tokenType, audience, scopes string) string {
	return signedTokenWithClaimsAndRole(t, privateKey, config, tokenType, audience, scopes, authx.RoleResident)
}

func signedTokenWithClaimsAndRole(t *testing.T, privateKey *rsa.PrivateKey, config Config, tokenType, audience, scopes, role string) string {
	t.Helper()
	keyID, err := authx.KeyIDForRSAPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": config.Issuer, "sub": "user-1", "aud": []string{audience}, "exp": time.Now().Add(time.Minute).Unix(), authx.ClaimSessionID: "session-1", authx.ClaimAuthorizedParty: "mcp-cli", authx.ClaimTokenType: tokenType, authx.ClaimScope: scopes, "role": role})
	token.Header["kid"] = keyID
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}
