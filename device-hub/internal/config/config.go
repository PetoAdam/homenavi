package config

import (
	"log/slog"

	"github.com/PetoAdam/homenavi/shared/envx"
)

type Config struct {
	Port          string
	MQTTBrokerURL string
	LogLevel      string
	Postgres      DBConfig
	RedisAddr     string
	RedisPassword string
	EnableMatter  bool
	EnableThread  bool
}

type DBConfig struct {
	User     string
	Password string
	DBName   string
	Host     string
	Port     string
	SSLMode  string
}

func Load() *Config {
	cfg := &Config{
		Port:          envx.String("DEVICE_HUB_PORT", "8090"),
		MQTTBrokerURL: envx.String("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
		LogLevel:      envx.String("LOG_LEVEL", "info"),
		Postgres: DBConfig{
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
	return cfg
}
