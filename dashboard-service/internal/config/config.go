package config

import "github.com/PetoAdam/homenavi/shared/envx"

type PostgresConfig struct {
	User     string
	Password string
	DBName   string
	Host     string
	Port     string
	SSLMode  string
}

type Config struct {
	Port                string
	JWTPublicKeyPath    string
	IntegrationProxyURL string
	Postgres            PostgresConfig
}

func Load() Config {
	return Config{
		Port:                envx.String("DASHBOARD_SERVICE_PORT", "8097"),
		JWTPublicKeyPath:    envx.String("JWT_PUBLIC_KEY_PATH", ""),
		IntegrationProxyURL: envx.String("INTEGRATION_PROXY_URL", ""),
		Postgres: PostgresConfig{
			User:     envx.String("POSTGRES_USER", "postgres"),
			Password: envx.String("POSTGRES_PASSWORD", "postgres"),
			DBName:   envx.String("POSTGRES_DB", "homenavi"),
			Host:     envx.String("POSTGRES_HOST", "postgres"),
			Port:     envx.String("POSTGRES_PORT", "5432"),
			SSLMode:  envx.String("POSTGRES_SSLMODE", "disable"),
		},
	}
}
