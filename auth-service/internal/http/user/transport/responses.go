package transport

import clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"

type UserResponse struct {
	ID                string `json:"id"`
	UserName          string `json:"user_name"`
	Email             string `json:"email"`
	FirstName         string `json:"first_name"`
	LastName          string `json:"last_name"`
	Role              string `json:"role"`
	EmailConfirmed    bool   `json:"email_confirmed"`
	TwoFactorEnabled  bool   `json:"two_factor_enabled"`
	TwoFactorType     string `json:"two_factor_type"`
	ProfilePictureURL string `json:"profile_picture_url"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

func NewUserResponse(user *clientsinfra.User) UserResponse {
	return UserResponse{
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
}

type DeleteUserResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
