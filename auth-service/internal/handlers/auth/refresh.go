package auth

import (
	"encoding/json"
	"net/http"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	"github.com/PetoAdam/homenavi/auth-service/internal/constants"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/PetoAdam/homenavi/auth-service/internal/models/requests"
	"github.com/PetoAdam/homenavi/auth-service/internal/models/responses"
	"github.com/PetoAdam/homenavi/auth-service/pkg/errors"
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
	var req requests.RefreshRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	userID, err := h.authService.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		errors.WriteError(w, errors.Unauthorized("invalid or expired refresh token"))
		return
	}

	// Revoke old refresh token
	h.authService.RevokeRefreshToken(req.RefreshToken)

	// Get user
	user, err := h.userService.GetUser(userID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("user not found"))
		return
	}
	if user.LockoutEnabled {
		errors.WriteError(w, errors.NewAppError(http.StatusLocked, "account locked", nil).
			WithField("reason", constants.ReasonAdminLock))
		return
	}

	// Issue new tokens
	accessToken, err := h.authService.IssueAccessToken(user)
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to issue access token", err))
		return
	}

	refreshToken, err := h.authService.IssueRefreshToken(user.ID)
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to issue refresh token", err))
		return
	}

	response := responses.RefreshResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
