package auth

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
)

const (
	MCPAccessTokenTTL       = time.Hour
	DelegatedAccessTokenTTL = 2 * time.Minute
)

func (s *Service) AuthorizeOAuth(ctx context.Context, request OAuthAuthorizationRequest, subject string) (string, error) {
	return s.AuthorizeOAuthForSession(ctx, request, subject, "")
}

func (s *Service) AuthorizeOAuthForSession(ctx context.Context, request OAuthAuthorizationRequest, subject, sessionID string) (string, error) {
	if s.oauthClients == nil || s.oauthCodes == nil || s.oauthConsents == nil || strings.TrimSpace(subject) == "" || strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("OAuth authorization is not configured")
	}
	if _, err := s.validateOAuthRequest(ctx, request); err != nil {
		return "", err
	}
	granted, err := s.oauthConsents.Has(ctx, subject, request)
	if err != nil {
		return "", err
	}
	if !granted {
		return "", fmt.Errorf("OAuth consent is required")
	}
	return s.oauthCodes.Issue(ctx, OAuthAuthorizationCodeInput{
		Subject:       subject,
		SessionID:     sessionID,
		ClientID:      request.ClientID,
		RedirectURI:   request.RedirectURI,
		Scope:         strings.Join(strings.Fields(request.Scope), " "),
		Resource:      request.Resource,
		CodeChallenge: request.CodeChallenge,
	})
}

func (s *Service) GrantOAuthConsent(ctx context.Context, request OAuthAuthorizationRequest, subject, sessionID string) error {
	if s.oauthClients == nil || s.oauthConsents == nil || strings.TrimSpace(subject) == "" || strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("OAuth authorization is not configured")
	}
	if _, err := s.validateOAuthRequest(ctx, request); err != nil {
		return err
	}
	return s.oauthConsents.Grant(ctx, subject, request)
}

func (s *Service) RevokeOAuthConsent(ctx context.Context, subject, clientID string) error {
	return s.oauthConsents.Revoke(ctx, subject, clientID)
}

func (s *Service) ExchangeOAuthAuthorizationCode(ctx context.Context, code string, exchange OAuthAuthorizationCodeExchange) (OAuthAuthorizationGrant, error) {
	if s.oauthCodes == nil {
		return OAuthAuthorizationGrant{}, fmt.Errorf("OAuth authorization is not configured")
	}
	return s.oauthCodes.Consume(ctx, code, exchange)
}

func (s *Service) IssueMCPAccessToken(user *clientsinfra.User, grant OAuthAuthorizationGrant) (string, error) {
	if user == nil || user.ID == "" || grant.Subject != user.ID || grant.SessionID == "" || grant.ClientID == "" || grant.Resource == "" {
		return "", fmt.Errorf("invalid MCP token grant")
	}
	if !MCPRoleAllowed(user.Role) {
		return "", ErrMCPRoleRequired
	}
	tokenID, err := newTokenID()
	if err != nil {
		return "", fmt.Errorf("generate MCP token ID: %w", err)
	}
	now := time.Now()
	issuer, err := s.mcpAuthorizationServerIssuer(grant.Resource)
	if err != nil {
		return "", err
	}
	claims := jwt.MapClaims{
		"iss":                      issuer,
		"sub":                      user.ID,
		"aud":                      []string{grant.Resource},
		"exp":                      now.Add(MCPAccessTokenTTL).Unix(),
		"iat":                      now.Unix(),
		"nbf":                      now.Unix(),
		"jti":                      tokenID,
		authx.ClaimSessionID:       grant.SessionID,
		"role":                     user.Role,
		authx.ClaimAuthorizedParty: grant.ClientID,
		authx.ClaimScope:           grant.Scope,
		authx.ClaimTokenType:       authx.TokenTypeMCP,
	}
	return s.keyRing.Sign(claims)
}

func (s *Service) ExchangeMCPAccessToken(ctx context.Context, tokenString, resource, requestedScope string) (string, error) {
	issuer, err := s.mcpAuthorizationServerIssuer(resource)
	if err != nil {
		return "", err
	}
	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing algorithm")
		}
		keyID, _ := token.Header["kid"].(string)
		return s.keyRing.PublicKeyForID(keyID)
	}, jwt.WithIssuer(issuer), jwt.WithAudience(resource), jwt.WithExpirationRequired(), jwt.WithLeeway(30*time.Second))
	if err != nil || !token.Valid {
		return "", fmt.Errorf("validate MCP subject token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if requestedScope != "home.automation.read" && requestedScope != "home.automation.execute" {
		return "", fmt.Errorf("unsupported delegated scope")
	}
	if !ok || claims[authx.ClaimTokenType] != authx.TokenTypeMCP || !authx.HasScope(fmt.Sprint(claims[authx.ClaimScope]), requestedScope) {
		return "", fmt.Errorf("MCP token is not authorized for the requested automation scope")
	}
	subject, _ := claims.GetSubject()
	sessionID, _ := claims[authx.ClaimSessionID].(string)
	role, _ := claims["role"].(string)
	if subject == "" || sessionID == "" || role == "" {
		return "", fmt.Errorf("MCP token is missing delegation claims")
	}
	if err := s.requireActiveSession(sessionID); err != nil {
		return "", err
	}
	tokenID, err := newTokenID()
	if err != nil {
		return "", fmt.Errorf("generate delegated token ID: %w", err)
	}
	now := time.Now()
	return s.keyRing.Sign(jwt.MapClaims{
		"iss":                s.config.JWTIssuer,
		"sub":                subject,
		"aud":                []string{authx.AudienceAutomation},
		"exp":                now.Add(DelegatedAccessTokenTTL).Unix(),
		"iat":                now.Unix(),
		"nbf":                now.Unix(),
		"jti":                tokenID,
		"role":               role,
		authx.ClaimSessionID: sessionID,
		authx.ClaimScope:     requestedScope,
		authx.ClaimTokenType: authx.TokenTypeDelegated,
	})
}

func (s *Service) mcpAuthorizationServerIssuer(resource string) (string, error) {
	if issuer := strings.TrimSpace(s.config.MCPAuthorizationServerIssuer); issuer != "" {
		return issuer, nil
	}
	parsed, err := url.Parse(resource)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "/mcp" {
		return "", fmt.Errorf("invalid MCP resource for authorization-server issuer")
	}
	parsed.Path = "/api/auth"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
