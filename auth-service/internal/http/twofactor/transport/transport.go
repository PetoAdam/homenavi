package transport

import (
	"fmt"

	sharedtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/transport"
)

type TwoFactorSetupRequest struct {
	UserID string `json:"user_id"`
}

func (r *TwoFactorSetupRequest) Validate() error {
	if err := sharedtransport.ValidateRequired(map[string]string{"user_id": r.UserID}); err != nil {
		return err
	}
	if !sharedtransport.IsValidUUID(r.UserID) {
		return fmt.Errorf("invalid user ID format")
	}
	return nil
}

type TwoFactorVerifyRequest struct {
	UserID string `json:"user_id"`
	Code   string `json:"code"`
}

func (r *TwoFactorVerifyRequest) Validate() error {
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

type TwoFactorEmailRequest struct {
	UserID string `json:"user_id"`
}

func (r *TwoFactorEmailRequest) Validate() error {
	if err := sharedtransport.ValidateRequired(map[string]string{"user_id": r.UserID}); err != nil {
		return err
	}
	if !sharedtransport.IsValidUUID(r.UserID) {
		return fmt.Errorf("invalid user ID format")
	}
	return nil
}

type TwoFactorEmailVerifyRequest struct {
	UserID string `json:"user_id"`
	Code   string `json:"code"`
}

func (r *TwoFactorEmailVerifyRequest) Validate() error {
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

type TwoFactorSetupResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
}

type TwoFactorVerifyResponse struct {
	Verified bool   `json:"verified"`
	Message  string `json:"message"`
}

type TwoFactorEmailResponse struct {
	Message  string `json:"message"`
	CodeSent bool   `json:"code_sent"`
}
