package config

import (
	"crypto/rsa"
	"io/ioutil"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	Port                    string
	JWTPrivateKey          *rsa.PrivateKey
	UserServiceURL         string
	EmailServiceURL        string
	ProfilePictureServiceURL string
	RedisAddr              string
	RedisPassword          string
	AccessTokenTTL         time.Duration
	RefreshTokenTTL        time.Duration
	EmailVerificationTTL   time.Duration
	PasswordResetTTL       time.Duration
	TwoFactorTTL          time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                    getEnv("PORT", "8000"),
		UserServiceURL:         getEnv("USER_SERVICE_URL", "http://user-service:8001"),
		EmailServiceURL:        getEnv("EMAIL_SERVICE_URL", "http://email-service:8002"),
		ProfilePictureServiceURL: getEnv("PROFILE_PICTURE_SERVICE_URL", "http://profile-picture-service:8003"),
		RedisAddr:              getEnv("REDIS_ADDR", "redis:6379"),
		RedisPassword:          getEnv("REDIS_PASSWORD", ""),
		AccessTokenTTL:         15 * time.Minute,
		RefreshTokenTTL:        7 * 24 * time.Hour,
		EmailVerificationTTL:   10 * time.Minute,
		PasswordResetTTL:       10 * time.Minute,
		TwoFactorTTL:          10 * time.Minute,
	}

	// Load JWT private key (only needed for signing tokens)
	privateKeyPath := getEnv("JWT_PRIVATE_KEY_PATH", "./keys/jwt_private.pem")

	privateKeyData, err := ioutil.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}

	cfg.JWTPrivateKey, err = jwt.ParseRSAPrivateKeyFromPEM(privateKeyData)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
