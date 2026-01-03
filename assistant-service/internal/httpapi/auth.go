package httpapi

import (
	"context"
	"crypto/rsa"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents JWT claims
type Claims struct {
	Role   string `json:"role"`
	Name   string `json:"name"`
	HomeID string `json:"home_id,omitempty"`
	jwt.RegisteredClaims
}

type contextKey string

const claimsKey contextKey = "claims"

// JWTAuthMiddleware validates JWT tokens
func JWTAuthMiddleware(pubKey *rsa.PublicKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing token")
				return
			}

			token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
				return pubKey, nil
			})
			if err != nil || !token.Valid {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			claims, ok := token.Claims.(*Claims)
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "invalid claims")
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRoleMiddleware enforces minimum role requirement
func RequireRoleMiddleware(required string) func(http.Handler) http.Handler {
	roleRank := map[string]int{
		"public":   0,
		"user":     1,
		"resident": 2,
		"admin":    3,
		"service":  4,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			reqRank, ok := roleRank[required]
			if !ok {
				writeJSONError(w, http.StatusForbidden, "forbidden")
				return
			}

			userRank := roleRank[claims.Role]
			if userRank < reqRank {
				writeJSONError(w, http.StatusForbidden, "forbidden")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetClaims extracts claims from context
func GetClaims(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsKey).(*Claims)
	return claims
}

func extractToken(r *http.Request) string {
	// Try Authorization header first
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}

	// Try cookie for WebSocket connections
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}

	return ""
}
