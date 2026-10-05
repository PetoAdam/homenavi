package middleware

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Role      string `json:"role"`
	Name      string `json:"name"`
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

type ValidationConfig struct {
	Issuer    string
	Audience  string
	TokenType string
	ClockSkew time.Duration
}

type claimsKeyType struct{}

// ClaimsKey is the context key used to store JWT claims.
var ClaimsKey claimsKeyType

func LoadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return jwt.ParseRSAPublicKeyFromPEM(keyData)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "code": status})
}

func JWTAuthMiddlewareRS256(pubKey *rsa.PublicKey, cfg ValidationConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing token")
				return
			}
			if pubKey == nil {
				writeJSONError(w, http.StatusUnauthorized, "token verification unavailable")
				return
			}
			parserOptions := []jwt.ParserOption{jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()})}
			if cfg.Issuer != "" {
				parserOptions = append(parserOptions, jwt.WithIssuer(cfg.Issuer))
			}
			if cfg.Audience != "" {
				parserOptions = append(parserOptions, jwt.WithAudience(cfg.Audience))
			}
			if cfg.ClockSkew > 0 {
				parserOptions = append(parserOptions, jwt.WithLeeway(cfg.ClockSkew))
			}
			token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
				return pubKey, nil
			}, parserOptions...)
			if err != nil || !token.Valid {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			claims, ok := token.Claims.(*Claims)
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "invalid claims")
				return
			}
			if cfg.TokenType != "" && claims.TokenType != cfg.TokenType {
				writeJSONError(w, http.StatusUnauthorized, "invalid token type")
				return
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func APIValidationConfig(issuer, audience string) ValidationConfig {
	return ValidationConfig{
		Issuer:    issuer,
		Audience:  audience,
		TokenType: authx.TokenTypeAPI,
		ClockSkew: 30 * time.Second,
	}
}

// RoleAtLeastMiddleware enforces that the user's role is at least the required role
func RoleAtLeastMiddleware(required string) func(http.Handler) http.Handler {
	roleRank := map[string]int{
		"public":   0,
		"user":     1,
		"resident": 2,
		"admin":    3,
		// internal roles
		"service": 4,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(ClaimsKey).(*Claims)
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			reqRank, ok := roleRank[required]
			if !ok {
				// Unknown requirement -> deny by default
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

func extractToken(r *http.Request) string {
	// First, try Authorization header (for API calls)
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}

	// Try cookie (for WebSocket connections)
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}

	return ""
}
