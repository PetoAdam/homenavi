package transport

type ProfilePictureResponse struct {
	Success bool   `json:"success"`
	URL     string `json:"url"`
	Message string `json:"message"`
}

type ProfilePictureUploadURLResponse struct {
	Success   bool              `json:"success"`
	UploadURL string            `json:"upload_url"`
	ObjectKey string            `json:"object_key"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresIn int               `json:"expires_in"`
	Message   string            `json:"message"`
}
