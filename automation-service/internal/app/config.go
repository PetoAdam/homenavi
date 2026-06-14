package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/PetoAdam/homenavi/shared/redisx"
)

// Config holds bootstrap configuration for automation-service.
type Config struct {
	Port                string
	LogLevel            string
	MQTT                mqttx.Config
	JWTPublicKeyPath    string
	UserServiceURL      string
	EmailServiceURL     string
	ERSServiceURL       string
	IntegrationProxyURL string
	DB                  dbx.PostgresConfig
	Redis               redisx.Config
	ListCacheTTL        time.Duration
}

func LoadConfig() (Config, error) {
	redisConfig, err := redisx.LoadConfig(redisx.Config{Addrs: []string{"redis:6379"}})
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Port:                envx.String("AUTOMATION_SERVICE_PORT", "8094"),
		LogLevel:            envx.String("LOG_LEVEL", "info"),
		MQTT:                mqttx.LoadConfig("mqtt://emqx:1883", "AUTOMATION_SERVICE_MQTT_CLIENT_ID"),
		JWTPublicKeyPath:    envx.String("JWT_PUBLIC_KEY_PATH", ""),
		UserServiceURL:      envx.String("USER_SERVICE_URL", "http://user-service:8001"),
		EmailServiceURL:     envx.String("EMAIL_SERVICE_URL", "http://email-service:8002"),
		ERSServiceURL:       envx.String("ERS_SERVICE_URL", "http://entity-registry-service:8095"),
		IntegrationProxyURL: envx.String("INTEGRATION_PROXY_URL", "http://integration-proxy:8099"),
		DB:                  dbx.LoadPostgresConfig(dbx.PostgresConfig{SSLMode: "disable"}),
		Redis:               redisConfig,
		ListCacheTTL:        envx.Duration("AUTOMATION_WORKFLOW_CACHE_TTL", 30*time.Second),
	}
	if err := cfg.MQTT.Validate(); err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(cfg.JWTPublicKeyPath) == "" {
		return Config{}, fmt.Errorf("JWT_PUBLIC_KEY_PATH is required")
	}
	if err := cfg.DB.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
