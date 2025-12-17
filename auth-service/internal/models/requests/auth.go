package requests

import (
	"fmt"
)

type SignupRequest struct {
	UserName  string `json:"user_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func (r *SignupRequest) Validate() error {
	if err := ValidateRequired(map[string]string{
		"user_name":  r.UserName,
		"email":      r.Email,
		"password":   r.Password,
		"first_name": r.FirstName,
		"last_name":  r.LastName,
	}); err != nil {
		return err
	}

	if !IsValidUserName(r.UserName) {
		return fmt.Errorf("username must be 3-50 characters long and contain only letters, numbers, underscores, and hyphens")
	}

	if !IsValidEmail(r.Email) {
		return fmt.Errorf("invalid email format")
	}

	if !IsValidPassword(r.Password) {
		return fmt.Errorf("%s", PasswordPolicyError(r.Password))
	}

	if !IsValidName(r.FirstName) {
		return fmt.Errorf("first name must be 1-100 characters and contain only letters, spaces, apostrophes, and hyphens")
	}

	if !IsValidName(r.LastName) {
		return fmt.Errorf("last name must be 1-100 characters and contain only letters, spaces, apostrophes, and hyphens")
	}

	return nil
}

type LoginStartRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r *LoginStartRequest) Validate() error {
	if err := ValidateRequired(map[string]string{
		"email":    r.Email,
		"password": r.Password,
	}); err != nil {
		return err
	}

	if !IsValidEmail(r.Email) {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

type LoginFinishRequest struct {
	UserID string `json:"user_id"`
	Code   string `json:"code"`
}

func (r *LoginFinishRequest) Validate() error {
	if err := ValidateRequired(map[string]string{
		"user_id": r.UserID,
		"code":    r.Code,
	}); err != nil {
		return err
	}

	if !IsValidUUID(r.UserID) {
		return fmt.Errorf("invalid user ID format")
	}

	if !IsValidCode(r.Code) {
		return fmt.Errorf("code must be exactly 6 digits")
	}

	return nil
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (r *RefreshRequest) Validate() error {
	return ValidateRequired(map[string]string{
		"refresh_token": r.RefreshToken,
	})
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (r *LogoutRequest) Validate() error {
	return ValidateRequired(map[string]string{
		"refresh_token": r.RefreshToken,
	})
}
