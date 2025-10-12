package config

import (
	"log/slog"
	"os"
)

type Config struct {
	Port          string
	MQTTBrokerURL string
	LogLevel      string
	Postgres      DBConfig
	RedisAddr     string
	RedisPassword string
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
