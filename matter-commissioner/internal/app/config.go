package app

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port           string
	BackendCommand string
	BackendArgs    []string
	BackendTimeout time.Duration
}

func LoadConfig() Config {
	return Config{
		Port:           getenv("MATTER_COMMISSIONER_PORT", "8098"),
		BackendCommand: strings.TrimSpace(os.Getenv("MATTER_COMMISSIONER_BACKEND_COMMAND")),
		BackendArgs:    strings.Fields(os.Getenv("MATTER_COMMISSIONER_BACKEND_ARGS")),
		BackendTimeout: getDuration("MATTER_COMMISSIONER_BACKEND_TIMEOUT", 2*time.Minute),
	}
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}