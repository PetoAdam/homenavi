package http

type verificationEmailRequest struct {
	To       string `json:"to"`
	UserName string `json:"user_name"`
	Code     string `json:"code"`
}

type passwordResetEmailRequest struct {
	To   string `json:"to"`
	Name string `json:"name"`
	Code string `json:"code"`
}

type twoFactorEmailRequest struct {
	To   string `json:"to"`
	Name string `json:"name"`
	Code string `json:"code"`
}

type notifyEmailRequest struct {
	To       string `json:"to"`
	UserName string `json:"user_name"`
	Subject  string `json:"subject"`
	Message  string `json:"message"`
}
