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
	DeviceHubURL       string
	HistoryURL         string
	EntityRegistryURL  string
	AutomationURL      string
	AuthServiceURL     string
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
		DeviceHubURL:       value("DEVICE_HUB_URL", "http://device-hub:8090"),
		HistoryURL:         value("HISTORY_SERVICE_URL", "http://history-service:8093"),
		EntityRegistryURL:  value("ENTITY_REGISTRY_SERVICE_URL", "http://entity-registry-service:8095"),
		AutomationURL:      value("AUTOMATION_SERVICE_URL", "http://automation-service:8094"),
		AuthServiceURL:     value("AUTH_SERVICE_URL", "http://auth-service:8000"),
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
