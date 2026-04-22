package gateway

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMergesRouteFilesAndEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "gateway.yaml")
	routesDir := filepath.Join(dir, "routes")
	if err := os.MkdirAll(routesDir, 0o755); err != nil {
		t.Fatalf("mkdir routes: %v", err)
	}

	if err := os.WriteFile(configPath, []byte("listen_addr: ':8080'\njwt_secret: old\nrate_limit:\n  enabled: true\n  rps: 10\n  burst: 20\nroutes:\n  - path: /inline\n    upstream: http://inline\n    methods: [GET]\n    access: public\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(routesDir, "extra.yaml"), []byte("routes:\n  - path: /extra\n    upstream: ${TEST_UPSTREAM_URL:-http://default-upstream}\n    methods: [POST]\n    access: admin\n"), 0o644); err != nil {
		t.Fatalf("write route file: %v", err)
	}

	t.Setenv("JWT_PUBLIC_KEY_PATH", "/tmp/jwt.pem")
	t.Setenv("TEST_UPSTREAM_URL", "ws://emqx:8083/mqtt")

	cfg, err := LoadConfig(configPath, routesDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("expected listen addr, got %q", cfg.ListenAddr)
	}
	if cfg.JWTPublicKeyPath != "/tmp/jwt.pem" {
		t.Fatalf("expected JWT public key path override, got %q", cfg.JWTPublicKeyPath)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(cfg.Routes))
	}
	if cfg.Routes[1].Upstream != "ws://emqx:8083/mqtt" {
		t.Fatalf("expected expanded route upstream, got %q", cfg.Routes[1].Upstream)
	}
}
