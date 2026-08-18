package app

import (
	"testing"

	"github.com/PetoAdam/homenavi/shared/mqttx"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("HISTORY_SERVICE_PORT", "9999")
	t.Setenv("MQTT_BROKER_URL", "mqtt://broker:1883")
	t.Setenv("MQTT_BROKER_KIND", "emqx")
	t.Setenv("HISTORY_SERVICE_MQTT_SHARED_GROUP", "history-ingest")
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
	if cfg.MQTT.BrokerKind != mqttx.BrokerKindEMQX {
		t.Fatalf("unexpected mqtt broker kind: %#v", cfg.MQTT)
	}
	if cfg.MQTTSharedGroup != "history-ingest" {
		t.Fatalf("unexpected shared group: %q", cfg.MQTTSharedGroup)
	}
	if cfg.DB.DBName != "history" || cfg.DB.Host != "db" {
		t.Fatalf("unexpected db config: %#v", cfg.DB)
	}
}

func TestLoadConfigRejectsSharedGroupForGenericBroker(t *testing.T) {
	t.Setenv("MQTT_BROKER_URL", "mqtt://broker:1883")
	t.Setenv("MQTT_BROKER_KIND", "generic")
	t.Setenv("HISTORY_SERVICE_MQTT_SHARED_GROUP", "history-ingest")
	t.Setenv("POSTGRES_USER", "postgres")
	t.Setenv("POSTGRES_DB", "history")
	t.Setenv("POSTGRES_HOST", "db")
	t.Setenv("POSTGRES_PORT", "5432")

	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected generic broker shared-group configuration error")
	}
}
