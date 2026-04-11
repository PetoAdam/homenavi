package app

import (
	"log/slog"

	"github.com/PetoAdam/homenavi/shared/envx"
	dbinfra "github.com/PetoAdam/homenavi/zigbee-adapter/internal/infra/db"
)

// Config holds bootstrap configuration for zigbee-adapter.
type Config struct {
	Port           string
	MQTTBrokerURL  string
	LogLevel       string
	DB             dbinfra.Config
	RedisAddr      string
	RedisPassword  string
	AdapterID      string
	AdapterVersion string
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Port:           envx.String("ZIGBEE_ADAPTER_PORT", envx.String("DEVICE_HUB_ZIGBEE_PORT", "8091")),
		MQTTBrokerURL:  envx.String("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
		LogLevel:       envx.String("LOG_LEVEL", "info"),
		AdapterID:      envx.String("ZIGBEE_ADAPTER_ID", envx.String("DEVICE_HUB_ZIGBEE_ADAPTER_ID", "zigbee-adapter")),
		AdapterVersion: envx.String("ZIGBEE_ADAPTER_VERSION", envx.String("DEVICE_HUB_ZIGBEE_VERSION", "dev")),
		DB: dbinfra.Config{
			User:     envx.String("POSTGRES_USER", "postgres"),
			Password: envx.String("POSTGRES_PASSWORD", ""),
			DBName:   envx.String("POSTGRES_DB", "homenavi"),
			Host:     envx.String("POSTGRES_HOST", "postgres"),
			Port:     envx.String("POSTGRES_PORT", "5432"),
			SSLMode:  envx.String("POSTGRES_SSLMODE", "disable"),
		},
		RedisAddr:     envx.String("REDIS_ADDR", "redis:6379"),
		RedisPassword: envx.String("REDIS_PASSWORD", ""),
	}
	slog.Info("zigbee-adapter config loaded", "port", cfg.Port, "mqtt", cfg.MQTTBrokerURL, "adapter_id", cfg.AdapterID)
	return cfg, nil
}
