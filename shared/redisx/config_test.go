package redisx

import "testing"

func TestLoadConfigStandaloneFallback(t *testing.T) {
	cfg, err := LoadConfig(Config{Addrs: []string{"redis:6379"}})
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Mode != ModeStandalone {
		t.Fatalf("expected standalone mode, got %q", cfg.Mode)
	}
	if len(cfg.Addrs) != 1 || cfg.Addrs[0] != "redis:6379" {
		t.Fatalf("unexpected addrs: %#v", cfg.Addrs)
	}
}

func TestLoadConfigSentinel(t *testing.T) {
	t.Setenv("REDIS_MODE", "sentinel")
	t.Setenv("REDIS_SENTINEL_ADDRS", "redis-sentinel-0:26379,redis-sentinel-1:26379")
	t.Setenv("REDIS_MASTER_NAME", "homenavi-redis")

	cfg, err := LoadConfig(Config{})
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Mode != ModeSentinel {
		t.Fatalf("expected sentinel mode, got %q", cfg.Mode)
	}
	if cfg.MasterName != "homenavi-redis" {
		t.Fatalf("unexpected master name: %q", cfg.MasterName)
	}
	if len(cfg.Addrs) != 2 {
		t.Fatalf("unexpected addrs: %#v", cfg.Addrs)
	}
}

func TestFailoverOptionsUsesSentinelAddresses(t *testing.T) {
	cfg := Config{
		Mode:       ModeSentinel,
		Addrs:      []string{"redis-sentinel-0:26379", "redis-sentinel-1:26379"},
		MasterName: "homenavi-redis",
		Password:   "secret",
		DB:         2,
	}

	options := cfg.FailoverOptions()
	if options.MasterName != "homenavi-redis" {
		t.Fatalf("unexpected master name: %q", options.MasterName)
	}
	if options.Password != "secret" {
		t.Fatalf("unexpected password: %q", options.Password)
	}
	if options.DB != 2 {
		t.Fatalf("unexpected db: %d", options.DB)
	}
	if len(options.SentinelAddrs) != 2 || options.SentinelAddrs[0] != "redis-sentinel-0:26379" {
		t.Fatalf("unexpected sentinel addrs: %#v", options.SentinelAddrs)
	}
}
