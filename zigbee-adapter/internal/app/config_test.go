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
	if cfg.AdapterID != "zigbee-adapter" {
		t.Fatalf("expected default adapter id, got %q", cfg.AdapterID)
	}
	if cfg.AdapterVersion != "dev" {
		t.Fatalf("expected default version dev, got %q", cfg.AdapterVersion)
	}
}

func TestLoadConfigLegacyFallbacks(t *testing.T) {
	t.Setenv("ZIGBEE_ADAPTER_PORT", "")
	t.Setenv("DEVICE_HUB_ZIGBEE_PORT", "18181")
	t.Setenv("ZIGBEE_ADAPTER_ID", "")
	t.Setenv("DEVICE_HUB_ZIGBEE_ADAPTER_ID", "legacy-zigbee")
	t.Setenv("ZIGBEE_ADAPTER_VERSION", "")
	t.Setenv("DEVICE_HUB_ZIGBEE_VERSION", "legacy")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Port != "18181" || cfg.AdapterID != "legacy-zigbee" || cfg.AdapterVersion != "legacy" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}
