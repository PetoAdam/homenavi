package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// EmailService is the use-case surface required by the HTTP transport.
type EmailService interface {
	SendVerificationEmail(to, userName, code string) error
	SendPasswordResetEmail(to, userName, code string) error
	Send2FAEmail(to, userName, code string) error
	SendNotifyEmail(to, userName, subject, message string) error
}

type Handler struct {
	emailService EmailService
}

func NewHandler(emailService EmailService) *Handler {
	return &Handler{emailService: emailService}
}

func (h *Handler) Register(mux interface {
	Post(string, http.HandlerFunc)
	Get(string, http.HandlerFunc)
	Handle(string, http.Handler)
}, promHandler http.Handler) {
	mux.Handle("/metrics", promHandler)
	mux.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.Post("/send/verification", h.SendVerificationEmail)
	mux.Post("/send/password-reset", h.SendPasswordResetEmail)
	mux.Post("/send/2fa", h.Send2FAEmail)
	mux.Post("/send/notify", h.SendNotifyEmail)
}

func (h *Handler) SendVerificationEmail(w http.ResponseWriter, r *http.Request) {
	var req verificationEmailRequest
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (h *Handler) SendPasswordResetEmail(w http.ResponseWriter, r *http.Request) {
	var req passwordResetEmailRequest
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (h *Handler) Send2FAEmail(w http.ResponseWriter, r *http.Request) {
	var req twoFactorEmailRequest
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (h *Handler) SendNotifyEmail(w http.ResponseWriter, r *http.Request) {
	var req notifyEmailRequest
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
	if err := h.emailService.SendNotifyEmail(req.To, req.UserName, req.Subject, req.Message); err != nil {
		slog.Error("failed to send notify email", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to send email")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}
