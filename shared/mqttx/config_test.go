package mqttx

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("MQTT_BROKER_URL", "mqtt://broker:1883")
	t.Setenv("SERVICE_CLIENT_ID", "service-1")

	cfg := LoadConfig("mqtt://default:1883", "SERVICE_CLIENT_ID")
	if cfg.BrokerURL != "mqtt://broker:1883" {
		t.Fatalf("unexpected broker url: %q", cfg.BrokerURL)
	}
	if cfg.ClientID != "service-1" {
		t.Fatalf("unexpected client id: %q", cfg.ClientID)
	}
}
