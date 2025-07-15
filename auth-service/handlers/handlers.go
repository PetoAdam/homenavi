package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
)

func setupRedisClient() *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	password := os.Getenv("REDIS_PASSWORD")
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	if pong, err := client.Ping(context.Background()).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	} else {
		log.Printf("Connected to Redis: %s", pong)
	}
	return client
}

var (
	userServiceURL = os.Getenv("USER_SERVICE_URL")
	redisClient    = setupRedisClient()
)

func random6DigitCode() string {
	return fmt.Sprintf("%06d", rand.Intn(1000000))
}

type UserPatchRequest map[string]interface{}

type User struct {
	ID               string `json:"id"`
	UserName         string `json:"user_name"`
	Email            string `json:"email"`
	TwoFactorEnabled bool   `json:"two_factor_enabled"`
	TwoFactorType    string `json:"two_factor_type"`
	TwoFactorSecret  string `json:"two_factor_secret"`
}

func logReq(r *http.Request, msg string) {
	log.Printf("[%s] %s %s", r.Method, r.URL.Path, msg)
}

// --- SIGNUP ---
func HandleSignup(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Signup request received")
	var req struct {
		UserName string `json:"user_name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid signup request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(userServiceURL+"/users", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[ERROR] User service unreachable: %v", err)
		http.Error(w, "User service error", 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("[ERROR] User service returned %d: %s", resp.StatusCode, resp.Status)
		http.Error(w, "User service error", 502)
		return
	}
	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		log.Printf("[ERROR] Failed to decode user-service response: %v", err)
		http.Error(w, "User service error", 502)
		return
	}
	log.Printf("[INFO] Signup successful for user_id=%s", user.ID)
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(user)
}

// --- LOGIN ---
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Login request received")
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid login request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	body, _ := json.Marshal(map[string]string{"email": req.Email, "password": req.Password})
	resp, err := http.Post(userServiceURL+"/users/validate", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[ERROR] User service unreachable: %v", err)
		http.Error(w, "User service error", 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("[ERROR] User service returned %d: %s", resp.StatusCode, resp.Status)
		http.Error(w, "Invalid credentials", 401)
		return
	}
	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		log.Printf("[ERROR] Failed to decode user-service response: %v", err)
		http.Error(w, "User service error", 502)
		return
	}
	if user.TwoFactorEnabled {
		switch user.TwoFactorType {
		case "totp":
			if !totp.Validate(req.Code, user.TwoFactorSecret) {
				log.Printf("[ERROR] Invalid TOTP code for user_id=%s", user.ID)
				http.Error(w, "Invalid TOTP code", 401)
				return
			}
			log.Printf("[INFO] TOTP verified for user_id=%s", user.ID)
		case "email":
			ctx := context.Background()
			val, err := redisClient.Get(ctx, "2fa_email:"+user.ID).Result()
			if err != nil || val != req.Code {
				log.Printf("[ERROR] Invalid or expired email 2FA code for user_id=%s", user.ID)
				http.Error(w, "Invalid or expired email 2FA code", 401)
				return
			}
			log.Printf("[INFO] Email 2FA verified for user_id=%s", user.ID)
		}
	}
	// TODO: Issue JWT
	log.Printf("[INFO] Login successful for user_id=%s", user.ID)
	w.WriteHeader(200)
	w.Write([]byte("Login successful (JWT TODO)"))
}

// --- 2FA SETUP ---
func Handle2FASetup(w http.ResponseWriter, r *http.Request) {
	logReq(r, "2FA setup request received")
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		log.Printf("[ERROR] Missing user_id for 2FA setup")
		http.Error(w, "Missing user_id", 400)
		return
	}
	secret, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "homenavi",
		AccountName: userID,
	})
	if err != nil {
		log.Printf("[ERROR] Failed to generate TOTP secret: %v", err)
		http.Error(w, "Failed to generate TOTP secret", 500)
		return
	}
	patch := UserPatchRequest{
		"two_factor_secret":  secret.Secret(),
		"two_factor_type":    "totp",
		"two_factor_enabled": false,
	}
	patchBody, _ := json.Marshal(patch)
	reqPatch, _ := http.NewRequest(http.MethodPatch, userServiceURL+"/users/"+userID, bytes.NewReader(patchBody))
	reqPatch.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(reqPatch)
	if err != nil || res.StatusCode >= 300 {
		log.Printf("[ERROR] Failed to update user-service for 2FA setup: %v", err)
		http.Error(w, "Failed to update user-service", 500)
		return
	}
	respData := map[string]string{
		"secret":      secret.Secret(),
		"otpauth_url": secret.URL(),
	}
	log.Printf("[INFO] 2FA TOTP setup for user_id=%s", userID)
	json.NewEncoder(w).Encode(respData)
}

// --- 2FA VERIFY ---
func Handle2FAVerify(w http.ResponseWriter, r *http.Request) {
	logReq(r, "2FA verify request received")
	var req struct {
		UserID string `json:"user_id"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid 2FA verify request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	resp, err := http.Get(fmt.Sprintf("%s/users/%s", userServiceURL, req.UserID))
	if err != nil || resp.StatusCode != 200 {
		log.Printf("[ERROR] User not found for 2FA verify: %v", err)
		http.Error(w, "User not found", 404)
		return
	}
	defer resp.Body.Close()
	var user User
	json.NewDecoder(resp.Body).Decode(&user)
	if user.TwoFactorType == "totp" {
		if !totp.Validate(req.Code, user.TwoFactorSecret) {
			log.Printf("[ERROR] Invalid TOTP code for user_id=%s", user.ID)
			http.Error(w, "Invalid TOTP code", 400)
			return
		}
		patch := UserPatchRequest{"two_factor_enabled": true}
		patchBody, _ := json.Marshal(patch)
		reqPatch, _ := http.NewRequest(http.MethodPatch, userServiceURL+"/users/"+req.UserID, bytes.NewReader(patchBody))
		reqPatch.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 5 * time.Second}
		client.Do(reqPatch)
		log.Printf("[INFO] 2FA enabled for user_id=%s", user.ID)
		w.Write([]byte("2FA enabled"))
		return
	}
	log.Printf("[WARN] 2FA verify (email) not implemented for user_id=%s", user.ID)
	w.Write([]byte("2FA verify (email) not implemented"))
}

