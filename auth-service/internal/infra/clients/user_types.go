package clients

import "time"

type User struct {
	ID                string    `json:"id"`
	UserName          string    `json:"user_name"`
	Email             string    `json:"email"`
	FirstName         string    `json:"first_name"`
	LastName          string    `json:"last_name"`
	Role              string    `json:"role"`
	EmailConfirmed    bool      `json:"email_confirmed"`
	TwoFactorEnabled  bool      `json:"two_factor_enabled"`
	TwoFactorType     string    `json:"two_factor_type"`
	TwoFactorSecret   string    `json:"-"`
	ProfilePictureURL string    `json:"profile_picture_url"`
	GoogleID          string    `json:"google_id,omitempty"`
	LockoutEnabled    bool      `json:"lockout_enabled"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type GoogleUserInfo struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"given_name"`
	LastName  string `json:"family_name"`
	Picture   string `json:"picture"`
}
