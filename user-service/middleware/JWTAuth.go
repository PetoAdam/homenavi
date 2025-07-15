package middleware

import (
	"log"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Role string `json:"role"`
	Sub  string `json:"sub"`
	jwt.RegisteredClaims
}

func GetClaims(r *http.Request) *Claims {
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
