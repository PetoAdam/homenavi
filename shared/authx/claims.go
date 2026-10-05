// Package authx defines shared authentication claim and authorization constants.
package authx

import "strings"

const (
	ClaimAuthorizedParty = "azp"
	ClaimHomeID          = "home_id"
	ClaimScope           = "scope"
	ClaimTokenType       = "typ"

	DefaultIssuer = "homenavi-auth-service"

	AudienceAPI = "homenavi-api"

	TokenTypeAPI       = "homenavi-api"
	TokenTypeDelegated = "homenavi-delegated"
	TokenTypeMCP       = "homenavi-mcp"

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
