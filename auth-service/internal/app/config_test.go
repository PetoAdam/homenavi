package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "jwt_private.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	t.Setenv("JWT_PRIVATE_KEY_PATH", keyPath)
	t.Setenv("AUTH_SERVICE_PORT", "9000")
	t.Setenv("REDIS_ADDR", "redis:6380")
	t.Setenv("USER_SERVICE_URL", "http://user-service:9001")
	t.Setenv("LOGIN_MAX_FAILURES", "7")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Port != "9000" || cfg.RedisAddr != "redis:6380" || cfg.UserServiceURL != "http://user-service:9001" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.LoginMaxFailures != 7 {
		t.Fatalf("expected login max failures override, got %d", cfg.LoginMaxFailures)
	}
	if cfg.JWTPrivateKey == nil {
		t.Fatal("expected jwt private key to be loaded")
	}
}
