package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"email-service/internal/services"
)

type EmailHandler struct {
	emailService *services.EmailService
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "code": status})
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

type NotifyEmailRequest struct {
	To       string `json:"to"`
	UserName string `json:"user_name"`
	Subject  string `json:"subject"`
	Message  string `json:"message"`
}

func (h *EmailHandler) SendVerificationEmail(w http.ResponseWriter, r *http.Request) {
	var req VerificationEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("invalid verification email request", "error", err)
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.To == "" || req.UserName == "" || req.Code == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	if err := h.emailService.SendVerificationEmail(req.To, req.UserName, req.Code); err != nil {
		slog.Error("failed to send verification email", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to send email")
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
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.To == "" || req.Name == "" || req.Code == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	if err := h.emailService.SendPasswordResetEmail(req.To, req.Name, req.Code); err != nil {
		slog.Error("failed to send password reset email", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to send email")
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
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.To == "" || req.Name == "" || req.Code == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	if err := h.emailService.Send2FAEmail(req.To, req.Name, req.Code); err != nil {
		slog.Error("failed to send 2fa email", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to send email")
		return
	}

	slog.Info("2fa email sent", "to", req.To)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

func (h *EmailHandler) SendNotifyEmail(w http.ResponseWriter, r *http.Request) {
	var req NotifyEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("invalid notify email request", "error", err)
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}

	req.To = strings.TrimSpace(req.To)
	req.UserName = strings.TrimSpace(req.UserName)
	req.Subject = strings.TrimSpace(req.Subject)
	req.Message = strings.TrimSpace(req.Message)

	if req.To == "" || req.UserName == "" || req.Subject == "" || req.Message == "" {
		writeJSONError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	data := services.EmailData{
		UserName: req.UserName,
		Subject:  req.Subject,
		Title:    req.Subject,
		Message:  req.Message,
		AppName:  "Homenavi",
	}

	if err := h.emailService.SendEmail(req.To, req.Subject, "notify", data); err != nil {
		slog.Error("failed to send notify email", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to send email")
		return
	}

	slog.Info("notify email sent", "to", req.To)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}
