package middleware

import (
	"context"
	"crypto/rsa"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims we care about.
type Claims struct {
	Role string `json:"role"`
	Sub  string `json:"sub"`
	jwt.RegisteredClaims
}

type claimsKeyType struct{}

var claimsKey claimsKeyType

// JWTAuthMiddleware verifies an incoming Bearer token using the given RSA public key.
func JWTAuthMiddleware(pubKey *rsa.PublicKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				http.Error(w, "Missing token", http.StatusUnauthorized)
				return
			}
			token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
					return nil, jwt.ErrTokenUnverifiable
				}
				return pubKey, nil
			})
			if err != nil || !token.Valid {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			claims, ok := token.Claims.(*Claims)
			if !ok {
				http.Error(w, "Invalid claims", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RoleAtLeastMiddleware ensures caller has at least the required role (service>admin>resident>user>public).
func RoleAtLeastMiddleware(required string) func(http.Handler) http.Handler {
	roleRank := map[string]int{
		"public":   0,
		"user":     1,
		"resident": 2,
		"admin":    3,
		"service":  4,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(claimsKey).(*Claims)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			reqRank, ok := roleRank[required]
			if !ok {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			if roleRank[claims.Role] < reqRank {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	return ""
}

// GetClaims returns claims previously validated by middleware.
func GetClaims(r *http.Request) *Claims {
	claims, _ := r.Context().Value(claimsKey).(*Claims)
	return claims
}

