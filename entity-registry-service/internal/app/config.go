package app

import (
	"log/slog"

	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/envx"
)

// Config holds bootstrap configuration for entity-registry-service.
type Config struct {
	Port          string
	MQTTBrokerURL string
	DeviceHubURL  string
	AutoImport    bool
	DB            dbinfra.Config
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Port:          envx.String("ENTITY_REGISTRY_PORT", "8095"),
		MQTTBrokerURL: envx.String("MQTT_BROKER_URL", "tcp://mosquitto:1883"),
		DeviceHubURL:  envx.String("DEVICE_HUB_URL", "http://device-hub:8090"),
		AutoImport:    envx.Bool("ENTITY_REGISTRY_AUTO_IMPORT", true),
		DB: dbinfra.Config{
			User:     envx.String("POSTGRES_USER", "postgres"),
			Password: envx.String("POSTGRES_PASSWORD", "postgres"),
			DBName:   envx.String("POSTGRES_DB", "homenavi"),
			Host:     envx.String("POSTGRES_HOST", "postgres"),
			Port:     envx.String("POSTGRES_PORT", "5432"),
			SSLMode:  envx.String("POSTGRES_SSLMODE", "disable"),
		},
	}
	slog.Info("entity-registry-service config loaded", "port", cfg.Port, "mqtt", cfg.MQTTBrokerURL, "device_hub", cfg.DeviceHubURL, "auto_import", cfg.AutoImport)
	return cfg, nil
}
