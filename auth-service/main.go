package main

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	userServiceURL    = os.Getenv("USER_SERVICE_URL")
	jwtPrivateKeyPath = os.Getenv("JWT_PRIVATE_KEY_PATH")
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
	privateKey, err := loadPrivateKey(jwtPrivateKeyPath)
	if err != nil {
		log.Fatalf("Failed to load private key: %v", err)
	}
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		handleLogin(w, r, privateKey)
	})
	http.ListenAndServe(":8000", nil)
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	keyData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return jwt.ParseRSAPrivateKeyFromPEM(keyData)
}

func handleLogin(w http.ResponseWriter, r *http.Request, privateKey *rsa.PrivateKey) {
	log.Println("Received login request")
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
	claims := jwt.MapClaims{
		"sub":  userResp.UserID,
		"role": role,
		"name": name,
		"iat":  now,
		"exp":  exp,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenStr, err := token.SignedString(privateKey)
	if err != nil {
		http.Error(w, "Token signing error", 500)
		return
	}
	json.NewEncoder(w).Encode(LoginResponse{Token: tokenStr})
}
