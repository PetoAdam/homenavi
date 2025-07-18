package oauth

import (
	"encoding/json"
	"net/http"

	"auth-service/internal/models/requests"
	"auth-service/internal/models/responses"
	"auth-service/internal/services"
	"auth-service/pkg/errors"
)

type GoogleHandler struct {
	authService *services.AuthService
	userService *services.UserService
}

func NewGoogleHandler(authService *services.AuthService, userService *services.UserService) *GoogleHandler {
	return &GoogleHandler{
		authService: authService,
		userService: userService,
	}
}

func (h *GoogleHandler) HandleOAuthGoogle(w http.ResponseWriter, r *http.Request) {
	var req requests.GoogleOAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.BadRequest("invalid JSON"))
		return
	}

	if err := req.Validate(); err != nil {
		errors.WriteError(w, errors.BadRequest(err.Error()))
		return
	}

	// Exchange Google OAuth code for user information
	userInfo, err := h.authService.ExchangeGoogleOAuthCode(req.Code, req.RedirectURI)
	if err != nil {
		errors.WriteError(w, errors.BadRequest("failed to exchange OAuth code"))
		return
	}

	// Check if user exists or create new user
	user, err := h.userService.GetUserByEmail(userInfo.Email)
	if err != nil {
		// User doesn't exist, create new user using a map instead of the struct
		userReq := map[string]interface{}{
			"user_name":           userInfo.Email,
			"email":               userInfo.Email,
			"first_name":          userInfo.FirstName,
			"last_name":           userInfo.LastName,
			"role":                "user",
			"email_confirmed":     true,
			"profile_picture_url": userInfo.Picture,
		}
		
		user, err = h.userService.CreateUserFromMap(userReq)
		if err != nil {
			errors.WriteError(w, errors.InternalServerError("failed to create user", err))
			return
		}
	}

	// Generate JWT tokens
	accessToken, refreshToken, err := h.authService.GenerateTokens(user.ID)
	if err != nil {
		errors.WriteError(w, errors.InternalServerError("failed to generate tokens", err))
		return
	}

	// Return tokens and user info
	response := responses.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		UserID:       user.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
