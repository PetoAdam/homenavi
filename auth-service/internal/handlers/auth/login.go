package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"auth-service/internal/models/requests"
	"auth-service/internal/models/responses"
	"auth-service/internal/services"
	"auth-service/internal/constants"
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

    // Check lockout status for email before validating credentials
	if locked, ttl, err := h.authService.IsLoginLocked(req.Email); err == nil && locked {
		unlockAt := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
		errors.WriteError(w, errors.NewAppError(http.StatusLocked, "account locked", nil).
			WithFields(map[string]interface{}{"lockout_remaining": ttl, "reason": constants.ReasonLoginLockout, "unlock_at": unlockAt}))
		return
	}

	user, err := h.userService.ValidateCredentials(req.Email, req.Password)
	if err != nil {
		// Register failure (only for invalid credentials / unauthorized)
		_ , _, _ = h.authService.RegisterLoginFailure(req.Email)
		if appErr, ok := err.(*errors.AppError); ok {
			if appErr.Code == http.StatusForbidden && appErr.Message == "account is locked" {
				if locked, ttl, _ := h.authService.IsLoginLocked(req.Email); locked {
					unlockAt := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
					errors.WriteError(w, errors.NewAppError(http.StatusLocked, "account locked", nil).WithFields(map[string]interface{}{"lockout_remaining": ttl, "reason": constants.ReasonLoginLockout, "unlock_at": unlockAt}))
                } else {
					// Admin lock (no TTL countdown) -> reason admin_lock
					errors.WriteError(w, errors.NewAppError(http.StatusLocked, "account locked", nil).WithField("reason", constants.ReasonAdminLock))
                }
				return
			}
			errors.WriteError(w, appErr)
			return
		}
		if locked, ttl, _ := h.authService.IsLoginLocked(req.Email); locked {
			unlockAt := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
			errors.WriteError(w, errors.NewAppError(http.StatusLocked, "account locked", nil).WithFields(map[string]interface{}{"lockout_remaining": ttl, "reason": constants.ReasonLoginLockout, "unlock_at": unlockAt}))
			return
		}
		errors.WriteError(w, errors.Unauthorized("invalid credentials"))
		return
	}

    // Successful credential validation clears failure counter
    h.authService.ClearLoginFailures(req.Email)


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

	// If already in a code lockout window, short-circuit so we don't reset the TTL by re-registering failures
	if locked, ttl, _ := h.authService.IsCodeLocked(req.UserID, user.TwoFactorType); locked {
		unlockAt := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
		errors.WriteError(w, errors.NewAppError(http.StatusLocked, "2fa locked", nil).WithFields(map[string]interface{}{ "lockout_remaining": ttl, "reason": constants.ReasonTwoFALockout, "unlock_at": unlockAt }))
		return
	}

	// Validate 2FA code
	switch user.TwoFactorType {
	case "totp":
		if !totp.Validate(req.Code, user.TwoFactorSecret) {
			locked, ttl, _ := h.authService.RegisterCodeFailure(req.UserID, "totp")
			if locked {
				unlockAt := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
				errors.WriteError(w, errors.NewAppError(http.StatusLocked, "2fa locked", nil).WithFields(map[string]interface{}{"lockout_remaining": ttl, "reason": constants.ReasonTwoFALockout, "unlock_at": unlockAt}))
				return
			}
			errors.WriteError(w, errors.Unauthorized("invalid TOTP code"))
			return
		}
		// success -> clear failures
		h.authService.ClearCodeFailures(req.UserID, "totp")
	case "email":
		if err := h.authService.ValidateVerificationCode("2fa_email", req.UserID, req.Code); err != nil {
			locked, ttl, _ := h.authService.RegisterCodeFailure(req.UserID, "email")
			if locked {
				unlockAt := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
				errors.WriteError(w, errors.NewAppError(http.StatusLocked, "2fa locked", nil).WithFields(map[string]interface{}{"lockout_remaining": ttl, "reason": constants.ReasonTwoFALockout, "unlock_at": unlockAt}))
				return
			}
			errors.WriteError(w, errors.Unauthorized("invalid or expired 2FA code"))
			return
		}
		// success
		h.authService.ClearCodeFailures(req.UserID, "email")
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
