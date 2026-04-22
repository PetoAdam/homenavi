package app

import (
	"log/slog"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
)

// Config holds bootstrap configuration for history-service.
type Config struct {
	Port           string
	MQTT           mqttx.Config
	LogLevel       string
	IngestRetained bool
	TopicPrefix    string
	DB             dbx.PostgresConfig
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Port:           envx.String("HISTORY_SERVICE_PORT", "8093"),
		MQTT:           mqttx.LoadConfig("", "HISTORY_SERVICE_MQTT_CLIENT_ID"),
		LogLevel:       envx.String("LOG_LEVEL", "info"),
		IngestRetained: envx.Bool("HISTORY_INGEST_RETAINED", false),
		TopicPrefix:    envx.String("HISTORY_HDP_STATE_PREFIX", "homenavi/hdp/device/state/"),
		DB:             dbx.LoadPostgresConfig(dbx.PostgresConfig{SSLMode: "disable"}),
	}
	slog.Info("history-service config loaded", "port", cfg.Port, "mqtt", cfg.MQTT.BrokerURL, "topic_prefix", cfg.TopicPrefix)
	if err := cfg.MQTT.Validate(); err != nil {
		return Config{}, err
	}
	if err := cfg.DB.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
