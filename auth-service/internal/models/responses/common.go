package responses

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}

type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type VerificationResponse struct {
	Message  string `json:"message"`
	CodeSent bool   `json:"code_sent"`
}
