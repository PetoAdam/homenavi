package services

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"strconv"

	"email-service/internal/config"

	"github.com/jordan-wright/email"
)

type EmailService struct {
	config *config.Config
}

func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{
		config: cfg,
	}
}

type EmailData struct {
	UserName string
	Code     string
	Subject  string
	Title    string
	Message  string
	AppName  string
}

func (s *EmailService) SendEmail(to, subject, templateName string, data EmailData) error {
	// Load template
	tmpl, err := s.loadTemplate(templateName)
	if err != nil {
		return fmt.Errorf("failed to load template: %v", err)
	}

	// Render HTML
	var htmlBuffer bytes.Buffer
	if err := tmpl.Execute(&htmlBuffer, data); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	// Create email
	e := email.NewEmail()
	e.From = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromEmail)
	e.To = []string{to}
	e.Subject = subject
	e.HTML = htmlBuffer.Bytes()

	// SMTP config for Gmail (requires STARTTLS on port 587)
	port, _ := strconv.Atoi(s.config.SMTPPort)
	auth := smtp.PlainAuth("", s.config.SMTPUsername, s.config.SMTPPassword, s.config.SMTPHost)

	// Send email with STARTTLS (required for Gmail)
	return e.SendWithStartTLS(
		fmt.Sprintf("%s:%d", s.config.SMTPHost, port),
		auth,
		&tls.Config{
			ServerName:         s.config.SMTPHost,
			InsecureSkipVerify: false,
		},
	)
}

func (s *EmailService) SendVerificationEmail(to, userName, code string) error {
	data := EmailData{
		UserName: userName,
		Code:     code,
		Subject:  "Verify Your Email Address",
		Title:    "Email Verification",
		Message:  "Please use the verification code below to confirm your email address:",
		AppName:  "Homenavi",
	}

	return s.SendEmail(to, data.Subject, "verification", data)
}

func (s *EmailService) SendPasswordResetEmail(to, userName, code string) error {
	data := EmailData{
		UserName: userName,
		Code:     code,
		Subject:  "Password Reset Request",
		Title:    "Reset Your Password",
		Message:  "You requested a password reset. Use the code below to set a new password:",
		AppName:  "Homenavi",
	}

	return s.SendEmail(to, data.Subject, "password-reset", data)
}

func (s *EmailService) Send2FAEmail(to, userName, code string) error {
	data := EmailData{
		UserName: userName,
		Code:     code,
		Subject:  "Two-Factor Authentication Code",
		Title:    "2FA Verification",
		Message:  "Here's your two-factor authentication code:",
		AppName:  "Homenavi",
	}

	return s.SendEmail(to, data.Subject, "2fa", data)
}

func (s *EmailService) loadTemplate(templateName string) (*template.Template, error) {
	templateContent := s.getTemplate(templateName)
	return template.New(templateName).Parse(templateContent)
}

