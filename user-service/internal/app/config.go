package app

import (
	"strings"

	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/redisx"
)

// Config holds bootstrap configuration for user-service.
type Config struct {
	Port               string
	JWTPublicKeyPath   string
	JWTPublicKeyPaths  []string
	JWTIssuer          string
	JWTAPIAudience     string
	JWTServiceAudience string
	DB                 dbx.PostgresConfig
	Redis              redisx.Config
}

func LoadConfig() Config {
	publicKeyPath := envx.String("JWT_PUBLIC_KEY_PATH", "")
	return Config{
		Port:               envx.String("USER_SERVICE_PORT", "8001"),
		JWTPublicKeyPath:   publicKeyPath,
		JWTPublicKeyPaths:  jwtPublicKeyPaths(envx.String("JWT_PUBLIC_KEY_PATHS", ""), publicKeyPath),
		JWTIssuer:          envx.String("JWT_ISSUER", authx.DefaultIssuer),
		JWTAPIAudience:     envx.String("JWT_API_AUDIENCE", authx.AudienceAPI),
		JWTServiceAudience: envx.String("JWT_USER_SERVICE_AUDIENCE", authx.AudienceUserService),
		DB:                 dbx.LoadPostgresConfig(dbx.PostgresConfig{Host: "postgres", User: "postgres", DBName: "homenavi", Port: "5432", SSLMode: "disable"}),
		Redis:              redisx.Config{Addrs: []string{"redis:6379"}},
	}
}

func jwtPublicKeyPaths(paths, fallbackPath string) []string {
	if strings.TrimSpace(paths) == "" {
		paths = fallbackPath
	}
	values := strings.Split(paths, ",")
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
