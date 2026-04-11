package transport

import (
	"fmt"

	sharedtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/transport"
)

type EmailVerifyRequest struct {
	UserID string `json:"user_id"`
}

func (r *EmailVerifyRequest) Validate() error {
	if err := sharedtransport.ValidateRequired(map[string]string{"user_id": r.UserID}); err != nil {
		return err
	}
	if !sharedtransport.IsValidUUID(r.UserID) {
		return fmt.Errorf("invalid user ID format")
	}
	return nil
}

type EmailVerifyConfirmRequest struct {
	UserID string `json:"user_id"`
	Code   string `json:"code"`
}

func (r *EmailVerifyConfirmRequest) Validate() error {
	if err := sharedtransport.ValidateRequired(map[string]string{
		"user_id": r.UserID,
		"code":    r.Code,
	}); err != nil {
		return err
	}
	if !sharedtransport.IsValidUUID(r.UserID) {
		return fmt.Errorf("invalid user ID format")
	}
	if !sharedtransport.IsValidCode(r.Code) {
		return fmt.Errorf("code must be exactly 6 digits")
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
