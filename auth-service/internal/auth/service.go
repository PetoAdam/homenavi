package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp/totp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Config holds the auth business configuration.
type Config struct {
	JWTPrivateKey           *rsa.PrivateKey
	AccessTokenTTL          time.Duration
	RefreshTokenTTL         time.Duration
	EmailVerificationTTL    time.Duration
	PasswordResetTTL        time.Duration
	TwoFactorTTL            time.Duration
	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	GoogleOAuthRedirectURL  string
	LoginMaxFailures        int
	LoginLockoutSeconds     int
	CodeMaxFailures         int
	CodeLockoutSeconds      int
}

// Service implements authentication use cases and token/code lifecycle behavior.
type Service struct {
	config            Config
	cacheStore        cacheinfra.Store
	googleOAuthConfig *oauth2.Config
}

type CredentialUserProvider interface {
	ValidateCredentials(email, password string) (*clientsinfra.User, error)
	GetUser(userID string) (*clientsinfra.User, error)
}

type UserProvider interface {
	GetUser(userID string) (*clientsinfra.User, error)
}

type TwoFactorCodeSender interface {
	Send2FACode(email, firstName, code string) error
}

type LoginStartResult struct {
	TwoFARequired bool
	UserID        string
	TwoFAType     string
	AccessToken   string
	RefreshToken  string
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

func NewService(cfg Config, cacheStore cacheinfra.Store) *Service {
	googleOAuthConfig := &oauth2.Config{
		RedirectURL:  cfg.GoogleOAuthRedirectURL,
		ClientID:     cfg.GoogleOAuthClientID,
		ClientSecret: cfg.GoogleOAuthClientSecret,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}

	return &Service{config: cfg, cacheStore: cacheStore, googleOAuthConfig: googleOAuthConfig}
}

func (s *Service) Close() error {
	if s.cacheStore == nil {
		return nil
	}
	return s.cacheStore.Close()
}

func (s *Service) IssueAccessToken(user *clientsinfra.User) (string, error) {
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

func (s *Service) IssueRefreshToken(userID string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %v", err)
	}

	tokenID := base64.URLEncoding.EncodeToString(tokenBytes)
	ctx := context.Background()
	if err := s.cacheStore.Set(ctx, "refresh_token:"+tokenID, userID, s.config.RefreshTokenTTL); err != nil {
		return "", err
	}
	return tokenID, nil
}

func (s *Service) ValidateRefreshToken(token string) (string, error) {
	ctx := context.Background()
	userID, err := s.cacheStore.Get(ctx, "refresh_token:"+token)
	if err != nil {
		return "", errors.Unauthorized("invalid or expired refresh token")
	}
	return userID, nil
}

func (s *Service) IsLoginLocked(email string) (bool, int64, error) {
	ctx := context.Background()
	ttl, err := s.cacheStore.TTL(ctx, "login_lockout:"+email)
	if err != nil {
		return false, 0, err
	}
	if ttl > 0 {
		return true, int64(ttl.Seconds()), nil
	}
	return false, 0, nil
}

func (s *Service) RegisterLoginFailure(email string) (bool, int64, error) {
	ctx := context.Background()
	failKey := "login_fail:" + email
	count, err := s.cacheStore.Increment(ctx, failKey)
	if err != nil {
		return false, 0, err
	}
	_ = s.cacheStore.Expire(ctx, failKey, time.Duration(s.config.LoginLockoutSeconds)*time.Second)
	if int(count) >= s.config.LoginMaxFailures {
		_ = s.cacheStore.Set(ctx, "login_lockout:"+email, "1", time.Duration(s.config.LoginLockoutSeconds)*time.Second)
		return true, int64(s.config.LoginLockoutSeconds), nil
	}
	return false, 0, nil
}

func (s *Service) ClearLoginFailures(email string) {
	ctx := context.Background()
	_ = s.cacheStore.Delete(ctx, "login_fail:"+email)
}

func (s *Service) IsCodeLocked(userID, codeType string) (bool, int64, error) {
	ctx := context.Background()
	ttl, err := s.cacheStore.TTL(ctx, "code_lockout:"+userID+":"+codeType)
	if err != nil {
		return false, 0, err
	}
	if ttl > 0 {
		return true, int64(ttl.Seconds()), nil
	}
	return false, 0, nil
}

func (s *Service) RegisterCodeFailure(userID, codeType string) (bool, int64, error) {
	ctx := context.Background()
	failKey := "code_fail:" + userID + ":" + codeType
	count, err := s.cacheStore.Increment(ctx, failKey)
	if err != nil {
		return false, 0, err
	}
	_ = s.cacheStore.Expire(ctx, failKey, time.Duration(s.config.CodeLockoutSeconds)*time.Second)
	lockKey := "code_lockout:" + userID + ":" + codeType
	if int(count) >= s.config.CodeMaxFailures {
		if ttl, err := s.cacheStore.TTL(ctx, lockKey); err == nil && ttl > 0 {
			return true, int64(ttl.Seconds()), nil
		}
		_ = s.cacheStore.Set(ctx, lockKey, "1", time.Duration(s.config.CodeLockoutSeconds)*time.Second)
		return true, int64(s.config.CodeLockoutSeconds), nil
	}
	return false, 0, nil
}

func (s *Service) ClearCodeFailures(userID, codeType string) {
	ctx := context.Background()
	_ = s.cacheStore.Delete(ctx, "code_fail:"+userID+":"+codeType)
}

func (s *Service) RevokeRefreshToken(token string) error {
	ctx := context.Background()
	return s.cacheStore.Delete(ctx, "refresh_token:"+token)
}

func (s *Service) StoreVerificationCode(codeType, userID, code string) error {
	ctx := context.Background()
	key := fmt.Sprintf("%s:%s", codeType, userID)

	ttl := 10 * time.Minute
	switch codeType {
	case "email_verify":
		ttl = s.config.EmailVerificationTTL
	case "password_reset":
		ttl = s.config.PasswordResetTTL
	case "2fa_email":
		ttl = s.config.TwoFactorTTL
	}

	return s.cacheStore.Set(ctx, key, code, ttl)
}

func (s *Service) ValidateVerificationCode(codeType, userID, code string) error {
	ctx := context.Background()
	key := fmt.Sprintf("%s:%s", codeType, userID)

	storedCode, err := s.cacheStore.Get(ctx, key)
	if err != nil {
		return errors.BadRequest("invalid or expired verification code")
	}
	if storedCode != code {
		return errors.BadRequest("verification code does not match")
	}
	_ = s.cacheStore.Delete(ctx, key)
	return nil
}

func (s *Service) GenerateVerificationCode() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err == nil {
		n := binary.BigEndian.Uint32(buf) % 1000000
		return fmt.Sprintf("%06d", n)
	}
	return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
}

