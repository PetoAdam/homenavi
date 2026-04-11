package auth

import (
	"encoding/json"
	"net/http"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	authtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/auth/transport"
	sharedtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/transport"
)

type LoginHandler struct {
	authService  *authdomain.Service
	userService  *clientsinfra.UserClient
	emailService *clientsinfra.EmailClient
}

func NewLoginHandler(authService *authdomain.Service, userService *clientsinfra.UserClient, emailService *clientsinfra.EmailClient) *LoginHandler {
	return &LoginHandler{
		authService:  authService,
		userService:  userService,
		emailService: emailService,
	}
}

func (h *LoginHandler) HandleLoginStart(w http.ResponseWriter, r *http.Request) {
	var req authtransport.LoginStartRequest
	if err := sharedtransport.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	result, err := h.authService.StartLogin(req.Email, req.Password, h.userService, h.emailService)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.WriteError(w, appErr)
			return
		}
		errors.WriteError(w, errors.InternalServerError("failed to start login", err))
		return
	}

	response := authtransport.LoginResponse{
		TwoFARequired: result.TwoFARequired,
		UserID:        result.UserID,
		TwoFAType:     result.TwoFAType,
		AccessToken:   result.AccessToken,
		RefreshToken:  result.RefreshToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *LoginHandler) HandleLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req authtransport.LoginFinishRequest
	if err := sharedtransport.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	tokens, err := h.authService.FinishLogin(req.UserID, req.Code, h.userService)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.WriteError(w, appErr)
			return
		}
		errors.WriteError(w, errors.InternalServerError("failed to complete login", err))
		return
	}

	response := authtransport.LoginResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
