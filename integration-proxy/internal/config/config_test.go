package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReturnsEmptyConfigWhenDirectoryPathIsUsed(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, "installed.yaml")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	cfg, err := Load(cfgDir)
	if err != nil {
		t.Fatalf("load config from directory path: %v", err)
	}
	if cfg.Integrations == nil {
		t.Fatalf("expected integrations slice to be initialized")
	}
	if len(cfg.Integrations) != 0 {
		t.Fatalf("expected empty integrations, got %d", len(cfg.Integrations))
	}
}

func TestSaveUsesInstalledYamlInsideDirectoryPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, "installed.yaml")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	expected := Config{
		Integrations: []IntegrationConfig{{
			ID:       "spotify",
			Upstream: "http://spotify:8099",
		}},
	}
	if err := Save(cfgDir, expected); err != nil {
		t.Fatalf("save config using directory path: %v", err)
	}

	resolved := filepath.Join(cfgDir, "installed.yaml")
	if _, err := os.Stat(resolved); err != nil {
		t.Fatalf("expected resolved config file to exist: %v", err)
	}

	actual, err := Load(cfgDir)
	if err != nil {
		t.Fatalf("reload config using directory path: %v", err)
	}
	if len(actual.Integrations) != 1 {
		t.Fatalf("expected 1 integration, got %d", len(actual.Integrations))
	}
	if actual.Integrations[0].ID != expected.Integrations[0].ID {
		t.Fatalf("expected integration id %q, got %q", expected.Integrations[0].ID, actual.Integrations[0].ID)
	}
}
