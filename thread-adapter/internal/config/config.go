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
	AdapterID     string
	Version       string
}

func Load() *Config {
	cfg := &Config{
		Port:          getEnv("THREAD_ADAPTER_PORT", "8092"),
		MQTTBrokerURL: getEnv("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
		AdapterID:     getEnv("THREAD_ADAPTER_ID", "thread-adapter-1"),
		Version:       getEnv("THREAD_ADAPTER_VERSION", "dev"),
	}
	slog.Info("thread-adapter config loaded", "port", cfg.Port, "mqtt", cfg.MQTTBrokerURL, "adapter_id", cfg.AdapterID, "version", cfg.Version)
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
