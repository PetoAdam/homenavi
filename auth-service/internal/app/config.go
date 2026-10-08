package app

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/golang-jwt/jwt/v5"
)

// Config holds bootstrap settings for auth-service.
type Config struct {
	Port                         string
	Redis                        redisx.Config
	UserServiceURL               string
	EmailServiceURL              string
	ProfilePictureServiceURL     string
	JWTPrivateKey                *rsa.PrivateKey
	JWTVerificationPublicKeys    []*rsa.PublicKey
	JWTKeyID                     string
	JWTIssuer                    string
	JWTAPIAudience               string
	JWTUserServiceAudience       string
	AccessTokenTTL               time.Duration
	RefreshTokenTTL              time.Duration
	EmailVerificationTTL         time.Duration
	PasswordResetTTL             time.Duration
	TwoFactorTTL                 time.Duration
	GoogleOAuthClientID          string
	GoogleOAuthClientSecret      string
	GoogleOAuthRedirectURL       string
	LoginMaxFailures             int
	LoginLockoutSeconds          int
	CodeMaxFailures              int
	CodeLockoutSeconds           int
	TOTPEncryptionKey            []byte
	OAuthMCPResource             string
	MCPAuthorizationServerIssuer string
	OAuthAuthorizationUIURL      string
	OAuthTrustedClients          []authdomain.OAuthClient
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
	configuredKeyID := strings.TrimSpace(envx.String("JWT_KEY_ID", ""))
	if configuredKeyID != "" {
		derivedKeyID, err := authx.KeyIDForRSAPublicKey(&privateKey.PublicKey)
		if err != nil {
			return Config{}, fmt.Errorf("derive signing key ID: %w", err)
		}
		if configuredKeyID != derivedKeyID {
			return Config{}, fmt.Errorf("JWT_KEY_ID must equal the signing public-key fingerprint")
		}
	}
	verificationPublicKeys, err := loadRSAPublicKeys(envx.String("JWT_VERIFICATION_PUBLIC_KEY_PATHS", ""))
	if err != nil {
		return Config{}, err
	}

	redisConfig, err := redisx.LoadConfig(redisx.Config{Addrs: []string{"redis:6379"}})
	if err != nil {
		return Config{}, err
	}
	totpEncryptionKeyValue := strings.TrimSpace(envx.String("TOTP_ENCRYPTION_KEY", ""))
	if totpEncryptionKeyValue == "" {
		keyPath := envx.String("TOTP_ENCRYPTION_KEY_PATH", "")
		if keyPath != "" {
			keyData, readErr := os.ReadFile(keyPath)
			if readErr != nil {
				return Config{}, fmt.Errorf("read TOTP encryption key: %w", readErr)
			}
			totpEncryptionKeyValue = strings.TrimSpace(string(keyData))
		}
	}
	totpEncryptionKey, err := base64.StdEncoding.DecodeString(totpEncryptionKeyValue)
	if err != nil || len(totpEncryptionKey) != 32 {
		return Config{}, fmt.Errorf("TOTP_ENCRYPTION_KEY must be base64-encoded 32-byte key")
	}
	oauthTrustedClients, err := loadOAuthTrustedClients(envx.String("OAUTH_TRUSTED_CLIENTS", ""))
	if err != nil {
		return Config{}, err
	}
	oauthMCPResource := strings.TrimSpace(envx.String("MCP_RESOURCE_URI", ""))
	if len(oauthTrustedClients) > 0 {
		if _, err := authdomain.NewOAuthClientRegistry(oauthTrustedClients, oauthMCPResource); err != nil {
			return Config{}, fmt.Errorf("validate OAuth trusted clients: %w", err)
		}
	}

	return Config{
		Port:                         envx.String("AUTH_SERVICE_PORT", "8000"),
		Redis:                        redisConfig,
		UserServiceURL:               envx.String("USER_SERVICE_URL", "http://user-service:8001"),
		EmailServiceURL:              envx.String("EMAIL_SERVICE_URL", "http://email-service:8002"),
		ProfilePictureServiceURL:     envx.String("PROFILE_PICTURE_SERVICE_URL", "http://profile-picture-service:8003"),
		JWTPrivateKey:                privateKey,
		JWTVerificationPublicKeys:    verificationPublicKeys,
		JWTKeyID:                     configuredKeyID,
		JWTIssuer:                    envx.String("JWT_ISSUER", authx.DefaultIssuer),
		JWTAPIAudience:               envx.String("JWT_API_AUDIENCE", authx.AudienceAPI),
		JWTUserServiceAudience:       envx.String("JWT_USER_SERVICE_AUDIENCE", authx.AudienceUserService),
		AccessTokenTTL:               15 * time.Minute,
		RefreshTokenTTL:              7 * 24 * time.Hour,
		EmailVerificationTTL:         24 * time.Hour,
		PasswordResetTTL:             time.Hour,
		TwoFactorTTL:                 5 * time.Minute,
		GoogleOAuthClientID:          envx.String("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleOAuthClientSecret:      envx.String("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		GoogleOAuthRedirectURL:       envx.String("GOOGLE_OAUTH_REDIRECT_URL", "http://localhost/api/auth/oauth/google/callback"),
		LoginMaxFailures:             envx.Int("LOGIN_MAX_FAILURES", 5),
		LoginLockoutSeconds:          envx.Int("LOGIN_LOCKOUT_SECONDS", 900),
		CodeMaxFailures:              envx.Int("CODE_MAX_FAILURES", 5),
		CodeLockoutSeconds:           envx.Int("CODE_LOCKOUT_SECONDS", 600),
		TOTPEncryptionKey:            totpEncryptionKey,
		OAuthMCPResource:             oauthMCPResource,
		MCPAuthorizationServerIssuer: strings.TrimSpace(envx.String("MCP_AUTHORIZATION_SERVER_ISSUER", "")),
		OAuthAuthorizationUIURL:      strings.TrimSpace(envx.String("OAUTH_AUTHORIZATION_UI_URL", "http://localhost:5173/oauth/authorize")),
		OAuthTrustedClients:          oauthTrustedClients,
	}, nil
}

func loadOAuthTrustedClients(value string) ([]authdomain.OAuthClient, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var clients []authdomain.OAuthClient
	if err := json.Unmarshal([]byte(value), &clients); err != nil {
		return nil, fmt.Errorf("parse OAUTH_TRUSTED_CLIENTS: %w", err)
	}
	return clients, nil
}

func loadRSAPublicKeys(paths string) ([]*rsa.PublicKey, error) {
	var publicKeys []*rsa.PublicKey
	for _, path := range strings.Split(paths, ",") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		keyData, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read verification public key %q: %w", path, err)
		}
		publicKey, err := jwt.ParseRSAPublicKeyFromPEM(keyData)
		if err != nil {
			return nil, fmt.Errorf("parse verification public key %q: %w", path, err)
		}
		publicKeys = append(publicKeys, publicKey)
	}
	return publicKeys, nil
}
