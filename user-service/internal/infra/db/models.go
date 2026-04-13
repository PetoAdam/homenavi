package db

import (
	"time"

	"github.com/PetoAdam/homenavi/user-service/internal/users"
	"github.com/google/uuid"
)

type userRow struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserName           string    `gorm:"uniqueIndex"`
	NormalizedUserName string
	Email              string `gorm:"uniqueIndex"`
	NormalizedEmail    string
	FirstName          string
	LastName           string
	Role               string `gorm:"type:varchar(16);default:'user'"`
	EmailConfirmed     bool
	ProfilePictureURL  *string `gorm:"type:varchar(255)"`
	PasswordHash       *string
	GoogleID           *string `gorm:"uniqueIndex"`
	TwoFactorEnabled   bool
	TwoFactorType      string `gorm:"type:varchar(16)"`
	TwoFactorSecret    string `gorm:"type:varchar(64)"`
	LockoutEnd         *time.Time
	LockoutEnabled     bool
	AccessFailedCount  int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type emailVerificationRow struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;index"`
	Code      string    `gorm:"size:16;index"`
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

func toUserRow(user users.User) userRow {
	return userRow{
		ID:                 user.ID,
		UserName:           user.UserName,
		NormalizedUserName: user.NormalizedUserName,
		Email:              user.Email,
		NormalizedEmail:    user.NormalizedEmail,
		FirstName:          user.FirstName,
		LastName:           user.LastName,
		Role:               user.Role,
		EmailConfirmed:     user.EmailConfirmed,
		ProfilePictureURL:  user.ProfilePictureURL,
		PasswordHash:       user.PasswordHash,
		GoogleID:           user.GoogleID,
		TwoFactorEnabled:   user.TwoFactorEnabled,
		TwoFactorType:      user.TwoFactorType,
		TwoFactorSecret:    user.TwoFactorSecret,
		LockoutEnd:         user.LockoutEnd,
		LockoutEnabled:     user.LockoutEnabled,
		AccessFailedCount:  user.AccessFailedCount,
		CreatedAt:          user.CreatedAt,
		UpdatedAt:          user.UpdatedAt,
	}
}

func toUser(row userRow) users.User {
	return users.User{
		ID:                 row.ID,
		UserName:           row.UserName,
		NormalizedUserName: row.NormalizedUserName,
		Email:              row.Email,
		NormalizedEmail:    row.NormalizedEmail,
		FirstName:          row.FirstName,
		LastName:           row.LastName,
		Role:               row.Role,
		EmailConfirmed:     row.EmailConfirmed,
		ProfilePictureURL:  row.ProfilePictureURL,
		PasswordHash:       row.PasswordHash,
		GoogleID:           row.GoogleID,
		TwoFactorEnabled:   row.TwoFactorEnabled,
		TwoFactorType:      row.TwoFactorType,
		TwoFactorSecret:    row.TwoFactorSecret,
		LockoutEnd:         row.LockoutEnd,
		LockoutEnabled:     row.LockoutEnabled,
		AccessFailedCount:  row.AccessFailedCount,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}
