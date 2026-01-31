package config

import "os"

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

func getenv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func Load() Config {
	return Config{
		Port:                getenv("DASHBOARD_SERVICE_PORT", "8097"),
		JWTPublicKeyPath:    getenv("JWT_PUBLIC_KEY_PATH", ""),
		IntegrationProxyURL: getenv("INTEGRATION_PROXY_URL", ""),
		Postgres: PostgresConfig{
			User:     getenv("POSTGRES_USER", "postgres"),
			Password: getenv("POSTGRES_PASSWORD", "postgres"),
			DBName:   getenv("POSTGRES_DB", "homenavi"),
			Host:     getenv("POSTGRES_HOST", "postgres"),
			Port:     getenv("POSTGRES_PORT", "5432"),
			SSLMode:  getenv("POSTGRES_SSLMODE", "disable"),
		},
	}
}
