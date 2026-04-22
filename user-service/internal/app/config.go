package app

import (
	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
)

// Config holds bootstrap configuration for user-service.
type Config struct {
	Port             string
	JWTPublicKeyPath string
	DB               dbx.PostgresConfig
}

func LoadConfig() Config {
	return Config{
		Port:             envx.String("USER_SERVICE_PORT", "8001"),
		JWTPublicKeyPath: envx.String("JWT_PUBLIC_KEY_PATH", ""),
		DB:               dbx.LoadPostgresConfig(dbx.PostgresConfig{Host: "postgres", User: "postgres", DBName: "homenavi", Port: "5432", SSLMode: "disable"}),
	}
}
