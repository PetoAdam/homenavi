package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type RouteConfig struct {
	Path      string   `mapstructure:"path"`
	Upstream  string   `mapstructure:"upstream"`
	Methods   []string `mapstructure:"methods"`
	Access    string   `mapstructure:"access"` // "public", "auth", "admin"
	RateLimit *struct {
		RPS   int `mapstructure:"rps"`
		Burst int `mapstructure:"burst"`
	} `mapstructure:"rate_limit"`
}

type GatewayConfig struct {
	ListenAddr      string        `mapstructure:"listen_addr"`
	Routes          []RouteConfig `mapstructure:"routes"`
	JWTSecret       string        `mapstructure:"jwt_secret"`
	JWTPrivateKeyPath string      `mapstructure:"jwt_private_key_path"`
	JWTPublicKeyPath  string      `mapstructure:"jwt_public_key_path"`
	LogLevel        string        `mapstructure:"log_level"`
	RateLimit       struct {
		Enabled bool `mapstructure:"enabled"`
		RPS     int  `mapstructure:"rps"`
		Burst   int  `mapstructure:"burst"`
	} `mapstructure:"rate_limit"`
}

// LoadConfig loads the main config and merges in all route yamls from routesDir
func LoadConfig(configPath, routesDir string) (*GatewayConfig, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg GatewayConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Load all route yamls from routesDir
	if routesDir != "" {
		files, err := filepath.Glob(filepath.Join(routesDir, "*.yaml"))
		if err != nil {
			return nil, fmt.Errorf("failed to list route yamls: %w", err)
		}
		for _, f := range files {
			v2 := viper.New()
			v2.SetConfigFile(f)
			v2.SetConfigType("yaml")
			if err := v2.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("failed to read route config %s: %w", f, err)
			}
			var routes []RouteConfig
			if err := v2.UnmarshalKey("routes", &routes); err != nil {
				return nil, fmt.Errorf("failed to unmarshal routes in %s: %w", f, err)
			}
			cfg.Routes = append(cfg.Routes, routes...)
		}
	}

	// Allow env override for JWT secret and key paths
	if envSecret := os.Getenv("JWT_SECRET"); envSecret != "" {
		cfg.JWTSecret = envSecret
	}
	if envPriv := os.Getenv("JWT_PRIVATE_KEY_PATH"); envPriv != "" {
		cfg.JWTPrivateKeyPath = envPriv
	}
	if envPub := os.Getenv("JWT_PUBLIC_KEY_PATH"); envPub != "" {
		cfg.JWTPublicKeyPath = envPub
	}

	return &cfg, nil
}
