package app

import (
	"fmt"
	"strings"

	"github.com/PetoAdam/homenavi/api-gateway/internal/gateway"
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/redisx"
)

// Config holds bootstrap settings for api-gateway.
type Config struct {
	Gateway          gateway.Config
	JWTPublicKeyPath string
	Redis            redisx.Config
	CORSAllowOrigins string
}

func LoadConfig(args []string) (Config, error) {
	configPath := "config/gateway.yaml"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		configPath = strings.TrimSpace(args[0])
	}
	routesDir := envx.String("API_GATEWAY_ROUTES_DIR", "config/routes")
	gatewayConfig, err := gateway.LoadConfig(configPath, routesDir)
	if err != nil {
		return Config{}, err
	}
	redisConfig, err := redisx.LoadConfig(redisx.Config{Addrs: []string{"redis:6379"}})
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Gateway:          gatewayConfig,
		JWTPublicKeyPath: envx.String("JWT_PUBLIC_KEY_PATH", gatewayConfig.JWTPublicKeyPath),
		Redis:            redisConfig,
		CORSAllowOrigins: envx.String("CORS_ALLOW_ORIGINS", ""),
	}
	if strings.TrimSpace(cfg.JWTPublicKeyPath) == "" {
		return Config{}, fmt.Errorf("JWT_PUBLIC_KEY_PATH not set")
	}
	return cfg, nil
}
