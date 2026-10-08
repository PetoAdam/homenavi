package twofactor

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"log/slog"
	"net/http"
	"strings"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	sharedtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/transport"
	twofactortransport "github.com/PetoAdam/homenavi/auth-service/internal/http/twofactor/transport"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"

	"github.com/pquerna/otp/totp"
)

type SetupHandler struct {
	authService *authdomain.Service
	userService *clientsinfra.UserClient
}

func NewSetupHandler(authService *authdomain.Service, userService *clientsinfra.UserClient) *SetupHandler {
	return &SetupHandler{
		authService: authService,
		userService: userService,
	}
}

func (h *SetupHandler) Handle2FASetup(w http.ResponseWriter, r *http.Request) {
	var req twofactortransport.TwoFactorSetupRequest
	if err := sharedtransport.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	userID, authErr := authenticatedUserID(r, h.authService)
	if authErr != nil {
		errors.WriteError(w, authErr)
		return
	}

	user, err := h.userService.GetUser(userID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("user not found"))
		return
	}

	if user.TwoFactorEnabled {
		errors.WriteError(w, errors.BadRequest("2FA is already enabled for this user"))
		return
	}

	// Generate TOTP secret
	secret, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "HomeNavi",
		AccountName: user.Email,
		SecretSize:  32,
	})
	if err != nil {
		slog.Error("failed to generate totp secret", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to generate TOTP secret", err))
		return
	}
	qrCodeDataURL, err := provisioningQRCodeDataURL(secret)
	if err != nil {
		slog.Error("failed to create totp enrollment QR code", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to create TOTP enrollment QR code", err))
		return
	}

	// Issue a short-lived token for updating user
	token, err := h.authService.IssueServiceToken()
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to authorize operation", err))
		return
	}

	encryptedSecret, err := h.authService.EncryptTOTPSecret(secret.Secret())
	if err != nil {
		slog.Error("failed to encrypt TOTP secret", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to protect TOTP secret", err))
		return
	}

	updates := map[string]interface{}{
		"two_factor_secret":  encryptedSecret,
		"two_factor_type":    "totp",
		"two_factor_enabled": false,
	}

	if err := h.userService.UpdateUser(userID, updates, token); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to update user", err))
		return
	}

	slog.Info("2fa totp setup initiated", "user_id", userID)

	response := twofactortransport.TwoFactorSetupResponse{
		Secret:        secret.Secret(),
		OTPAuthURL:    secret.URL(),
		QRCodeDataURL: qrCodeDataURL,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type VerifyHandler struct {
	authService *authdomain.Service
	userService *clientsinfra.UserClient
}

func NewVerifyHandler(authService *authdomain.Service, userService *clientsinfra.UserClient) *VerifyHandler {
	return &VerifyHandler{
		authService: authService,
		userService: userService,
	}
}

func (h *VerifyHandler) Handle2FAVerify(w http.ResponseWriter, r *http.Request) {
	var req twofactortransport.TwoFactorVerifyRequest
	if err := sharedtransport.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	userID, authErr := authenticatedUserID(r, h.authService)
	if authErr != nil {
		errors.WriteError(w, authErr)
		return
	}

	user, err := h.userService.GetUser(userID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("user not found"))
		return
	}

	if user.TwoFactorEnabled {
		errors.WriteError(w, errors.BadRequest("2FA is already enabled"))
		return
	}

	if user.TwoFactorSecret == "" {
		errors.WriteError(w, errors.BadRequest("2FA setup must be completed first"))
		return
	}

	secret, err := h.authService.DecryptTOTPSecret(user.TwoFactorSecret)
	if err != nil {
		slog.Error("failed to decrypt TOTP secret", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to read TOTP secret", err))
		return
	}

	if user.TwoFactorType == "totp" {
		if !totp.Validate(req.Code, secret) {
			errors.WriteError(w, errors.Unauthorized("invalid TOTP code"))
			return
		}
	} else {
		errors.WriteError(w, errors.BadRequest("unsupported 2FA type"))
		return
	}

	// Issue a short-lived token for updating user
	token, err := h.authService.IssueServiceToken()
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to authorize operation", err))
		return
	}
	recoveryCodes, recoveryCodeHashes, err := authdomain.GenerateRecoveryCodes()
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to generate recovery codes", err))
		return
	}
	if err := h.userService.ReplaceRecoveryCodes(userID, recoveryCodeHashes, token); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to protect recovery codes", err))
		return
	}

	// Enable 2FA
	updates := map[string]interface{}{
		"two_factor_enabled": true,
	}

	if err := h.userService.UpdateUser(userID, updates, token); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to update user", err))
		return
	}

	slog.Info("2fa enabled", "user_id", userID)

	response := twofactortransport.TwoFactorVerifyResponse{
		Verified:      true,
		Message:       "2FA has been enabled successfully",
		RecoveryCodes: recoveryCodes,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type EmailHandler struct {
	authService  *authdomain.Service
	userService  *clientsinfra.UserClient
	emailService *clientsinfra.EmailClient
}

func NewEmailHandler(authService *authdomain.Service, userService *clientsinfra.UserClient, emailService *clientsinfra.EmailClient) *EmailHandler {
	return &EmailHandler{
		authService:  authService,
		userService:  userService,
		emailService: emailService,
	}
}

func (h *EmailHandler) Handle2FAEmailRequest(w http.ResponseWriter, r *http.Request) {
	var req twofactortransport.TwoFactorEmailRequest
	if err := sharedtransport.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	userID, authErr := authenticatedUserID(r, h.authService)
	if authErr != nil {
		errors.WriteError(w, authErr)
		return
	}

	user, err := h.userService.GetUser(userID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("user not found"))
		return
	}

	// Generate and store 2FA code
	code := h.authService.GenerateVerificationCode()
	if err := h.authService.StoreVerificationCode("2fa_email", userID, code); err != nil {
		slog.Error("failed to store 2fa email code", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to store 2FA code", err))
		return
	}

	if err := h.emailService.Send2FACode(user.Email, user.FirstName, code); err != nil {
		slog.Error("failed to send 2fa email", "error", err)
		errors.WriteError(w, errors.ServiceUnavailable("2FA email delivery is temporarily unavailable", err))
		return
	}

	slog.Info("2fa email code sent", "user_id", userID)

	response := twofactortransport.TwoFactorEmailResponse{
		Message:  "2FA code sent to your email",
		CodeSent: true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *EmailHandler) Handle2FAEmailVerify(w http.ResponseWriter, r *http.Request) {
	var req twofactortransport.TwoFactorEmailVerifyRequest
	if err := sharedtransport.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	userID, authErr := authenticatedUserID(r, h.authService)
	if authErr != nil {
		errors.WriteError(w, authErr)
		return
	}

	if err := h.authService.ValidateVerificationCode("2fa_email", userID, req.Code); err != nil {
		errors.WriteError(w, errors.Unauthorized("invalid or expired 2FA code"))
		return
	}

	// Issue a short-lived token for updating user
	token, err := h.authService.IssueServiceToken()
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to authorize operation", err))
		return
	}
	recoveryCodes, recoveryCodeHashes, err := authdomain.GenerateRecoveryCodes()
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to generate recovery codes", err))
		return
	}
	if err := h.userService.ReplaceRecoveryCodes(userID, recoveryCodeHashes, token); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to protect recovery codes", err))
		return
	}

	// Enable email-based 2FA
	updates := map[string]interface{}{
		"two_factor_enabled": true,
		"two_factor_type":    "email",
	}

	if err := h.userService.UpdateUser(userID, updates, token); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to update user", err))
		return
	}

	slog.Info("email 2fa enabled", "user_id", userID)

	response := twofactortransport.TwoFactorVerifyResponse{
		Verified:      true,
		Message:       "Email-based 2FA has been enabled successfully",
		RecoveryCodes: recoveryCodes,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func authenticatedUserID(r *http.Request, authService *authdomain.Service) (string, *errors.AppError) {
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Bearer ") {
		return "", errors.Unauthorized("missing or invalid authorization header")
	}

	token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	if token == "" {
		return "", errors.Unauthorized("missing or invalid authorization header")
	}

	userID, err := authService.ExtractUserIDFromToken(token)
	if err != nil {
		return "", errors.Unauthorized("invalid token")
	}
	return userID, nil
}

func provisioningQRCodeDataURL(key interface {
	Image(int, int) (image.Image, error)
}) (string, error) {
	qrCode, err := key.Image(256, 256)
	if err != nil {
		return "", err
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, qrCode); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}
