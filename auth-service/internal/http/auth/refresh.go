package auth

import (
	"encoding/json"
	"net/http"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	authtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/auth/transport"
	sharedtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/transport"
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
	var req authtransport.RefreshRequest
	if err := sharedtransport.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	tokens, err := h.authService.RefreshSession(req.RefreshToken, h.userService)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.WriteError(w, appErr)
			return
		}
		errors.WriteError(w, errors.InternalServerError("failed to refresh tokens", err))
		return
	}

	response := authtransport.RefreshResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
