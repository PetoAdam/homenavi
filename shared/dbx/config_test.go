package dbx

import "testing"

func TestLoadPostgresConfigDefaultsSSLMode(t *testing.T) {
	cfg := LoadPostgresConfig(PostgresConfig{SSLMode: ""})
	if cfg.SSLMode != "disable" {
		t.Fatalf("expected default sslmode disable, got %q", cfg.SSLMode)
	}
}

func TestPostgresConfigValidate(t *testing.T) {
	err := (PostgresConfig{User: "postgres", DBName: "homenavi", Host: "db", Port: "5432"}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
