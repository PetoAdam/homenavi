package auth

import (
	"encoding/json"
	"net/http"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	authtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/auth/transport"
	sharedtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/transport"
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
	var req authtransport.LogoutRequest
	if err := sharedtransport.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	// Revoke refresh token
	h.authService.RevokeRefreshToken(req.RefreshToken)

	response := authtransport.LogoutResponse{
		Message: "logged out successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
