package app

import (
	"time"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/redisx"
)

// Config holds the bootstrap configuration for dashboard-service.
type Config struct {
	Port                string
	JWTPublicKeyPath    string
	IntegrationProxyURL string
	DB                  dbx.PostgresConfig
	Redis               redisx.Config
	ReadCacheTTL        time.Duration
}

func LoadConfig() (Config, error) {
	redisConfig, err := redisx.LoadConfig(redisx.Config{Addrs: []string{"redis:6379"}})
	if err != nil {
		return Config{}, err
	}
	return Config{
		Port:                envx.String("DASHBOARD_SERVICE_PORT", "8097"),
		JWTPublicKeyPath:    envx.String("JWT_PUBLIC_KEY_PATH", ""),
		IntegrationProxyURL: envx.String("INTEGRATION_PROXY_URL", ""),
		DB:                  dbx.LoadPostgresConfig(dbx.PostgresConfig{User: "postgres", Password: "postgres", DBName: "homenavi", Host: "postgres", Port: "5432", SSLMode: "disable"}),
		Redis:               redisConfig,
		ReadCacheTTL:        envx.Duration("DASHBOARD_READ_CACHE_TTL", 30*time.Second),
	}, nil
}
