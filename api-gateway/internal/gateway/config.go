package gateway

import (
	"fmt"
	"path/filepath"

	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/spf13/viper"
)

type RouteConfig struct {
	Path      string   `mapstructure:"path"`
	Upstream  string   `mapstructure:"upstream"`
	Methods   []string `mapstructure:"methods"`
	Access    string   `mapstructure:"access"`
	Type      string   `mapstructure:"type"`
	RateLimit *struct {
		RPS   int `mapstructure:"rps"`
		Burst int `mapstructure:"burst"`
	} `mapstructure:"rate_limit"`
}

type RateLimitConfig struct {
	Enabled bool `mapstructure:"enabled"`
	RPS     int  `mapstructure:"rps"`
	Burst   int  `mapstructure:"burst"`
}

type Config struct {
	ListenAddr       string          `mapstructure:"listen_addr"`
	Routes           []RouteConfig   `mapstructure:"routes"`
	JWTPublicKeyPath string          `mapstructure:"jwt_public_key_path"`
	RateLimit        RateLimitConfig `mapstructure:"rate_limit"`
}

func LoadConfig(configPath, routesDir string) (Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if routesDir != "" {
		files, err := filepath.Glob(filepath.Join(routesDir, "*.yaml"))
		if err != nil {
			return Config{}, fmt.Errorf("failed to list route yamls: %w", err)
		}
		for _, f := range files {
			v2 := viper.New()
			v2.SetConfigFile(f)
			v2.SetConfigType("yaml")
			if err := v2.ReadInConfig(); err != nil {
				return Config{}, fmt.Errorf("failed to read route config %s: %w", f, err)
			}
			var routes []RouteConfig
			if err := v2.UnmarshalKey("routes", &routes); err != nil {
				return Config{}, fmt.Errorf("failed to unmarshal routes in %s: %w", f, err)
			}
			cfg.Routes = append(cfg.Routes, routes...)
		}
	}

	if envPub := envx.String("JWT_PUBLIC_KEY_PATH", ""); envPub != "" {
		cfg.JWTPublicKeyPath = envPub
	}

	return cfg, nil
}
