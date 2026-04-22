package app

import (
	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/PetoAdam/homenavi/shared/redisx"
)

// Config holds bootstrap configuration for zigbee-adapter.
type Config struct {
	Port  string
	MQTT  mqttx.Config
	DB    dbx.PostgresConfig
	Redis redisx.Config
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Port:  envx.String("ZIGBEE_ADAPTER_PORT", envx.String("DEVICE_HUB_ZIGBEE_PORT", "8091")),
		MQTT:  mqttx.LoadConfig("mqtt://emqx:1883"),
		DB:    dbx.LoadPostgresConfig(dbx.PostgresConfig{User: "postgres", DBName: "homenavi", Host: "postgres", Port: "5432", SSLMode: "disable"}),
		Redis: redisx.Config{},
	}
	redisConfig, err := redisx.LoadConfig(redisx.Config{Addrs: []string{"redis:6379"}})
	if err != nil {
		return Config{}, err
	}
	cfg.Redis = redisConfig
	return cfg, nil
}
