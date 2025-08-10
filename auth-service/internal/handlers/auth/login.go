package auth

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

type LoginHandler struct {
	authService  *services.AuthService
	userService  *services.UserService
	emailService *services.EmailService
}

func NewLoginHandler(authService *services.AuthService, userService *services.UserService, emailService *services.EmailService) *LoginHandler {
	return &LoginHandler{
		authService:  authService,
		userService:  userService,
		emailService: emailService,
	}
}

func (h *LoginHandler) HandleLoginStart(w http.ResponseWriter, r *http.Request) {
	var req requests.LoginStartRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	user, err := h.userService.ValidateCredentials(req.Email, req.Password)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			// Translate internal forbidden/lockout to 423 Locked for clarity
			if appErr.Code == http.StatusForbidden && appErr.Message == "account is locked" {
				errors.WriteError(w, errors.NewAppError(http.StatusLocked, "account locked", nil))
				return
			}
			errors.WriteError(w, appErr)
			return
		}
		errors.WriteError(w, errors.Unauthorized("invalid credentials"))
		return
	}


	// Check if 2FA is enabled
	if user.TwoFactorEnabled {
		response := responses.LoginResponse{
			TwoFARequired: true,
			UserID:        user.ID,
			TwoFAType:     user.TwoFactorType,
		}

		// Send 2FA code if email-based
		if user.TwoFactorType == "email" {
			code := h.authService.GenerateVerificationCode()
			if err := h.authService.StoreVerificationCode("2fa_email", user.ID, code); err != nil {
				errors.WriteError(w, errors.InternalServerError("failed to store 2FA code", err))
				return
			}

			if err := h.emailService.Send2FACode(user.Email, user.FirstName, code); err != nil {
				slog.Error("failed to send 2fa email", "error", err)
				// Don't fail the request if email service is down
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Issue tokens directly
	accessToken, err := h.authService.IssueAccessToken(user)
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to issue access token", err))
		return
	}

	refreshToken, err := h.authService.IssueRefreshToken(user.ID)
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to issue refresh token", err))
		return
	}

	response := responses.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *LoginHandler) HandleLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req requests.LoginFinishRequest
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

	if !user.TwoFactorEnabled {
		errors.WriteError(w, errors.BadRequest("2FA not enabled for user"))
		return
	}

	// Validate 2FA code
	switch user.TwoFactorType {
	case "totp":
		if !totp.Validate(req.Code, user.TwoFactorSecret) {
			errors.WriteError(w, errors.Unauthorized("invalid TOTP code"))
			return
		}
	case "email":
		if err := h.authService.ValidateVerificationCode("2fa_email", req.UserID, req.Code); err != nil {
			errors.WriteError(w, errors.Unauthorized("invalid or expired 2FA code"))
			return
		}
	default:
		errors.WriteError(w, errors.BadRequest("unsupported 2FA type"))
		return
	}

	// Issue tokens
	accessToken, err := h.authService.IssueAccessToken(user)
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to issue access token", err))
		return
	}

	refreshToken, err := h.authService.IssueRefreshToken(user.ID)
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to issue refresh token", err))
		return
	}

	response := responses.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
