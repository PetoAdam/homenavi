package app

import (
	"testing"
	"time"
)

func TestLoadConfigPrefersPortEnv(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("WEATHER_SERVICE_PORT", "8095")

	cfg := LoadConfig()
	if cfg.Port != "9999" {
		t.Fatalf("expected PORT to win, got %q", cfg.Port)
	}
}

func TestLoadConfigParsesDurationFallbacks(t *testing.T) {
	t.Setenv("CACHE_TTL_MINUTES", "")
	t.Setenv("WEATHER_CACHE_TTL_MINUTES", "")
	t.Setenv("WEATHER_CACHE_TTL", "30s")

	cfg := LoadConfig()
	if cfg.CacheTTL != 30*time.Second {
		t.Fatalf("expected 30s cache TTL, got %s", cfg.CacheTTL)
	}
}
