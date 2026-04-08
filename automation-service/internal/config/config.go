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
	Port                string
	LogLevel            string
	MQTTBrokerURL       string
	MQTTClientID        string
	JWTPublicKeyPath    string
	UserServiceURL      string
	EmailServiceURL     string
	ERSServiceURL       string
	IntegrationProxyURL string
	Postgres            Postgres
}

func Load() Config {
	return Config{
		Port:                envx.String("AUTOMATION_SERVICE_PORT", "8094"),
		LogLevel:            envx.String("LOG_LEVEL", "info"),
		MQTTBrokerURL:       envx.String("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
		MQTTClientID:        envx.String("AUTOMATION_SERVICE_MQTT_CLIENT_ID", ""),
		JWTPublicKeyPath:    envx.String("JWT_PUBLIC_KEY_PATH", ""),
		UserServiceURL:      envx.String("USER_SERVICE_URL", "http://user-service:8001"),
		EmailServiceURL:     envx.String("EMAIL_SERVICE_URL", "http://email-service:8002"),
		ERSServiceURL:       envx.String("ERS_SERVICE_URL", "http://entity-registry-service:8095"),
		IntegrationProxyURL: envx.String("INTEGRATION_PROXY_URL", "http://integration-proxy:8099"),
		Postgres: Postgres{
			User:     envx.String("POSTGRES_USER", ""),
			Password: envx.String("POSTGRES_PASSWORD", ""),
			DBName:   envx.String("POSTGRES_DB", ""),
			Host:     envx.String("POSTGRES_HOST", ""),
			Port:     envx.String("POSTGRES_PORT", ""),
			SSLMode:  envx.String("POSTGRES_SSLMODE", "disable"),
		},
	}
}
