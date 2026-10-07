package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"time"

	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp/totp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Config holds the auth business configuration.
type Config struct {
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
	OAuthTrustedClients          []OAuthClient
}

var ErrMCPRoleRequired = stdErrors.New("MCP access requires resident role or above")

// Service implements authentication use cases and token/code lifecycle behavior.
type Service struct {
	config            Config
	cacheStore        cacheinfra.Store
	keyRing           *KeyRing
	googleOAuthConfig *oauth2.Config
	oauthClients      *OAuthClientRegistry
	oauthCodes        *OAuthAuthorizationCodeManager
	oauthConsents     *OAuthConsentManager
}

type CredentialUserProvider interface {
	ValidateCredentials(email, password string) (*clientsinfra.User, error)
	GetUser(userID string) (*clientsinfra.User, error)
}

type UserProvider interface {
	GetUser(userID string) (*clientsinfra.User, error)
}

type RecoveryCodeConsumer interface {
	ConsumeRecoveryCode(userID, code string) (bool, error)
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

type OAuthSession struct {
	Subject   string
	SessionID string
	Role      string
}

func MCPRoleAllowed(role string) bool {
	return role == authx.RoleResident || role == authx.RoleAdmin
}

const (
	refreshTokenPrefix         = "refresh_token:"
	refreshTokenConsumedPrefix = "refresh_token_consumed:"
	refreshTokenFamilyPrefix   = "refresh_token_family:"
	refreshTokenFamilyActive   = "active"
	refreshTokenFamilyRevoked  = "revoked"
)

type refreshTokenRecord struct {
	UserID    string `json:"user_id"`
	FamilyID  string `json:"family_id"`
	SessionID string `json:"session_id"`
}

func NewService(cfg Config, cacheStore cacheinfra.Store) *Service {
	googleOAuthConfig := &oauth2.Config{
		RedirectURL:  cfg.GoogleOAuthRedirectURL,
		ClientID:     cfg.GoogleOAuthClientID,
		ClientSecret: cfg.GoogleOAuthClientSecret,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}

	oauthClients, _ := NewOAuthClientRegistry(cfg.OAuthTrustedClients, cfg.OAuthMCPResource)
	return &Service{config: cfg, cacheStore: cacheStore, keyRing: NewKeyRing(cfg.JWTKeyID, cfg.JWTPrivateKey, cfg.JWTVerificationPublicKeys...), googleOAuthConfig: googleOAuthConfig, oauthClients: oauthClients, oauthCodes: NewOAuthAuthorizationCodeManager(cacheStore), oauthConsents: NewOAuthConsentManager(cacheStore)}
}

func (s *Service) Close() error {
	if s.cacheStore == nil {
		return nil
	}
	return s.cacheStore.Close()
}

func (s *Service) IssueAccessToken(user *clientsinfra.User) (string, error) {
	if user == nil {
		return "", fmt.Errorf("user is required")
	}
	sessionID, err := newTokenID()
	if err != nil {
		return "", fmt.Errorf("generate session ID: %w", err)
	}
	if err := s.activateSession(sessionID); err != nil {
		return "", err
	}
	return s.issueAccessToken(user, sessionID)
}

func (s *Service) issueAccessToken(user *clientsinfra.User, sessionID string) (string, error) {
	if user == nil || sessionID == "" {
		return "", fmt.Errorf("user and session ID are required")
	}
	tokenID, err := newTokenID()
	if err != nil {
		return "", fmt.Errorf("generate token id: %w", err)
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":                s.config.JWTIssuer,
		"sub":                user.ID,
		"aud":                []string{s.config.JWTAPIAudience},
		"exp":                now.Add(s.config.AccessTokenTTL).Unix(),
		"iat":                now.Unix(),
		"nbf":                now.Unix(),
		"jti":                tokenID,
		authx.ClaimSessionID: sessionID,
		authx.ClaimTokenType: authx.TokenTypeAPI,
		"role":               user.Role,
		"name":               user.FirstName + " " + user.LastName,
	}

	return s.keyRing.Sign(claims)
}

// JSONWebKeySet returns the public key document for the active signing key.
func (s *Service) JSONWebKeySet() (JSONWebKeySet, error) {
	return s.keyRing.JSONWebKeySet()
}

func newTokenID() (string, error) {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(tokenBytes), nil
}

func (s *Service) IssueRefreshToken(userID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("user ID is required")
	}
	familyID, err := newTokenID()
	if err != nil {
		return "", fmt.Errorf("generate refresh token family ID: %w", err)
	}
	sessionID, err := newTokenID()
	if err != nil {
		return "", fmt.Errorf("generate session ID: %w", err)
	}
	if err := s.activateSession(sessionID); err != nil {
		return "", err
	}
	return s.issueRefreshToken(userID, familyID, sessionID)
}

