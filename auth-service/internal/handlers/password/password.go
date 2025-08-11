package password

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"auth-service/internal/models/requests"
	"auth-service/internal/models/responses"
	"auth-service/internal/services"
	"auth-service/pkg/errors"
)

type ResetHandler struct {
	authService  *services.AuthService
	userService  *services.UserService
	emailService *services.EmailService
}

func NewResetHandler(authService *services.AuthService, userService *services.UserService, emailService *services.EmailService) *ResetHandler {
	return &ResetHandler{
		authService:  authService,
		userService:  userService,
		emailService: emailService,
	}
}

func (h *ResetHandler) HandlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	var req requests.PasswordResetRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	user, err := h.userService.GetUserByEmail(req.Email)
	if err != nil {
		// Don't reveal if user exists or not for security
		slog.Info("password reset requested for non-existent email", "email", req.Email)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responses.VerificationResponse{
			Message:  "If the email exists, a password reset code has been sent",
			CodeSent: true,
		})
		return
	}

	code := h.authService.GenerateVerificationCode()
	if err := h.authService.StoreVerificationCode("password_reset", user.ID, code); err != nil {
		slog.Error("failed to store password reset code", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to store reset code", err))
		return
	}

	// Send email (mock for now, will use real email service when available)
	if err := h.emailService.SendPasswordResetCode(user.Email, user.FirstName, code); err != nil {
		slog.Error("failed to send password reset email", "error", err)
		// Mock email sending
		slog.Info("mock password reset email sent", "email", user.Email, "code", code)
	}

	slog.Info("password reset code sent", "code", code, "user_id", user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses.VerificationResponse{
		Message:  "Password reset code sent to your email",
		CodeSent: true,
	})
}

func (h *ResetHandler) HandlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	var req requests.PasswordResetConfirmRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	user, err := h.userService.GetUserByEmail(req.Email)
	if err != nil {
		errors.WriteError(w, errors.NotFound("user not found"))
		return
	}

	if err := h.authService.ValidateVerificationCode("password_reset", user.ID, req.Code); err != nil {
		errors.WriteError(w, errors.Unauthorized("invalid or expired reset code"))
		return
	}

	if err := h.userService.UpdatePassword(user.ID, req.NewPassword); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to update password", err))
		return
	}

	slog.Info("password reset completed", "user_id", user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses.SuccessResponse{
		Message: "Password reset successfully",
	})
}

type ChangeHandler struct {
	authService *services.AuthService
	userService *services.UserService
}

func NewChangeHandler(authService *services.AuthService, userService *services.UserService) *ChangeHandler {
	return &ChangeHandler{
		authService: authService,
		userService: userService,
	}
}

func (h *ChangeHandler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req requests.ChangePasswordRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	// Extract JWT token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		errors.WriteError(w, errors.Unauthorized("missing or invalid authorization header"))
		return
	}

	token := authHeader[7:]
	userID, err := h.authService.ExtractUserIDFromToken(token)
	if err != nil {
		errors.WriteError(w, errors.Unauthorized("invalid token"))
		return
	}

	// Get user to validate current password
	user, err := h.userService.GetUser(userID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("user not found"))
		return
	}

	// Validate current password by attempting login
	_, err = h.userService.ValidateCredentials(user.Email, req.CurrentPassword)
	if err != nil {
		errors.WriteError(w, errors.Unauthorized("current password is incorrect"))
		return
	}

	// Update password
	if err := h.userService.UpdatePassword(userID, req.NewPassword); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to update password", err))
		return
	}

	slog.Info("password changed", "user_id", userID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses.SuccessResponse{
		Message: "Password changed successfully",
	})
}