func (s *Service) GenerateOAuthState() (string, error) {
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("failed to generate OAuth state: %v", err)
	}

	state := base64.URLEncoding.EncodeToString(stateBytes)
	ctx := context.Background()
	if err := s.cacheStore.Set(ctx, "oauth_state:"+state, "valid", 10*time.Minute); err != nil {
		return "", fmt.Errorf("failed to store OAuth state: %v", err)
	}
	return state, nil
}

func (s *Service) ValidateOAuthState(state string) error {
	ctx := context.Background()
	if _, err := s.cacheStore.GetDelete(ctx, "oauth_state:"+state); err != nil {
		return errors.BadRequest("invalid or expired OAuth state")
	}
	return nil
}

func (s *Service) IssueShortLivedToken(userID string) (string, error) {
	claims := jwt.MapClaims{"sub": userID, "exp": time.Now().Add(2 * time.Minute).Unix(), "iat": time.Now().Unix(), "role": "user"}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.config.JWTPrivateKey)
}

func (s *Service) ValidateToken(tokenString string) (*jwt.Token, error) {
	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return &s.config.JWTPrivateKey.PublicKey, nil
	})
}

func (s *Service) ExtractUserIDFromToken(tokenString string) (string, error) {
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

func (s *Service) ExchangeGoogleOAuthCode(code, redirectURI string) (*clientsinfra.GoogleUserInfo, error) {
	token, err := s.googleOAuthConfig.Exchange(context.Background(), code)
	if err != nil {
		return nil, errors.BadRequest("failed to exchange OAuth code")
	}

	client := s.googleOAuthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, errors.InternalServerError("failed to get user info from Google", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.InternalServerError("Google API returned error", nil)
	}

	var userInfo clientsinfra.GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, errors.InternalServerError("failed to decode user info", err)
	}
	return &userInfo, nil
}