func (s *Service) issueRefreshToken(userID, familyID, sessionID string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	recordBytes, err := json.Marshal(refreshTokenRecord{UserID: userID, FamilyID: familyID, SessionID: sessionID})
	if err != nil {
		return "", fmt.Errorf("encode refresh token record: %w", err)
	}
	ctx := context.Background()
	if err := s.cacheStore.Set(ctx, refreshTokenFamilyKey(familyID), refreshTokenFamilyActive, s.config.RefreshTokenTTL); err != nil {
		return "", fmt.Errorf("store refresh token family: %w", err)
	}
	if err := s.cacheStore.Set(ctx, refreshTokenKey(token), string(recordBytes), s.config.RefreshTokenTTL); err != nil {
		_ = s.cacheStore.Delete(ctx, refreshTokenFamilyKey(familyID))
		return "", fmt.Errorf("store refresh token: %w", err)
	}
	return token, nil
}

func (s *Service) ValidateRefreshToken(token string) (string, error) {
	ctx := context.Background()
	record, err := s.loadRefreshTokenRecord(ctx, refreshTokenKey(token))
	if err != nil {
		return "", errors.Unauthorized("invalid or expired refresh token")
	}
	status, err := s.cacheStore.Get(ctx, refreshTokenFamilyKey(record.FamilyID))
	if err != nil || status != refreshTokenFamilyActive {
		return "", errors.Unauthorized("invalid or expired refresh token")
	}
	return record.UserID, nil
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
	record, err := s.loadRefreshTokenRecord(ctx, refreshTokenKey(token))
	if err != nil {
		consumedRecord, consumedErr := s.cacheStore.Get(ctx, refreshTokenConsumedKey(token))
		if consumedErr != nil {
			if stdErrors.Is(consumedErr, cacheinfra.ErrNotFound) {
				return nil
			}
			return fmt.Errorf("load consumed refresh token: %w", consumedErr)
		}
		record, err = decodeRefreshTokenRecord(consumedRecord)
		if err != nil {
			return fmt.Errorf("decode consumed refresh token: %w", err)
		}
	}
	familyKey := refreshTokenFamilyKey(record.FamilyID)
	if ttl, err := s.cacheStore.TTL(ctx, familyKey); err == nil && ttl > 0 {
		if err := s.cacheStore.Set(ctx, familyKey, refreshTokenFamilyRevoked, ttl); err != nil {
			return err
		}
	} else if err != nil && !stdErrors.Is(err, cacheinfra.ErrNotFound) {
		return fmt.Errorf("read refresh token family: %w", err)
	}
	if err := s.revokeSession(record.SessionID); err != nil {
		return err
	}
	return s.cacheStore.Delete(ctx, refreshTokenKey(token))
}

func (s *Service) rotateRefreshToken(token string) (refreshTokenRecord, string, error) {
	newTokenBytes := make([]byte, 32)
	if _, err := rand.Read(newTokenBytes); err != nil {
		return refreshTokenRecord{}, "", fmt.Errorf("generate replacement refresh token: %w", err)
	}
	replacementToken := base64.RawURLEncoding.EncodeToString(newTokenBytes)
	ctx := context.Background()
	recordJSON, status, err := s.cacheStore.RotateRefreshToken(ctx, refreshTokenKey(token), refreshTokenConsumedKey(token), refreshTokenKey(replacementToken), refreshTokenFamilyPrefix)
	if err != nil {
		return refreshTokenRecord{}, "", fmt.Errorf("rotate refresh token: %w", err)
	}
	if status == cacheinfra.RefreshTokenReplayed {
		if err := s.RevokeRefreshToken(token); err != nil {
			return refreshTokenRecord{}, "", errors.ServiceUnavailable("refresh replay revocation unavailable", err)
		}
	}
	if status != cacheinfra.RefreshTokenRotated {
		return refreshTokenRecord{}, "", errors.Unauthorized("invalid or expired refresh token")
	}
	record, err := decodeRefreshTokenRecord(recordJSON)
	if err != nil {
		return refreshTokenRecord{}, "", errors.Unauthorized("invalid or expired refresh token")
	}
	return record, replacementToken, nil
}

