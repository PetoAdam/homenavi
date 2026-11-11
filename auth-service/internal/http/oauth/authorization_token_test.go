package oauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	authhttp "github.com/PetoAdam/homenavi/auth-service/internal/http"
	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
)

type fakeOAuthAuthorizer struct {
	subject                string
	sessionID              string
	role                   string
	err                    error
	request                authdomain.OAuthAuthorizationRequest
	registeredName         string
	registeredRedirectURIs []string
	registration           authdomain.DynamicOAuthClientRegistration
	granted                bool
	grantedScope           string
	revoked                string
	consents               []authdomain.OAuthConsent
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
	f.grantedScope = request.Scope
	return f.err
}

func (f *fakeOAuthAuthorizer) RevokeOAuthConsent(_ context.Context, _ string, clientID string) error {
	f.revoked = clientID
	return f.err
}

func (f *fakeOAuthAuthorizer) ListOAuthConsents(_ context.Context, _ string) ([]authdomain.OAuthConsent, error) {
	return f.consents, f.err
}

func (f *fakeOAuthAuthorizer) RegisterDynamicOAuthClient(_ context.Context, registration authdomain.DynamicOAuthClientRegistration) (authdomain.OAuthClient, error) {
	if f.err != nil {
		return authdomain.OAuthClient{}, f.err
	}
	f.registeredName = registration.Name
	f.registeredRedirectURIs = append([]string(nil), registration.RedirectURIs...)
	f.registration = registration
	return authdomain.OAuthClient{ClientID: "dynamic-client", DisplayName: registration.Name, RedirectURIs: registration.RedirectURIs, ApplicationType: registration.ApplicationType, TokenEndpointAuthMethod: registration.TokenEndpointAuthMethod}, nil
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
	if authorizer.registration.ApplicationType != "web" || authorizer.registration.TokenEndpointAuthMethod != "none" {
		t.Fatalf("unexpected registration metadata: %#v", authorizer.registration)
	}
}

func TestAuthorizationHandlerRegistersNativeCustomSchemeClient(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{}
	handler := NewAuthorizationHandler(authorizer)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/register", strings.NewReader(`{"client_name":"Homenavi Mobile","redirect_uris":["mcp-agent://callback"],"application_type":"native","token_endpoint_auth_method":"none"}`))
	response := httptest.NewRecorder()

	handler.HandleRegisterClient(response, request)

	if response.Code != http.StatusCreated || authorizer.registration.ApplicationType != "native" || authorizer.registration.RedirectURIs[0] != "mcp-agent://callback" {
		t.Fatalf("unexpected registration response: status=%d registration=%#v", response.Code, authorizer.registration)
	}
}

func TestAuthorizationHandlerIssuesRedirectBoundCode(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{subject: "user-1", sessionID: "session-1"}
	handler := NewAuthorizationHandler(authorizer)
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
	if cookies := response.Result().Cookies(); len(cookies) != 1 || cookies[0].Secure {
		t.Fatalf("expected an insecure transaction cookie for HTTP: %#v", cookies)
	}
}

func TestAuthorizationHandlerDerivesBrowserRedirectFromForwardedRequest(t *testing.T) {
	handler := NewAuthorizationHandler(&fakeOAuthAuthorizer{subject: "user-1", sessionID: "session-1"})
	request := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/authorize?client_id=mcp-cli", nil)
	request.Header.Set("X-Forwarded-Host", "home.example.test")
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()

	handler.HandleAuthorize(response, request)

	if response.Code != http.StatusFound || response.Header().Get("Location") != "https://home.example.test/oauth/authorize" {
		t.Fatalf("unexpected browser transaction response: status=%d headers=%#v", response.Code, response.Header())
	}
	if cookies := response.Result().Cookies(); len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("expected a secure transaction cookie for HTTPS: %#v", cookies)
	}
}

