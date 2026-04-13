package app

import (
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg := LoadConfig()
	if cfg.ListenAddr != ":8099" {
		t.Fatalf("expected default listen addr, got %q", cfg.ListenAddr)
	}
	if cfg.RefreshInterval != 30*time.Second {
		t.Fatalf("expected default refresh interval, got %s", cfg.RefreshInterval)
	}
	if cfg.UpdateCheckInterval != 15*time.Minute {
		t.Fatalf("expected default update interval, got %s", cfg.UpdateCheckInterval)
	}
}
