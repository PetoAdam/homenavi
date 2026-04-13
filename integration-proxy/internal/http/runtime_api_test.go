package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/PetoAdam/homenavi/integration-proxy/internal/config"
)

func TestInstallAcceptsLegacyComposeFileField(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "compose")
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "installed.yaml")
	if err := config.Save(cfgPath, config.Config{Integrations: []config.IntegrationConfig{}}); err != nil {
		t.Fatalf("seed empty config: %v", err)
	}

	s := New(log.New(io.Discard, "", 0), nil, nil, "", cfgPath)
	body := []byte(`{"id":"spotify","upstream":"http://spotify:8099","version":"0.1.0","compose_file":"compose.yml"}`)
	req := httptest.NewRequest(http.MethodPost, "/integrations/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	s.Routes().ServeHTTP(rw, req)

	if rw.Code == http.StatusBadRequest {
		t.Fatalf("expected legacy compose_file payload to be accepted, got status %d body=%s", rw.Code, rw.Body.String())
	}
}

func TestInstallStatusResponseRemainsFlat(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "helm")
	s := New(log.New(io.Discard, "", 0), nil, nil, "", "")
	s.setInstallStatus("spotify", "starting", 45, "Starting integration")
	req := httptest.NewRequest(http.MethodGet, "/integrations/install-status/spotify", nil)
	rw := httptest.NewRecorder()
	s.Routes().ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rw.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&payload); err != nil {
		t.Fatalf("decode install status response: %v", err)
	}
	if got := payload["management_mode"]; got != "helm" {
		t.Fatalf("expected management_mode helm, got %v", got)
	}
	if got := payload["stage"]; got != "starting" {
		t.Fatalf("expected flat stage field, got %v", got)
	}
	if _, ok := payload["status"]; ok {
		t.Fatalf("did not expect nested status field in install status response")
	}
}

func TestNormalizeIntegrationUpstreamRewritesLegacyHelmReleaseHost(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "helm")
	s := New(log.New(io.Discard, "", 0), nil, nil, "", "")
	got := s.normalizeIntegrationUpstream(config.IntegrationConfig{
		ID:              "spotify",
		Upstream:        "http://homenavi-int-spotify.homenavi-integrations:8099",
		HelmReleaseName: "homenavi-int-spotify",
		HelmNamespace:   "homenavi-integrations",
		HelmChartRef:    "oci://ghcr.io/petoadam/homenavi-spotify",
	})
	want := "http://homenavi-int-spotify-homenavi-spotify.homenavi-integrations:8099"
	if got != want {
		t.Fatalf("expected rewritten upstream %q, got %q", want, got)
	}
}

func TestDefaultHelmUpstreamUsesServiceName(t *testing.T) {
	s := New(log.New(io.Discard, "", 0), nil, nil, "", "")
	got := s.defaultHelmUpstream(helmDeploymentSpec{
		ReleaseName: "homenavi-int-spotify",
		Namespace:   "homenavi-integrations",
		ChartRef:    "oci://ghcr.io/petoadam/homenavi-spotify",
	})
	want := "http://homenavi-int-spotify-homenavi-spotify.homenavi-integrations:8099"
	if got != want {
		t.Fatalf("expected upstream %q, got %q", want, got)
	}
}

func TestInstallBlockedInGitOpsMode(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "gitops")
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "installed.yaml")
	if err := config.Save(cfgPath, config.Config{Integrations: []config.IntegrationConfig{}}); err != nil {
		t.Fatalf("seed empty config: %v", err)
	}

	s := New(log.New(io.Discard, "", 0), nil, nil, "", cfgPath)
	body := []byte(`{"id":"spotify","upstream":"http://spotify:8099","version":"0.1.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/integrations/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	s.Routes().ServeHTTP(rw, req)

	if rw.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rw.Code)
	}
}

func TestUpdatesResponseIncludesManagementMode(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "gitops")
	s := New(log.New(io.Discard, "", 0), nil, nil, "", "")
	req := httptest.NewRequest(http.MethodGet, "/integrations/updates", nil)
	rw := httptest.NewRecorder()
	s.Routes().ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rw.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&payload); err != nil {
		t.Fatalf("decode updates response: %v", err)
	}
	if got := payload["management_mode"]; got != "gitops" {
		t.Fatalf("expected management_mode gitops, got %v", got)
	}
}
