package config

import (
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	Port           string
	MQTTBrokerURL  string
	MQTTClientID   string
	LogLevel       string
	IngestRetained bool
	TopicPrefix    string
	Postgres       DBConfig
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
		Port:           getEnv("HISTORY_SERVICE_PORT", "8093"),
		MQTTBrokerURL:  strings.TrimSpace(os.Getenv("MQTT_BROKER_URL")),
		MQTTClientID:   getEnv("HISTORY_SERVICE_MQTT_CLIENT_ID", "history-service"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		IngestRetained: parseBool(getEnv("HISTORY_INGEST_RETAINED", "false")),
		TopicPrefix:    getEnv("HISTORY_HDP_STATE_PREFIX", "homenavi/hdp/device/state/"),
		Postgres: DBConfig{
			User:     strings.TrimSpace(os.Getenv("POSTGRES_USER")),
			Password: os.Getenv("POSTGRES_PASSWORD"),
			DBName:   strings.TrimSpace(os.Getenv("POSTGRES_DB")),
			Host:     strings.TrimSpace(os.Getenv("POSTGRES_HOST")),
			Port:     strings.TrimSpace(os.Getenv("POSTGRES_PORT")),
			SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		},
	}

	slog.Info("history-service config loaded", "port", cfg.Port, "mqtt", cfg.MQTTBrokerURL, "topic_prefix", cfg.TopicPrefix)
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
