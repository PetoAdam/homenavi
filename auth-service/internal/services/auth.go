package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	mathrand "math/rand"
	"time"

	"auth-service/internal/config"
	"auth-service/internal/models/entities"
	"auth-service/pkg/errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

type AuthService struct {
	config      *config.Config
	redisClient *redis.Client
}

func NewAuthService(cfg *config.Config) *AuthService {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0,
	})

	return &AuthService{
		config:      cfg,
		redisClient: rdb,
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

func (s *AuthService) GenerateVerificationCode() string {
	return fmt.Sprintf("%06d", mathrand.Intn(1000000))
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
	// This is a placeholder implementation
	// In a real implementation, you would:
	// 1. Exchange the code for an access token with Google
	// 2. Use the access token to get user info from Google
	// 3. Return the user info

	// For now, return an error indicating this needs to be implemented
	return nil, errors.BadRequest("Google OAuth integration not configured")
}

func (s *AuthService) GenerateTokens(userID string) (string, string, error) {
	// This would typically require the user entity to generate proper claims
	// For now, generate basic tokens
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(s.config.AccessTokenTTL).Unix(),
		"iat": time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	accessToken, err := token.SignedString(s.config.JWTPrivateKey)
	if err != nil {
		return "", "", err
	}

	refreshToken, err := s.IssueRefreshToken(userID)
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}
