package oauth

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"html"
	"net/http"
	"net/url"
	"strings"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
)

type oauthAuthorizer interface {
	ExtractOAuthSession(string) (authdomain.OAuthSession, error)
	BeginOAuthAuthorization(context.Context, authdomain.OAuthAuthorizationRequest) (authdomain.OAuthAuthorizationTransaction, error)
	GetOAuthAuthorizationTransaction(context.Context, string) (authdomain.OAuthAuthorizationTransaction, error)
	SetOAuthAuthorizationSession(context.Context, string, string, string) error
	SetOAuthAuthorizationPendingUser(context.Context, string, string) error
	DeleteOAuthAuthorizationTransaction(context.Context, string) error
	AuthorizeOAuthForSession(context.Context, authdomain.OAuthAuthorizationRequest, string, string) (string, error)
	GrantOAuthConsent(context.Context, authdomain.OAuthAuthorizationRequest, string, string) error
	RevokeOAuthConsent(context.Context, string, string) error
	RegisterDynamicOAuthClient(context.Context, authdomain.DynamicOAuthClientRegistration) (authdomain.OAuthClient, error)
}

func (h *AuthorizationHandler) HandleRegisterClient(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ClientName              string   `json:"client_name"`
		RedirectURIs            []string `json:"redirect_uris"`
		ApplicationType         string   `json:"application_type"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&request); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "invalid registration request")
		return
	}
	applicationType := request.ApplicationType
	if applicationType == "" {
		applicationType = "native"
	}
	if applicationType != "native" && applicationType != "web" || request.TokenEndpointAuthMethod != "" && request.TokenEndpointAuthMethod != "none" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "only public web or native clients are supported")
		return
	}
	client, err := h.authorizer.RegisterDynamicOAuthClient(r.Context(), authdomain.DynamicOAuthClientRegistration{
		Name:                    request.ClientName,
		RedirectURIs:            request.RedirectURIs,
		ApplicationType:         applicationType,
		TokenEndpointAuthMethod: request.TokenEndpointAuthMethod,
	})
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "client registration was rejected")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"client_id": client.ClientID, "client_name": client.DisplayName, "redirect_uris": client.RedirectURIs, "token_endpoint_auth_method": client.TokenEndpointAuthMethod, "grant_types": []string{"authorization_code"}, "response_types": []string{"code"}, "application_type": client.ApplicationType, "scope": strings.Join(client.Scopes, " ")})
}

type browserAuthenticator interface {
	StartLogin(string, string, authdomain.CredentialUserProvider, authdomain.TwoFactorCodeSender) (*authdomain.LoginStartResult, error)
	FinishLogin(string, string, authdomain.UserProvider) (*authdomain.TokenPair, error)
}

const (
	browserTransactionCookie = "homenavi_oauth_transaction"
	browserSessionCookie     = "homenavi_oauth_session"
	appSessionCookie         = "auth_token"
)

// AuthorizationHandler requires an existing authenticated browser/API session and
// explicit approval before issuing a redirect-bound authorization code.
type AuthorizationHandler struct {
	authorizer         oauthAuthorizer
	browser            browserAuthenticator
	users              authdomain.CredentialUserProvider
	email              authdomain.TwoFactorCodeSender
	authorizationUIURL string
}

func (h *AuthorizationHandler) ConfigureBrowserLogin(browser browserAuthenticator, users authdomain.CredentialUserProvider, email authdomain.TwoFactorCodeSender, authorizationUIURL string) {
	h.browser = browser
	h.users = users
	h.email = email
	h.authorizationUIURL = strings.TrimSpace(authorizationUIURL)
}

func NewAuthorizationHandler(authorizer oauthAuthorizer) *AuthorizationHandler {
	return &AuthorizationHandler{authorizer: authorizer}
}

func (h *AuthorizationHandler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	request := oauthAuthorizationRequestFromQuery(r.URL.Query())
	bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if bearer == "" {
		transaction, err := h.authorizer.BeginOAuthAuthorization(r.Context(), request)
		authorizationUIURL, urlErr := h.authorizationUIURLForRequest(r)
		if err != nil || urlErr != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request was rejected")
			return
		}
		if session, ok := h.appBrowserSession(r); ok {
			if !authdomain.MCPRoleAllowed(session.Role) {
				writeOAuthError(w, http.StatusForbidden, "insufficient_permissions", "MCP access requires resident role or above")
				return
			}
			if err := h.authorizer.SetOAuthAuthorizationSession(r.Context(), transaction.ID, session.Subject, session.SessionID); err != nil {
				writeOAuthError(w, http.StatusGone, "invalid_request", "authorization session expired")
				return
			}
		}
		h.setBrowserTransaction(w, r, transaction.ID)
		http.Redirect(w, r, authorizationUIURL, http.StatusFound)
		return
	}
	session, err := h.authorizer.ExtractOAuthSession(bearer)
	if err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token", "authentication is required")
		return
	}
	code, err := h.authorizer.AuthorizeOAuthForSession(r.Context(), request, session.Subject, session.SessionID)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request was rejected")
		return
	}
	redirect, err := url.Parse(request.RedirectURI)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request was rejected")
		return
	}
	values := redirect.Query()
	values.Set("code", code)
	values.Set("state", request.State)
	redirect.RawQuery = values.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (h *AuthorizationHandler) authorizationUIURLForRequest(request *http.Request) (string, error) {
	if h.authorizationUIURL != "" {
		return h.authorizationUIURL, nil
	}
	origin, err := publicOriginForRequest(request)
	if err != nil {
		return "", err
	}
	return origin + "/oauth/authorize", nil
}

func (h *AuthorizationHandler) HandleAuthorizationTransaction(w http.ResponseWriter, r *http.Request) {
	transaction, ok := h.browserTransaction(w, r)
	if !ok {
		return
	}
	writeOAuthJSON(w, http.StatusOK, map[string]any{"client_id": transaction.Request.ClientID, "scopes": strings.Fields(transaction.Request.Scope), "authenticated": transaction.Subject != "", "two_fa_required": transaction.PendingUserID != ""})
}

func (h *AuthorizationHandler) HandleAuthorizationLogin(w http.ResponseWriter, r *http.Request) {
	transaction, ok := h.browserTransaction(w, r)
	if !ok || h.browser == nil || h.users == nil {
		return
	}
	var request struct{ Email, Password string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&request); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid sign-in request")
		return
	}
	result, err := h.browser.StartLogin(request.Email, request.Password, h.users, h.email)
	if err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "access_denied", "sign-in failed")
		return
	}
	if result.TwoFARequired {
		if err := h.authorizer.SetOAuthAuthorizationPendingUser(r.Context(), transaction.ID, result.UserID); err != nil {
			writeOAuthError(w, http.StatusGone, "invalid_request", "authorization session expired")
			return
		}
		writeOAuthJSON(w, http.StatusOK, map[string]any{"two_fa_required": true})
		return
	}
	if err := h.setTransactionSession(r.Context(), transaction.ID, result.AccessToken); err != nil {
		h.writeTransactionSessionError(w, err)
		return
	}
	writeOAuthJSON(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (h *AuthorizationHandler) HandleAuthorizationVerification(w http.ResponseWriter, r *http.Request) {
	transaction, ok := h.browserTransaction(w, r)
	if !ok || h.browser == nil || h.users == nil || transaction.PendingUserID == "" {
		if ok {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "verification is not pending")
		}
		return
	}
	var request struct{ Code string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&request); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid verification request")
		return
	}
	tokens, err := h.browser.FinishLogin(transaction.PendingUserID, request.Code, h.users)
	if err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "access_denied", "verification failed")
		return
	}
	if err := h.setTransactionSession(r.Context(), transaction.ID, tokens.AccessToken); err != nil {
		h.writeTransactionSessionError(w, err)
		return
	}
	writeOAuthJSON(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (h *AuthorizationHandler) HandleAuthorizationApproval(w http.ResponseWriter, r *http.Request) {
	transaction, ok := h.browserTransaction(w, r)
	if !ok || transaction.Subject == "" || transaction.SessionID == "" {
		if ok {
			writeOAuthError(w, http.StatusUnauthorized, "invalid_token", "sign-in is required")
		}
		return
	}
	if err := h.authorizer.GrantOAuthConsent(r.Context(), transaction.Request, transaction.Subject, transaction.SessionID); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "consent request was rejected")
		return
	}
	code, err := h.authorizer.AuthorizeOAuthForSession(r.Context(), transaction.Request, transaction.Subject, transaction.SessionID)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request was rejected")
		return
	}
	redirect, err := authorizationRedirect(transaction.Request, code, "")
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request was rejected")
		return
	}
	_ = h.authorizer.DeleteOAuthAuthorizationTransaction(r.Context(), transaction.ID)
	h.clearBrowserTransaction(w, r)
	writeOAuthJSON(w, http.StatusOK, map[string]string{"redirect_uri": redirect})
}

func (h *AuthorizationHandler) HandleAuthorizationDenial(w http.ResponseWriter, r *http.Request) {
	transaction, ok := h.browserTransaction(w, r)
	if !ok {
		return
	}
	redirect, err := authorizationRedirect(transaction.Request, "", "access_denied")
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request was rejected")
		return
	}
	_ = h.authorizer.DeleteOAuthAuthorizationTransaction(r.Context(), transaction.ID)
	h.clearBrowserTransaction(w, r)
	writeOAuthJSON(w, http.StatusOK, map[string]string{"redirect_uri": redirect})
}

func (h *AuthorizationHandler) browserTransaction(w http.ResponseWriter, r *http.Request) (authdomain.OAuthAuthorizationTransaction, bool) {
	cookie, err := r.Cookie(browserTransactionCookie)
	if err != nil || cookie.Value == "" {
		writeOAuthError(w, http.StatusGone, "invalid_request", "authorization session expired")
		return authdomain.OAuthAuthorizationTransaction{}, false
	}
	transaction, err := h.authorizer.GetOAuthAuthorizationTransaction(r.Context(), cookie.Value)
	if err != nil {
		writeOAuthError(w, http.StatusGone, "invalid_request", "authorization session expired")
		return authdomain.OAuthAuthorizationTransaction{}, false
	}
	return transaction, true
}

func (h *AuthorizationHandler) setTransactionSession(ctx context.Context, transactionID, accessToken string) error {
	session, err := h.authorizer.ExtractOAuthSession(accessToken)
	if err != nil {
		return err
	}
	if !authdomain.MCPRoleAllowed(session.Role) {
		return authdomain.ErrMCPRoleRequired
	}
	return h.authorizer.SetOAuthAuthorizationSession(ctx, transactionID, session.Subject, session.SessionID)
}

func (h *AuthorizationHandler) writeTransactionSessionError(w http.ResponseWriter, err error) {
	if stdErrors.Is(err, authdomain.ErrMCPRoleRequired) {
		writeOAuthError(w, http.StatusForbidden, "insufficient_permissions", authdomain.ErrMCPRoleRequired.Error())
		return
	}
	writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to establish authorization session")
}

func (h *AuthorizationHandler) setBrowserTransaction(w http.ResponseWriter, r *http.Request, transactionID string) {
	http.SetCookie(w, &http.Cookie{Name: browserTransactionCookie, Value: transactionID, Path: "/api/auth/oauth", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), MaxAge: 900})
}

func (h *AuthorizationHandler) clearBrowserTransaction(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: browserTransactionCookie, Value: "", Path: "/api/auth/oauth", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), MaxAge: -1})
}

func authorizationRedirect(request authdomain.OAuthAuthorizationRequest, code, oauthError string) (string, error) {
	redirect, err := url.Parse(request.RedirectURI)
	if err != nil {
		return "", err
	}
	values := redirect.Query()
	if code != "" {
		values.Set("code", code)
	}
	if oauthError != "" {
		values.Set("error", oauthError)
	}
	values.Set("state", request.State)
	redirect.RawQuery = values.Encode()
	return redirect.String(), nil
}

func writeOAuthJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *AuthorizationHandler) HandleBrowserAuthorization(w http.ResponseWriter, r *http.Request) {
	if h.browser == nil || h.users == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "browser authorization is unavailable")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderLogin(w, r, oauthAuthorizationRequestFromQuery(r.URL.Query()), "Invalid form submission.")
		return
	}
	request := oauthAuthorizationRequestFromQuery(r.URL.Query())
	switch r.Form.Get("action") {
	case "login":
		result, err := h.browser.StartLogin(r.Form.Get("email"), r.Form.Get("password"), h.users, h.email)
		if err != nil {
			h.renderLogin(w, r, request, "Sign-in failed. Check your credentials and try again.")
			return
		}
		if result.TwoFARequired {
			h.renderVerification(w, r, request, result.UserID)
			return
		}
		h.setBrowserSession(w, r, result.AccessToken)
		session, err := h.authorizer.ExtractOAuthSession(result.AccessToken)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to establish authorization session")
			return
		}
		h.renderConsent(w, r, request, session)
	case "verify":
		tokens, err := h.browser.FinishLogin(r.Form.Get("user_id"), r.Form.Get("code"), h.users)
		if err != nil {
			h.renderVerification(w, r, request, r.Form.Get("user_id"))
			return
		}
		h.setBrowserSession(w, r, tokens.AccessToken)
		session, err := h.authorizer.ExtractOAuthSession(tokens.AccessToken)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to establish authorization session")
			return
		}
		h.renderConsent(w, r, request, session)
	case "approve":
		session, ok := h.browserSession(r)
		if !ok {
			h.renderLogin(w, r, request, "Your authorization session expired. Please sign in again.")
			return
		}
		if err := h.authorizer.GrantOAuthConsent(r.Context(), request, session.Subject, session.SessionID); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "consent request was rejected")
			return
		}
		h.redirectWithCode(w, r, request, session)
	case "deny":
		h.redirectWithError(w, request, "access_denied")
	default:
		h.renderLogin(w, r, request, "Invalid authorization action.")
	}
}

func (h *AuthorizationHandler) browserSession(r *http.Request) (authdomain.OAuthSession, bool) {
	cookie, err := r.Cookie(browserSessionCookie)
	if err != nil || cookie.Value == "" {
		return authdomain.OAuthSession{}, false
	}
	session, err := h.authorizer.ExtractOAuthSession(cookie.Value)
	return session, err == nil
}

// appBrowserSession reuses the current Homenavi web session for consent without
// copying its bearer credential into the OAuth transaction or issuing a new cookie.
func (h *AuthorizationHandler) appBrowserSession(r *http.Request) (authdomain.OAuthSession, bool) {
	cookie, err := r.Cookie(appSessionCookie)
	if err != nil || cookie.Value == "" {
		return authdomain.OAuthSession{}, false
	}
	session, err := h.authorizer.ExtractOAuthSession(cookie.Value)
	return session, err == nil
}

func (h *AuthorizationHandler) setBrowserSession(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: token, Path: "/api/auth/oauth", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), MaxAge: 900})
}

func (h *AuthorizationHandler) redirectWithCode(w http.ResponseWriter, r *http.Request, request authdomain.OAuthAuthorizationRequest, session authdomain.OAuthSession) {
	code, err := h.authorizer.AuthorizeOAuthForSession(r.Context(), request, session.Subject, session.SessionID)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request was rejected")
		return
	}
	redirect, err := url.Parse(request.RedirectURI)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request was rejected")
		return
	}
	values := redirect.Query()
	values.Set("code", code)
	values.Set("state", request.State)
	redirect.RawQuery = values.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (h *AuthorizationHandler) redirectWithError(w http.ResponseWriter, request authdomain.OAuthAuthorizationRequest, code string) {
	redirect, err := url.Parse(request.RedirectURI)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request was rejected")
		return
	}
	values := redirect.Query()
	values.Set("error", code)
	values.Set("state", request.State)
	redirect.RawQuery = values.Encode()
	w.Header().Set("Location", redirect.String())
	w.WriteHeader(http.StatusFound)
}

func (h *AuthorizationHandler) renderLogin(w http.ResponseWriter, r *http.Request, request authdomain.OAuthAuthorizationRequest, message string) {
	h.renderPage(w, "Sign in to Homenavi", r.URL.RequestURI(), `<label>Email<input name="email" type="email" required autofocus></label><label>Password<input name="password" type="password" required></label><button name="action" value="login">Sign in</button>`, message, request)
}

func (h *AuthorizationHandler) renderVerification(w http.ResponseWriter, r *http.Request, request authdomain.OAuthAuthorizationRequest, userID string) {
	form := `<input name="user_id" type="hidden" value="` + html.EscapeString(userID) + `"><label>Verification code<input name="code" inputmode="numeric" autocomplete="one-time-code" required autofocus></label><button name="action" value="verify">Verify</button>`
	h.renderPage(w, "Verify your identity", r.URL.RequestURI(), form, "", request)
}

func (h *AuthorizationHandler) renderConsent(w http.ResponseWriter, r *http.Request, request authdomain.OAuthAuthorizationRequest, _ authdomain.OAuthSession) {
	form := `<p><strong>` + html.EscapeString(request.ClientID) + `</strong> requests access to:</p><ul>`
	for _, scope := range strings.Fields(request.Scope) {
		form += `<li>` + html.EscapeString(scope) + `</li>`
	}
	form += `</ul><button name="action" value="approve">Allow</button><button name="action" value="deny">Deny</button>`
	h.renderPage(w, "Authorize Homenavi access", r.URL.RequestURI(), form, "", request)
}

func (h *AuthorizationHandler) renderPage(w http.ResponseWriter, title, action, form, message string, _ authdomain.OAuthAuthorizationRequest) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	messageHTML := ""
	if message != "" {
		messageHTML = `<p role="alert">` + html.EscapeString(message) + `</p>`
	}
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>` + html.EscapeString(title) + `</title><style>body{font-family:system-ui,sans-serif;max-width:34rem;margin:4rem auto;padding:0 1rem}label{display:block;margin:1rem 0}input{display:block;width:100%;box-sizing:border-box;padding:.6rem;margin-top:.3rem}button{padding:.6rem 1rem;margin:.5rem .5rem .5rem 0}</style></head><body><h1>` + html.EscapeString(title) + `</h1>` + messageHTML + `<form method="post" action="` + html.EscapeString(action) + `">` + form + `</form></body></html>`))
}

