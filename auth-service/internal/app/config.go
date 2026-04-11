package app

import (
	"crypto/rsa"
	"os"
	"time"

	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/golang-jwt/jwt/v5"
)

// Config holds bootstrap settings for auth-service.
type Config struct {
	Port                     string
	RedisAddr                string
	RedisPassword            string
	UserServiceURL           string
	EmailServiceURL          string
	ProfilePictureServiceURL string
	JWTPrivateKey            *rsa.PrivateKey
	AccessTokenTTL           time.Duration
	RefreshTokenTTL          time.Duration
	EmailVerificationTTL     time.Duration
	PasswordResetTTL         time.Duration
	TwoFactorTTL             time.Duration
	GoogleOAuthClientID      string
	GoogleOAuthClientSecret  string
	GoogleOAuthRedirectURL   string
	LoginMaxFailures         int
	LoginLockoutSeconds      int
	CodeMaxFailures          int
	CodeLockoutSeconds       int
}

func LoadConfig() (Config, error) {
	privateKeyPath := envx.String("JWT_PRIVATE_KEY_PATH", "./keys/jwt_private.pem")
	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return Config{}, err
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyData)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:                     envx.String("AUTH_SERVICE_PORT", "8000"),
		RedisAddr:                envx.String("REDIS_ADDR", "redis:6379"),
		RedisPassword:            envx.String("REDIS_PASSWORD", ""),
		UserServiceURL:           envx.String("USER_SERVICE_URL", "http://user-service:8001"),
		EmailServiceURL:          envx.String("EMAIL_SERVICE_URL", "http://email-service:8002"),
		ProfilePictureServiceURL: envx.String("PROFILE_PICTURE_SERVICE_URL", "http://profile-picture-service:8003"),
		JWTPrivateKey:            privateKey,
		AccessTokenTTL:           15 * time.Minute,
		RefreshTokenTTL:          7 * 24 * time.Hour,
		EmailVerificationTTL:     24 * time.Hour,
		PasswordResetTTL:         time.Hour,
		TwoFactorTTL:             5 * time.Minute,
		GoogleOAuthClientID:      envx.String("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleOAuthClientSecret:  envx.String("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		GoogleOAuthRedirectURL:   envx.String("GOOGLE_OAUTH_REDIRECT_URL", "http://localhost/api/auth/oauth/google/callback"),
		LoginMaxFailures:         envx.Int("LOGIN_MAX_FAILURES", 5),
		LoginLockoutSeconds:      envx.Int("LOGIN_LOCKOUT_SECONDS", 900),
		CodeMaxFailures:          envx.Int("CODE_MAX_FAILURES", 5),
		CodeLockoutSeconds:       envx.Int("CODE_LOCKOUT_SECONDS", 600),
	}, nil
}
