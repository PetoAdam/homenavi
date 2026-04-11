package users

import (
	"time"

	"github.com/google/uuid"
)

// User is the persisted user aggregate.
type User struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserName           string     `gorm:"uniqueIndex" json:"user_name"`
	NormalizedUserName string     `json:"normalized_user_name"`
	Email              string     `gorm:"uniqueIndex" json:"email"`
	NormalizedEmail    string     `json:"normalized_email"`
	FirstName          string     `json:"first_name"`
	LastName           string     `json:"last_name"`
	Role               string     `json:"role" gorm:"type:varchar(16);default:'user'"`
	EmailConfirmed     bool       `json:"email_confirmed"`
	ProfilePictureURL  *string    `json:"profile_picture_url" gorm:"type:varchar(255)"`
	PasswordHash       *string    `json:"password_hash"`
	GoogleID           *string    `gorm:"uniqueIndex" json:"google_id"`
	TwoFactorEnabled   bool       `json:"two_factor_enabled"`
	TwoFactorType      string     `json:"two_factor_type" gorm:"type:varchar(16)"`
	TwoFactorSecret    string     `json:"two_factor_secret" gorm:"type:varchar(64)"`
	LockoutEnd         *time.Time `json:"lockout_end"`
	LockoutEnabled     bool       `json:"lockout_enabled"`
	AccessFailedCount  int        `json:"access_failed_count"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type EmailVerification struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;index"`
	Code      string    `gorm:"size:16;index"`
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
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
	RequestedRole     string
	GoogleID          *string
	ProfilePictureURL *string
}
