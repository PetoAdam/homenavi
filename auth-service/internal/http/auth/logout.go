package auth

import (
	"encoding/json"
	"net/http"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	authtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/auth/transport"
)

type LogoutHandler struct {
	authService *authdomain.Service
}

func NewLogoutHandler(authService *authdomain.Service) *LogoutHandler {
	return &LogoutHandler{
		authService: authService,
	}
}

func (h *LogoutHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	refreshToken := RefreshTokenFromRequest(r)
	if refreshToken == "" {
		ClearSessionCookies(w, r)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authtransport.LogoutResponse{Message: "logged out successfully"})
		return
	}

	if err := h.authService.RevokeRefreshToken(refreshToken); err != nil {
		errors.WriteError(w, errors.ServiceUnavailable("logout temporarily unavailable", err))
		return
	}
	ClearSessionCookies(w, r)

	response := authtransport.LogoutResponse{
		Message: "logged out successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
