package users

import (
	"time"

	"github.com/google/uuid"
)

// User is the user domain entity.
type User struct {
	ID                 uuid.UUID  `json:"id"`
	UserName           string     `json:"user_name"`
	NormalizedUserName string     `json:"normalized_user_name"`
	Email              string     `json:"email"`
	NormalizedEmail    string     `json:"normalized_email"`
	FirstName          string     `json:"first_name"`
	LastName           string     `json:"last_name"`
	Role               string     `json:"role"`
	EmailConfirmed     bool       `json:"email_confirmed"`
	ProfilePictureURL  *string    `json:"profile_picture_url"`
	PasswordHash       *string    `json:"password_hash"`
	GoogleID           *string    `json:"google_id"`
	TwoFactorEnabled   bool       `json:"two_factor_enabled"`
	TwoFactorType      string     `json:"two_factor_type"`
	TwoFactorSecret    string     `json:"two_factor_secret"`
	LockoutEnd         *time.Time `json:"lockout_end"`
	LockoutEnabled     bool       `json:"lockout_enabled"`
	AccessFailedCount  int        `json:"access_failed_count"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type Actor struct {
	Subject string
	Role    string
}

type CreateInput struct {
	UserName          string
	Email             string
	Password          string
	FirstName         string
	LastName          string
	GoogleID          *string
	ProfilePictureURL *string
}
