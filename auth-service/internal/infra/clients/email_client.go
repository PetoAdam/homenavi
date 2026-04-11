package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmailClient wraps email-service HTTP calls.
type EmailClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewEmailClient(baseURL string) *EmailClient {
	return &EmailClient{baseURL: baseURL, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *EmailClient) SendVerificationEmail(email, userName, code string) error {
	return c.postJSON("/send/verification", map[string]interface{}{"to": email, "user_name": userName, "code": code})
}

func (c *EmailClient) SendPasswordResetCode(email, userName, code string) error {
	return c.postJSON("/send/password-reset", map[string]interface{}{"to": email, "name": userName, "code": code})
}

func (c *EmailClient) Send2FACode(email, userName, code string) error {
	return c.postJSON("/send/2fa", map[string]interface{}{"to": email, "name": userName, "code": code})
}

func (c *EmailClient) postJSON(path string, payload map[string]interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("email service returned status %d: %s", resp.StatusCode, string(responseBody))
	}
	return nil
}