func (h *AuthorizationHandler) HandleGrantConsent(w http.ResponseWriter, r *http.Request) {
	var request authdomain.OAuthAuthorizationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&request); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid consent request")
		return
	}
	session, ok := h.sessionFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.authorizer.GrantOAuthConsent(r.Context(), request, session.Subject, session.SessionID); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "consent request was rejected")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthorizationHandler) HandleRevokeConsent(w http.ResponseWriter, r *http.Request) {
	clientID := strings.TrimSpace(r.PathValue("clientID"))
	session, ok := h.sessionFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.authorizer.RevokeOAuthConsent(r.Context(), session.Subject, clientID); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "consent revocation was rejected")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthorizationHandler) sessionFromRequest(w http.ResponseWriter, r *http.Request) (authdomain.OAuthSession, bool) {
	bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if bearer == "" {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token", "authentication is required")
		return authdomain.OAuthSession{}, false
	}
	session, err := h.authorizer.ExtractOAuthSession(bearer)
	if err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token", "authentication is required")
		return authdomain.OAuthSession{}, false
	}
	return session, true
}

func oauthAuthorizationRequestFromQuery(values url.Values) authdomain.OAuthAuthorizationRequest {
	return authdomain.OAuthAuthorizationRequest{
		ClientID:            values.Get("client_id"),
		RedirectURI:         values.Get("redirect_uri"),
		ResponseType:        values.Get("response_type"),
		Scope:               values.Get("scope"),
		State:               values.Get("state"),
		CodeChallenge:       values.Get("code_challenge"),
		CodeChallengeMethod: values.Get("code_challenge_method"),
		Resource:            values.Get("resource"),
	}
}

func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": description})
}
