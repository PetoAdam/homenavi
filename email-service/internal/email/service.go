package email

import (
	"fmt"
)

// Sender sends already-rendered email HTML.
type Sender interface {
	Send(to, subject string, html []byte) error
}

// Renderer renders named templates with the provided data.
type Renderer interface {
	Render(templateName string, data EmailData) ([]byte, error)
}

// Service orchestrates rendering and sending application emails.
type Service struct {
	config   Config
	sender   Sender
	renderer Renderer
}

func NewService(config Config, sender Sender, renderer Renderer) *Service {
	return &Service{config: config, sender: sender, renderer: renderer}
}

func (s *Service) SendEmail(to, subject, templateName string, data EmailData) error {
	data.Subject = subject
	if data.AppName == "" {
		data.AppName = s.config.AppName
	}
	html, err := s.renderer.Render(templateName, data)
	if err != nil {
		return fmt.Errorf("render email: %w", err)
	}
	if err := s.sender.Send(to, subject, html); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

func (s *Service) SendVerificationEmail(to, userName, code string) error {
	return s.SendEmail(to, "Verify Your Email Address", "verification", EmailData{
		UserName: userName,
		Code:     code,
		Title:    "Email Verification",
		Message:  "Please use the verification code below to confirm your email address:",
		AppName:  s.config.AppName,
	})
}

func (s *Service) SendPasswordResetEmail(to, userName, code string) error {
	return s.SendEmail(to, "Password Reset Request", "password-reset", EmailData{
		UserName: userName,
		Code:     code,
		Title:    "Reset Your Password",
		Message:  "You requested a password reset. Use the code below to set a new password:",
		AppName:  s.config.AppName,
	})
}

func (s *Service) Send2FAEmail(to, userName, code string) error {
	return s.SendEmail(to, "Two-Factor Authentication Code", "2fa", EmailData{
		UserName: userName,
		Code:     code,
		Title:    "2FA Verification",
		Message:  "Here's your two-factor authentication code:",
		AppName:  s.config.AppName,
	})
}

func (s *Service) SendNotifyEmail(to, userName, subject, message string) error {
	return s.SendEmail(to, subject, "notify", EmailData{
		UserName: userName,
		Subject:  subject,
		Title:    subject,
		Message:  message,
		AppName:  s.config.AppName,
	})
}
