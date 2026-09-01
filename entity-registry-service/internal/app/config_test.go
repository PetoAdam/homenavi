package app

import (
	"testing"

	"github.com/PetoAdam/homenavi/shared/mqttx"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("ENTITY_REGISTRY_PORT", "9999")
	t.Setenv("MQTT_BROKER_URL", "mqtt://broker:1883")
	t.Setenv("MQTT_BROKER_KIND", "emqx")
	t.Setenv("ENTITY_REGISTRY_MQTT_SHARED_GROUP", "entity-registry-autoimport")
	t.Setenv("DEVICE_HUB_URL", "http://device-hub:8090")
	t.Setenv("ENTITY_REGISTRY_AUTO_IMPORT", "false")
	t.Setenv("POSTGRES_USER", "postgres")
	t.Setenv("POSTGRES_PASSWORD", "secret")
	t.Setenv("POSTGRES_DB", "entity_registry")
	t.Setenv("POSTGRES_HOST", "db")
	t.Setenv("POSTGRES_PORT", "5432")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Port != "9999" || cfg.MQTT.BrokerURL != "mqtt://broker:1883" || cfg.DeviceHubURL != "http://device-hub:8090" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.MQTT.BrokerKind != mqttx.BrokerKindEMQX {
		t.Fatalf("unexpected mqtt config: %#v", cfg.MQTT)
	}
	if cfg.MQTTSharedGroup != "entity-registry-autoimport" {
		t.Fatalf("unexpected shared group: %q", cfg.MQTTSharedGroup)
	}
	if cfg.AutoImport {
		t.Fatalf("expected auto import to be disabled")
	}
	if cfg.DB.DBName != "entity_registry" || cfg.DB.Host != "db" {
		t.Fatalf("unexpected db config: %#v", cfg.DB)
	}
}

func TestLoadConfigRejectsSharedGroupForGenericBroker(t *testing.T) {
	t.Setenv("MQTT_BROKER_URL", "mqtt://broker:1883")
	t.Setenv("MQTT_BROKER_KIND", "generic")
	t.Setenv("ENTITY_REGISTRY_MQTT_SHARED_GROUP", "entity-registry-autoimport")
	t.Setenv("POSTGRES_USER", "postgres")
	t.Setenv("POSTGRES_PASSWORD", "secret")
	t.Setenv("POSTGRES_DB", "entity_registry")
	t.Setenv("POSTGRES_HOST", "db")
	t.Setenv("POSTGRES_PORT", "5432")

	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected generic broker shared-group configuration error")
	}
}
