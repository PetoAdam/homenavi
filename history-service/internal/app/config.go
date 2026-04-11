package app

import (
	"fmt"
	"log/slog"
	"strings"

	dbinfra "github.com/PetoAdam/homenavi/history-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/envx"
)

// Config holds bootstrap configuration for history-service.
type Config struct {
	Port           string
	MQTTBrokerURL  string
	MQTTClientID   string
	LogLevel       string
	IngestRetained bool
	TopicPrefix    string
	DB             dbinfra.Config
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Port:           envx.String("HISTORY_SERVICE_PORT", "8093"),
		MQTTBrokerURL:  envx.String("MQTT_BROKER_URL", ""),
		MQTTClientID:   envx.String("HISTORY_SERVICE_MQTT_CLIENT_ID", ""),
		LogLevel:       envx.String("LOG_LEVEL", "info"),
		IngestRetained: envx.Bool("HISTORY_INGEST_RETAINED", false),
		TopicPrefix:    envx.String("HISTORY_HDP_STATE_PREFIX", "homenavi/hdp/device/state/"),
		DB: dbinfra.Config{
			User:     envx.String("POSTGRES_USER", ""),
			Password: envx.String("POSTGRES_PASSWORD", ""),
			DBName:   envx.String("POSTGRES_DB", ""),
			Host:     envx.String("POSTGRES_HOST", ""),
			Port:     envx.String("POSTGRES_PORT", ""),
			SSLMode:  envx.String("POSTGRES_SSLMODE", "disable"),
		},
	}
	slog.Info("history-service config loaded", "port", cfg.Port, "mqtt", cfg.MQTTBrokerURL, "topic_prefix", cfg.TopicPrefix)
	if strings.TrimSpace(cfg.MQTTBrokerURL) == "" {
		return Config{}, fmt.Errorf("MQTT_BROKER_URL is required")
	}
	if strings.TrimSpace(cfg.DB.User) == "" || strings.TrimSpace(cfg.DB.DBName) == "" || strings.TrimSpace(cfg.DB.Host) == "" || strings.TrimSpace(cfg.DB.Port) == "" {
		return Config{}, fmt.Errorf("POSTGRES_* configuration is required")
	}
	return cfg, nil
}
