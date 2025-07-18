package auth

import (
	"encoding/json"
	"log"
	"net/http"

	"auth-service/internal/models/requests"
	"auth-service/internal/models/responses"
	"auth-service/internal/services"
	"auth-service/pkg/errors"
)

type SignupHandler struct {
	userService *services.UserService
}

func NewSignupHandler(userService *services.UserService) *SignupHandler {
	return &SignupHandler{
		userService: userService,
	}
}

func (h *SignupHandler) HandleSignup(w http.ResponseWriter, r *http.Request) {
	var req requests.SignupRequest
	if err := requests.ParseAndValidateJSON(r, &req); err != nil {
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

	log.Printf("[INFO] User created successfully: %s", user.ID)
	
	response := responses.UserResponse{
		ID:                user.ID,
		UserName:          user.UserName,
		Email:             user.Email,
		FirstName:         user.FirstName,
		LastName:          user.LastName,
		Role:              user.Role,
		EmailConfirmed:    user.EmailConfirmed,
		TwoFactorEnabled:  user.TwoFactorEnabled,
		TwoFactorType:     user.TwoFactorType,
		ProfilePictureURL: user.ProfilePictureURL,
		CreatedAt:         user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:         user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}