func TestAuthorizationHandlerReusesAppBrowserSessionForTransaction(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{subject: "user-1", sessionID: "session-1"}
	handler := NewAuthorizationHandler(authorizer)
	handler.ConfigureBrowserLogin(nil, nil, nil, "http://localhost:5173/oauth/authorize")
	router := authhttp.NewRouter(authhttp.Routes{
		HandleOAuthAuthorize:   handler.HandleAuthorize,
		HandleOAuthTransaction: handler.HandleAuthorizationTransaction,
		HandleOAuthApproval:    handler.HandleAuthorizationApproval,
	}, http.NotFoundHandler(), otel.Tracer("test"))
	query := url.Values{
		"client_id": {"mcp-cli"}, "redirect_uri": {"https://client.example/callback"},
		"response_type": {"code"}, "scope": {"home.devices.read"}, "state": {"opaque-state"},
		"code_challenge":        {"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"},
		"code_challenge_method": {"S256"}, "resource": {"https://home.example/mcp"},
	}
	request := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/authorize?"+query.Encode(), nil)
	request.AddCookie(&http.Cookie{Name: appSessionCookie, Value: "app-access-token"})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusFound || response.Header().Get("Location") != "http://localhost:5173/oauth/authorize" || authorizer.transaction.Subject != "user-1" || authorizer.transaction.SessionID != "session-1" || authorizer.granted {
		t.Fatalf("unexpected browser session reuse: status=%d transaction=%#v", response.Code, authorizer.transaction)
	}
	var transactionCookie *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == browserTransactionCookie {
			transactionCookie = cookie
		}
	}
	if transactionCookie == nil {
		t.Fatal("missing transaction cookie")
	}
	transaction := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/transaction", nil)
	transaction.AddCookie(transactionCookie)
	transactionResponse := httptest.NewRecorder()
	router.ServeHTTP(transactionResponse, transaction)
	if transactionResponse.Code != http.StatusOK || !strings.Contains(transactionResponse.Body.String(), `"authenticated":true`) {
		t.Fatalf("unexpected transaction: status=%d body=%s", transactionResponse.Code, transactionResponse.Body.String())
	}
	approval := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/transaction/approve", strings.NewReader(`{}`))
	approval.AddCookie(transactionCookie)
	approvalResponse := httptest.NewRecorder()
	router.ServeHTTP(approvalResponse, approval)
	if approvalResponse.Code != http.StatusOK || !authorizer.granted || authorizer.request.CodeChallenge != query.Get("code_challenge") || !strings.Contains(approvalResponse.Body.String(), "authorization-code") || !strings.Contains(approvalResponse.Body.String(), "opaque-state") {
		t.Fatalf("unexpected approval: status=%d body=%s", approvalResponse.Code, approvalResponse.Body.String())
	}
}

