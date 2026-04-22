package app

import (
	"log/slog"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
)

// Config holds bootstrap configuration for entity-registry-service.
type Config struct {
	Port         string
	MQTT         mqttx.Config
	DeviceHubURL string
	AutoImport   bool
	DB           dbx.PostgresConfig
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Port:         envx.String("ENTITY_REGISTRY_PORT", "8095"),
		MQTT:         mqttx.LoadConfig("mqtt://emqx:1883"),
		DeviceHubURL: envx.String("DEVICE_HUB_URL", "http://device-hub:8090"),
		AutoImport:   envx.Bool("ENTITY_REGISTRY_AUTO_IMPORT", true),
		DB:           dbx.LoadPostgresConfig(dbx.PostgresConfig{User: "postgres", Password: "postgres", DBName: "homenavi", Host: "postgres", Port: "5432", SSLMode: "disable"}),
	}
	slog.Info("entity-registry-service config loaded", "port", cfg.Port, "mqtt", cfg.MQTT.BrokerURL, "device_hub", cfg.DeviceHubURL, "auto_import", cfg.AutoImport)
	return cfg, nil
}
