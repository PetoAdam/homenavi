package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"auth-service/internal/config"
	"auth-service/internal/models/entities"
	"auth-service/pkg/errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type AuthService struct {
	config            *config.Config
	redisClient       *redis.Client
	googleOAuthConfig *oauth2.Config
}

func NewAuthService(cfg *config.Config) *AuthService {
	googleOAuthConfig := &oauth2.Config{
		RedirectURL:  cfg.GoogleOAuthRedirectURL,
		ClientID:     cfg.GoogleOAuthClientID,
		ClientSecret: cfg.GoogleOAuthClientSecret,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0,
	})

	return &AuthService{
		config:            cfg,
		redisClient:       redisClient,
		googleOAuthConfig: googleOAuthConfig,
	}
}

func (s *AuthService) IssueAccessToken(user *entities.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":  user.ID,
		"exp":  time.Now().Add(s.config.AccessTokenTTL).Unix(),
		"iat":  time.Now().Unix(),
		"role": user.Role,
		"name": user.FirstName + " " + user.LastName,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.config.JWTPrivateKey)
}

func (s *AuthService) IssueRefreshToken(userID string) (string, error) {
	// Generate a secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %v", err)
	}

	// Create a base64 URL-safe encoded token
	tokenID := base64.URLEncoding.EncodeToString(tokenBytes)
	ctx := context.Background()

	key := "refresh_token:" + tokenID
	err := s.redisClient.Set(ctx, key, userID, s.config.RefreshTokenTTL).Err()
	if err != nil {
		return "", err
	}

	return tokenID, nil
}

func (s *AuthService) ValidateRefreshToken(token string) (string, error) {
	ctx := context.Background()
	key := "refresh_token:" + token

	userID, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		return "", errors.Unauthorized("invalid or expired refresh token")
	}

	return userID, nil
}

// --- Lockout & attempt tracking ---
// Keys:
// login_fail:<email> -> integer count (TTL = lock window)
// login_lockout:<email> -> "1" (TTL = lockout duration)
// code_fail:<userID>:<type> -> integer count
// code_lockout:<userID>:<type> -> "1"

func (s *AuthService) IsLoginLocked(email string) (bool, int64, error) {
	ctx := context.Background()
	key := "login_lockout:" + email
	ttl, err := s.redisClient.TTL(ctx, key).Result()
	if err != nil && err != redis.Nil { return false, 0, err }
	if ttl > 0 { return true, int64(ttl.Seconds()), nil }
	return false, 0, nil
}

func (s *AuthService) RegisterLoginFailure(email string) (locked bool, ttlSeconds int64, err error) {
	cfg := s.config
	ctx := context.Background()
	failKey := "login_fail:" + email
	count, err := s.redisClient.Incr(ctx, failKey).Result()
	if err != nil { return false, 0, err }
	// Ensure fail counter expires within lockout window length (same as lockout so it resets eventually)
	s.redisClient.Expire(ctx, failKey, time.Duration(cfg.LoginLockoutSeconds)*time.Second)
	if int(count) >= cfg.LoginMaxFailures {
		lockKey := "login_lockout:" + email
		_ = s.redisClient.Set(ctx, lockKey, "1", time.Duration(cfg.LoginLockoutSeconds)*time.Second).Err()
		return true, int64(cfg.LoginLockoutSeconds), nil
	}
	return false, 0, nil
}

func (s *AuthService) ClearLoginFailures(email string) {
	ctx := context.Background()
	s.redisClient.Del(ctx, "login_fail:"+email)
}

func (s *AuthService) IsCodeLocked(userID, codeType string) (bool, int64, error) {
	ctx := context.Background()
	key := "code_lockout:" + userID + ":" + codeType
	ttl, err := s.redisClient.TTL(ctx, key).Result()
	if err != nil && err != redis.Nil { return false, 0, err }
	if ttl > 0 { return true, int64(ttl.Seconds()), nil }
	return false, 0, nil
}

func (s *AuthService) RegisterCodeFailure(userID, codeType string) (locked bool, ttlSeconds int64, err error) {
	cfg := s.config
	ctx := context.Background()
	failKey := "code_fail:" + userID + ":" + codeType
	count, err := s.redisClient.Incr(ctx, failKey).Result()
	if err != nil { return false, 0, err }
	s.redisClient.Expire(ctx, failKey, time.Duration(cfg.CodeLockoutSeconds)*time.Second)
	if int(count) >= cfg.CodeMaxFailures {
		lockKey := "code_lockout:" + userID + ":" + codeType
		// If already locked, do NOT reset TTL; return remaining
		if ttl, err := s.redisClient.TTL(ctx, lockKey).Result(); err == nil && ttl > 0 {
			return true, int64(ttl.Seconds()), nil
		}
		// Not currently locked -> set fresh lock
		_ = s.redisClient.Set(ctx, lockKey, "1", time.Duration(cfg.CodeLockoutSeconds)*time.Second).Err()
		return true, int64(cfg.CodeLockoutSeconds), nil
	}
	return false, 0, nil
}

