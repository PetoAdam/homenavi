package app

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/PetoAdam/homenavi/shared/redisx"
)

// Config holds bootstrap configuration for entity-registry-service.
type Config struct {
	Port            string
	MQTT            mqttx.Config
	MQTTSharedGroup string
	DeviceHubURL    string
	AutoImport      bool
	DB              dbx.PostgresConfig
	Redis           redisx.Config
	ListCacheTTL    time.Duration
}

func LoadConfig() (Config, error) {
	redisConfig, err := redisx.LoadConfig(redisx.Config{Addrs: []string{"redis:6379"}})
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Port:            envx.String("ENTITY_REGISTRY_PORT", "8095"),
		MQTT:            mqttx.LoadConfig("mqtt://emqx:1883"),
		MQTTSharedGroup: envx.String("ENTITY_REGISTRY_MQTT_SHARED_GROUP", ""),
		DeviceHubURL:    envx.String("DEVICE_HUB_URL", "http://device-hub:8090"),
		AutoImport:      envx.Bool("ENTITY_REGISTRY_AUTO_IMPORT", true),
		DB:              dbx.LoadPostgresConfig(dbx.PostgresConfig{User: "postgres", Password: "postgres", DBName: "homenavi", Host: "postgres", Port: "5432", SSLMode: "disable"}),
		Redis:           redisConfig,
		ListCacheTTL:    envx.Duration("ERS_LIST_CACHE_TTL", 30*time.Second),
	}
	slog.Info("entity-registry-service config loaded", "port", cfg.Port, "mqtt", cfg.MQTT.BrokerURL, "mqtt_broker_kind", cfg.MQTT.BrokerKind, "mqtt_shared_group", cfg.MQTTSharedGroup, "device_hub", cfg.DeviceHubURL, "auto_import", cfg.AutoImport)
	if err := cfg.MQTT.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.MQTTSharedGroup != "" && cfg.MQTT.BrokerKind != mqttx.BrokerKindEMQX {
		return Config{}, fmt.Errorf("ENTITY_REGISTRY_MQTT_SHARED_GROUP requires MQTT_BROKER_KIND=emqx")
	}
	return cfg, nil
}
