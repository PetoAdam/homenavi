package config

import (
	"log/slog"
	"os"
	"strings"
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
}

func Load() *Config {
	cfg := &Config{
		Port:          getEnv("DEVICE_HUB_PORT", "8090"),
		MQTTBrokerURL: getEnv("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
		Postgres: DBConfig{
			User:     getEnv("POSTGRES_USER", "postgres"),
			Password: os.Getenv("POSTGRES_PASSWORD"),
			DBName:   getEnv("POSTGRES_DB", "homenavi"),
			Host:     getEnv("POSTGRES_HOST", "postgres"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
		},
		RedisAddr:     getEnv("REDIS_ADDR", "redis:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		EnableMatter:  parseBool(getEnv("DEVICE_HUB_ENABLE_MATTER", "false")),
		EnableThread:  parseBool(getEnv("DEVICE_HUB_ENABLE_THREAD", "false")),
	}
	slog.Info("device-hub config loaded", "port", cfg.Port, "mqtt", cfg.MQTTBrokerURL)
	return cfg
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func parseBool(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
