package transport

import (
	"fmt"

	sharedtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/transport"
)

type PasswordResetRequest struct {
	Email string `json:"email"`
}

func (r *PasswordResetRequest) Validate() error {
	if err := sharedtransport.ValidateRequired(map[string]string{"email": r.Email}); err != nil {
		return err
	}
	if !sharedtransport.IsValidEmail(r.Email) {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

type PasswordResetConfirmRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

func (r *PasswordResetConfirmRequest) Validate() error {
	if err := sharedtransport.ValidateRequired(map[string]string{
		"email":        r.Email,
		"code":         r.Code,
		"new_password": r.NewPassword,
	}); err != nil {
		return err
	}
	if !sharedtransport.IsValidEmail(r.Email) {
		return fmt.Errorf("invalid email format")
	}
	if !sharedtransport.IsValidCode(r.Code) {
		return fmt.Errorf("code must be exactly 6 digits")
	}
	if !sharedtransport.IsValidPassword(r.NewPassword) {
		return fmt.Errorf("%s", sharedtransport.PasswordPolicyError(r.NewPassword))
	}
	return nil
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (r *ChangePasswordRequest) Validate() error {
	if err := sharedtransport.ValidateRequired(map[string]string{
		"current_password": r.CurrentPassword,
		"new_password":     r.NewPassword,
	}); err != nil {
		return err
	}
	if !sharedtransport.IsValidPassword(r.NewPassword) {
		return fmt.Errorf("%s", sharedtransport.PasswordPolicyError(r.NewPassword))
	}
	if r.CurrentPassword == r.NewPassword {
		return fmt.Errorf("new password must be different from current password")
	}
	return nil
}

type VerificationResponse struct {
	Message  string `json:"message"`
	CodeSent bool   `json:"code_sent"`
}

type SuccessResponse struct {
	Message string `json:"message"`
}
