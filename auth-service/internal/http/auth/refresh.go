package auth

import (
	"encoding/json"
	"net/http"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	authtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/auth/transport"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

type RefreshHandler struct {
	authService *authdomain.Service
	userService *clientsinfra.UserClient
}

func NewRefreshHandler(authService *authdomain.Service, userService *clientsinfra.UserClient) *RefreshHandler {
	return &RefreshHandler{
		authService: authService,
		userService: userService,
	}
}

func (h *RefreshHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	refreshToken := RefreshTokenFromRequest(r)
	if refreshToken == "" {
		errors.WriteError(w, errors.Unauthorized("missing refresh session"))
		return
	}

	tokens, err := h.authService.RefreshSession(refreshToken, h.userService)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.WriteError(w, appErr)
			return
		}
		errors.WriteError(w, errors.InternalServerError("failed to refresh tokens", err))
		return
	}

	SetSessionCookies(w, r, tokens)
	response := authtransport.RefreshResponse{}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
