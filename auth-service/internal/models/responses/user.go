package responses

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

type ProfilePictureResponse struct {
	Success bool   `json:"success"`
	URL     string `json:"url"`
	Message string `json:"message"`
}

type DeleteUserResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
