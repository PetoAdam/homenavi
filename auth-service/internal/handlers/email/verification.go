package email

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"auth-service/internal/models/requests"
	"auth-service/internal/models/responses"
	"auth-service/internal/services"
	"auth-service/pkg/errors"
)

type VerificationHandler struct {
	authService  *services.AuthService
	userService  *services.UserService
	emailService *services.EmailService
}

func NewVerificationHandler(authService *services.AuthService, userService *services.UserService, emailService *services.EmailService) *VerificationHandler {
	return &VerificationHandler{
		authService:  authService,
		userService:  userService,
		emailService: emailService,
	}
}

func (h *VerificationHandler) HandleEmailVerifyRequest(w http.ResponseWriter, r *http.Request) {
	var req requests.EmailVerifyRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	user, err := h.userService.GetUser(req.UserID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("user not found"))
		return
	}

	if user.EmailConfirmed {
		errors.WriteError(w, errors.BadRequest("email is already verified"))
		return
	}

	code := h.authService.GenerateVerificationCode()
	if err := h.authService.StoreVerificationCode("email_verify", req.UserID, code); err != nil {
		slog.Error("failed to store email verification code", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to store verification code", err))
		return
	}

	// Send email
	if err := h.emailService.SendVerificationEmail(user.Email, user.FirstName, code); err != nil {
		slog.Error("failed to send verification email", "error", err)
		slog.Info("mock verification email sent", "email", user.Email, "code", code)
	}

	slog.Info("email verification code sent", "code", code, "user_id", req.UserID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses.VerificationResponse{
		Message:  "Verification code sent to your email",
		CodeSent: true,
	})
}

func (h *VerificationHandler) HandleEmailVerifyConfirm(w http.ResponseWriter, r *http.Request) {
	var req requests.EmailVerifyConfirmRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	// Validate the verification code
	if err := h.authService.ValidateVerificationCode("email_verify", req.UserID, req.Code); err != nil {
		errors.WriteError(w, errors.Unauthorized("invalid or expired verification code"))
		return
	}

	// Extract JWT token from Authorization header (if provided)
	authHeader := r.Header.Get("Authorization")
	var token string
	if authHeader != "" && len(authHeader) >= 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	} else {
		// Issue a short-lived token for this operation
		var err error
		token, err = h.authService.IssueShortLivedToken(req.UserID)
		if err != nil {
			errors.WriteError(w, errors.InternalServerError("failed to authorize operation", err))
			return
		}
	}

	// Update user's email confirmed status
	updates := map[string]interface{}{
		"email_confirmed": true,
	}

	if err := h.userService.UpdateUser(req.UserID, updates, token); err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to update user", err))
		return
	}

	slog.Info("email verified", "user_id", req.UserID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses.SuccessResponse{
		Message: "Email verified successfully",
	})
}
