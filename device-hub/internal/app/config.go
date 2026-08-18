package app

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/PetoAdam/homenavi/shared/redisx"
)

// Config holds bootstrap configuration for device-hub.
type Config struct {
	Port            string
	MQTT            mqttx.Config
	LogLevel        string
	DB              dbx.PostgresConfig
	Redis           redisx.Config
	ListCacheTTL    time.Duration
	MQTTSharedGroup string
}

func LoadConfig() (Config, error) {
	redisConfig, err := redisx.LoadConfig(redisx.Config{Addrs: []string{"redis:6379"}})
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Port:            envx.String("DEVICE_HUB_PORT", "8090"),
		MQTT:            mqttx.LoadConfig("mqtt://emqx:1883"),
		LogLevel:        envx.String("LOG_LEVEL", "info"),
		DB:              dbx.LoadPostgresConfig(dbx.PostgresConfig{User: "postgres", DBName: "homenavi", Host: "postgres", Port: "5432", SSLMode: "disable"}),
		Redis:           redisConfig,
		ListCacheTTL:    envx.Duration("DEVICE_HUB_LIST_CACHE_TTL", 5*time.Second),
		MQTTSharedGroup: strings.TrimSpace(envx.String("DEVICE_HUB_MQTT_SHARED_GROUP", "")),
	}
	slog.Info("device-hub config loaded", "port", cfg.Port, "mqtt", cfg.MQTT.BrokerURL)
	if err := cfg.MQTT.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.MQTTSharedGroup != "" && cfg.MQTT.BrokerKind != mqttx.BrokerKindEMQX {
		return Config{}, fmt.Errorf("DEVICE_HUB_MQTT_SHARED_GROUP requires MQTT_BROKER_KIND=emqx")
	}
	return cfg, nil
}
