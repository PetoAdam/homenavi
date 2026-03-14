package server

import (
	"io"
	"log"
	"testing"

	"homenavi/integration-proxy/internal/config"
)

func TestRuntimeModeAutoDetectHelmFromKubernetesHost(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "auto")
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("INTEGRATIONS_COMPOSE_ENABLED", "false")

	s := New(log.New(io.Discard, "", 0), nil, nil, "", "")
	mode, err := s.runtimeMode()
	if err != nil {
		t.Fatalf("runtimeMode returned error: %v", err)
	}
	if mode != runtimeHelm {
		t.Fatalf("expected runtime %q, got %q", runtimeHelm, mode)
	}
}

func TestEnsureMutableRuntimeRejectsGitOps(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "gitops")

	s := New(log.New(io.Discard, "", 0), nil, nil, "", "")
	_, err := s.ensureMutableRuntime()
	if err == nil {
		t.Fatalf("expected gitops read-only error")
	}
}

func TestHelmSpecForPrefersNativeHelmArtifact(t *testing.T) {
	s := New(log.New(io.Discard, "", 0), nil, nil, "", "")
	ic := config.IntegrationConfig{ID: "example"}
	target := &marketplaceIntegration{Version: "1.2.3"}
	target.DeploymentArtifacts.Helm.ChartRef = "oci://example/chart"
	target.DeploymentArtifacts.Helm.Version = "1.2.3"

	spec, err := s.helmSpecFor("example", ic, target)
	if err != nil {
		t.Fatalf("helmSpecFor returned error: %v", err)
	}
	if spec.ChartRef != "oci://example/chart" {
		t.Fatalf("unexpected chart ref: %q", spec.ChartRef)
	}
	if spec.ChartVersion != "1.2.3" {
		t.Fatalf("unexpected chart version: %q", spec.ChartVersion)
	}
	if spec.ReleaseName == "" {
		t.Fatalf("expected release name to be set")
	}
	if spec.Namespace == "" {
		t.Fatalf("expected namespace to be set")
	}
}

func TestHelmSpecForUsesGeneratedArtifactFallback(t *testing.T) {
	s := New(log.New(io.Discard, "", 0), nil, nil, "", "")
	ic := config.IntegrationConfig{ID: "example"}
	target := &marketplaceIntegration{Version: "2.0.0"}
	target.DeploymentArtifacts.K8sGenerated.ChartRef = "oci://example/generated"
	target.DeploymentArtifacts.K8sGenerated.Version = "2.0.0"

	spec, err := s.helmSpecFor("example", ic, target)
	if err != nil {
		t.Fatalf("helmSpecFor returned error: %v", err)
	}
	if spec.ChartRef != "oci://example/generated" {
		t.Fatalf("unexpected chart ref: %q", spec.ChartRef)
	}
}
