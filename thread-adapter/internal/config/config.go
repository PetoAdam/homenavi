package config

import (
	"log/slog"

	"github.com/PetoAdam/homenavi/shared/envx"
)

type Config struct {
	Port          string
	MQTTBrokerURL string
	LogLevel      string
	AdapterID     string
	Version       string
}

func Load() *Config {
	cfg := &Config{
		Port:          envx.String("THREAD_ADAPTER_PORT", "8092"),
		MQTTBrokerURL: envx.String("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
		LogLevel:      envx.String("LOG_LEVEL", "info"),
		AdapterID:     envx.String("THREAD_ADAPTER_ID", "thread-adapter-1"),
		Version:       envx.String("THREAD_ADAPTER_VERSION", "dev"),
	}
	slog.Info("thread-adapter config loaded", "port", cfg.Port, "mqtt", cfg.MQTTBrokerURL, "adapter_id", cfg.AdapterID, "version", cfg.Version)
	return cfg
}
