package user

import (
	"encoding/json"
	"log/slog"
	"net/http"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/PetoAdam/homenavi/auth-service/internal/models/responses"
	"github.com/PetoAdam/homenavi/auth-service/pkg/errors"
)

type DeleteHandler struct {
	authService *authdomain.Service
	userService *clientsinfra.UserClient
}

func NewDeleteHandler(authService *authdomain.Service, userService *clientsinfra.UserClient) *DeleteHandler {
	return &DeleteHandler{
		authService: authService,
		userService: userService,
	}
}

func (h *DeleteHandler) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	// Extract JWT token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		errors.WriteError(w, errors.Unauthorized("missing or invalid authorization header"))
		return
	}

	token := authHeader[7:]
	userID, err := h.authService.ExtractUserIDFromToken(token)
	if err != nil {
		errors.WriteError(w, errors.Unauthorized("invalid token"))
		return
	}

	// Get user to verify they exist
	user, err := h.userService.GetUser(userID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("user not found"))
		return
	}

	// Delete user
	if err := h.userService.DeleteUser(userID, token); err != nil {
		slog.Error("failed to delete user", "user_id", userID, "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to delete user", err))
		return
	}

	slog.Info("user deleted", "user_id", userID, "email", user.Email)

	response := responses.DeleteUserResponse{
		Success: true,
		Message: "User account deleted successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
