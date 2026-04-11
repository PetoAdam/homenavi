package app

import (
	"log/slog"

	dbinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/envx"
)

// Config holds bootstrap configuration for device-hub.
type Config struct {
	Port          string
	MQTTBrokerURL string
	LogLevel      string
	DB            dbinfra.Config
	RedisAddr     string
	RedisPassword string
	EnableMatter  bool
	EnableThread  bool
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Port:          envx.String("DEVICE_HUB_PORT", "8090"),
		MQTTBrokerURL: envx.String("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
		LogLevel:      envx.String("LOG_LEVEL", "info"),
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
		EnableMatter:  envx.Bool("DEVICE_HUB_ENABLE_MATTER", false),
		EnableThread:  envx.Bool("DEVICE_HUB_ENABLE_THREAD", false),
	}
	slog.Info("device-hub config loaded", "port", cfg.Port, "mqtt", cfg.MQTTBrokerURL)
	return cfg, nil
}
