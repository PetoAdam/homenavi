package middleware

import (
	"context"
	"log"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Role string `json:"role"`
	Sub  string `json:"sub"`
	jwt.RegisteredClaims
}

type claimsKeyType struct{}

var claimsKey claimsKeyType

func GetClaims(r *http.Request) *Claims {
	// First try to get claims from context (for tests)
	if claims, ok := r.Context().Value(claimsKey).(*Claims); ok {
		return claims
	}
	
	// Fall back to parsing from Authorization header
	auth := r.Header.Get("Authorization")
	log.Printf("[DEBUG] Extracting claims from request with Authorization header: %s", auth)
	if len(auth) < 8 || auth[:7] != "Bearer " {
		return nil
	}
	tokenStr := auth[7:]
	token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &Claims{})
	if err != nil {
		return nil
	}
	if claims, ok := token.Claims.(*Claims); ok {
		return claims
	}
	return nil
}

// SetTestClaims is a helper function for testing to set claims in context
func SetTestClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}