func TestBrowserAuthorizationPKCEIntegration(t *testing.T) {
	address := os.Getenv("HOMENAVI_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("HOMENAVI_TEST_REDIS_ADDR is not set")
	}
	store, err := cacheinfra.NewRedisStore(redisx.Config{Mode: redisx.ModeStandalone, Addrs: []string{address}})
	if err != nil {
		t.Fatalf("connect to test Redis: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	service := authdomain.NewService(authdomain.Config{
		JWTPrivateKey: key, JWTIssuer: "http://localhost/api/auth", JWTAPIAudience: "http://localhost/api",
		AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour, OAuthMCPResource: "http://localhost/mcp",
	}, store)
	user := &clientsinfra.User{ID: "oauth-test-user", Role: authx.RoleResident}
	sessionToken, err := service.IssueAccessToken(user)
	if err != nil {
		t.Fatalf("issue browser session: %v", err)
	}
	client, err := service.RegisterDynamicOAuthClient(context.Background(), authdomain.DynamicOAuthClientRegistration{
		Name: "PKCE integration test", RedirectURIs: []string{"http://127.0.0.1:12345/callback"}, ApplicationType: "native", TokenEndpointAuthMethod: "none",
	})
	if err != nil {
		t.Fatalf("register client: %v", err)
	}
	clientKey := "oauth_dynamic_client:" + client.ClientID
	if ttl, err := store.TTL(context.Background(), clientKey); err != nil || ttl != -time.Nanosecond {
		t.Fatalf("new client TTL = %v, error = %v; want no expiry", ttl, err)
	}
	authorization := NewAuthorizationHandler(service)
	token := NewTokenHandler(service, fakeOAuthUserProvider{user: user})
	router := authhttp.NewRouter(authhttp.Routes{
		HandleOAuthAuthorize: authorization.HandleAuthorize, HandleOAuthTransaction: authorization.HandleAuthorizationTransaction,
		HandleOAuthApproval: authorization.HandleAuthorizationApproval, HandleOAuthToken: token.HandleToken,
	}, http.NotFoundHandler(), otel.Tracer("test"))
	verifier := strings.Repeat("a", 43)
	digest := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"response_type": {"code"}, "client_id": {client.ClientID}, "redirect_uri": {client.RedirectURIs[0]},
		"scope": {"home.devices.read"}, "state": {"test-state"}, "resource": {"http://localhost/mcp"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}, "code_challenge_method": {"S256"},
	}
	issueCode := func() string {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "http://localhost/api/auth/oauth/authorize?"+query.Encode(), nil)
		request.AddCookie(&http.Cookie{Name: appSessionCookie, Value: sessionToken})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusFound || response.Header().Get("Location") != "http://localhost/oauth/authorize" {
			t.Fatalf("authorize: status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
		}
		var transactionCookie *http.Cookie
		for _, cookie := range response.Result().Cookies() {
			if cookie.Name == browserTransactionCookie {
				transactionCookie = cookie
			}
		}
		if transactionCookie == nil || transactionCookie.Secure {
			t.Fatal("expected an HTTP transaction cookie")
		}
		transaction := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/transaction", nil)
		transaction.AddCookie(transactionCookie)
		transactionResponse := httptest.NewRecorder()
		router.ServeHTTP(transactionResponse, transaction)
		if transactionResponse.Code != http.StatusOK || !strings.Contains(transactionResponse.Body.String(), `"authenticated":true`) {
			t.Fatalf("transaction: status=%d body=%s", transactionResponse.Code, transactionResponse.Body.String())
		}
		approval := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/transaction/approve", strings.NewReader(`{}`))
		approval.AddCookie(transactionCookie)
		approvalResponse := httptest.NewRecorder()
		router.ServeHTTP(approvalResponse, approval)
		if approvalResponse.Code != http.StatusOK {
			t.Fatalf("approval: status=%d body=%s", approvalResponse.Code, approvalResponse.Body.String())
		}
		var payload struct {
			RedirectURI string `json:"redirect_uri"`
		}
		if err := json.NewDecoder(approvalResponse.Body).Decode(&payload); err != nil {
			t.Fatalf("decode approval: %v", err)
		}
		redirect, err := url.Parse(payload.RedirectURI)
		if err != nil || redirect.Query().Get("state") != "test-state" || redirect.Query().Get("code") == "" {
			t.Fatalf("invalid approval redirect: %v", err)
		}
		return redirect.Query().Get("code")
	}
	exchange := func(code, codeVerifier string) *httptest.ResponseRecorder {
		t.Helper()
		form := url.Values{"grant_type": {"authorization_code"}, "client_id": {client.ClientID}, "redirect_uri": {client.RedirectURIs[0]}, "code": {code}, "code_verifier": {codeVerifier}, "resource": {"http://localhost/mcp"}}
		request := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/token", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	code := issueCode()
	response := exchange(code, verifier)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"access_token"`) {
		t.Fatalf("PKCE exchange failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := exchange(code, verifier); response.Code != http.StatusBadRequest {
		t.Fatalf("authorization code replay status=%d", response.Code)
	}
	if response := exchange(issueCode(), strings.Repeat("b", 43)); response.Code != http.StatusBadRequest {
		t.Fatalf("wrong PKCE verifier status=%d", response.Code)
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
	revoke.Header.Set("Authorization", "Bearer browser-token")
	revokeResponse := httptest.NewRecorder()
	router := chi.NewRouter()
	router.Delete("/api/auth/oauth/consents/{clientID}", handler.HandleRevokeConsent)
	router.ServeHTTP(revokeResponse, revoke)
	if revokeResponse.Code != http.StatusNoContent || authorizer.revoked != "mcp-cli" {
		t.Fatalf("unexpected revoke response: status=%d client=%q", revokeResponse.Code, authorizer.revoked)
	}
}

func TestAuthorizationHandlerListsAuthenticatedUserConsents(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{subject: "user-1", consents: []authdomain.OAuthConsent{{ClientID: "mcp-cli", Resource: "https://home.example/mcp", Scopes: []string{"home.devices.read"}, GrantedAt: 1}}}
	handler := NewAuthorizationHandler(authorizer)
	request := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/consents", nil)
	request.Header.Set("Authorization", "Bearer browser-token")
	response := httptest.NewRecorder()

	handler.HandleListConsents(response, request)

	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"client_id":"mcp-cli"`) {
		t.Fatalf("unexpected consent list response: status=%d headers=%#v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

func TestAuthorizationHandlerRevokesEncodedClientID(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{subject: "user-1", sessionID: "session-1"}
	router := chi.NewRouter()
	router.Delete("/api/auth/oauth/consents/{clientID}", NewAuthorizationHandler(authorizer).HandleRevokeConsent)
	for _, clientID := range []string{"https://client.example/app%1", "client%2Fname", "client+name"} {
		request := httptest.NewRequest(http.MethodDelete, "/api/auth/oauth/consents/"+url.PathEscape(clientID), nil)
		request.Header.Set("Authorization", "Bearer browser-token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || authorizer.revoked != clientID {
			t.Fatalf("client=%q status=%d revoked=%q body=%s", clientID, response.Code, authorizer.revoked, response.Body.String())
		}
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

func TestAuthorizationTransactionApprovalScopeSelection(t *testing.T) {
	for _, test := range []struct {
		name, body, wantScope string
		wantStatus            int
	}{
		{"subset", `{"scopes":["home.devices.read"]}`, "home.devices.read", http.StatusOK},
		{"legacy", `{}`, "home.devices.read home.devices.write", http.StatusOK},
		{"empty", `{"scopes":[]}`, "", http.StatusBadRequest},
		{"null", `{"scopes":null}`, "", http.StatusBadRequest},
		{"unrequested", `{"scopes":["home.automation.execute"]}`, "", http.StatusBadRequest},
		{"combined", `{"scopes":["home.devices.read home.devices.write"]}`, "", http.StatusBadRequest},
		{"duplicate", `{"scopes":["home.devices.read","home.devices.read"]}`, "", http.StatusBadRequest},
		{"malformed", `{"scopes":`, "", http.StatusBadRequest},
		{"trailing", `{} {}`, "", http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			authorizer := &fakeOAuthAuthorizer{transaction: authdomain.OAuthAuthorizationTransaction{
				ID: "transaction-1", Subject: "user-1", SessionID: "session-1",
				Request: authdomain.OAuthAuthorizationRequest{Scope: "home.devices.read home.devices.write", RedirectURI: "https://client.example/callback"},
			}}
			request := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/transaction/approve", strings.NewReader(test.body))
			request.AddCookie(&http.Cookie{Name: browserTransactionCookie, Value: "transaction-1"})
			response := httptest.NewRecorder()
			NewAuthorizationHandler(authorizer).HandleAuthorizationApproval(response, request)
			if response.Code != test.wantStatus || authorizer.grantedScope != test.wantScope || authorizer.request.Scope != test.wantScope {
				t.Fatalf("status=%d granted=%q authorized=%q body=%s", response.Code, authorizer.grantedScope, authorizer.request.Scope, response.Body.String())
			}
		})
	}
}

type fakeOAuthTokenIssuer struct {
	grant        authdomain.OAuthAuthorizationGrant
	refreshToken string
	err          error
}

func (f fakeOAuthTokenIssuer) ExchangeOAuthAuthorizationCode(_ context.Context, _ string, _ authdomain.OAuthAuthorizationCodeExchange) (authdomain.OAuthAuthorizationGrant, error) {
	return f.grant, f.err
}

func (f fakeOAuthTokenIssuer) IssueOAuthRefreshToken(authdomain.OAuthAuthorizationGrant) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.refreshToken, nil
}

func (f fakeOAuthTokenIssuer) RefreshOAuthAuthorizationGrant(string) (authdomain.OAuthAuthorizationGrant, string, error) {
	if f.err != nil {
		return authdomain.OAuthAuthorizationGrant{}, "", f.err
	}
	return f.grant, f.refreshToken, nil
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
		fakeOAuthTokenIssuer{grant: authdomain.OAuthAuthorizationGrant{Subject: "user-1", Scope: "home.devices.read"}, refreshToken: "refresh-token"},
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
	if payload["access_token"] != "mcp-access-token" || payload["refresh_token"] != "refresh-token" || payload["token_type"] != "Bearer" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload["expires_in"] != float64(authdomain.MCPAccessTokenTTL.Seconds()) {
		t.Fatalf("unexpected token lifetime: %#v", payload["expires_in"])
	}
}

func TestTokenHandlerRefreshesMCPToken(t *testing.T) {
	handler := NewTokenHandler(
		fakeOAuthTokenIssuer{grant: authdomain.OAuthAuthorizationGrant{Subject: "user-1", Scope: "home.devices.read"}, refreshToken: "rotated-refresh-token"},
		fakeOAuthUserProvider{user: &clientsinfra.User{ID: "user-1"}},
	)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/token", strings.NewReader("grant_type=refresh_token&refresh_token=refresh-token"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.HandleToken(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"refresh_token":"rotated-refresh-token"`) {
		t.Fatalf("unexpected refresh response: status=%d body=%s", response.Code, response.Body.String())
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
