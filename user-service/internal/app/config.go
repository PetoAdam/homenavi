package app

import (
	"github.com/PetoAdam/homenavi/shared/envx"
	dbinfra "github.com/PetoAdam/homenavi/user-service/internal/infra/db"
)

// Config holds bootstrap configuration for user-service.
type Config struct {
	Port             string
	JWTPublicKeyPath string
	DB               dbinfra.Config
}

func LoadConfig() Config {
	return Config{
		Port:             envx.String("USER_SERVICE_PORT", "8001"),
		JWTPublicKeyPath: envx.String("JWT_PUBLIC_KEY_PATH", ""),
		DB: dbinfra.Config{
			Host:     envx.String("POSTGRES_HOST", "postgres"),
			User:     envx.String("POSTGRES_USER", "postgres"),
			Password: envx.String("POSTGRES_PASSWORD", ""),
			DBName:   envx.String("POSTGRES_DB", "homenavi"),
			Port:     envx.String("POSTGRES_PORT", "5432"),
			SSLMode:  envx.String("POSTGRES_SSLMODE", "disable"),
		},
	}
}
