package dbx

import (
	"fmt"
	"strings"

	"github.com/PetoAdam/homenavi/shared/envx"
)

func LoadPostgresConfig(defaults PostgresConfig) PostgresConfig {
	return PostgresConfig{
		User:     envx.String("POSTGRES_USER", defaults.User),
		Password: envx.String("POSTGRES_PASSWORD", defaults.Password),
		DBName:   envx.String("POSTGRES_DB", defaults.DBName),
		Host:     envx.String("POSTGRES_HOST", defaults.Host),
		Port:     envx.String("POSTGRES_PORT", defaults.Port),
		SSLMode:  envx.String("POSTGRES_SSLMODE", defaultSSLMode(defaults.SSLMode)),
	}
}

func defaultSSLMode(value string) string {
	if strings.TrimSpace(value) == "" {
		return "disable"
	}
	return value
}

func (cfg PostgresConfig) Validate() error {
	missing := make([]string, 0, 4)
	if strings.TrimSpace(cfg.User) == "" {
		missing = append(missing, "POSTGRES_USER")
	}
	if strings.TrimSpace(cfg.DBName) == "" {
		missing = append(missing, "POSTGRES_DB")
	}
	if strings.TrimSpace(cfg.Host) == "" {
		missing = append(missing, "POSTGRES_HOST")
	}
	if strings.TrimSpace(cfg.Port) == "" {
		missing = append(missing, "POSTGRES_PORT")
	}
	if len(missing) > 0 {
		return fmt.Errorf("postgres configuration is required: %s", strings.Join(missing, ", "))
	}
	return nil
}
