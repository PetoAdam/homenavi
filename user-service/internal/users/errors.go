package users

import "errors"

var (
	ErrNotFound                   = errors.New("user not found")
	ErrDuplicateEmail             = errors.New("user with this email already exists")
	ErrDuplicateUserName          = errors.New("user with this username already exists")
	ErrPasswordOrGoogleIDRequired = errors.New("password or googleid required")
	ErrInvalidCredentials         = errors.New("invalid credentials")
	ErrAccountLocked              = errors.New("account locked")
	ErrForbidden                  = errors.New("forbidden")
	ErrUnauthorized               = errors.New("unauthorized")
	ErrInvalidRoleValue           = errors.New("invalid role value")
	ErrUnsupportedRole            = errors.New("unsupported role")
	ErrCannotChangeRole           = errors.New("insufficient permissions to change role")
	ErrResidentsGrantResidentOnly = errors.New("residents can only grant resident role")
	ErrCannotModifyAdminRole      = errors.New("cannot modify admin role")
	ErrNoValidFields              = errors.New("no valid fields to update")
)
