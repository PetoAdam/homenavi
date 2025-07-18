package handlers

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

func init() {
	keyPath := os.Getenv("JWT_PRIVATE_KEY_PATH")
	if keyPath == "" {
		log.Fatal("JWT_PRIVATE_KEY_PATH not set")
	}
	keyData, err := ioutil.ReadFile(keyPath)
	if err != nil {
		log.Fatalf("Failed to read JWT private key: %v", err)
	}
	jwtPrivateKey, err = jwt.ParseRSAPrivateKeyFromPEM(keyData)
	if err != nil {
		log.Fatalf("Failed to parse JWT private key: %v", err)
	}
}

var jwtPrivateKey *rsa.PrivateKey

type User struct {
	ID                string `json:"id"`
	UserName          string `json:"user_name"`
	Email             string `json:"email"`
	FirstName         string `json:"first_name"`
	LastName          string `json:"last_name"`
	Role              string `json:"role"`
	EmailConfirmed    bool   `json:"email_confirmed"`
	TwoFactorEnabled  bool   `json:"two_factor_enabled"`
	TwoFactorType     string `json:"two_factor_type"`
	ProfilePictureURL string `json:"profile_picture_url"`
	TwoFactorSecret   string `json:"-"` // never serialized
}

func logReq(r *http.Request, msg string) {
	log.Printf("[%s] %s %s", r.Method, r.URL.Path, msg)
}

// --- JWT/REFRESH TOKEN UTILS ---
func issueJWT(user User) (string, error) {
	claims := jwt.MapClaims{
		"sub":  user.ID,
		"exp":  time.Now().Add(15 * time.Minute).Unix(),
		"role": user.Role,
		"name": user.FirstName + " " + user.LastName,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(jwtPrivateKey)
}

func issueRefreshToken(userID string) (string, error) {
	token := fmt.Sprintf("rt_%s_%d_%d", userID, rand.Int63(), time.Now().UnixNano())
	ctx := context.Background()
	key := "refresh_token:" + token
	if err := redisClient.Set(ctx, key, userID, 7*24*time.Hour).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func validateAndRotateRefreshToken(token string) (string, error) {
	ctx := context.Background()
	key := "refresh_token:" + token
	userID, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("invalid or expired refresh token")
	}
	// Rotate: delete old, issue new
	redisClient.Del(ctx, key)
	return userID, nil
}

// --- SIGNUP ---
func isValidEmail(email string) bool {
	// Simple regex for email validation
	re := regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)
	return re.MatchString(email)
}

func isValidPassword(password string) bool {
	if len(password) < 8 {
		return false
	}
	var hasLower, hasUpper bool
	for _, c := range password {
		if 'a' <= c && c <= 'z' {
			hasLower = true
		}
		if 'A' <= c && c <= 'Z' {
			hasUpper = true
		}
	}
	return hasLower && hasUpper
}

