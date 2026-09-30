package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigReadsDemoSessionActivityDurations(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "gateway.yaml")
	routesDir := filepath.Join(dir, "routes")
	if err := os.MkdirAll(routesDir, 0o755); err != nil {
		t.Fatalf("mkdir routes: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("listen_addr: ':8080'\nroutes: []\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("API_GATEWAY_ROUTES_DIR", routesDir)
	t.Setenv("JWT_PUBLIC_KEY_PATH", "/tmp/jwt.pem")
	t.Setenv("DEMO_SESSION_ACTIVITY_DEBOUNCE", "75s")
	t.Setenv("DEMO_SESSION_ACTIVITY_TTL", "20m")

	cfg, err := LoadConfig([]string{configPath})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DemoActivityDebounce != 75*time.Second {
		t.Fatalf("expected 75s debounce, got %s", cfg.DemoActivityDebounce)
	}
	if cfg.DemoActivityTTL != 20*time.Minute {
		t.Fatalf("expected 20m ttl, got %s", cfg.DemoActivityTTL)
	}
}