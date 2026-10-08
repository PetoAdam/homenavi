package app

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Enabled            bool
	Port               string
	Issuer             string
	Resource           string
	PublicKeyPath      string
	AuthServiceURL     string
	APIGatewayURL      string
	AllowedOrigins     map[string]struct{}
	EnabledTools       map[string]struct{}
	RateLimitPerMinute int
}

func LoadConfig() Config {
	return Config{
		Enabled:            !strings.EqualFold(os.Getenv("MCP_ENABLED"), "false"),
		Port:               value("MCP_SERVICE_PORT", "8096"),
		Issuer:             strings.TrimSpace(os.Getenv("MCP_AUTHORIZATION_SERVER_ISSUER")),
		Resource:           strings.TrimSpace(os.Getenv("MCP_RESOURCE_URI")),
		PublicKeyPath:      value("JWT_PUBLIC_KEY_PATH", "/app/keys/jwt_public.pem"),
		AuthServiceURL:     value("AUTH_SERVICE_URL", "http://auth-service:8000"),
		APIGatewayURL:      value("API_GATEWAY_URL", "http://api-gateway:8080"),
		AllowedOrigins:     origins(os.Getenv("MCP_ALLOWED_ORIGINS")),
		EnabledTools:       tools(os.Getenv("MCP_ENABLED_TOOLS")),
		RateLimitPerMinute: positiveInt("MCP_RATE_LIMIT_PER_MINUTE", 60),
	}
}

func tools(value string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, tool := range strings.Split(value, ",") {
		if tool = strings.TrimSpace(tool); tool != "" {
			result[tool] = struct{}{}
		}
	}
	return result
}

func positiveInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func value(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func origins(value string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, origin := range strings.Split(value, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			result[origin] = struct{}{}
		}
	}
	return result
}
