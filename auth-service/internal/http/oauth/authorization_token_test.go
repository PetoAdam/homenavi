package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/PetoAdam/homenavi/shared/authx"
)

type fakeOAuthAuthorizer struct {
	subject                string
	sessionID              string
	role                   string
	err                    error
	request                authdomain.OAuthAuthorizationRequest
	registeredName         string
	registeredRedirectURIs []string
	granted                bool
	revoked                string
	transaction            authdomain.OAuthAuthorizationTransaction
}

func (f *fakeOAuthAuthorizer) ExtractOAuthSession(string) (authdomain.OAuthSession, error) {
	role := f.role
	if role == "" {
		role = authx.RoleResident
	}
	return authdomain.OAuthSession{Subject: f.subject, SessionID: f.sessionID, Role: role}, f.err
}

func (f *fakeOAuthAuthorizer) AuthorizeOAuthForSession(_ context.Context, request authdomain.OAuthAuthorizationRequest, _, _ string) (string, error) {
	f.request = request
	if f.err != nil {
		return "", f.err
	}
	return "authorization-code", nil
}

func (f *fakeOAuthAuthorizer) GrantOAuthConsent(_ context.Context, request authdomain.OAuthAuthorizationRequest, _, _ string) error {
	f.request = request
	f.granted = true
	return f.err
}

func (f *fakeOAuthAuthorizer) RevokeOAuthConsent(_ context.Context, _ string, clientID string) error {
	f.revoked = clientID
	return f.err
}

func (f *fakeOAuthAuthorizer) RegisterOAuthClient(_ context.Context, name string, redirectURIs []string) (authdomain.OAuthClient, error) {
	if f.err != nil {
		return authdomain.OAuthClient{}, f.err
	}
	f.registeredName = name
	f.registeredRedirectURIs = append([]string(nil), redirectURIs...)
	return authdomain.OAuthClient{ClientID: "dynamic-client", DisplayName: name, RedirectURIs: redirectURIs}, nil
}

func (f *fakeOAuthAuthorizer) BeginOAuthAuthorization(_ context.Context, request authdomain.OAuthAuthorizationRequest) (authdomain.OAuthAuthorizationTransaction, error) {
	if f.err != nil {
		return authdomain.OAuthAuthorizationTransaction{}, f.err
	}
	f.transaction = authdomain.OAuthAuthorizationTransaction{ID: "transaction-1", Request: request}
	return f.transaction, nil
}

func (f *fakeOAuthAuthorizer) GetOAuthAuthorizationTransaction(_ context.Context, id string) (authdomain.OAuthAuthorizationTransaction, error) {
	if f.err != nil || id != f.transaction.ID {
		return authdomain.OAuthAuthorizationTransaction{}, errors.New("transaction not found")
	}
	return f.transaction, nil
}

func (f *fakeOAuthAuthorizer) SetOAuthAuthorizationSession(_ context.Context, id, subject, sessionID string) error {
	if id != f.transaction.ID {
		return errors.New("transaction not found")
	}
	f.transaction.Subject = subject
	f.transaction.SessionID = sessionID
	return f.err
}

func (f *fakeOAuthAuthorizer) SetOAuthAuthorizationPendingUser(_ context.Context, id, userID string) error {
	if id != f.transaction.ID {
		return errors.New("transaction not found")
	}
	f.transaction.PendingUserID = userID
	return f.err
}

func (f *fakeOAuthAuthorizer) DeleteOAuthAuthorizationTransaction(_ context.Context, id string) error {
	if id != f.transaction.ID {
		return errors.New("transaction not found")
	}
	f.transaction = authdomain.OAuthAuthorizationTransaction{}
	return f.err
}

