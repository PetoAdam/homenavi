package services

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"auth-service/internal/config"
)

type EmailService struct {
	config          *config.Config
	emailServiceURL string
	httpClient      *http.Client
}

func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{
		config:          cfg,
		emailServiceURL: cfg.EmailServiceURL,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *EmailService) SendVerificationEmail(email, userName, code string) error {
	req := map[string]interface{}{
		"to":        email,
		"user_name": userName,
		"code":      code,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Post(s.emailServiceURL+"/send/verification", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return err
	}

	return nil
}

func (s *EmailService) SendPasswordResetCode(email, userName, code string) error {
	req := map[string]interface{}{
		"to":   email,
		"name": userName,
		"code": code,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Post(s.emailServiceURL+"/send/password-reset", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return err
	}

	return nil
}

func (s *EmailService) Send2FACode(email, userName, code string) error {
	req := map[string]interface{}{
		"to":   email,
		"name": userName,
		"code": code,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Post(s.emailServiceURL+"/send/2fa", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return err
	}

	return nil
}
