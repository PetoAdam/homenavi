package config

import "testing"

func TestLoadPostgresSSLModeDefault(t *testing.T) {
	t.Setenv("POSTGRES_SSLMODE", "")

	cfg := Load()

	if cfg.Postgres.SSLMode != "disable" {
		t.Fatalf("expected default sslmode disable, got %q", cfg.Postgres.SSLMode)
	}
}

func TestLoadPostgresSSLModeOverride(t *testing.T) {
	t.Setenv("POSTGRES_SSLMODE", "require")

	cfg := Load()

	if cfg.Postgres.SSLMode != "require" {
		t.Fatalf("expected sslmode override, got %q", cfg.Postgres.SSLMode)
	}
}
