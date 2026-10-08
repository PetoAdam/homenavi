package auth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims automation-service cares about.
type Claims struct {
	Role      string `json:"role"`
	Sub       string `json:"sub"`
	SessionID string `json:"sid"`
	Scope     string `json:"scope"`
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

type SessionValidator func(context.Context, string) (bool, error)

type claimsKeyType struct{}

var claimsKey claimsKeyType

func LoadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return jwt.ParseRSAPublicKeyFromPEM(keyData)
}

func JWTAuthMiddlewareRS256(pubKey *rsa.PublicKey, validators ...SessionValidator) func(http.Handler) http.Handler {
	var sessionValidator SessionValidator
	if len(validators) > 0 {
		sessionValidator = validators[0]
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing token")
				return
			}
			token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
				if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
					return nil, jwt.ErrTokenUnverifiable
				}
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
			if claims.Issuer != authx.DefaultIssuer || claims.Sub == "" || claims.Role == "" || claims.SessionID == "" || claims.ID == "" {
				writeJSONError(w, http.StatusUnauthorized, "invalid token claims")
				return
			}
			switch claims.TokenType {
			case authx.TokenTypeAPI:
				if !authx.HasAudience(claims.Audience, authx.AudienceAPI) {
					writeJSONError(w, http.StatusUnauthorized, "invalid token audience")
					return
				}
			case authx.TokenTypeDelegated:
				if !authx.HasAudience(claims.Audience, authx.AudienceAPI) || (!authx.HasScope(claims.Scope, "home.automation.read") && !authx.HasScope(claims.Scope, "home.automation.execute")) {
					writeJSONError(w, http.StatusUnauthorized, "invalid delegated token")
					return
				}
				if sessionValidator == nil {
					writeJSONError(w, http.StatusServiceUnavailable, "session verification unavailable")
					return
				}
				active, err := sessionValidator(r.Context(), claims.SessionID)
				if err != nil {
					writeJSONError(w, http.StatusServiceUnavailable, "session verification unavailable")
					return
				}
				if !active {
					writeJSONError(w, http.StatusUnauthorized, "session revoked")
					return
				}
			default:
				writeJSONError(w, http.StatusUnauthorized, "invalid token type")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, claims)))
		})
	}
}

func RequireDelegatedScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r)
			if claims != nil && claims.TokenType == authx.TokenTypeDelegated && !authx.HasScope(claims.Scope, scope) {
				writeJSONError(w, http.StatusForbidden, "delegated token lacks required scope")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

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
				writeJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			reqRank, ok := roleRank[required]
			if !ok || roleRank[claims.Role] < reqRank {
				writeJSONError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func GetClaims(r *http.Request) *Claims {
	claims, _ := r.Context().Value(claimsKey).(*Claims)
	return claims
}

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}
	return ""
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "code": status})
}
