package responses

type TwoFactorSetupResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
	QRCodeURL  string `json:"qr_code_url,omitempty"`
}

type TwoFactorVerifyResponse struct {
	Verified bool   `json:"verified"`
	Message  string `json:"message"`
}

type TwoFactorEmailResponse struct {
	Message  string `json:"message"`
	CodeSent bool   `json:"code_sent"`
}
