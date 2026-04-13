package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	authtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/auth/transport"
	sharedtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/transport"
	usertransport "github.com/PetoAdam/homenavi/auth-service/internal/http/user/transport"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

type SignupHandler struct {
	userService *clientsinfra.UserClient
}

func NewSignupHandler(userService *clientsinfra.UserClient) *SignupHandler {
	return &SignupHandler{
		userService: userService,
	}
}

func (h *SignupHandler) HandleSignup(w http.ResponseWriter, r *http.Request) {
	var req authtransport.SignupRequest
	if err := sharedtransport.ParseAndValidateJSON(r, &req); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	user, err := h.userService.CreateUser(&req)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.WriteError(w, appErr)
			return
		}
		errors.WriteError(w, errors.InternalServerError("failed to create user", err))
		return
	}

	slog.Info("user created", "user_id", user.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(usertransport.NewUserResponse(user))
}
