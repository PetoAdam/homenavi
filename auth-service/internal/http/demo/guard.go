package demo

import (
	"strings"

	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

const BlockedMutationMessage = "This action is disabled in the public demo"

func IsDemoUser(user *clientsinfra.User) bool {
	if user == nil {
		return false
	}
	userName := strings.TrimSpace(strings.ToLower(user.UserName))
	email := strings.TrimSpace(strings.ToLower(user.Email))
	return strings.HasPrefix(userName, "demo_") && strings.HasPrefix(email, "demo+") && strings.HasSuffix(email, "@example.com")
}

func MutationBlockedError() *errors.AppError {
	return errors.Forbidden(BlockedMutationMessage)
}
