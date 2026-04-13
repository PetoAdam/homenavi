package app

import (
	"fmt"
	"strings"

	dbinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/envx"
)

// Config holds bootstrap configuration for automation-service.
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
	DB                  dbinfra.Config
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Port:                envx.String("AUTOMATION_SERVICE_PORT", "8094"),
		LogLevel:            envx.String("LOG_LEVEL", "info"),
		MQTTBrokerURL:       envx.String("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
		MQTTClientID:        envx.String("AUTOMATION_SERVICE_MQTT_CLIENT_ID", ""),
		JWTPublicKeyPath:    envx.String("JWT_PUBLIC_KEY_PATH", ""),
		UserServiceURL:      envx.String("USER_SERVICE_URL", "http://user-service:8001"),
		EmailServiceURL:     envx.String("EMAIL_SERVICE_URL", "http://email-service:8002"),
		ERSServiceURL:       envx.String("ERS_SERVICE_URL", "http://entity-registry-service:8095"),
		IntegrationProxyURL: envx.String("INTEGRATION_PROXY_URL", "http://integration-proxy:8099"),
		DB: dbinfra.Config{
			User:     envx.String("POSTGRES_USER", ""),
			Password: envx.String("POSTGRES_PASSWORD", ""),
			DBName:   envx.String("POSTGRES_DB", ""),
			Host:     envx.String("POSTGRES_HOST", ""),
			Port:     envx.String("POSTGRES_PORT", ""),
			SSLMode:  envx.String("POSTGRES_SSLMODE", "disable"),
		},
	}
	if strings.TrimSpace(cfg.MQTTBrokerURL) == "" {
		return Config{}, fmt.Errorf("MQTT_BROKER_URL is required")
	}
	if strings.TrimSpace(cfg.JWTPublicKeyPath) == "" {
		return Config{}, fmt.Errorf("JWT_PUBLIC_KEY_PATH is required")
	}
	if strings.TrimSpace(cfg.DB.User) == "" || strings.TrimSpace(cfg.DB.DBName) == "" || strings.TrimSpace(cfg.DB.Host) == "" || strings.TrimSpace(cfg.DB.Port) == "" {
		return Config{}, fmt.Errorf("POSTGRES_* configuration is required")
	}
	return cfg, nil
}
