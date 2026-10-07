package transport

import (
	"fmt"

	sharedtransport "github.com/PetoAdam/homenavi/auth-service/internal/http/transport"
)

type TwoFactorSetupRequest struct {
}

func (r *TwoFactorSetupRequest) Validate() error {
	return nil
}

type TwoFactorVerifyRequest struct {
	Code string `json:"code"`
}

func (r *TwoFactorVerifyRequest) Validate() error {
	if err := sharedtransport.ValidateRequired(map[string]string{
		"code": r.Code,
	}); err != nil {
		return err
	}
	if !sharedtransport.IsValidCode(r.Code) {
		return fmt.Errorf("code must be exactly 6 digits")
	}
	return nil
}

type TwoFactorEmailRequest struct {
}

func (r *TwoFactorEmailRequest) Validate() error {
	return nil
}

type TwoFactorEmailVerifyRequest struct {
	Code string `json:"code"`
}

func (r *TwoFactorEmailVerifyRequest) Validate() error {
	if err := sharedtransport.ValidateRequired(map[string]string{
		"code": r.Code,
	}); err != nil {
		return err
	}
	if !sharedtransport.IsValidCode(r.Code) {
		return fmt.Errorf("code must be exactly 6 digits")
	}
	return nil
}

type TwoFactorSetupResponse struct {
	Secret        string `json:"secret"`
	OTPAuthURL    string `json:"otpauth_url"`
	QRCodeDataURL string `json:"qr_code_data_url"`
}

type TwoFactorVerifyResponse struct {
	Verified      bool     `json:"verified"`
	Message       string   `json:"message"`
	RecoveryCodes []string `json:"recovery_codes,omitempty"`
}

type TwoFactorEmailResponse struct {
	Message  string `json:"message"`
	CodeSent bool   `json:"code_sent"`
}
