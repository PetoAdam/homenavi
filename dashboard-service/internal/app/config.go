package app

import (
	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
)

// Config holds the bootstrap configuration for dashboard-service.
type Config struct {
	Port                string
	JWTPublicKeyPath    string
	IntegrationProxyURL string
	DB                  dbx.PostgresConfig
}

func LoadConfig() Config {
	return Config{
		Port:                envx.String("DASHBOARD_SERVICE_PORT", "8097"),
		JWTPublicKeyPath:    envx.String("JWT_PUBLIC_KEY_PATH", ""),
		IntegrationProxyURL: envx.String("INTEGRATION_PROXY_URL", ""),
		DB:                  dbx.LoadPostgresConfig(dbx.PostgresConfig{User: "postgres", Password: "postgres", DBName: "homenavi", Host: "postgres", Port: "5432", SSLMode: "disable"}),
	}
}
