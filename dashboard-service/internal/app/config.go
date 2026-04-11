package app

import (
	dbinfra "github.com/PetoAdam/homenavi/dashboard-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/envx"
)

// Config holds the bootstrap configuration for dashboard-service.
type Config struct {
	Port                string
	JWTPublicKeyPath    string
	IntegrationProxyURL string
	DB                  dbinfra.Config
}

func LoadConfig() Config {
	return Config{
		Port:                envx.String("DASHBOARD_SERVICE_PORT", "8097"),
		JWTPublicKeyPath:    envx.String("JWT_PUBLIC_KEY_PATH", ""),
		IntegrationProxyURL: envx.String("INTEGRATION_PROXY_URL", ""),
		DB: dbinfra.Config{
			User:     envx.String("POSTGRES_USER", "postgres"),
			Password: envx.String("POSTGRES_PASSWORD", "postgres"),
			DBName:   envx.String("POSTGRES_DB", "homenavi"),
			Host:     envx.String("POSTGRES_HOST", "postgres"),
			Port:     envx.String("POSTGRES_PORT", "5432"),
			SSLMode:  envx.String("POSTGRES_SSLMODE", "disable"),
		},
	}
}
