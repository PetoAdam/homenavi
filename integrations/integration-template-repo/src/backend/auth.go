package backend

import (
	"crypto/rsa"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Role string `json:"role"`
	Name string `json:"name"`
	jwt.RegisteredClaims
}

type AdminAuth struct {
	pubKey  *rsa.PublicKey
	enabled bool
}

func NewAdminAuthFromEnv() (*AdminAuth, error) {
	path := strings.TrimSpace(os.Getenv("JWT_PUBLIC_KEY_PATH"))
	if path == "" {
		return &AdminAuth{enabled: false}, nil
	}
	keyData, err := os.ReadFile(path) // #nosec G304 -- path comes from env/config
	if err != nil {
		return nil, err
	}
	pubKey, err := jwt.ParseRSAPublicKeyFromPEM(keyData)
	if err != nil {
		return nil, err
	}
	return &AdminAuth{pubKey: pubKey, enabled: true}, nil
}

func (a *AdminAuth) RequireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if a == nil || !a.enabled || a.pubKey == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "admin auth not configured")
		return false
	}
	tokenStr := extractToken(r)
	if tokenStr == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing token")
		return false
	}
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return a.pubKey, nil
	})
	if err != nil || !token.Valid {
		writeJSONError(w, http.StatusUnauthorized, "invalid token")
		return false
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "invalid claims")
		return false
	}
	if !roleAtLeast("admin", strings.TrimSpace(claims.Role)) {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
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

func roleAtLeast(required, actual string) bool {
	roleRank := map[string]int{
		"public":   0,
		"user":     1,
		"resident": 2,
		"admin":    3,
		"service":  4,
	}
	return roleRank[actual] >= roleRank[required]
}
