package twofactor

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"auth-service/internal/models/requests"
	"auth-service/internal/models/responses"
	"auth-service/internal/services"
	"auth-service/pkg/errors"

	"github.com/pquerna/otp/totp"
)

type SetupHandler struct {
	authService *services.AuthService
	userService *services.UserService
}

func NewSetupHandler(authService *services.AuthService, userService *services.UserService) *SetupHandler {
	return &SetupHandler{
		authService: authService,
		userService: userService,
	}
}

func (h *SetupHandler) Handle2FASetup(w http.ResponseWriter, r *http.Request) {
	var req requests.TwoFactorSetupRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	// Verify user exists
	user, err := h.userService.GetUser(req.UserID)
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

	// Issue a short-lived token for updating user
	token, err := h.authService.IssueShortLivedToken(req.UserID)
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to authorize operation", err))
		return
	}

	// Store the secret but don't enable 2FA yet (user needs to verify)
	updates := map[string]interface{}{
		"two_factor_secret":  secret.Secret(),
		"two_factor_type":    "totp",
		"two_factor_enabled": false,
	}

	if err := h.userService.UpdateUser(req.UserID, updates, token); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to update user", err))
		return
	}

	slog.Info("2fa totp setup initiated", "user_id", req.UserID)

	response := responses.TwoFactorSetupResponse{
		Secret:     secret.Secret(),
		OTPAuthURL: secret.URL(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type VerifyHandler struct {
	authService *services.AuthService
	userService *services.UserService
}

func NewVerifyHandler(authService *services.AuthService, userService *services.UserService) *VerifyHandler {
	return &VerifyHandler{
		authService: authService,
		userService: userService,
	}
}

func (h *VerifyHandler) Handle2FAVerify(w http.ResponseWriter, r *http.Request) {
	var req requests.TwoFactorVerifyRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	// Get user
	user, err := h.userService.GetUser(req.UserID)
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

	// Validate TOTP code
	if user.TwoFactorType == "totp" {
		if !totp.Validate(req.Code, user.TwoFactorSecret) {
			errors.WriteError(w, errors.Unauthorized("invalid TOTP code"))
			return
		}
	} else {
		errors.WriteError(w, errors.BadRequest("unsupported 2FA type"))
		return
	}

	// Issue a short-lived token for updating user
	token, err := h.authService.IssueShortLivedToken(req.UserID)
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to authorize operation", err))
		return
	}

	// Enable 2FA
	updates := map[string]interface{}{
		"two_factor_enabled": true,
	}

	if err := h.userService.UpdateUser(req.UserID, updates, token); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to update user", err))
		return
	}

	slog.Info("2fa enabled", "user_id", req.UserID)

	response := responses.TwoFactorVerifyResponse{
		Verified: true,
		Message:  "2FA has been enabled successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type EmailHandler struct {
	authService  *services.AuthService
	userService  *services.UserService
	emailService *services.EmailService
}

func NewEmailHandler(authService *services.AuthService, userService *services.UserService, emailService *services.EmailService) *EmailHandler {
	return &EmailHandler{
		authService:  authService,
		userService:  userService,
		emailService: emailService,
	}
}

func (h *EmailHandler) Handle2FAEmailRequest(w http.ResponseWriter, r *http.Request) {
	var req requests.TwoFactorEmailRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	// Get user
	user, err := h.userService.GetUser(req.UserID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("user not found"))
		return
	}

	// Generate and store 2FA code
	code := h.authService.GenerateVerificationCode()
	if err := h.authService.StoreVerificationCode("2fa_email", req.UserID, code); err != nil {
		slog.Error("failed to store 2fa email code", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to store 2FA code", err))
		return
	}

	// Send email (mock for now)
	if err := h.emailService.Send2FACode(user.Email, user.FirstName, code); err != nil {
		slog.Error("failed to send 2fa email", "error", err)
		// Mock email sending
		slog.Info("mock 2fa email sent", "email", user.Email, "code", code)
	}

	slog.Info("2fa email code sent", "code", code, "user_id", req.UserID)

	response := responses.TwoFactorEmailResponse{
		Message:  "2FA code sent to your email",
		CodeSent: true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *EmailHandler) Handle2FAEmailVerify(w http.ResponseWriter, r *http.Request) {
	var req requests.TwoFactorEmailVerifyRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	// Validate the 2FA code
	if err := h.authService.ValidateVerificationCode("2fa_email", req.UserID, req.Code); err != nil {
		errors.WriteError(w, errors.Unauthorized("invalid or expired 2FA code"))
		return
	}

	// Issue a short-lived token for updating user
	token, err := h.authService.IssueShortLivedToken(req.UserID)
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to authorize operation", err))
		return
	}

	// Enable email-based 2FA
	updates := map[string]interface{}{
		"two_factor_enabled": true,
		"two_factor_type":    "email",
	}

	if err := h.userService.UpdateUser(req.UserID, updates, token); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to update user", err))
		return
	}

	slog.Info("email 2fa enabled", "user_id", req.UserID)

	response := responses.TwoFactorVerifyResponse{
		Verified: true,
		Message:  "Email-based 2FA has been enabled successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
