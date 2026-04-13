package app

import "testing"

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Port != "8091" {
		t.Fatalf("expected default port 8091, got %q", cfg.Port)
	}
}

func TestLoadConfigLegacyFallbacks(t *testing.T) {
	t.Setenv("ZIGBEE_ADAPTER_PORT", "")
	t.Setenv("DEVICE_HUB_ZIGBEE_PORT", "18181")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Port != "18181" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}
