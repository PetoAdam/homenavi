package auth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims user-service cares about.
type Claims struct {
	Role      string `json:"role"`
	Sub       string `json:"sub"`
	SessionID string `json:"sid"`
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

type ValidationConfig struct {
	Issuer           string
	APIAudience      string
	ServiceAudience  string
	SessionValidator SessionValidator
}

type SessionValidator func(context.Context, string) (bool, error)

type claimsKeyType struct{}

var claimsKey claimsKeyType

func LoadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return jwt.ParseRSAPublicKeyFromPEM(data)
}

// RSAPublicKeySet holds all active and overlap JWT verification keys by kid.
type RSAPublicKeySet struct {
	keys map[string]*rsa.PublicKey
}

func NewRSAPublicKeySet(publicKeys ...*rsa.PublicKey) (RSAPublicKeySet, error) {
	keys := make(map[string]*rsa.PublicKey, len(publicKeys))
	for _, publicKey := range publicKeys {
		if publicKey == nil {
			return RSAPublicKeySet{}, fmt.Errorf("JWT public key is required")
		}
		keyID, err := authx.KeyIDForRSAPublicKey(publicKey)
		if err != nil {
			return RSAPublicKeySet{}, fmt.Errorf("derive JWT key ID: %w", err)
		}
		keys[keyID] = publicKey
	}
	if len(keys) == 0 {
		return RSAPublicKeySet{}, fmt.Errorf("at least one JWT public key is required")
	}
	return RSAPublicKeySet{keys: keys}, nil
}

func (s RSAPublicKeySet) PublicKeyForID(keyID string) (*rsa.PublicKey, bool) {
	publicKey, ok := s.keys[keyID]
	return publicKey, ok
}

// JWTAuthMiddleware verifies a bearer token against the configured key set.
func JWTAuthMiddleware(publicKeys RSAPublicKeySet, cfg ValidationConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := ExtractToken(r)
			if tokenStr == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing token")
				return
			}
			if len(publicKeys.keys) == 0 {
				writeJSONError(w, http.StatusUnauthorized, "token verification unavailable")
				return
			}
			token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
				if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
					return nil, jwt.ErrTokenUnverifiable
				}
				keyID, ok := token.Header["kid"].(string)
				if !ok || keyID == "" {
					return nil, jwt.ErrTokenUnverifiable
				}
				publicKey, ok := publicKeys.PublicKeyForID(keyID)
				if !ok {
					return nil, jwt.ErrTokenUnverifiable
				}
				return publicKey, nil
			}, jwt.WithIssuer(cfg.Issuer), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(30*time.Second))
			if err != nil || !token.Valid {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			claims, ok := token.Claims.(*Claims)
			if !ok || claims.Sub == "" || claims.Role == "" || claims.TokenType == "" || claims.ID == "" || claims.NotBefore == nil || claims.IssuedAt == nil || !hasAcceptedProfile(claims, cfg) {
				writeJSONError(w, http.StatusUnauthorized, "invalid claims")
				return
			}
			if claims.TokenType == authx.TokenTypeAPI {
				if claims.SessionID == "" {
					writeJSONError(w, http.StatusUnauthorized, "invalid claims")
					return
				}
				if cfg.SessionValidator != nil {
					active, err := cfg.SessionValidator(r.Context(), claims.SessionID)
					if err != nil {
						writeJSONError(w, http.StatusServiceUnavailable, "session verification unavailable")
						return
					}
					if !active {
						writeJSONError(w, http.StatusUnauthorized, "session revoked")
						return
					}
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, claims)))
		})
	}
}

func hasAcceptedProfile(claims *Claims, cfg ValidationConfig) bool {
	for _, audience := range claims.Audience {
		if claims.TokenType == authx.TokenTypeAPI && audience == cfg.APIAudience {
			return true
		}
		if claims.TokenType == authx.TokenTypeService && audience == cfg.ServiceAudience {
			return true
		}
	}
	return false
}

// RoleAtLeastMiddleware ensures caller has at least the required role.
func RoleAtLeastMiddleware(required string) func(http.Handler) http.Handler {
	roleRank := map[string]int{"public": 0, "user": 1, "resident": 2, "admin": 3, "service": 4}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r)
			if claims == nil {
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

func ExtractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	return ""
}

func GetClaims(r *http.Request) *Claims {
	claims, _ := r.Context().Value(claimsKey).(*Claims)
	return claims
}

// WithClaims injects claims into a context for tests and internal helpers.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "code": status})
}