func (s *AuthService) ClearCodeFailures(userID, codeType string) {
	ctx := context.Background()
	s.redisClient.Del(ctx, "code_fail:"+userID+":"+codeType)
}

func (s *AuthService) RevokeRefreshToken(token string) error {
	ctx := context.Background()
	key := "refresh_token:" + token
	return s.redisClient.Del(ctx, key).Err()
}

func (s *AuthService) StoreVerificationCode(codeType, userID, code string) error {
	ctx := context.Background()
	key := fmt.Sprintf("%s:%s", codeType, userID)

	var ttl time.Duration
	switch codeType {
	case "email_verify":
		ttl = s.config.EmailVerificationTTL
	case "password_reset":
		ttl = s.config.PasswordResetTTL
	case "2fa_email":
		ttl = s.config.TwoFactorTTL
	default:
		ttl = 10 * time.Minute
	}

	return s.redisClient.Set(ctx, key, code, ttl).Err()
}

func (s *AuthService) ValidateVerificationCode(codeType, userID, code string) error {
	ctx := context.Background()
	key := fmt.Sprintf("%s:%s", codeType, userID)

	storedCode, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		return errors.BadRequest("invalid or expired verification code")
	}

	if storedCode != code {
		return errors.BadRequest("verification code does not match")
	}

	// Delete code after successful validation
	s.redisClient.Del(ctx, key)
	return nil
}

// GenerateVerificationCode returns a 6‑digit numeric code using cryptographic randomness.
// Falls back to a time-based value only if the RNG fails (extremely unlikely).
func (s *AuthService) GenerateVerificationCode() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err == nil {
		n := binary.BigEndian.Uint32(buf) % 1000000
		return fmt.Sprintf("%06d", n)
	}
	// Fallback (non-crypto) – still returns a valid 6 digit string.
	return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
}

func (s *AuthService) GenerateOAuthState() (string, error) {
	// Generate a cryptographically secure random state
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("failed to generate OAuth state: %v", err)
	}
	
	state := base64.URLEncoding.EncodeToString(stateBytes)
	
	// Store the state in Redis with a short TTL for validation
	ctx := context.Background()
	key := "oauth_state:" + state
	err := s.redisClient.Set(ctx, key, "valid", 10*time.Minute).Err()
	if err != nil {
		return "", fmt.Errorf("failed to store OAuth state: %v", err)
	}
	
	return state, nil
}

func (s *AuthService) ValidateOAuthState(state string) error {
	ctx := context.Background()
	key := "oauth_state:" + state
	
	// Check if state exists and delete it (one-time use)
	result := s.redisClient.GetDel(ctx, key)
	if result.Err() != nil {
		return errors.BadRequest("invalid or expired OAuth state")
	}
	
	return nil
}

func (s *AuthService) IssueShortLivedToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"sub":  userID,
		"exp":  time.Now().Add(2 * time.Minute).Unix(),
		"iat":  time.Now().Unix(),
		"role": "user",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.config.JWTPrivateKey)
}

func (s *AuthService) ValidateToken(tokenString string) (*jwt.Token, error) {
	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// Use the public key derived from the private key
		return &s.config.JWTPrivateKey.PublicKey, nil
	})
}

func (s *AuthService) ExtractUserIDFromToken(tokenString string) (string, error) {
	token, err := s.ValidateToken(tokenString)
	if err != nil || !token.Valid {
		return "", errors.Unauthorized("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.Unauthorized("invalid token claims")
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		return "", errors.Unauthorized("invalid user ID in token")
	}

	return userID, nil
}

// GoogleUserInfo represents user information from Google OAuth
type GoogleUserInfo struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"given_name"`
	LastName  string `json:"family_name"`
	Picture   string `json:"picture"`
}

func (s *AuthService) ExchangeGoogleOAuthCode(code, redirectURI string) (*GoogleUserInfo, error) {
	// Exchange the authorization code for an access token
	token, err := s.googleOAuthConfig.Exchange(context.Background(), code)
	if err != nil {
		return nil, errors.BadRequest("failed to exchange OAuth code")
	}

	// Use the access token to get user info from Google
	client := s.googleOAuthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, errors.InternalServerError("failed to get user info from Google", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.InternalServerError("Google API returned error", nil)
	}

	var userInfo GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, errors.InternalServerError("failed to decode user info", err)
	}

	return &userInfo, nil
}

func (s *AuthService) GetGoogleAuthURL(state string) string {
	return s.googleOAuthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (s *AuthService) GoogleLoginURL(state string) string {
	return s.googleOAuthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (s *AuthService) HandleGoogleCallback(code string) (*GoogleUserInfo, error) {
	ctx := context.Background()

	// Exchange the authorization code for an access token
	token, err := s.googleOAuthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %v", err)
	}

	// Use the access token to get user info from Google
	client := s.googleOAuthConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info: %v", resp.Status)
	}

	var userInfo GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %v", err)
	}

	return &userInfo, nil
}
