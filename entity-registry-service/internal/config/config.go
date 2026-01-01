package config

import (
	"os"
	"strings"
)

type Postgres struct {
	User     string
	Password string
	DBName   string
	Host     string
	Port     string
	SSLMode  string
}

type Config struct {
	Port          string
	MQTTBrokerURL string
	DeviceHubURL  string
	AutoImport    bool
	Postgres      Postgres
}

func env(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func Load() Config {
	return Config{
		Port:          env("ENTITY_REGISTRY_PORT", "8095"),
		MQTTBrokerURL: env("MQTT_BROKER_URL", "tcp://mosquitto:1883"),
		DeviceHubURL:  env("DEVICE_HUB_URL", "http://device-hub:8090"),
		AutoImport:    strings.TrimSpace(strings.ToLower(os.Getenv("ENTITY_REGISTRY_AUTO_IMPORT"))) != "false",
		Postgres: Postgres{
			User:     env("POSTGRES_USER", "postgres"),
			Password: env("POSTGRES_PASSWORD", "postgres"),
			DBName:   env("POSTGRES_DB", "homenavi"),
			Host:     env("POSTGRES_HOST", "postgres"),
			Port:     env("POSTGRES_PORT", "5432"),
			SSLMode:  env("POSTGRES_SSLMODE", "disable"),
		},
	}
}
