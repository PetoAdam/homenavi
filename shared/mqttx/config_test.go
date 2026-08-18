package mqttx

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("MQTT_BROKER_URL", "mqtt://broker:1883")
	t.Setenv("MQTT_BROKER_KIND", "generic")
	t.Setenv("SERVICE_CLIENT_ID", "service-1")

	cfg := LoadConfig("mqtt://default:1883", "SERVICE_CLIENT_ID")
	if cfg.BrokerURL != "mqtt://broker:1883" {
		t.Fatalf("unexpected broker url: %q", cfg.BrokerURL)
	}
	if cfg.BrokerKind != BrokerKindGeneric {
		t.Fatalf("unexpected broker kind: %q", cfg.BrokerKind)
	}
	if cfg.ClientID != "service-1" {
		t.Fatalf("unexpected client id: %q", cfg.ClientID)
	}
}
