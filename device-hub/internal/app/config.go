package app

import (
	"log/slog"
	"time"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/PetoAdam/homenavi/shared/redisx"
)

// Config holds bootstrap configuration for device-hub.
type Config struct {
	Port         string
	MQTT         mqttx.Config
	LogLevel     string
	DB           dbx.PostgresConfig
	Redis        redisx.Config
	ListCacheTTL time.Duration
}

func LoadConfig() (Config, error) {
	redisConfig, err := redisx.LoadConfig(redisx.Config{Addrs: []string{"redis:6379"}})
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Port:         envx.String("DEVICE_HUB_PORT", "8090"),
		MQTT:         mqttx.LoadConfig("mqtt://emqx:1883"),
		LogLevel:     envx.String("LOG_LEVEL", "info"),
		DB:           dbx.LoadPostgresConfig(dbx.PostgresConfig{User: "postgres", DBName: "homenavi", Host: "postgres", Port: "5432", SSLMode: "disable"}),
		Redis:        redisConfig,
		ListCacheTTL: envx.Duration("DEVICE_HUB_LIST_CACHE_TTL", 5*time.Second),
	}
	slog.Info("device-hub config loaded", "port", cfg.Port, "mqtt", cfg.MQTT.BrokerURL)
	return cfg, nil
}
