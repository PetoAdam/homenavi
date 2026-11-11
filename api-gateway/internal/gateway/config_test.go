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

	if err := os.WriteFile(configPath, []byte("listen_addr: ':8080'\njwt_secret: old\njwt_issuer: https://file-auth.example.test\njwt_api_audience: https://file-api.example.test\nrate_limit:\n  enabled: true\n  rps: 10\n  burst: 20\nroutes:\n  - path: /inline\n    upstream: http://inline\n    methods: [GET]\n    access: public\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(routesDir, "extra.yaml"), []byte("routes:\n  - path: /extra\n    upstream: ${TEST_UPSTREAM_URL:-http://default-upstream}\n    methods: [POST]\n    access: admin\n"), 0o644); err != nil {
		t.Fatalf("write route file: %v", err)
	}

	t.Setenv("JWT_PUBLIC_KEY_PATH", "/tmp/jwt.pem")
	t.Setenv("JWT_ISSUER", "https://env-auth.example.test")
	t.Setenv("JWT_API_AUDIENCE", "https://env-api.example.test")
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
	if cfg.JWTIssuer != "https://env-auth.example.test" {
		t.Fatalf("expected JWT issuer environment override, got %q", cfg.JWTIssuer)
	}
	if cfg.JWTAPIAudience != "https://env-api.example.test" {
		t.Fatalf("expected JWT audience environment override, got %q", cfg.JWTAPIAudience)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(cfg.Routes))
	}
	if cfg.Routes[1].Upstream != "ws://emqx:8083/mqtt" {
		t.Fatalf("expected expanded route upstream, got %q", cfg.Routes[1].Upstream)
	}
}

func TestLoadConfigUsesFileJWTValidationValues(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "gateway.yaml")
	if err := os.WriteFile(configPath, []byte("listen_addr: ':8080'\njwt_public_key_path: /tmp/jwt.pem\njwt_issuer: https://file-auth.example.test\njwt_api_audience: https://file-api.example.test\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("JWT_ISSUER", "")
	t.Setenv("JWT_API_AUDIENCE", "")

	cfg, err := LoadConfig(configPath, "")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.JWTIssuer != "https://file-auth.example.test" {
		t.Fatalf("expected file JWT issuer, got %q", cfg.JWTIssuer)
	}
	if cfg.JWTAPIAudience != "https://file-api.example.test" {
		t.Fatalf("expected file JWT audience, got %q", cfg.JWTAPIAudience)
	}
}

func TestDiscoveryRoutesRequireBothScopes(t *testing.T) {
	cfg, err := LoadConfig("../../config/gateway.yaml", "../../config/routes")
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"/api/mcp/ers/discovery:search": "search_devices", "/api/mcp/ers/discovery:room-state": "get_room_state"}
	for _, route := range cfg.Routes {
		tool, found := expected[route.Path]
		if !found {
			continue
		}
		if route.Access != "delegated" || route.Scope != "home.devices.read home.inventory.read" || route.Tool != tool || len(route.Methods) != 1 || route.Methods[0] != "POST" || route.Upstream != "http://entity-registry-service:8095/internal/discovery/search" {
			t.Fatalf("unsafe discovery route: %+v", route)
		}
		delete(expected, route.Path)
	}
	if len(expected) != 0 {
		t.Fatalf("missing discovery routes: %v", expected)
	}
}