func (s *Service) loadRefreshTokenRecord(ctx context.Context, key string) (refreshTokenRecord, error) {
	recordJSON, err := s.cacheStore.Get(ctx, key)
	if err != nil {
		return refreshTokenRecord{}, err
	}
	return decodeRefreshTokenRecord(recordJSON)
}

func decodeRefreshTokenRecord(recordJSON string) (refreshTokenRecord, error) {
	var record refreshTokenRecord
	if err := json.Unmarshal([]byte(recordJSON), &record); err != nil {
		return refreshTokenRecord{}, err
	}
	if record.UserID == "" || record.FamilyID == "" || record.SessionID == "" {
		return refreshTokenRecord{}, fmt.Errorf("invalid refresh token record")
	}
	return record, nil
}

func refreshTokenKey(token string) string {
	digest := sha256.Sum256([]byte(token))
	return refreshTokenPrefix + base64.RawURLEncoding.EncodeToString(digest[:])
}

func refreshTokenConsumedKey(token string) string {
	digest := sha256.Sum256([]byte(token))
	return refreshTokenConsumedPrefix + base64.RawURLEncoding.EncodeToString(digest[:])
}

func refreshTokenFamilyKey(familyID string) string {
	return refreshTokenFamilyPrefix + familyID
}

func (s *Service) activateSession(sessionID string) error {
	if s.cacheStore == nil {
		return fmt.Errorf("session cache is not configured")
	}
	return s.cacheStore.Set(context.Background(), authx.SessionStatusKey(sessionID), authx.SessionStatusActive, s.config.RefreshTokenTTL)
}

func (s *Service) revokeSession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}
	ttl, err := s.cacheStore.TTL(context.Background(), authx.SessionStatusKey(sessionID))
	if err != nil {
		if stdErrors.Is(err, cacheinfra.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("read session status: %w", err)
	}
	if ttl <= 0 {
		return nil
	}
	if err := s.cacheStore.Set(context.Background(), authx.SessionStatusKey(sessionID), authx.SessionStatusRevoked, ttl); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
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

func (s *Service) IssueServiceToken() (string, error) {
	tokenID, err := newTokenID()
	if err != nil {
		return "", fmt.Errorf("generate service token ID: %w", err)
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":                s.config.JWTIssuer,
		"sub":                authx.ServicePrincipalAuth,
		"aud":                []string{s.config.JWTUserServiceAudience},
		"exp":                now.Add(2 * time.Minute).Unix(),
		"iat":                now.Unix(),
		"nbf":                now.Unix(),
		"jti":                tokenID,
		authx.ClaimTokenType: authx.TokenTypeService,
		"role":               authx.RoleService,
	}
	return s.keyRing.Sign(claims)
}

func (s *Service) ValidateToken(tokenString string) (*jwt.Token, error) {
	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		keyID, ok := token.Header["kid"].(string)
		if !ok || keyID == "" {
			return nil, fmt.Errorf("missing JWT key ID")
		}
		return s.keyRing.PublicKeyForID(keyID)
	}, jwt.WithIssuer(s.config.JWTIssuer), jwt.WithAudience(s.config.JWTAPIAudience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(30*time.Second))
	if err != nil || !token.Valid {
		return token, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("unexpected token claims type")
	}
	if claims[authx.ClaimTokenType] != authx.TokenTypeAPI || claims["sub"] == "" || claims["role"] == "" || claims["jti"] == "" || claims["nbf"] == nil || claims["iat"] == nil {
		return nil, fmt.Errorf("invalid API token claims")
	}
	sessionID, ok := claims[authx.ClaimSessionID].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("missing API token session ID")
	}
	if err := s.requireActiveSession(sessionID); err != nil {
		return nil, err
	}
	return token, nil
}

func (s *Service) requireActiveSession(sessionID string) error {
	if s.cacheStore == nil {
		return fmt.Errorf("session cache is not configured")
	}
	status, err := s.cacheStore.Get(context.Background(), authx.SessionStatusKey(sessionID))
	if err != nil {
		return fmt.Errorf("load session status: %w", err)
	}
	if status != authx.SessionStatusActive {
		return fmt.Errorf("session is not active")
	}
	return nil
}

