package app

import (
	"log/slog"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
)

// Config holds bootstrap configuration for device-hub.
type Config struct {
	Port     string
	MQTT     mqttx.Config
	LogLevel string
	DB       dbx.PostgresConfig
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Port:     envx.String("DEVICE_HUB_PORT", "8090"),
		MQTT:     mqttx.LoadConfig("mqtt://emqx:1883"),
		LogLevel: envx.String("LOG_LEVEL", "info"),
		DB:       dbx.LoadPostgresConfig(dbx.PostgresConfig{User: "postgres", DBName: "homenavi", Host: "postgres", Port: "5432", SSLMode: "disable"}),
	}
	slog.Info("device-hub config loaded", "port", cfg.Port, "mqtt", cfg.MQTT.BrokerURL)
	return cfg, nil
}
