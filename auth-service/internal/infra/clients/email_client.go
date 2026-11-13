package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/PetoAdam/homenavi/shared/observability"
)

// EmailClient wraps email-service HTTP calls.
type EmailClient struct {
	baseURL    string
	httpClient *http.Client
	ctx        context.Context
}

func NewEmailClient(baseURL string) *EmailClient {
	return &EmailClient{baseURL: baseURL, httpClient: observability.InstrumentHTTPClient(&http.Client{Timeout: 10 * time.Second})}
}

func (c *EmailClient) WithContext(ctx context.Context) *EmailClient {
	client := *c
	client.ctx = ctx
	return &client
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

	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(request)
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