func (s *Service) ExtractUserIDFromToken(tokenString string) (string, error) {
	session, err := s.ExtractOAuthSession(tokenString)
	if err != nil {
		return "", err
	}
	return session.Subject, nil
}

func (s *Service) ExtractOAuthSession(tokenString string) (OAuthSession, error) {
	token, err := s.ValidateToken(tokenString)
	if err != nil || !token.Valid {
		return OAuthSession{}, errors.Unauthorized("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return OAuthSession{}, errors.Unauthorized("invalid token claims")
	}
	userID, ok := claims["sub"].(string)
	if !ok {
		return OAuthSession{}, errors.Unauthorized("invalid user ID in token")
	}
	sessionID, _ := claims[authx.ClaimSessionID].(string)
	if sessionID == "" {
		return OAuthSession{}, errors.Unauthorized("invalid token session")
	}
	role, _ := claims["role"].(string)
	return OAuthSession{Subject: userID, SessionID: sessionID, Role: role}, nil
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
	if user == nil {
		return nil, errors.BadRequest("user is required")
	}
	sessionID, err := newTokenID()
	if err != nil {
		return nil, errors.InternalServerError("generate session ID", err)
	}
	if err := s.activateSession(sessionID); err != nil {
		return nil, errors.InternalServerError("activate session", err)
	}
	accessToken, err := s.issueAccessToken(user, sessionID)
	if err != nil {
		return nil, errors.InternalServerError("failed to issue access token", err)
	}

	familyID, err := newTokenID()
	if err != nil {
		return nil, errors.InternalServerError("generate refresh token family ID", err)
	}
	refreshToken, err := s.issueRefreshToken(user.ID, familyID, sessionID)
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
		secret, err := s.DecryptTOTPSecret(user.TwoFactorSecret)
		if err != nil {
			return nil, errors.InternalServerError("failed to read TOTP secret", err)
		}
		if !validateTOTP(code, secret) {
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

func (s *Service) FinishLoginWithRecoveryCode(userID, code string, users UserProvider, recoveryCodes RecoveryCodeConsumer) (*TokenPair, error) {
	user, err := users.GetUser(userID)
	if err != nil {
		return nil, errors.NotFound("user not found")
	}
	if user.LockoutEnabled {
		return nil, errors.NewAppError(423, "account locked", nil).WithField("reason", ReasonAdminLock)
	}
	if !user.TwoFactorEnabled || recoveryCodes == nil {
		return nil, errors.BadRequest("recovery login is unavailable")
	}
	if locked, ttl, _ := s.IsCodeLocked(userID, "recovery"); locked {
		return nil, timedLockoutError("recovery codes locked", ReasonTwoFALockout, ttl)
	}
	consumed, err := recoveryCodes.ConsumeRecoveryCode(userID, code)
	if err != nil {
		return nil, errors.InternalServerError("failed to verify recovery code", err)
	}
	if !consumed {
		locked, ttl, _ := s.RegisterCodeFailure(userID, "recovery")
		if locked {
			return nil, timedLockoutError("recovery codes locked", ReasonTwoFALockout, ttl)
		}
		return nil, errors.Unauthorized("invalid recovery code")
	}
	s.ClearCodeFailures(userID, "recovery")
	return s.IssueTokenPair(user)
}

func (s *Service) RefreshSession(refreshToken string, users UserProvider) (*TokenPair, error) {
	record, rotatedRefreshToken, err := s.rotateRefreshToken(refreshToken)
	if err != nil {
		return nil, errors.Unauthorized("invalid or expired refresh token")
	}

	user, err := users.GetUser(record.UserID)
	if err != nil {
		_ = s.RevokeRefreshToken(rotatedRefreshToken)
		return nil, errors.NotFound("user not found")
	}
	if user.LockoutEnabled {
		_ = s.RevokeRefreshToken(rotatedRefreshToken)
		return nil, errors.NewAppError(423, "account locked", nil).WithField("reason", ReasonAdminLock)
	}

	accessToken, err := s.issueAccessToken(user, record.SessionID)
	if err != nil {
		_ = s.RevokeRefreshToken(rotatedRefreshToken)
		return nil, err
	}
	return &TokenPair{AccessToken: accessToken, RefreshToken: rotatedRefreshToken}, nil
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
