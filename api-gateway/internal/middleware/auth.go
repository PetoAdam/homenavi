package middleware

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Role string `json:"role"`
	Name string `json:"name"`
	Demo bool   `json:"demo"`
	jwt.RegisteredClaims
}

type claimsKeyType struct{}

// ClaimsKey is the context key used to store JWT claims.
var ClaimsKey claimsKeyType

type DemoActivityTracker struct {
	redisClient   redis.UniversalClient
	debounce      time.Duration
	ttl           time.Duration
	now           func() time.Time
}

func NewDemoActivityTracker(redisClient redis.UniversalClient, debounce, ttl time.Duration) *DemoActivityTracker {
	if redisClient == nil || debounce <= 0 || ttl <= 0 {
		return nil
	}
	return &DemoActivityTracker{
		redisClient: redisClient,
		debounce:    debounce,
		ttl:         ttl,
		now:         time.Now,
	}
}

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

func JWTAuthMiddlewareRS256(pubKey *rsa.PublicKey, tracker *DemoActivityTracker) func(http.Handler) http.Handler {
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
			if tracker != nil {
				tracker.Track(r.Context(), claims)
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (t *DemoActivityTracker) Track(ctx context.Context, claims *Claims) {
	if t == nil || claims == nil || !claims.Demo || claims.Subject == "" {
		return
	}
	key := "demo:last-active:" + claims.Subject
	cutoff := t.now().Add(-t.debounce).Unix()
	updatedAt := t.now().Unix()
	script := `
local current = redis.call("GET", KEYS[1])
if current then
  local currentNum = tonumber(current)
  if currentNum and currentNum > tonumber(ARGV[1]) then
    return 0
  end
end
redis.call("SET", KEYS[1], ARGV[2], "EX", ARGV[3])
return 1
`
	_ = t.redisClient.Eval(ctx, script, []string{key}, cutoff, updatedAt, int(t.ttl/time.Second)).Err()
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
