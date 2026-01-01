package config

import "os"

type Postgres struct {
	User     string
	Password string
	DBName   string
	Host     string
	Port     string
	SSLMode  string
}

type Config struct {
	Port             string
	LogLevel         string
	MQTTBrokerURL    string
	MQTTClientID     string
	JWTPublicKeyPath string
	UserServiceURL   string
	EmailServiceURL  string
	ERSServiceURL    string
	Postgres         Postgres
}

func Load() Config {
	return Config{
		Port:             getenv("AUTOMATION_SERVICE_PORT", "8094"),
		LogLevel:         getenv("LOG_LEVEL", "info"),
		MQTTBrokerURL:    getenv("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
		MQTTClientID:     getenv("AUTOMATION_SERVICE_MQTT_CLIENT_ID", "automation-service"),
		JWTPublicKeyPath: getenv("JWT_PUBLIC_KEY_PATH", ""),
		UserServiceURL:   getenv("USER_SERVICE_URL", "http://user-service:8001"),
		EmailServiceURL:  getenv("EMAIL_SERVICE_URL", "http://email-service:8002"),
		ERSServiceURL:    getenv("ERS_SERVICE_URL", "http://entity-registry-service:8095"),
		Postgres: Postgres{
			User:     getenv("POSTGRES_USER", ""),
			Password: getenv("POSTGRES_PASSWORD", ""),
			DBName:   getenv("POSTGRES_DB", ""),
			Host:     getenv("POSTGRES_HOST", ""),
			Port:     getenv("POSTGRES_PORT", ""),
			SSLMode:  getenv("POSTGRES_SSLMODE", "disable"),
		},
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
