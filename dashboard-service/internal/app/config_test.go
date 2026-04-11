package app

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("DASHBOARD_SERVICE_PORT", "9999")
	t.Setenv("JWT_PUBLIC_KEY_PATH", "/tmp/public.pem")
	t.Setenv("INTEGRATION_PROXY_URL", "http://integration-proxy:8096")
	t.Setenv("POSTGRES_HOST", "db")

	cfg := LoadConfig()
	if cfg.Port != "9999" || cfg.JWTPublicKeyPath != "/tmp/public.pem" || cfg.IntegrationProxyURL != "http://integration-proxy:8096" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.DB.Host != "db" {
		t.Fatalf("unexpected db config: %#v", cfg.DB)
	}
}
