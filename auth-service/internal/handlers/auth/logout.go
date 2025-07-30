package auth

import (
	"encoding/json"
	"net/http"

	"auth-service/internal/models/requests"
	"auth-service/internal/models/responses"
	"auth-service/internal/services"
	"auth-service/pkg/errors"
)

type LogoutHandler struct {
	authService *services.AuthService
}

func NewLogoutHandler(authService *services.AuthService) *LogoutHandler {
	return &LogoutHandler{
		authService: authService,
	}
}

func (h *LogoutHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	var req requests.LogoutRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	// Revoke refresh token
	h.authService.RevokeRefreshToken(req.RefreshToken)

	response := responses.LogoutResponse{
		Message: "logged out successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