// --- EMAIL VERIFY REQUEST ---
// TODO: Create actual email service
func HandleEmailVerifyRequest(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Email verify request received")
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid email verify request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	code := random6DigitCode()
	ctx := context.Background()
	log.Printf("[DEBUG] Setting Redis key: email_verify:%s with code=%s", req.UserID, code)
	if err := redisClient.Set(ctx, "email_verify:"+req.UserID, code, 10*time.Minute).Err(); err != nil {
		log.Printf("[ERROR] Redis SET failed: %v", err)
		http.Error(w, "Redis error", 500)
		return
	}
	log.Printf("[INFO] Mock email sent to user_id=%s with code=%s", req.UserID, code)
	w.Write([]byte("Verification code sent (mocked)"))
}

// --- EMAIL VERIFY CONFIRM ---
func HandleEmailVerifyConfirm(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Email verify confirm request received")
	var req struct {
		UserID string `json:"user_id"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid email verify confirm request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	ctx := context.Background()
	log.Printf("[DEBUG] Getting Redis key: email_verify:%s", req.UserID)
	val, err := redisClient.Get(ctx, "email_verify:"+req.UserID).Result()
	if err != nil {
		log.Printf("[ERROR] Redis GET failed: %v", err)
		http.Error(w, "Redis error", 500)
		return
	}
	if val != req.Code {
		log.Printf("[ERROR] Invalid or expired email verify code for user_id=%s (expected %s, got %s)", req.UserID, val, req.Code)
		http.Error(w, "Invalid or expired code", 400)
		return
	}
	patch := UserPatchRequest{"email_confirmed": true}
	patchBody, _ := json.Marshal(patch)
	reqPatch, _ := http.NewRequest(http.MethodPatch, userServiceURL+"/users/"+req.UserID, bytes.NewReader(patchBody))
	reqPatch.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	client.Do(reqPatch)
	log.Printf("[INFO] Email verified for user_id=%s", req.UserID)
	w.Write([]byte("Email verified"))
}

// --- PASSWORD RESET REQUEST ---
func HandlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Password reset request received")
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid password reset request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	code := random6DigitCode()
	ctx := context.Background()
	redisClient.Set(ctx, "pwreset:"+req.Email, code, 10*time.Minute)
	log.Printf("[INFO] Mock password reset email sent to email=%s with code=%s", req.Email, code)
	w.Write([]byte("Password reset code sent (mocked)"))
}

// --- PASSWORD RESET CONFIRM ---
func HandlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Password reset confirm request received")
	var req struct {
		Email   string `json:"email"`
		Code    string `json:"code"`
		NewPass string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid password reset confirm request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	ctx := context.Background()
	val, err := redisClient.Get(ctx, "pwreset:"+req.Email).Result()
	if err != nil || val != req.Code {
		log.Printf("[ERROR] Invalid or expired password reset code for email=%s", req.Email)
		http.Error(w, "Invalid or expired code", 400)
		return
	}
	// Fetch user by email to get ID
	getResp, err := http.Get(fmt.Sprintf("%s/users?email=%s", userServiceURL, req.Email))
	if err != nil || getResp.StatusCode != 200 {
		log.Printf("[ERROR] Could not fetch user by email: %v", err)
		http.Error(w, "User not found", 404)
		return
	}
	var user User
	json.NewDecoder(getResp.Body).Decode(&user)
	getResp.Body.Close()
	if user.ID == "" {
		log.Printf("[ERROR] User not found for email=%s", req.Email)
		http.Error(w, "User not found", 404)
		return
	}
	patch := UserPatchRequest{"password": req.NewPass}
	patchBody, _ := json.Marshal(patch)
	reqPatch, _ := http.NewRequest(http.MethodPatch, userServiceURL+"/users/"+user.ID, bytes.NewReader(patchBody))
	reqPatch.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	client.Do(reqPatch)
	log.Printf("[INFO] Password reset for email=%s", req.Email)
	w.Write([]byte("Password reset successful"))
}

// --- EMAIL 2FA REQUEST ---
func Handle2FAEmailRequest(w http.ResponseWriter, r *http.Request) {
	logReq(r, "2FA email request received")
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid 2FA email request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	code := random6DigitCode()
	ctx := context.Background()
	redisClient.Set(ctx, "2fa_email:"+req.UserID, code, 10*time.Minute)
	log.Printf("[INFO] Mock 2FA email sent to user_id=%s with code=%s", req.UserID, code)
	w.Write([]byte("2FA email code sent (mocked)"))
}

// --- EMAIL 2FA VERIFY ---
func Handle2FAEmailVerify(w http.ResponseWriter, r *http.Request) {
	logReq(r, "2FA email verify request received")
	var req struct {
		UserID string `json:"user_id"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid 2FA email verify request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	ctx := context.Background()
	val, err := redisClient.Get(ctx, "2fa_email:"+req.UserID).Result()
	if err != nil || val != req.Code {
		log.Printf("[ERROR] Invalid or expired 2FA email code for user_id=%s", req.UserID)
		http.Error(w, "Invalid or expired code", 400)
		return
	}
	log.Printf("[INFO] 2FA email verified for user_id=%s", req.UserID)
	w.Write([]byte("2FA email verified"))
}

// --- USER DELETE ---
func HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Delete user request received")
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid delete request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	if req.UserID == "" {
		log.Printf("[ERROR] Missing user_id for delete")
		http.Error(w, "Missing user_id", 400)
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	deleteReq, _ := http.NewRequest(http.MethodDelete, userServiceURL+"/users/"+req.UserID, nil)
	resp, err := client.Do(deleteReq)
	if err != nil {
		log.Printf("[ERROR] User service unreachable: %v", err)
		http.Error(w, "User service error", 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("[ERROR] User service returned %d: %s", resp.StatusCode, resp.Status)
		http.Error(w, "User service error", resp.StatusCode)
		return
	}
	log.Printf("[INFO] User deleted: user_id=%s", req.UserID)
	w.Write([]byte("User deleted"))
}
