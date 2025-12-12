package config

import (
    "log/slog"
    "os"
    "strings"
)

type Config struct {
    Port          string
    MQTTBrokerURL string
    LogLevel      string
    Postgres      DBConfig
    RedisAddr     string
    RedisPassword string
    AdapterID     string
    AdapterVersion string
}

type DBConfig struct {
    User     string
    Password string
    DBName   string
    Host     string
    Port     string
}

func Load() *Config {
    cfg := &Config{
        Port:           getEnv("ZIGBEE_ADAPTER_PORT", getEnv("DEVICE_HUB_ZIGBEE_PORT", "8091")),
        MQTTBrokerURL:  getEnv("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
        LogLevel:       getEnv("LOG_LEVEL", "info"),
        AdapterID:      getEnv("ZIGBEE_ADAPTER_ID", getEnv("DEVICE_HUB_ZIGBEE_ADAPTER_ID", "zigbee-adapter")),
        AdapterVersion: getEnv("ZIGBEE_ADAPTER_VERSION", getEnv("DEVICE_HUB_ZIGBEE_VERSION", "dev")),
        Postgres: DBConfig{
            User:     getEnv("POSTGRES_USER", "postgres"),
            Password: os.Getenv("POSTGRES_PASSWORD"),
            DBName:   getEnv("POSTGRES_DB", "homenavi"),
            Host:     getEnv("POSTGRES_HOST", "postgres"),
            Port:     getEnv("POSTGRES_PORT", "5432"),
        },
        RedisAddr:     getEnv("REDIS_ADDR", "redis:6379"),
        RedisPassword: os.Getenv("REDIS_PASSWORD"),
    }
    slog.Info("zigbee-adapter config loaded", "port", cfg.Port, "mqtt", cfg.MQTTBrokerURL, "adapter_id", cfg.AdapterID)
    return cfg
}

func getEnv(k, def string) string {
    if v := os.Getenv(k); v != "" {
        return v
    }
    return def
}

func parseBool(val string) bool {
    switch strings.ToLower(strings.TrimSpace(val)) {
    case "1", "true", "yes", "y", "on":
        return true
    default:
        return false
    }
}
