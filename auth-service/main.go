package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	userServiceURL = os.Getenv("USER_SERVICE_URL")
	jwtSecret      = []byte(os.Getenv("JWT_SECRET"))
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type UserValidateResponse struct {
	UserID string `json:"user_id"`
	Valid  bool   `json:"valid"`
}

func main() {
	http.HandleFunc("/login", handleLogin)
	http.ListenAndServe(":8000", nil)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	// Forward to user-service for validation
	userReq, _ := json.Marshal(req)
	resp, err := http.Post(userServiceURL+"/validate", "application/json", bytes.NewReader(userReq))
	if err != nil {
		http.Error(w, "User service error", 502)
		return
	}
	defer resp.Body.Close()
	var userResp UserValidateResponse
	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil || !userResp.Valid {
		http.Error(w, "Invalid credentials", 401)
		return
	}
	// For demo, assign role and name statically (expand as needed)
	role := "user"
	name := "Test User"
	if userResp.UserID == "" {
		http.Error(w, "User not found", 404)
		return
	}
	now := time.Now().Unix()
	exp := time.Now().Add(time.Hour * 1).Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userResp.UserID,
		"role": role,
		"name": name,
		"iat":  now,
		"exp":  exp,
	})
	tokenStr, _ := token.SignedString(jwtSecret)
	json.NewEncoder(w).Encode(LoginResponse{Token: tokenStr})
}