func TestAuthorizationHandlerIssuesRedirectBoundCode(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{subject: "user-1", sessionID: "session-1"}
	handler := NewAuthorizationHandler(authorizer)
func TestAuthorizationHandlerRegistersPublicWebClient(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{}
	handler := NewAuthorizationHandler(authorizer)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/register", strings.NewReader(`{"client_name":"Open WebUI","redirect_uris":["https://openwebui.example/oauth/clients/homenavi-mcp/callback"],"application_type":"web","token_endpoint_auth_method":"none","grant_types":["authorization_code","refresh_token"],"response_types":["code"]}`))
	response := httptest.NewRecorder()

	handler.HandleRegisterClient(response, request)

	if response.Code != http.StatusCreated || authorizer.registeredName != "Open WebUI" || len(authorizer.registeredRedirectURIs) != 1 || authorizer.registeredRedirectURIs[0] != "https://openwebui.example/oauth/clients/homenavi-mcp/callback" {
		t.Fatalf("unexpected registration response: status=%d authorizer=%#v", response.Code, authorizer)
	}
	var registered struct {
		ApplicationType string `json:"application_type"`
		ClientID        string `json:"client_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&registered); err != nil {
		t.Fatalf("decode registration response: %v", err)
	}
	if registered.ApplicationType != "web" || registered.ClientID != "dynamic-client" {
		t.Fatalf("unexpected registered client: %#v", registered)
	}
}

	query := url.Values{
		"client_id":             {"mcp-cli"},
		"redirect_uri":          {"https://client.example/callback?from=client"},
		"response_type":         {"code"},
		"scope":                 {"home.devices.read"},
		"state":                 {"opaque-state"},
		"code_challenge":        {"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"},
		"code_challenge_method": {"S256"},
		"resource":              {"https://home.example/mcp"},
		"consent":               {"approve"},
	}.Encode()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/authorize?"+query, nil)
	request.Header.Set("Authorization", "Bearer browser-token")
	response := httptest.NewRecorder()

	handler.HandleAuthorize(response, request)

	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusFound)
	}
	redirect, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if redirect.Query().Get("code") != "authorization-code" || redirect.Query().Get("state") != "opaque-state" || redirect.Query().Get("from") != "client" {
		t.Fatalf("unexpected redirect query: %s", redirect.RawQuery)
	}
	if authorizer.request.ClientID != "mcp-cli" || authorizer.request.Resource != "https://home.example/mcp" {
		t.Fatalf("unexpected authorization request: %#v", authorizer.request)
	}
}

func TestAuthorizationHandlerStartsBrowserTransactionWithoutBearer(t *testing.T) {
	handler := NewAuthorizationHandler(&fakeOAuthAuthorizer{subject: "user-1", sessionID: "session-1"})
	handler.ConfigureBrowserLogin(nil, nil, nil, "http://localhost:5173/oauth/authorize")
	request := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/authorize?client_id=mcp-cli", nil)
	response := httptest.NewRecorder()
	handler.HandleAuthorize(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "http://localhost:5173/oauth/authorize" || !strings.Contains(response.Header().Get("Set-Cookie"), "homenavi_oauth_transaction=transaction-1") {
		t.Fatalf("unexpected browser transaction response: status=%d headers=%#v", response.Code, response.Header())
	}
}

func TestAuthorizationHandlerReusesAppBrowserSessionForTransaction(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{subject: "user-1", sessionID: "session-1"}
	handler := NewAuthorizationHandler(authorizer)
	handler.ConfigureBrowserLogin(nil, nil, nil, "http://localhost:5173/oauth/authorize")
	request := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/authorize?client_id=mcp-cli", nil)
	request.AddCookie(&http.Cookie{Name: appSessionCookie, Value: "app-access-token"})
	response := httptest.NewRecorder()

	handler.HandleAuthorize(response, request)

	if response.Code != http.StatusFound || authorizer.transaction.Subject != "user-1" || authorizer.transaction.SessionID != "session-1" {
		t.Fatalf("unexpected browser session reuse: status=%d transaction=%#v", response.Code, authorizer.transaction)
	}
}

func TestWriteTransactionSessionError(t *testing.T) {
	handler := NewAuthorizationHandler(&fakeOAuthAuthorizer{})
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "role denied", err: authdomain.ErrMCPRoleRequired, wantStatus: http.StatusForbidden, wantCode: "insufficient_permissions"},
		{name: "session extraction failed", err: errors.New("invalid access token"), wantStatus: http.StatusInternalServerError, wantCode: "server_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.writeTransactionSessionError(response, test.err)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantCode) {
				t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAuthorizationHandlerGrantsAndRevokesConsent(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{subject: "user-1", sessionID: "session-1"}
	handler := NewAuthorizationHandler(authorizer)
	body := `{"client_id":"mcp-cli","redirect_uri":"https://client.example/callback","response_type":"code","scope":"home.devices.read","state":"opaque-state","code_challenge":"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~","code_challenge_method":"S256","resource":"https://home.example/mcp"}`
	grant := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/consents", strings.NewReader(body))
	grant.Header.Set("Authorization", "Bearer browser-token")
	grantResponse := httptest.NewRecorder()
	handler.HandleGrantConsent(grantResponse, grant)
	if grantResponse.Code != http.StatusNoContent || !authorizer.granted || authorizer.request.ClientID != "mcp-cli" {
		t.Fatalf("unexpected grant response: status=%d authorizer=%#v", grantResponse.Code, authorizer)
	}

	revoke := httptest.NewRequest(http.MethodDelete, "/api/auth/oauth/consents/mcp-cli", nil)
	revoke.SetPathValue("clientID", "mcp-cli")
	revoke.Header.Set("Authorization", "Bearer browser-token")
	revokeResponse := httptest.NewRecorder()
	handler.HandleRevokeConsent(revokeResponse, revoke)
	if revokeResponse.Code != http.StatusNoContent || authorizer.revoked != "mcp-cli" {
		t.Fatalf("unexpected revoke response: status=%d client=%q", revokeResponse.Code, authorizer.revoked)
	}
}

func TestAuthorizationTransactionApprovalReturnsRedirect(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{subject: "user-1", sessionID: "session-1", transaction: authdomain.OAuthAuthorizationTransaction{
		ID:      "transaction-1",
		Request: authdomain.OAuthAuthorizationRequest{ClientID: "mcp-cli", RedirectURI: "http://127.0.0.1:33418/", ResponseType: "code", State: "opaque-state", Scope: "home.devices.read", CodeChallenge: strings.Repeat("a", 43), CodeChallengeMethod: "S256", Resource: "http://localhost:8080/mcp"},
		Subject: "user-1", SessionID: "session-1",
	}}
	handler := NewAuthorizationHandler(authorizer)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/transaction/approve", nil)
	request.AddCookie(&http.Cookie{Name: browserTransactionCookie, Value: "transaction-1"})
	response := httptest.NewRecorder()

	handler.HandleAuthorizationApproval(response, request)

	if response.Code != http.StatusOK || !authorizer.granted || !strings.Contains(response.Body.String(), "authorization-code") || !strings.Contains(response.Body.String(), "opaque-state") {
		t.Fatalf("unexpected approval response: status=%d body=%s", response.Code, response.Body.String())
	}
}

type fakeOAuthTokenIssuer struct {
	grant authdomain.OAuthAuthorizationGrant
	err   error
}

func (f fakeOAuthTokenIssuer) ExchangeOAuthAuthorizationCode(_ context.Context, _ string, _ authdomain.OAuthAuthorizationCodeExchange) (authdomain.OAuthAuthorizationGrant, error) {
	return f.grant, f.err
}

func (f fakeOAuthTokenIssuer) IssueMCPAccessToken(_ *clientsinfra.User, _ authdomain.OAuthAuthorizationGrant) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "mcp-access-token", nil
}

func (f fakeOAuthTokenIssuer) ExchangeMCPAccessToken(_ context.Context, _, _, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "delegated-access-token", nil
}

type fakeOAuthUserProvider struct {
	user *clientsinfra.User
	err  error
}

func (f fakeOAuthUserProvider) GetUser(string) (*clientsinfra.User, error) {
	return f.user, f.err
}

func TestTokenHandlerExchangesAuthorizationCode(t *testing.T) {
	handler := NewTokenHandler(
		fakeOAuthTokenIssuer{grant: authdomain.OAuthAuthorizationGrant{Subject: "user-1", Scope: "home.devices.read"}},
		fakeOAuthUserProvider{user: &clientsinfra.User{ID: "user-1"}},
	)
	form := url.Values{"grant_type": {"authorization_code"}, "code": {"code"}, "client_id": {"mcp-cli"}, "redirect_uri": {"https://client.example/callback"}, "resource": {"https://home.example/mcp"}, "code_verifier": {"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"}}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.HandleToken(response, request)

	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response status=%d cache=%q", response.Code, response.Header().Get("Cache-Control"))
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["access_token"] != "mcp-access-token" || payload["token_type"] != "Bearer" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload["expires_in"] != float64(authdomain.MCPAccessTokenTTL.Seconds()) {
		t.Fatalf("unexpected token lifetime: %#v", payload["expires_in"])
	}
}

func TestTokenHandlerRejectsInvalidGrant(t *testing.T) {
	handler := NewTokenHandler(fakeOAuthTokenIssuer{err: errors.New("invalid code")}, fakeOAuthUserProvider{})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/token", strings.NewReader("grant_type=authorization_code&code=code"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.HandleToken(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_grant") {
		t.Fatalf("unexpected error response: status=%d body=%s", response.Code, response.Body.String())
	}
}
