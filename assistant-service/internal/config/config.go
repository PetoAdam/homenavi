package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port                string
	OllamaHost          string
	OllamaModel         string
	OllamaContextLength int
	MaxTokens           int
	Temperature         float64

	// Database
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresSSLMode  string

	// Redis
	RedisAddr     string
	RedisPassword string

	// JWT
	JWTPublicKeyPath string

	// Service URLs for tools
	DeviceHubURL   string
	ERSURL         string
	AutomationURL  string
	HistoryURL     string
	UserServiceURL string

	// Logging
	LogLevel string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                getEnv("ASSISTANT_SERVICE_PORT", "8096"),
		OllamaHost:          getEnv("OLLAMA_HOST", "http://ollama:11434"),
		OllamaModel:         getEnv("OLLAMA_MODEL", "llama3.1:8b"),
		OllamaContextLength: getEnvInt("OLLAMA_CONTEXT_LENGTH", 4096),
		MaxTokens:           getEnvInt("ASSISTANT_MAX_TOKENS", 2048),
		Temperature:         getEnvFloat("ASSISTANT_TEMPERATURE", 0.7),

		PostgresHost:     getEnv("POSTGRES_HOST", "postgres"),
		PostgresPort:     getEnv("POSTGRES_PORT", "5432"),
		PostgresUser:     getEnv("POSTGRES_USER", "user"),
		PostgresPassword: getEnv("POSTGRES_PASSWORD", "password"),
		PostgresDB:       getEnv("POSTGRES_DB", "users"),
		PostgresSSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),

		RedisAddr:     getEnv("REDIS_ADDR", "redis:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		JWTPublicKeyPath: getEnv("JWT_PUBLIC_KEY_PATH", "/app/keys/jwt_public.pem"),

		DeviceHubURL:   getEnv("DEVICE_HUB_URL", "http://device-hub:8090"),
		ERSURL:         getEnv("ERS_URL", "http://entity-registry-service:8095"),
		AutomationURL:  getEnv("AUTOMATION_URL", "http://automation-service:8094"),
		HistoryURL:     getEnv("HISTORY_URL", "http://history-service:8093"),
		UserServiceURL: getEnv("USER_SERVICE_URL", "http://user-service:8001"),

		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	return cfg, nil
}

func (c *Config) DatabaseURL() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.PostgresUser,
		c.PostgresPassword,
		c.PostgresHost,
		c.PostgresPort,
		c.PostgresDB,
		c.PostgresSSLMode,
	)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}
