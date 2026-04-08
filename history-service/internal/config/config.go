package config

import (
	"log/slog"

	"github.com/PetoAdam/homenavi/shared/envx"
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
		Port:           envx.String("HISTORY_SERVICE_PORT", "8093"),
		MQTTBrokerURL:  envx.String("MQTT_BROKER_URL", ""),
		MQTTClientID:   envx.String("HISTORY_SERVICE_MQTT_CLIENT_ID", ""),
		LogLevel:       envx.String("LOG_LEVEL", "info"),
		IngestRetained: envx.Bool("HISTORY_INGEST_RETAINED", false),
		TopicPrefix:    envx.String("HISTORY_HDP_STATE_PREFIX", "homenavi/hdp/device/state/"),
		Postgres: DBConfig{
			User:     envx.String("POSTGRES_USER", ""),
			Password: envx.String("POSTGRES_PASSWORD", ""),
			DBName:   envx.String("POSTGRES_DB", ""),
			Host:     envx.String("POSTGRES_HOST", ""),
			Port:     envx.String("POSTGRES_PORT", ""),
			SSLMode:  envx.String("POSTGRES_SSLMODE", "disable"),
		},
	}

	slog.Info("history-service config loaded", "port", cfg.Port, "mqtt", cfg.MQTTBrokerURL, "topic_prefix", cfg.TopicPrefix)
	return cfg
}
