// Package authx defines shared authentication claim and authorization constants.
package authx

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	ClaimAuthorizedParty = "azp"
	ClaimHomeID          = "home_id"
	ClaimSessionID       = "sid"
	ClaimScope           = "scope"
	ClaimTokenType       = "typ"

	DefaultIssuer = "homenavi-auth-service"

	AudienceAPI         = "homenavi-api"
	AudienceAutomation  = "homenavi-automation-service"
	AudienceUserService = "homenavi-user-service"

	TokenTypeAPI       = "homenavi-api"
	TokenTypeDelegated = "homenavi-delegated"
	TokenTypeMCP       = "homenavi-mcp"
	TokenTypeService   = "homenavi-service"

	ServicePrincipalAuth = "homenavi-auth-service"

	SessionStatusActive  = "active"
	SessionStatusRevoked = "revoked"

	RoleUser     = "user"
	RoleResident = "resident"
	RoleAdmin    = "admin"
	RoleService  = "service"
)

// ParseScopes returns a de-duplicated scope set from a space-delimited claim value.
// Empty whitespace is ignored so callers can safely evaluate optional scope claims.
func ParseScopes(value string) map[string]struct{} {
	scopes := make(map[string]struct{})
	for _, scope := range strings.Fields(value) {
		scopes[scope] = struct{}{}
	}
	return scopes
}

// HasScope reports whether a space-delimited scope claim includes requiredScope.
func HasScope(value, requiredScope string) bool {
	if requiredScope == "" {
		return false
	}
	_, ok := ParseScopes(value)[requiredScope]
	return ok
}

// HasAudience reports whether audiences includes requiredAudience exactly.
func HasAudience(audiences []string, requiredAudience string) bool {
	if requiredAudience == "" {
		return false
	}
	for _, audience := range audiences {
		if audience == requiredAudience {
			return true
		}
	}
	return false
}

// SessionStatusKey returns the shared Redis key used to enforce access-token session revocation.
func SessionStatusKey(sessionID string) string {
	return "auth_session:" + sessionID
}

// KeyIDForRSAPublicKey returns the stable identifier shared by JWT issuers and verifiers.
func KeyIDForRSAPublicKey(publicKey *rsa.PublicKey) (string, error) {
	if publicKey == nil {
		return "", fmt.Errorf("RSA public key is required")
	}
	publicKeyDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("marshal RSA public key: %w", err)
	}
	digest := sha256.Sum256(publicKeyDER)
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}
