package app

import (
	"strings"
	"testing"
)

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

func TestLoadConfigRejectsSharedGroupOnGenericMQTT(t *testing.T) {
	t.Setenv("MQTT_BROKER_KIND", "generic")
	t.Setenv("DEVICE_HUB_MQTT_SHARED_GROUP", "device-hub-ingest")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "DEVICE_HUB_MQTT_SHARED_GROUP") {
		t.Fatalf("expected shared-group broker validation error, got %v", err)
	}
}
