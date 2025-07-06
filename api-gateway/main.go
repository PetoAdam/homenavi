package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

var (
	authServiceURL = os.Getenv("AUTH_SERVICE_URL")
	userServiceURL = os.Getenv("USER_SERVICE_URL")
	jwtSecret      = []byte(os.Getenv("JWT_SECRET"))
)

type Claims struct {
	Role string `json:"role"`
	Name string `json:"name"`
	jwt.RegisteredClaims
}

func main() {
	r := chi.NewRouter()

	r.Post("/api/login", handleLogin)
	r.Route("/api/user", func(r chi.Router) {
		r.Use(jwtMiddleware)
		r.Get("/{id}", handleUserGet)
	})

	fmt.Println("API Gateway running on :8080")
	http.ListenAndServe(":8080", r)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Post(authServiceURL+"/login", "application/json", r.Body)
	if err != nil {
		http.Error(w, "Auth service error", 502)
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func handleUserGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, _ := http.NewRequest("GET", userServiceURL+"/user/"+id, nil)
	for k, v := range r.Header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "User service error", 502)
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func jwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := extractToken(r)
		if tokenStr == "" {
			http.Error(w, "Missing token", 401)
			return
		}
		token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			http.Error(w, "Invalid token", 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
