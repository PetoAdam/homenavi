package auth

import (
	"net/http"
	"strings"
	"time"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
)

const (
	AccessTokenCookieName  = "auth_token"
	RefreshTokenCookieName = "homenavi_refresh_token"
)

func SetSessionCookies(w http.ResponseWriter, r *http.Request, tokens *authdomain.TokenPair) {
	if tokens == nil {
		return
	}
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	setCookie(w, AccessTokenCookieName, tokens.AccessToken, 15*time.Minute, secure)
	setCookie(w, RefreshTokenCookieName, tokens.RefreshToken, 7*24*time.Hour, secure)
}

func ClearSessionCookies(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	setCookie(w, AccessTokenCookieName, "", -time.Hour, secure)
	setCookie(w, RefreshTokenCookieName, "", -time.Hour, secure)
}

func RefreshTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(RefreshTokenCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func setCookie(w http.ResponseWriter, name, value string, ttl time.Duration, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: int(ttl.Seconds())})
}
