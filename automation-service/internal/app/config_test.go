package app

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("AUTOMATION_SERVICE_PORT", "9999")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("MQTT_BROKER_URL", "mqtt://broker:1883")
	t.Setenv("JWT_PUBLIC_KEY_PATH", "/tmp/public.pem")
	t.Setenv("POSTGRES_USER", "postgres")
	t.Setenv("POSTGRES_DB", "automation")
	t.Setenv("POSTGRES_HOST", "db")
	t.Setenv("POSTGRES_PORT", "5432")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Port != "9999" || cfg.LogLevel != "debug" || cfg.MQTTBrokerURL != "mqtt://broker:1883" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.DB.Host != "db" || cfg.DB.DBName != "automation" {
		t.Fatalf("unexpected db config: %#v", cfg.DB)
	}
}
