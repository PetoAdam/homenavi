package config

import (
	"github.com/PetoAdam/homenavi/shared/envx"
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

func Load() Config {
	return Config{
		Port:          envx.String("ENTITY_REGISTRY_PORT", "8095"),
		MQTTBrokerURL: envx.String("MQTT_BROKER_URL", "tcp://mosquitto:1883"),
		DeviceHubURL:  envx.String("DEVICE_HUB_URL", "http://device-hub:8090"),
		AutoImport:    envx.Bool("ENTITY_REGISTRY_AUTO_IMPORT", true),
		Postgres: Postgres{
			User:     envx.String("POSTGRES_USER", "postgres"),
			Password: envx.String("POSTGRES_PASSWORD", "postgres"),
			DBName:   envx.String("POSTGRES_DB", "homenavi"),
			Host:     envx.String("POSTGRES_HOST", "postgres"),
			Port:     envx.String("POSTGRES_PORT", "5432"),
			SSLMode:  envx.String("POSTGRES_SSLMODE", "disable"),
		},
	}
}
