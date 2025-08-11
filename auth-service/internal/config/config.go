package config

import (
	"crypto/rsa"
	"os"
	"time"
	"strconv"

	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	Port                     string `json:"port"`
	RedisAddr                string `json:"redis_addr"`
	RedisPassword            string `json:"redis_password"`
	UserServiceURL           string `json:"user_service_url"`
	EmailServiceURL          string `json:"email_service_url"`
	ProfilePictureServiceURL string `json:"profile_picture_service_url"`
	JWTPrivateKey            *rsa.PrivateKey
	AccessTokenTTL           time.Duration `json:"access_token_ttl"`
	RefreshTokenTTL          time.Duration `json:"refresh_token_ttl"`
	EmailVerificationTTL     time.Duration `json:"email_verification_ttl"`
	PasswordResetTTL         time.Duration `json:"password_reset_ttl"`
	TwoFactorTTL             time.Duration `json:"two_factor_ttl"`
	GoogleOAuthClientID      string        `json:"google_oauth_client_id"`
	GoogleOAuthClientSecret  string        `json:"google_oauth_client_secret"`
	GoogleOAuthRedirectURL   string        `json:"google_oauth_redirect_url"`
	LoginMaxFailures         int           `json:"login_max_failures"`
	LoginLockoutSeconds      int           `json:"login_lockout_seconds"`
	CodeMaxFailures          int           `json:"code_max_failures"`
	CodeLockoutSeconds       int           `json:"code_lockout_seconds"`
}

func Load() (*Config, error) {
	// Load JWT private key
	privateKeyPath := getEnv("JWT_PRIVATE_KEY_PATH", "./keys/jwt_private.pem")

	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyData)
	if err != nil {
		return nil, err
	}

	config := &Config{
		Port:                     getEnv("AUTH_SERVICE_PORT", "8000"),
		RedisAddr:                getEnv("REDIS_ADDR", "redis:6379"),
		RedisPassword:            getEnv("REDIS_PASSWORD", ""),
		UserServiceURL:           getEnv("USER_SERVICE_URL", "http://user-service:8001"),
		EmailServiceURL:          getEnv("EMAIL_SERVICE_URL", "http://email-service:8002"),
		ProfilePictureServiceURL: getEnv("PROFILE_PICTURE_SERVICE_URL", "http://profile-picture-service:8003"),
		JWTPrivateKey:            privateKey,
		AccessTokenTTL:           15 * time.Minute,
		RefreshTokenTTL:          7 * 24 * time.Hour,
		EmailVerificationTTL:     24 * time.Hour,
		PasswordResetTTL:         1 * time.Hour,
		TwoFactorTTL:             5 * time.Minute,
		GoogleOAuthClientID:      getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleOAuthClientSecret:  getEnv("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		GoogleOAuthRedirectURL:   getEnv("GOOGLE_OAUTH_REDIRECT_URL", "http://localhost/api/auth/oauth/google/callback"),
		LoginMaxFailures:         intFromEnv("LOGIN_MAX_FAILURES", 5),
		LoginLockoutSeconds:      intFromEnv("LOGIN_LOCKOUT_SECONDS", 900),
		CodeMaxFailures:          intFromEnv("CODE_MAX_FAILURES", 5),
		CodeLockoutSeconds:       intFromEnv("CODE_LOCKOUT_SECONDS", 600),
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func intFromEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			return iv
		}
	}
	return def
}
