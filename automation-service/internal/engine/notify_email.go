package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type notifyEmailRequest struct {
	To       string `json:"to"`
	UserName string `json:"user_name"`
	Subject  string `json:"subject"`
	Message  string `json:"message"`
}

func (e *Engine) sendNotifyEmail(ctx context.Context, to string, userName string, subject string, message string) error {
	if e.emailServiceURL == "" {
		return errors.New("email service url not configured")
	}

	payload := notifyEmailRequest{
		To:       strings.TrimSpace(to),
		UserName: strings.TrimSpace(userName),
		Subject:  strings.TrimSpace(subject),
		Message:  strings.TrimSpace(message),
	}
	b, _ := json.Marshal(payload)

	url := e.emailServiceURL + "/send/notify"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("email-service %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func (e *Engine) executeNotifyEmail(ctx context.Context, a ActionNotifyEmail) error {
	if e.httpClient == nil {
		return errors.New("http client not configured")
	}

	subject := strings.TrimSpace(a.Subject)
	message := strings.TrimSpace(a.Message)
	if subject == "" || message == "" {
		return errors.New("subject and message required")
	}
	if len(a.Recipients) == 0 {
		// We intentionally avoid calling user-service at runtime (no private key / service JWT).
		// Recipients are resolved and stored when the workflow is created/updated.
		return errors.New("notify_email recipients are missing; re-save the workflow to refresh recipients")
	}

	seen := map[string]struct{}{}
	for _, r := range a.Recipients {
		id := strings.TrimSpace(r.UserID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		email := strings.TrimSpace(r.Email)
		name := strings.TrimSpace(r.UserName)
		if email == "" {
			return errors.New("notify_email recipient missing email")
		}
		if name == "" {
			name = "Homenavi user"
		}

		mailCtx, cancel2 := context.WithTimeout(ctx, 10*time.Second)
		err := e.sendNotifyEmail(mailCtx, email, name, subject, message)
		cancel2()
		if err != nil {
			return err
		}
	}
	return nil
}
