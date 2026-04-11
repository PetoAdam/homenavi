package app

import (
	"time"

	"github.com/PetoAdam/homenavi/shared/envx"
)

// Config holds bootstrap configuration for integration-proxy.
type Config struct {
	ListenAddr          string
	ConfigPath          string
	SchemaPath          string
	RefreshInterval     time.Duration
	UpdateCheckInterval time.Duration
	JWTPublicKeyPath    string
}

func LoadConfig() Config {
	return Config{
		ListenAddr:          envx.String("INTEGRATION_PROXY_LISTEN_ADDR", ":8099"),
		ConfigPath:          envx.String("INTEGRATIONS_CONFIG_PATH", "/config/integrations.yaml"),
		SchemaPath:          envx.String("INTEGRATIONS_SCHEMA_PATH", "/config/homenavi-integration.schema.json"),
		RefreshInterval:     envx.Duration("INTEGRATIONS_REFRESH_INTERVAL", 30*time.Second),
		UpdateCheckInterval: envx.Duration("INTEGRATIONS_UPDATE_CHECK_INTERVAL", 15*time.Minute),
		JWTPublicKeyPath:    envx.String("JWT_PUBLIC_KEY_PATH", ""),
	}
}
