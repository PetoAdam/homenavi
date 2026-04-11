package app

import "testing"

func TestLoadPostgresSSLModeDefault(t *testing.T) {
	t.Setenv("POSTGRES_SSLMODE", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.DB.SSLMode != "disable" {
		t.Fatalf("expected default sslmode disable, got %q", cfg.DB.SSLMode)
	}
}

func TestLoadPostgresSSLModeOverride(t *testing.T) {
	t.Setenv("POSTGRES_SSLMODE", "require")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.DB.SSLMode != "require" {
		t.Fatalf("expected sslmode override, got %q", cfg.DB.SSLMode)
	}
}
