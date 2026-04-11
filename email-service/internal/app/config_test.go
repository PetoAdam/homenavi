package app

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("EMAIL_SERVICE_PORT", "9000")
	t.Setenv("SMTP_HOST", "smtp.test")
	t.Setenv("FROM_EMAIL", "noreply@test")
	t.Setenv("APP_NAME", "HN")

	cfg := LoadConfig()
	if cfg.Port != "9000" || cfg.SMTPHost != "smtp.test" || cfg.FromEmail != "noreply@test" || cfg.AppName != "HN" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}
