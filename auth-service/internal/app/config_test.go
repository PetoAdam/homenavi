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
	t.Setenv("TOTP_ENCRYPTION_KEY", "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=")
	t.Setenv("AUTH_SERVICE_PORT", "9000")
	t.Setenv("REDIS_ADDR", "redis:6380")
	t.Setenv("USER_SERVICE_URL", "http://user-service:9001")
	t.Setenv("LOGIN_MAX_FAILURES", "7")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Port != "9000" || len(cfg.Redis.Addrs) != 1 || cfg.Redis.Addrs[0] != "redis:6380" || cfg.UserServiceURL != "http://user-service:9001" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.Redis.Mode != "standalone" {
		t.Fatalf("expected standalone redis mode, got %q", cfg.Redis.Mode)
	}
	if cfg.LoginMaxFailures != 7 {
		t.Fatalf("expected login max failures override, got %d", cfg.LoginMaxFailures)
	}
	if cfg.JWTPrivateKey == nil {
		t.Fatal("expected jwt private key to be loaded")
	}
}

func TestLoadConfigSentinelRedis(t *testing.T) {
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
	t.Setenv("TOTP_ENCRYPTION_KEY", "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=")
	t.Setenv("REDIS_MODE", "sentinel")
	t.Setenv("REDIS_SENTINEL_ADDRS", "redis-sentinel-0:26379, redis-sentinel-1:26379")
	t.Setenv("REDIS_MASTER_NAME", "homenavi-redis")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Redis.Mode != "sentinel" {
		t.Fatalf("expected sentinel mode, got %q", cfg.Redis.Mode)
	}
	if len(cfg.Redis.Addrs) != 2 {
		t.Fatalf("expected two sentinel addresses, got %#v", cfg.Redis.Addrs)
	}
	if cfg.Redis.MasterName != "homenavi-redis" {
		t.Fatalf("expected master name to be set, got %q", cfg.Redis.MasterName)
	}
}

func TestLoadConfigReadsTOTPEncryptionKeyFile(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "jwt_private.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write jwt key: %v", err)
	}
	totpKeyPath := filepath.Join(dir, "totp_encryption.key")
	if err := os.WriteFile(totpKeyPath, []byte("MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=\n"), 0o600); err != nil {
		t.Fatalf("write TOTP key: %v", err)
	}
	t.Setenv("JWT_PRIVATE_KEY_PATH", keyPath)
	t.Setenv("TOTP_ENCRYPTION_KEY", "")
	t.Setenv("TOTP_ENCRYPTION_KEY_PATH", totpKeyPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.TOTPEncryptionKey) != 32 {
		t.Fatalf("TOTP key length = %d, want 32", len(cfg.TOTPEncryptionKey))
	}
}

func TestLoadConfigParsesTrustedOAuthClients(t *testing.T) {
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
	t.Setenv("TOTP_ENCRYPTION_KEY", "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=")
	t.Setenv("MCP_RESOURCE_URI", "https://home.example/mcp")
	t.Setenv("OAUTH_TRUSTED_CLIENTS", `[{"client_id":"mcp-cli","display_name":"MCP CLI","redirect_uris":["https://client.example/callback"],"scopes":["home.devices.read"],"enabled":true}]`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.OAuthMCPResource != "https://home.example/mcp" || len(cfg.OAuthTrustedClients) != 1 || cfg.OAuthTrustedClients[0].ClientID != "mcp-cli" {
		t.Fatalf("unexpected OAuth config: %#v", cfg)
	}
}
