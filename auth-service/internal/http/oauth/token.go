package oauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

type oauthTokenIssuer interface {
	ExchangeOAuthAuthorizationCode(context.Context, string, authdomain.OAuthAuthorizationCodeExchange) (authdomain.OAuthAuthorizationGrant, error)
	IssueMCPAccessToken(user *clientsinfra.User, grant authdomain.OAuthAuthorizationGrant) (string, error)
	ExchangeMCPAccessToken(context.Context, string, string, string) (string, error)
}

type oauthUserProvider interface {
	GetUser(userID string) (*clientsinfra.User, error)
}

type TokenHandler struct {
	issuer       oauthTokenIssuer
	userProvider oauthUserProvider
}

func NewTokenHandler(issuer oauthTokenIssuer, userProvider oauthUserProvider) *TokenHandler {
	return &TokenHandler{issuer: issuer, userProvider: userProvider}
}

func (h *TokenHandler) HandleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	form := r.Form
	if form.Get("grant_type") == "" && r.Method == http.MethodPost {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
		if err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
			return
		}
		form, err = url.ParseQuery(string(body))
		if err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
			return
		}
	}
	if form.Get("grant_type") == "urn:ietf:params:oauth:grant-type:token-exchange" {
		h.exchangeDelegatedToken(w, r, form)
		return
	}
	if form.Get("grant_type") != "authorization_code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "authorization_code is required")
		return
	}
	grant, err := h.issuer.ExchangeOAuthAuthorizationCode(r.Context(), form.Get("code"), authdomain.OAuthAuthorizationCodeExchange{
		ClientID:     form.Get("client_id"),
		RedirectURI:  form.Get("redirect_uri"),
		Resource:     form.Get("resource"),
		CodeVerifier: form.Get("code_verifier"),
	})
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code exchange was rejected")
		return
	}
	user, err := h.userProvider.GetUser(grant.Subject)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization subject is unavailable")
		return
	}
	accessToken, err := h.issuer.IssueMCPAccessToken(user, grant)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to issue access token")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": accessToken, "token_type": "Bearer", "expires_in": int(authdomain.MCPAccessTokenTTL.Seconds()), "scope": grant.Scope})
}

func (h *TokenHandler) exchangeDelegatedToken(w http.ResponseWriter, r *http.Request, form url.Values) {
	resource := form.Get("resource")
	subjectToken := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	accessToken, err := h.issuer.ExchangeMCPAccessToken(r.Context(), subjectToken, resource, form.Get("scope"))
	if err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_grant", "MCP token exchange was rejected")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": accessToken, "token_type": "Bearer", "expires_in": int(authdomain.DelegatedAccessTokenTTL.Seconds()), "scope": form.Get("scope")})
}
