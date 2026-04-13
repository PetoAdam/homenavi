package app

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("USER_SERVICE_PORT", "9001")
	t.Setenv("JWT_PUBLIC_KEY_PATH", "/tmp/public.pem")
	t.Setenv("POSTGRES_HOST", "db")
	t.Setenv("POSTGRES_USER", "user")

	cfg := LoadConfig()
	if cfg.Port != "9001" || cfg.JWTPublicKeyPath != "/tmp/public.pem" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.DB.Host != "db" || cfg.DB.User != "user" {
		t.Fatalf("unexpected db config: %#v", cfg.DB)
	}
}
