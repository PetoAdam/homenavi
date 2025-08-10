package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"email-service/internal/services"
)

type EmailHandler struct {
	emailService *services.EmailService
}

func NewEmailHandler(emailService *services.EmailService) *EmailHandler {
	return &EmailHandler{
		emailService: emailService,
	}
}

type VerificationEmailRequest struct {
	To       string `json:"to"`
	UserName string `json:"user_name"`
	Code     string `json:"code"`
}

type PasswordResetEmailRequest struct {
	To   string `json:"to"`
	Name string `json:"name"`
	Code string `json:"code"`
}

type TwoFactorEmailRequest struct {
	To   string `json:"to"`
	Name string `json:"name"`
	Code string `json:"code"`
}

func (h *EmailHandler) SendVerificationEmail(w http.ResponseWriter, r *http.Request) {
	var req VerificationEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("invalid verification email request", "error", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.To == "" || req.UserName == "" || req.Code == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	if err := h.emailService.SendVerificationEmail(req.To, req.UserName, req.Code); err != nil {
		slog.Error("failed to send verification email", "error", err)
		http.Error(w, "Failed to send email", http.StatusInternalServerError)
		return
	}

	slog.Info("verification email sent", "code", req.Code, "to", req.To)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

func (h *EmailHandler) SendPasswordResetEmail(w http.ResponseWriter, r *http.Request) {
	var req PasswordResetEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("invalid password reset email request", "error", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.To == "" || req.Name == "" || req.Code == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	if err := h.emailService.SendPasswordResetEmail(req.To, req.Name, req.Code); err != nil {
		slog.Error("failed to send password reset email", "error", err)
		http.Error(w, "Failed to send email", http.StatusInternalServerError)
		return
	}

	slog.Info("password reset email sent", "to", req.To)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

func (h *EmailHandler) Send2FAEmail(w http.ResponseWriter, r *http.Request) {
	var req TwoFactorEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("invalid 2fa email request", "error", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.To == "" || req.Name == "" || req.Code == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	if err := h.emailService.Send2FAEmail(req.To, req.Name, req.Code); err != nil {
		slog.Error("failed to send 2fa email", "error", err)
		http.Error(w, "Failed to send email", http.StatusInternalServerError)
		return
	}

	slog.Info("2fa email sent", "to", req.To)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}
