package app

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("HISTORY_SERVICE_PORT", "9999")
	t.Setenv("MQTT_BROKER_URL", "mqtt://broker:1883")
	t.Setenv("POSTGRES_USER", "postgres")
	t.Setenv("POSTGRES_DB", "history")
	t.Setenv("POSTGRES_HOST", "db")
	t.Setenv("POSTGRES_PORT", "5432")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Port != "9999" || cfg.MQTT.BrokerURL != "mqtt://broker:1883" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.DB.DBName != "history" || cfg.DB.Host != "db" {
		t.Fatalf("unexpected db config: %#v", cfg.DB)
	}
}
