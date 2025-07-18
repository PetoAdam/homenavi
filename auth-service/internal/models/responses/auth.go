package responses

type LoginResponse struct {
	AccessToken   string `json:"access_token,omitempty"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	TwoFARequired bool   `json:"2fa_required,omitempty"`
	UserID        string `json:"user_id,omitempty"`
	TwoFAType     string `json:"2fa_type,omitempty"`
}

type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type LogoutResponse struct {
	Message string `json:"message"`
}