func HandleSignup(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Signup request received")
	var req struct {
		UserName  string `json:"user_name"`
		Email     string `json:"email"`
		Password  string `json:"password"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid signup request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	if !isValidEmail(req.Email) {
		log.Printf("[ERROR] Invalid email format: %s", req.Email)
		http.Error(w, "Invalid email format", 400)
		return
	}
	if !isValidPassword(req.Password) {
		log.Printf("[ERROR] Invalid password format")
		http.Error(w, "Password must be at least 8 characters and contain both uppercase and lowercase letters", 400)
		return
	}
	userReq := map[string]interface{}{
		"user_name":  req.UserName,
		"email":      req.Email,
		"password":   req.Password,
		"first_name": req.FirstName,
		"last_name":  req.LastName,
	}
	user, status, msg := internalUserCreate(userReq, extractJWTFromRequest(r))
	if status != 201 {
		log.Printf("[ERROR] User service error: %s", msg)
		http.Error(w, msg, status)
		return
	}
	log.Printf("[INFO] Signup successful for user_id=%s", user.ID)
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(user)
}

// --- LOGIN STEP 1: Start ---
func HandleLoginStart(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Login start request received")
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid login start request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	body, _ := json.Marshal(map[string]string{"email": req.Email, "password": req.Password})
	resp, err := internalUserRequest("POST", "/users/validate", body, extractJWTFromRequest(r))
	if err != nil || resp.StatusCode != 200 {
		log.Printf("[ERROR] User service returned %d: %v", resp.StatusCode, err)
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"2fa_required": true,
			"user_id":      user.ID,
			"2fa_type":     user.TwoFactorType,
		})
		return
	}
	// No 2FA, issue tokens directly
	accessToken, err := issueJWT(user)
	if err != nil {
		log.Printf("[ERROR] Failed to issue JWT: %v", err)
		http.Error(w, "Failed to issue token", 500)
		return
	}
	refreshToken, err := issueRefreshToken(user.ID)
	if err != nil {
		log.Printf("[ERROR] Failed to issue refresh token: %v", err)
		http.Error(w, "Failed to issue refresh token", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// --- LOGIN STEP 2: Finish (2FA) ---
func HandleLoginFinish(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Login finish (2FA) request received")
	var req struct {
		UserID string `json:"user_id"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	jwtForGet, err := issueShortLivedJWT(req.UserID)
	if err != nil {
		log.Printf("[ERROR] Failed to issue short-lived JWT for login finish: %v", err)
		http.Error(w, "Failed to authorize login finish", 500)
		return
	}
	user, err := internalUserGet(req.UserID, jwtForGet)
	if err != nil {
		http.Error(w, "User not found", 404)
		return
	}
	if !user.TwoFactorEnabled {
		http.Error(w, "2FA not enabled for user", 400)
		return
	}
	switch user.TwoFactorType {
	case "totp":
		if !totp.Validate(req.Code, user.TwoFactorSecret) {
			http.Error(w, "Invalid TOTP code", 401)
			return
		}
	case "email":
		ctx := context.Background()
		val, err := redisClient.Get(ctx, "2fa_email:"+user.ID).Result()
		if err != nil || val != req.Code {
			http.Error(w, "Invalid or expired email 2FA code", 401)
			return
		}
	default:
		http.Error(w, "Unsupported 2FA type", 400)
		return
	}
	accessToken, err := issueJWT(*user)
	if err != nil {
		http.Error(w, "Failed to issue token", 500)
		return
	}
	refreshToken, err := issueRefreshToken(user.ID)
	if err != nil {
		http.Error(w, "Failed to issue refresh token", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
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
	user, err := internalUserGet(req.UserID, "")
	if err != nil {
		log.Printf("[ERROR] User not found for 2FA verify: %v", err)
		http.Error(w, "User not found", 404)
		return
	}
	if user.TwoFactorType == "totp" {
		if !totp.Validate(req.Code, user.TwoFactorSecret) {
			log.Printf("[ERROR] Invalid TOTP code for user_id=%s", user.ID)
			http.Error(w, "Invalid TOTP code", 400)
			return
		}
		patch := UserPatchRequest{"two_factor_enabled": true}
		if err := internalUserPatch(req.UserID, patch, ""); err != nil {
			log.Printf("[ERROR] Failed to patch user for 2FA enable: %v", err)
			http.Error(w, "Failed to update user-service", 500)
			return
		}
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

	// Extract JWT from frontend request and forward it to user-service
	jwt := extractJWT(r)
	patch := UserPatchRequest{"email_confirmed": true}
	if err := internalUserPatch(req.UserID, patch, jwt); err != nil {
		log.Printf("[ERROR] Failed to patch user for email confirmation: %v", err)
		http.Error(w, "Failed to update user-service", 500)
		return
	}

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
	// Fetch user by email to get ID (internal auth)
	resp, err := internalUserRequest("GET", "/users?email="+req.Email, nil, extractJWTFromRequest(r))
	if err != nil || resp.StatusCode != 200 {
		log.Printf("[ERROR] Could not fetch user by email: %v", err)
		http.Error(w, "User not found", 404)
		return
	}
	var user User
	json.NewDecoder(resp.Body).Decode(&user)
	resp.Body.Close()
	if user.ID == "" {
		log.Printf("[ERROR] User not found for email=%s", req.Email)
		http.Error(w, "User not found", 404)
		return
	}
	patch := UserPatchRequest{"password": req.NewPass}
	jwtForPatch, err := issueShortLivedJWT(user.ID)
	if err != nil {
		log.Printf("[ERROR] Failed to issue short-lived JWT for password reset: %v", err)
		http.Error(w, "Failed to authorize password reset", 500)
		return
	}
	if err := internalUserPatch(user.ID, patch, jwtForPatch); err != nil {
		log.Printf("[ERROR] Failed to patch user password: %v", err)
		http.Error(w, "Failed to update password", 500)
		return
	}
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
	// Set 2FA enabled and type in user-service using JWT from frontend
	jwt := extractJWT(r)
	patch := UserPatchRequest{"two_factor_enabled": true, "two_factor_type": "email"}
	if err := internalUserPatch(req.UserID, patch, jwt); err != nil {
		log.Printf("[ERROR] Failed to patch user for email 2FA enable: %v", err)
		http.Error(w, "Failed to update user-service", 500)
		return
	}
	log.Printf("[INFO] 2FA email verified and enabled for user_id=%s", req.UserID)
	w.Write([]byte("2FA email verified and enabled"))
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
	jwt := extractJWTFromRequest(r)
	if err := internalUserDelete(req.UserID, jwt); err != nil {
		log.Printf("[ERROR] Failed to delete user: %v", err)
		http.Error(w, err.Error(), 502)
		return
	}
	log.Printf("[INFO] User deleted: user_id=%s", req.UserID)
	w.Write([]byte("User deleted"))
}

// --- REFRESH TOKEN ---
func HandleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	userID, err := validateAndRotateRefreshToken(req.RefreshToken)
	if err != nil {
		http.Error(w, "Invalid or expired refresh token", http.StatusUnauthorized)
		return
	}
	jwtForGet, err := issueShortLivedJWT(userID)
	if err != nil {
		log.Printf("[ERROR] Failed to issue short-lived JWT for refresh: %v", err)
		http.Error(w, "Failed to authorize refresh", http.StatusInternalServerError)
		return
	}
	user, err := internalUserGet(userID, jwtForGet)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	accessToken, err := issueJWT(*user)
	if err != nil {
		http.Error(w, "Failed to issue token", http.StatusInternalServerError)
		return
	}
	refreshToken, err := issueRefreshToken(user.ID)
	if err != nil {
		http.Error(w, "Failed to issue refresh token", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// --- LOGOUT ---
func HandleLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	ctx := context.Background()
	key := "refresh_token:" + req.RefreshToken
	redisClient.Del(ctx, key)
	w.WriteHeader(200)
	w.Write([]byte("Logged out"))
}

// --- OAUTH2 GOOGLE ---
func HandleOAuthGoogle(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement Google OAuth2 login (exchange code, get user info, upsert user, issue tokens)
	w.Write([]byte("OAuth2 Google endpoint (not yet implemented)"))
}

// --- ME (CURRENT USER PROFILE) ---
func HandleMe(w http.ResponseWriter, r *http.Request) {
	logReq(r, "HandleMe")

	// Extract JWT token from request
	token := extractJWT(r)
	if token == "" {
		http.Error(w, "Missing or invalid JWT token", http.StatusUnauthorized)
		return
	}

	// Parse and validate JWT to get user ID
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return &jwtPrivateKey.PublicKey, nil
	})

	if err != nil || !parsedToken.Valid {
		http.Error(w, "Invalid JWT token", http.StatusUnauthorized)
		return
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		http.Error(w, "Invalid JWT claims", http.StatusUnauthorized)
		return
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		http.Error(w, "Invalid user ID in token", http.StatusUnauthorized)
		return
	}

	// Fetch user from user-service
	user, err := getUserWithJWT(userID, token)
	if err != nil {
		log.Printf("[ERROR] Failed to fetch user %s: %v", userID, err)
		http.Error(w, "Failed to fetch user data", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// --- PROFILE PICTURE ENDPOINTS ---
func HandleGenerateAvatar(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Generate avatar request received")

	// Extract JWT to get user ID
	token := extractJWT(r)
	if token == "" {
		http.Error(w, "Missing or invalid JWT token", http.StatusUnauthorized)
		return
	}

	// Parse JWT to get user ID
	userID, err := getUserIDFromToken(token)
	if err != nil {
		http.Error(w, "Invalid JWT token", http.StatusUnauthorized)
		return
	}

	// Call profile picture service to generate avatar
	resp, err := http.Get(fmt.Sprintf("%s/generate/%s", os.Getenv("PROFILE_PICTURE_SERVICE_URL"), userID))
	if err != nil {
		log.Printf("[ERROR] Failed to call profile picture service: %v", err)
		http.Error(w, "Failed to generate avatar", 500)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[ERROR] Profile picture service returned status %d", resp.StatusCode)
		http.Error(w, "Avatar generation failed", 500)
		return
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[ERROR] Failed to parse avatar generation response: %v", err)
		http.Error(w, "Failed to process avatar", 500)
		return
	}

	// Update user profile picture URL in user-service
	avatarURL := result["url"].(string)
	patch := UserPatchRequest{"profile_picture_url": avatarURL}
	if err := internalUserPatch(userID, patch, token); err != nil {
		log.Printf("[ERROR] Failed to update user profile picture URL: %v", err)
		http.Error(w, "Failed to save avatar URL", 500)
		return
	}

	log.Printf("[INFO] Avatar generated for user_id=%s", userID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func HandleUploadProfilePicture(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Upload profile picture request received")

	// Extract JWT to get user ID
	token := extractJWT(r)
	if token == "" {
		http.Error(w, "Missing or invalid JWT token", http.StatusUnauthorized)
		return
	}

	// Parse JWT to get user ID and role
	userID, err := getUserIDFromToken(token)
	if err != nil {
		http.Error(w, "Invalid JWT token", http.StatusUnauthorized)
		return
	}

	// Get target user ID from URL path
	targetUserID := r.URL.Query().Get("user_id")
	if targetUserID == "" {
		targetUserID = userID // Default to current user
	}

	// Check authorization (user can only upload their own picture, unless admin)
	if targetUserID != userID {
		user, err := internalUserGet(userID, token)
		if err != nil || user.Role != "admin" {
			http.Error(w, "Unauthorized: can only change your own profile picture", http.StatusForbidden)
			return
		}
	}

	// Parse multipart form
	err = r.ParseMultipartForm(10 << 20) // 10MB max
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create multipart form for profile picture service
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	// Add user_id field
	userIDField, _ := writer.CreateFormField("user_id")
	userIDField.Write([]byte(targetUserID))

	// Add file field
	fileField, _ := writer.CreateFormFile("file", header.Filename)
	io.Copy(fileField, file)
	writer.Close()

	// Send to profile picture service
	req, err := http.NewRequest("POST", os.Getenv("PROFILE_PICTURE_SERVICE_URL")+"/upload", &b)
	if err != nil {
		http.Error(w, "Failed to create request", 500)
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ERROR] Failed to call profile picture service: %v", err)
		http.Error(w, "Failed to upload image", 500)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ERROR] Profile picture service returned status %d: %s", resp.StatusCode, string(body))
		http.Error(w, "Image upload failed", 500)
		return
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[ERROR] Failed to parse upload response: %v", err)
		http.Error(w, "Failed to process upload", 500)
		return
	}

	// Update user profile picture URL in user-service
	avatarURL := result["primary_url"].(string)
	patch := UserPatchRequest{"profile_picture_url": avatarURL}
	if err := internalUserPatch(targetUserID, patch, token); err != nil {
		log.Printf("[ERROR] Failed to update user profile picture URL: %v", err)
		http.Error(w, "Failed to save image URL", 500)
		return
	}

	log.Printf("[INFO] Profile picture uploaded for user_id=%s", targetUserID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func getUserIDFromToken(token string) (string, error) {
	parsedToken, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return &jwtPrivateKey.PublicKey, nil
	})

	if err != nil || !parsedToken.Valid {
		return "", fmt.Errorf("invalid token")
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims")
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		return "", fmt.Errorf("invalid user ID")
	}

	return userID, nil
}

// Helper for internal user-service requests with internal auth header and optional JWT
func internalUserRequest(method, path string, body []byte, jwtToken string) (*http.Response, error) {
	url := userServiceURL + path
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+jwtToken)
	}
	if method == "PATCH" || method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	return client.Do(req)
}

// Helper to extract JWT from incoming request
func extractJWT(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	// Check for "Bearer " prefix
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}

	return ""
}

// Alias for compatibility with existing code
func extractJWTFromRequest(r *http.Request) string {
	return extractJWT(r)
}

// Helper for GET user (with JWT)
func getUserWithJWT(userID, jwt string) (*User, error) {
	url := fmt.Sprintf("%s/users/%s", userServiceURL, userID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add JWT token for authorization
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("user service returned status %d", resp.StatusCode)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

// Helper for PATCH user (with JWT)
func internalUserPatch(userID string, patch UserPatchRequest, jwtToken string) error {
	patchBody, _ := json.Marshal(patch)
	resp, err := internalUserRequest("PATCH", "/users/"+userID, patchBody, jwtToken)
	if err != nil || resp.StatusCode >= 300 {
		return fmt.Errorf("patch failed: %v", err)
	}
	return nil
}

// Helper for POST user (signup)
func internalUserCreate(userReq map[string]interface{}, jwtToken string) (*User, int, string) {
	body, _ := json.Marshal(userReq)
	resp, err := internalUserRequest("POST", "/users", body, jwtToken)
	if err != nil {
		return nil, 502, "User service error"
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, string(msg)
	}
	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, 502, "Failed to decode user-service response"
	}
	return &user, 201, ""
}

// Helper for GET user (with JWT)
func internalUserGet(userID string, jwtToken string) (*User, error) {
	resp, err := internalUserRequest("GET", "/users/"+userID, nil, jwtToken)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("user not found or error: %v", err)
	}
	defer resp.Body.Close()
	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func internalUserDelete(userID string, jwtToken string) error {
	resp, err := internalUserRequest("DELETE", "/users/"+userID, nil, jwtToken)
	if err != nil {
		return fmt.Errorf("delete failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed: %s", string(msg))
	}
	return nil
}

func issueShortLivedJWT(userID string) (string, error) {
	claims := jwt.MapClaims{
		"sub":  userID,
		"role": "user",
		"exp":  time.Now().Add(2 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(jwtPrivateKey)
}

// --- CHANGE PASSWORD ---
func HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	logReq(r, "Change password request received")
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid change password request: %v", err)
		http.Error(w, "Invalid request", 400)
		return
	}
	// Extract JWT to get user ID
	token := extractJWT(r)
	if token == "" {
		http.Error(w, "Missing or invalid JWT token", http.StatusUnauthorized)
		return
	}

	// Parse JWT to get user ID
	parsedToken, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return &jwtPrivateKey.PublicKey, nil
	})

	if err != nil || !parsedToken.Valid {
		http.Error(w, "Invalid JWT token", http.StatusUnauthorized)
		return
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		http.Error(w, "Invalid JWT claims", http.StatusUnauthorized)
		return
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		http.Error(w, "Invalid user ID in token", http.StatusUnauthorized)
		return
	}

	// Get user details to validate current password
	user, err := internalUserGet(userID, token)
	if err != nil {
		log.Printf("[ERROR] Failed to get user for password change: %v", err)
		http.Error(w, "User not found", 404)
		return
	}

	// Validate current password first
	body, _ := json.Marshal(map[string]string{"email": user.Email, "password": req.CurrentPassword})
	resp, err := internalUserRequest("POST", "/users/validate", body, "")
	if err != nil || resp.StatusCode != 200 {
		log.Printf("[ERROR] Current password validation failed")
		http.Error(w, "Current password is incorrect", 400)
		return
	}

	// Update password in user-service
	patch := UserPatchRequest{"password": req.NewPassword}
	if err := internalUserPatch(userID, patch, token); err != nil {
		log.Printf("[ERROR] Failed to patch user password: %v", err)
		http.Error(w, "Failed to update password", 500)
		return
	}

	log.Printf("[INFO] Password changed for user_id=%s", userID)
	w.Write([]byte("Password changed successfully"))
}
