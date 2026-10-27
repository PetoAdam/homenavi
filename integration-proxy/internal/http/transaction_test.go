package http

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PetoAdam/homenavi/integration-proxy/internal/config"
)

func TestUpdateFailureDoesNotPersistVersion(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "compose")
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "installed.yaml")
	initial := config.Config{
		Integrations: []config.IntegrationConfig{{ID: "spotify", Upstream: "http://spotify:8099", Version: "0.1.0"}},
	}
	if err := config.Save(cfgPath, initial); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	s := New(log.New(io.Discard, "", 0), nil, nil, "", cfgPath)
	if err := s.ReloadFromConfig(); err != nil {
		t.Fatalf("reload config: %v", err)
	}

	target := &marketplaceIntegration{ID: "spotify", Version: "0.2.0", ComposeFile: "http://%"}
	err := s.updateInstalledIntegration(context.Background(), "spotify", target)
	if err == nil {
		t.Fatalf("expected update error")
	}

	after, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config after failure: %v", err)
	}
	if len(after.Integrations) != 1 {
		t.Fatalf("expected 1 integration, got %d", len(after.Integrations))
	}
	if got := after.Integrations[0].Version; got != "0.1.0" {
		t.Fatalf("expected version to remain 0.1.0, got %q", got)
	}
}

func TestResolveComposeFileForOperationPinsHNVersion(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "compose")
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "installed.yaml")
	composeSource := filepath.Join(tmpDir, "source-compose.yml")
	if err := os.WriteFile(composeSource, []byte("services:\n  spotify:\n    image: ghcr.io/petoadam/homenavi-spotify:${HN_VERSION:-latest}\n"), 0o644); err != nil {
		t.Fatalf("write compose source: %v", err)
	}

	s := New(log.New(io.Discard, "", 0), nil, nil, "", cfgPath)
	resolved, err := s.resolveComposeFileForOperation("spotify", composeSource, "v0.6.0")
	if err != nil {
		t.Fatalf("resolve compose file: %v", err)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatalf("read resolved compose file: %v", err)
	}
	if !strings.Contains(string(data), "ghcr.io/petoadam/homenavi-spotify:v0.6.0") {
		t.Fatalf("expected pinned image tag in compose file, got %q", string(data))
	}
}

func TestInstallFailureDoesNotPersistIntegration(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "compose")
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "installed.yaml")
	if err := config.Save(cfgPath, config.Config{Integrations: []config.IntegrationConfig{}}); err != nil {
		t.Fatalf("seed empty config: %v", err)
	}

	s := New(log.New(io.Discard, "", 0), nil, nil, "", cfgPath)

	body := []byte(`{"id":"spotify","upstream":"http://spotify:8099","version":"0.1.0","compose_file":"http://%"}`)
	req := httptest.NewRequest(http.MethodPost, "/integrations/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	s.Routes().ServeHTTP(rw, req)

	if rw.Code < 400 {
		t.Fatalf("expected install failure response, got status %d", rw.Code)
	}

	after, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config after failure: %v", err)
	}
	if len(after.Integrations) != 0 {
		t.Fatalf("expected no persisted integrations after failed install, got %d", len(after.Integrations))
	}
}

func TestInstallReloadFailureRollsBackConfig(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "compose")
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "installed.yaml")
	if err := config.Save(cfgPath, config.Config{Integrations: []config.IntegrationConfig{}}); err != nil {
		t.Fatalf("seed empty config: %v", err)
	}

	s := New(log.New(io.Discard, "", 0), nil, nil, "", cfgPath)

	body := []byte(`{"id":"spotify","upstream":"not-a-url","version":"0.1.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/integrations/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	s.Routes().ServeHTTP(rw, req)

	if rw.Code < 400 {
		t.Fatalf("expected install failure response, got status %d", rw.Code)
	}

	after, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config after rollback: %v", err)
	}
	if len(after.Integrations) != 0 {
		t.Fatalf("expected rollback to empty config, got %d entries", len(after.Integrations))
	}
}

func TestHelmInstallationLifecycleSupportsReinstall(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "helm")
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "installed.yaml")
	if err := config.Save(cfgPath, config.Config{}); err != nil {
		t.Fatalf("seed empty config: %v", err)
	}

	helmLog := filepath.Join(tmpDir, "helm.log")
	helmBin := filepath.Join(tmpDir, "helm")
	if err := os.WriteFile(helmBin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$HELM_LOG\"\n"), 0o755); err != nil {
		t.Fatalf("write fake helm: %v", err)
	}
	t.Setenv("INTEGRATIONS_HELM_BIN", helmBin)
	t.Setenv("HELM_LOG", helmLog)

	marketplace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/integrations/test" {
			t.Fatalf("unexpected marketplace request path %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"id":"test","version":"1.0.0","deployment_artifacts":{"helm":{"chart_ref":"oci://registry.example/test","version":"1.0.0"}}}`)
	}))
	defer marketplace.Close()
	t.Setenv("INTEGRATIONS_MARKETPLACE_API_BASE", marketplace.URL)

	s := New(log.New(io.Discard, "", 0), nil, nil, "", cfgPath)
	post := func(path, body string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rw := httptest.NewRecorder()
		s.Routes().ServeHTTP(rw, req)
		if rw.Code != http.StatusOK {
			t.Fatalf("%s returned status %d: %s", path, rw.Code, rw.Body.String())
		}
	}

	post("/integrations/install", `{"id":"test"}`)
	post("/integrations/uninstall", `{"id":"test"}`)
	post("/integrations/install", `{"id":"test"}`)

	stored, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load final config: %v", err)
	}
	if len(stored.Integrations) != 1 || stored.Integrations[0].ID != "test" {
		t.Fatalf("expected reinstalled integration, got %#v", stored.Integrations)
	}

	commands, err := os.ReadFile(helmLog)
	if err != nil {
		t.Fatalf("read helm log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(commands)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected install, uninstall, reinstall commands, got %q", lines)
	}
	if !strings.HasPrefix(lines[0], "upgrade --install homenavi-int-test oci://registry.example/test") {
		t.Fatalf("unexpected first helm command %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "uninstall homenavi-int-test") {
		t.Fatalf("unexpected uninstall helm command %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "upgrade --install homenavi-int-test oci://registry.example/test") {
		t.Fatalf("unexpected reinstall helm command %q", lines[2])
	}
}