func (s *Service) GetGoogleAuthURL(state string) string {
	return s.googleOAuthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (s *Service) IssueTokenPair(user *clientsinfra.User) (*TokenPair, error) {
	accessToken, err := s.IssueAccessToken(user)
	if err != nil {
		return nil, errors.InternalServerError("failed to issue access token", err)
	}

	refreshToken, err := s.IssueRefreshToken(user.ID)
	if err != nil {
		return nil, errors.InternalServerError("failed to issue refresh token", err)
	}

	return &TokenPair{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}

func (s *Service) StartLogin(email, password string, users CredentialUserProvider, emailSender TwoFactorCodeSender) (*LoginStartResult, error) {
	if locked, ttl, err := s.IsLoginLocked(email); err == nil && locked {
		return nil, timedLockoutError("account locked", ReasonLoginLockout, ttl)
	}

	user, err := users.ValidateCredentials(email, password)
	if err != nil {
		_, _, _ = s.RegisterLoginFailure(email)
		if appErr, ok := err.(*errors.AppError); ok {
			if appErr.Code == 0 {
				appErr.Code = 500
			}
			if appErr.Code == 403 && appErr.Message == "account is locked" {
				if locked, ttl, _ := s.IsLoginLocked(email); locked {
					return nil, timedLockoutError("account locked", ReasonLoginLockout, ttl)
				}
				return nil, errors.NewAppError(423, "account locked", nil).WithField("reason", ReasonAdminLock)
			}
			if locked, ttl, _ := s.IsLoginLocked(email); locked {
				return nil, timedLockoutError("account locked", ReasonLoginLockout, ttl)
			}
			return nil, appErr
		}
		if locked, ttl, _ := s.IsLoginLocked(email); locked {
			return nil, timedLockoutError("account locked", ReasonLoginLockout, ttl)
		}
		return nil, errors.Unauthorized("invalid credentials")
	}

	s.ClearLoginFailures(email)

	result := &LoginStartResult{}
	if user.TwoFactorEnabled {
		result.TwoFARequired = true
		result.UserID = user.ID
		result.TwoFAType = user.TwoFactorType

		if user.TwoFactorType == "email" {
			code := s.GenerateVerificationCode()
			if err := s.StoreVerificationCode("2fa_email", user.ID, code); err != nil {
				return nil, errors.InternalServerError("failed to store 2FA code", err)
			}
			if emailSender != nil {
				_ = emailSender.Send2FACode(user.Email, user.FirstName, code)
			}
		}

		return result, nil
	}

	tokens, err := s.IssueTokenPair(user)
	if err != nil {
		return nil, err
	}
	result.AccessToken = tokens.AccessToken
	result.RefreshToken = tokens.RefreshToken
	return result, nil
}

func (s *Service) FinishLogin(userID, code string, users UserProvider) (*TokenPair, error) {
	user, err := users.GetUser(userID)
	if err != nil {
		return nil, errors.NotFound("user not found")
	}
	if user.LockoutEnabled {
		return nil, errors.NewAppError(423, "account locked", nil).WithField("reason", ReasonAdminLock)
	}
	if !user.TwoFactorEnabled {
		return nil, errors.BadRequest("2FA not enabled for user")
	}

	if locked, ttl, _ := s.IsCodeLocked(userID, user.TwoFactorType); locked {
		return nil, timedLockoutError("2fa locked", ReasonTwoFALockout, ttl)
	}

	switch user.TwoFactorType {
	case "totp":
		if !validateTOTP(code, user.TwoFactorSecret) {
			locked, ttl, _ := s.RegisterCodeFailure(userID, "totp")
			if locked {
				return nil, timedLockoutError("2fa locked", ReasonTwoFALockout, ttl)
			}
			return nil, errors.Unauthorized("invalid TOTP code")
		}
		s.ClearCodeFailures(userID, "totp")
	case "email":
		if err := s.ValidateVerificationCode("2fa_email", userID, code); err != nil {
			locked, ttl, _ := s.RegisterCodeFailure(userID, "email")
			if locked {
				return nil, timedLockoutError("2fa locked", ReasonTwoFALockout, ttl)
			}
			return nil, errors.Unauthorized("invalid or expired 2FA code")
		}
		s.ClearCodeFailures(userID, "email")
	default:
		return nil, errors.BadRequest("unsupported 2FA type")
	}

	return s.IssueTokenPair(user)
}

func (s *Service) RefreshSession(refreshToken string, users UserProvider) (*TokenPair, error) {
	userID, err := s.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, errors.Unauthorized("invalid or expired refresh token")
	}

	_ = s.RevokeRefreshToken(refreshToken)

	user, err := users.GetUser(userID)
	if err != nil {
		return nil, errors.NotFound("user not found")
	}
	if user.LockoutEnabled {
		return nil, errors.NewAppError(423, "account locked", nil).WithField("reason", ReasonAdminLock)
	}

	return s.IssueTokenPair(user)
}

func timedLockoutError(message, reason string, ttl int64) *errors.AppError {
	unlockAt := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
	return errors.NewAppError(423, message, nil).WithFields(map[string]interface{}{
		"lockout_remaining": ttl,
		"reason":            reason,
		"unlock_at":         unlockAt,
	})
}

func validateTOTP(code, secret string) bool {
	return code != "" && secret != "" && totp.Validate(code, secret)
}
