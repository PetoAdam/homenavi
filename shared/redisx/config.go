package redisx

import (
	"context"
	"fmt"
	"strings"

	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/redis/go-redis/v9"
)

type Mode string

const (
	ModeStandalone Mode = "standalone"
	ModeSentinel   Mode = "sentinel"
)

type Config struct {
	Mode       Mode
	Addrs      []string
	MasterName string
	Password   string
	DB         int
}

func ParseAddresses(value string) []string {
	parts := strings.Split(value, ",")
	addrs := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			addrs = append(addrs, trimmed)
		}
	}
	return addrs
}

func LoadConfig(defaults Config) (Config, error) {
	mode := defaults.Mode
	if mode == "" {
		mode = ModeStandalone
	}
	addrs := ParseAddresses(envx.String("REDIS_SENTINEL_ADDRS", ""))
	if len(addrs) == 0 {
		fallbackAddr := "redis:6379"
		if len(defaults.Addrs) > 0 && strings.TrimSpace(defaults.Addrs[0]) != "" {
			fallbackAddr = defaults.Addrs[0]
		}
		addrs = []string{envx.String("REDIS_ADDR", fallbackAddr)}
	}
	cfg := Config{
		Mode:       Mode(envx.String("REDIS_MODE", string(mode))),
		Addrs:      addrs,
		MasterName: envx.String("REDIS_MASTER_NAME", defaults.MasterName),
		Password:   envx.String("REDIS_PASSWORD", defaults.Password),
		DB:         envx.Int("REDIS_DB", defaults.DB),
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	mode := c.Mode
	if mode == "" {
		mode = ModeStandalone
	}
	if len(c.Addrs) == 0 {
		return fmt.Errorf("redis addresses are required")
	}
	switch mode {
	case ModeStandalone:
		return nil
	case ModeSentinel:
		if strings.TrimSpace(c.MasterName) == "" {
			return fmt.Errorf("redis master name is required for sentinel mode")
		}
		return nil
	default:
		return fmt.Errorf("unsupported redis mode %q", c.Mode)
	}
}

func (c Config) UniversalOptions() *redis.UniversalOptions {
	options := &redis.UniversalOptions{
		Addrs:    c.Addrs,
		Password: c.Password,
		DB:       c.DB,
	}
	if c.Mode == ModeSentinel {
		options.MasterName = c.MasterName
	}
	return options
}

func (c Config) FailoverOptions() *redis.FailoverOptions {
	return &redis.FailoverOptions{
		SentinelAddrs: c.Addrs,
		MasterName:    c.MasterName,
		Password:      c.Password,
		DB:            c.DB,
	}
}

func Connect(ctx context.Context, cfg Config) (redis.UniversalClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	var client redis.UniversalClient
	switch cfg.Mode {
	case ModeSentinel:
		client = redis.NewFailoverClient(cfg.FailoverOptions())
	default:
		client = redis.NewUniversalClient(cfg.UniversalOptions())
	}
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