func (s *EmailService) getTemplate(templateName string) string {
	// Base template with modern design
	baseTemplate := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Subject}}</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            line-height: 1.6;
            color: #333;
            background-color: #f8fafc;
        }
        .container {
            max-width: 600px;
            margin: 0 auto;
            background-color: #ffffff;
            border-radius: 12px;
            box-shadow: 0 10px 25px rgba(0, 0, 0, 0.1);
            overflow: hidden;
            margin-top: 40px;
            margin-bottom: 40px;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 40px 30px;
            text-align: center;
        }
        .header h1 {
            font-size: 28px;
            font-weight: 600;
            margin-bottom: 10px;
        }
        .header p {
            font-size: 16px;
            opacity: 0.9;
        }
        .content {
            padding: 40px 30px;
        }
        .greeting {
            font-size: 18px;
            margin-bottom: 25px;
            color: #2d3748;
        }
        .message {
            font-size: 16px;
            margin-bottom: 30px;
            color: #4a5568;
            line-height: 1.7;
        }
        .code-container {
            background-color: #f7fafc;
            border: 2px dashed #e2e8f0;
            border-radius: 8px;
            padding: 25px;
            text-align: center;
            margin: 30px 0;
        }
        .code {
            font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace;
            font-size: 32px;
            font-weight: 700;
            color: #2d3748;
            letter-spacing: 4px;
            background-color: #ffffff;
            padding: 15px 25px;
            border-radius: 6px;
            border: 1px solid #e2e8f0;
            display: inline-block;
            margin: 10px 0;
        }
        .code-label {
            font-size: 14px;
            color: #718096;
            margin-bottom: 10px;
            text-transform: uppercase;
            letter-spacing: 1px;
            font-weight: 600;
        }
        .expiry {
            font-size: 14px;
            color: #e53e3e;
            margin-top: 10px;
        }
        .footer {
            background-color: #f7fafc;
            padding: 30px;
            text-align: center;
            border-top: 1px solid #e2e8f0;
        }
        .footer p {
            font-size: 14px;
            color: #718096;
            margin-bottom: 5px;
        }
        .footer a {
            color: #667eea;
            text-decoration: none;
        }
        .security-note {
            background-color: #fef5e7;
            border-left: 4px solid #f6ad55;
            padding: 15px 20px;
            margin: 25px 0;
            border-radius: 0 6px 6px 0;
        }
        .security-note p {
            font-size: 14px;
            color: #744210;
            margin: 0;
        }
        @media (max-width: 600px) {
            .container {
                margin: 20px;
                border-radius: 8px;
            }
            .header, .content, .footer {
                padding: 25px 20px;
            }
            .code {
                font-size: 28px;
                letter-spacing: 3px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>{{.AppName}}</h1>
            <p>{{.Title}}</p>
        </div>
        <div class="content">
            <div class="greeting">
                Hello {{.UserName}},
            </div>
            <div class="message">
                {{.Message}}
            </div>
            <div class="code-container">
                <div class="code-label">Verification Code</div>
                <div class="code">{{.Code}}</div>
                <div class="expiry">⏰ This code expires in 10 minutes</div>
            </div>
            <div class="security-note">
                <p><strong>🔒 Security Notice:</strong> Never share this code with anyone. If you didn't request this, please ignore this email.</p>
            </div>
        </div>
        <div class="footer">
            <p>&copy; 2025 {{.AppName}}. All rights reserved.</p>
            <p>This is an automated message, please do not reply.</p>
        </div>
    </div>
</body>
</html>`

	// For different email types, we can customize specific parts
	switch templateName {
	case "verification":
		return baseTemplate
	case "password-reset":
		return baseTemplate
	case "2fa":
		return baseTemplate
	case "notify":
		return `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Subject}}</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif; line-height: 1.6; color: #333; background-color: #f8fafc; }
        .container { max-width: 600px; margin: 40px auto; background-color: #ffffff; border-radius: 12px; box-shadow: 0 10px 25px rgba(0, 0, 0, 0.1); overflow: hidden; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 32px 28px; text-align: center; }
        .header h1 { font-size: 26px; font-weight: 600; margin: 0; }
        .content { padding: 34px 28px; }
        .greeting { font-size: 18px; margin-bottom: 16px; color: #2d3748; }
        .message { font-size: 16px; color: #4a5568; white-space: pre-wrap; }
        .footer { background-color: #f7fafc; padding: 26px; text-align: center; border-top: 1px solid #e2e8f0; }
        .footer p { font-size: 14px; color: #718096; margin: 0; }
        @media (max-width: 600px) { .container { margin: 20px; border-radius: 8px; } .header, .content, .footer { padding: 22px 18px; } }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>{{.AppName}}</h1>
        </div>
        <div class="content">
            <div class="greeting">Hello {{.UserName}},</div>
            <div class="message">{{.Message}}</div>
        </div>
        <div class="footer">
            <p>This is an automated message, please do not reply.</p>
        </div>
    </div>
</body>
</html>`
	default:
		return baseTemplate
	}
}
