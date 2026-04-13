package app

import "github.com/PetoAdam/homenavi/shared/envx"

// Config holds the bootstrap configuration for email-service.
type Config struct {
	Port         string
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	FromEmail    string
	FromName     string
	AppName      string
}

func LoadConfig() Config {
	return Config{
		Port:         envx.String("EMAIL_SERVICE_PORT", "8002"),
		SMTPHost:     envx.String("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:     envx.String("SMTP_PORT", "587"),
		SMTPUsername: envx.String("SMTP_USERNAME", ""),
		SMTPPassword: envx.String("SMTP_PASSWORD", ""),
		FromEmail:    envx.String("FROM_EMAIL", "noreply@homenavi.org"),
		FromName:     envx.String("FROM_NAME", "Homenavi"),
		AppName:      envx.String("APP_NAME", "Homenavi"),
	}
}
