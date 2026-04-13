package users

import (
	"context"

	"github.com/google/uuid"
)

// Repository provides persistence operations for users.
type Repository interface {
	Create(context.Context, *User) error
	FindByID(context.Context, uuid.UUID) (User, error)
	FindByEmail(context.Context, string) (User, error)
	FindByUserName(context.Context, string) (User, error)
	FindByGoogleID(context.Context, string) (User, error)
	List(context.Context, string, int, int) ([]User, int64, error)
	UpdateFields(context.Context, uuid.UUID, map[string]any) error
	Delete(context.Context, uuid.UUID) error
}
