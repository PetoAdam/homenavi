package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/integration-proxy/internal/config"
	"github.com/PetoAdam/homenavi/shared/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRuntimeOperationPreservesTraceButNotRequestCancellation(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	requestCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), "secret", "value"))
	requestCtx, parent := provider.Tracer("test").Start(requestCtx, "request")
	cancelRequest()
	operationCtx, finish := startRuntimeOperation(requestCtx, "integration.install", time.Second)
	if operationCtx.Err() != nil || operationCtx.Value("secret") != nil {
		t.Fatal("runtime operation inherited request cancellation or values")
	}
	if _, ok := operationCtx.Deadline(); !ok {
		t.Fatal("runtime operation must retain its timeout")
	}
	finish()
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 2 || spans[0].Name() != "integration.install" || spans[0].Parent().SpanID() != spans[1].SpanContext().SpanID() || spans[0].SpanContext().TraceID() != spans[1].SpanContext().TraceID() {
		t.Fatal("runtime operation did not continue the request trace")
	}
}

func TestProxySupportsIntegrationWithoutTelemetry(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/.well-known/homenavi-integration.json" {
			_, _ = writer.Write([]byte(`{"id":"plain"}`))
			return
		}
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte("integration response"))
	}))
	defer upstream.Close()
	server := New(log.New(io.Discard, "", 0), nil, nil, "", "")
	if err := server.AddIntegration(config.IntegrationConfig{ID: "plain", Upstream: upstream.URL}); err != nil {
		t.Fatal(err)
	}
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	handler := observability.WrapHandler(provider.Tracer("proxy"), "integration-proxy", server.Routes())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/integrations/plain/action", nil))
	if response.Code != http.StatusAccepted || response.Body.String() != "integration response" {
		t.Fatalf("uninstrumented integration failed: %d %q", response.Code, response.Body.String())
	}
	spans := recorder.Ended()
	if len(spans) != 2 || spans[0].Parent().SpanID() != spans[1].SpanContext().SpanID() || spans[0].SpanContext().TraceID() != spans[1].SpanContext().TraceID() {
		t.Fatal("proxy did not record its own connected client span")
	}
}

func TestRuntimeCommandSpansRedactArguments(t *testing.T) {
	t.Setenv("INTEGRATIONS_HELM_BIN", "/bin/false")
	t.Setenv("INTEGRATIONS_COMPOSE_BIN", "/bin/false")
	t.Setenv("INTEGRATIONS_COMPOSE_ENABLED", "true")
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	server := New(nil, nil, nil, "", "")
	ctx, parent := provider.Tracer("test").Start(context.Background(), "operation")
	if server.runHelm(ctx, "secret-helm-argument") == nil || server.runCompose(ctx, "secret-compose-path", "secret-compose-argument") == nil {
		t.Fatal("expected commands to fail")
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 3 {
		t.Fatalf("expected command and parent spans, got %d", len(spans))
	}
	for _, span := range spans[:2] {
		if span.Status().Code != codes.Error || span.Parent().SpanID() != parent.SpanContext().SpanID() || len(span.Attributes()) != 0 || len(span.Events()) != 0 {
			t.Fatalf("command span is unconnected or exposes command details: %#v", span)
		}
	}
}

func TestUpdateRequestConnectsMarketplaceAndOperation(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "compose")
	marketplace := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("traceparent") == "" {
			t.Error("marketplace request has no trace context")
		}
		_, _ = writer.Write([]byte(`{"id":"plain","version":"0.1.0"}`))
	}))
	defer marketplace.Close()
	t.Setenv("INTEGRATIONS_MARKETPLACE_API_BASE", marketplace.URL)
	path := filepath.Join(t.TempDir(), "installed.yaml")
	if err := config.Save(path, config.Config{Integrations: []config.IntegrationConfig{{ID: "plain", Upstream: "http://plain:8099", Version: "0.1.0"}}}); err != nil {
		t.Fatal(err)
	}
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	server := New(log.New(io.Discard, "", 0), nil, nil, "", path)
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	response := httptest.NewRecorder()
	server.handleUpdateIntegration(response, httptest.NewRequest(http.MethodPost, "/integrations/update", bytes.NewBufferString(`{"id":"plain"}`)).WithContext(ctx))
	parent.End()
	if response.Code != http.StatusOK {
		t.Fatalf("update returned %d: %s", response.Code, response.Body.String())
	}
	spans := recorder.Ended()
	if len(spans) != 4 || spans[0].Parent().SpanID() != spans[2].SpanContext().SpanID() || spans[1].Name() != "integration.update" || spans[1].Parent().SpanID() != spans[2].SpanContext().SpanID() || spans[2].Name() != "integration.update_request" || spans[2].Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("marketplace lookup and update operation are not connected to the request")
	}
}

func TestReloadConnectsManifestRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("traceparent") == "" {
			t.Error("manifest request has no trace context")
		}
		_, _ = writer.Write([]byte(`{"id":"plain"}`))
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "installed.yaml")
	if err := config.Save(path, config.Config{Integrations: []config.IntegrationConfig{{ID: "plain", Upstream: upstream.URL}}}); err != nil {
		t.Fatal(err)
	}
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	server := New(log.New(io.Discard, "", 0), nil, nil, "", path)
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	if err := server.reloadFromConfig(ctx); err != nil {
		t.Fatal(err)
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 3 || spans[0].Parent().SpanID() != spans[1].SpanContext().SpanID() || spans[1].Name() != "integration.reload" || spans[1].Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("manifest request was not connected to the reload operation")
	}
}

type restartSpanRecorder struct {
	*tracetest.SpanRecorder
	finished chan struct{}
}

func (recorder *restartSpanRecorder) OnEnd(span trace.ReadOnlySpan) {
	recorder.SpanRecorder.OnEnd(span)
	if span.Name() == "integration.restart" {
		close(recorder.finished)
	}
}

func TestRestartBackgroundContinuesRequestTrace(t *testing.T) {
	t.Setenv("INTEGRATIONS_RUNTIME_MODE", "compose")
	t.Setenv("INTEGRATIONS_COMPOSE_ENABLED", "false")
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"id":"plain"}`))
	}))
	defer upstream.Close()
	server := New(nil, nil, nil, "", "")
	if err := server.AddIntegration(config.IntegrationConfig{ID: "plain", Upstream: upstream.URL}); err != nil {
		t.Fatal(err)
	}
	recorder := &restartSpanRecorder{SpanRecorder: tracetest.NewSpanRecorder(), finished: make(chan struct{})}
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	requestCtx, cancel := context.WithCancel(context.Background())
	requestCtx, parent := provider.Tracer("test").Start(requestCtx, "request")
	response := httptest.NewRecorder()
	server.handleRestartIntegration(response, httptest.NewRequest(http.MethodPost, "/integrations/restart/plain", nil).WithContext(requestCtx))
	cancel()
	parent.End()
	if response.Code != http.StatusAccepted {
		t.Fatalf("restart returned %d", response.Code)
	}
	select {
	case <-recorder.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("background restart did not finish")
	}
	spans := recorder.Ended()
	var restart trace.ReadOnlySpan
	for _, span := range spans {
		if span.Name() == "integration.restart" {
			restart = span
		}
	}
	if restart == nil || restart.Parent().SpanID() != parent.SpanContext().SpanID() || restart.SpanContext().TraceID() != parent.SpanContext().TraceID() {
		t.Fatal("background restart did not continue the request trace")
	}
}

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

func TestRegistryIncludesUpdateAvailability(t *testing.T) {
	s := New(log.New(io.Discard, "", 0), nil, nil, "", "")
	s.proxies["emodul"] = nil
	s.setUpdateState("emodul", func(state *integrationUpdateStatus) {
		state.InstalledVersion = "v0.2.2"
		state.LatestVersion = "v0.2.4"
		state.UpdateAvailable = true
	})

	req := httptest.NewRequest(http.MethodGet, "/integrations/registry.json", nil)
	rw := httptest.NewRecorder()
	s.Routes().ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rw.Code)
	}
	var payload Registry
	if err := json.NewDecoder(rw.Body).Decode(&payload); err != nil {
		t.Fatalf("decode registry response: %v", err)
	}
	if len(payload.Integrations) != 1 || !payload.Integrations[0].UpdateAvailable {
		t.Fatalf("expected an integration with an available update, got %#v", payload.Integrations)
	}
}
