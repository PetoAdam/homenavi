package entities

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
	TwoFactorSecret   string    `json:"-"` // never serialized
	ProfilePictureURL string    `json:"profile_picture_url"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
