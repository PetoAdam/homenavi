package app

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("THREAD_ADAPTER_PORT", "9999")
	t.Setenv("MQTT_BROKER_URL", "mqtt://broker:1883")
	t.Setenv("THREAD_ADAPTER_ID", "adapter-x")
	t.Setenv("THREAD_ADAPTER_VERSION", "1.2.3")

	cfg := LoadConfig()
	if cfg.Port != "9999" || cfg.MQTT.BrokerURL != "mqtt://broker:1883" || cfg.AdapterID != "adapter-x" || cfg.Version != "1.2.3" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}
