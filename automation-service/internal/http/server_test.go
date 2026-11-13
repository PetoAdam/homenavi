package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestIntegrationStepsPropagatesTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	var upstreamContext string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamContext = request.Header.Get("traceparent")
		_, _ = writer.Write([]byte(`{"actions":[],"triggers":[],"conditions":[]}`))
	}))
	defer upstream.Close()
	originalClient := upstream.Client()
	originalTransport := originalClient.Transport
	server := NewServer(nil, nil, nil, "", upstream.URL, originalClient)
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	response := httptest.NewRecorder()
	server.handleIntegrationSteps(response, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
	parent.End()
	remote := propagation.TraceContext{}.Extract(t.Context(), propagation.HeaderCarrier{"Traceparent": []string{upstreamContext}})
	spans := recorder.Ended()
	if response.Code != http.StatusOK || len(spans) != 2 || spans[0].Parent().SpanID() != spans[1].SpanContext().SpanID() || oteltrace.SpanContextFromContext(remote).SpanID() != spans[0].SpanContext().SpanID() {
		t.Fatal("integration steps request did not propagate the client span")
	}
	if originalClient.Transport != originalTransport {
		t.Fatal("constructor modified the supplied HTTP client")
	}
}

func TestValidateDefinition_DeviceState_MinimalOK(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{"id": "t1", "kind": "trigger.device_state", "x": 0, "y": 0, "data": map[string]any{"targets": map[string]any{"type": "device", "ids": []string{"dev-1"}}}},
			{"id": "a1", "kind": "action.send_command", "x": 100, "y": 0, "data": map[string]any{"targets": map[string]any{"type": "device", "ids": []string{"dev-1"}}, "command": "set_state"}},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	out, err := validateDefinition(b, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected normalized definition bytes")
	}
}

func TestValidateDefinition_DeviceState_RequiresDeviceID(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{"id": "t1", "kind": "trigger.device_state", "x": 0, "y": 0, "data": map[string]any{}},
			{"id": "a1", "kind": "action.send_command", "x": 100, "y": 0, "data": map[string]any{"targets": map[string]any{"type": "device", "ids": []string{"dev-1"}}, "command": "set_state"}},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	_, err := validateDefinition(b, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateDefinition_Schedule_RequiresCron(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{"id": "t1", "kind": "trigger.schedule", "x": 0, "y": 0, "data": map[string]any{"cron": ""}},
			{"id": "a1", "kind": "action.send_command", "x": 100, "y": 0, "data": map[string]any{"targets": map[string]any{"type": "device", "ids": []string{"dev-1"}}, "command": "set_state"}},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	_, err := validateDefinition(b, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateDefinition_DefaultsCommand(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{"id": "t1", "kind": "trigger.device_state", "x": 0, "y": 0, "data": map[string]any{"targets": map[string]any{"type": "device", "ids": []string{"dev-1"}}}},
			{"id": "a1", "kind": "action.send_command", "x": 100, "y": 0, "data": map[string]any{"targets": map[string]any{"type": "device", "ids": []string{"dev-1"}}, "command": ""}},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	out, err := validateDefinition(b, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected normalized definition bytes")
	}
}

func TestValidateDefinition_Manual_OK(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{"id": "t1", "kind": "trigger.manual", "x": 0, "y": 0, "data": map[string]any{}},
			{"id": "a1", "kind": "action.send_command", "x": 100, "y": 0, "data": map[string]any{"targets": map[string]any{"type": "device", "ids": []string{"dev-1"}}, "command": "set_state"}},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	_, err := validateDefinition(b, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateDefinition_IntegrationAction_OK(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{"id": "t1", "kind": "trigger.manual", "x": 0, "y": 0, "data": map[string]any{}},
			{"id": "a1", "kind": "action.integration", "x": 100, "y": 0, "data": map[string]any{"integration_id": "lg-thinq", "action_id": "integration.lgthinq.sync_now"}},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	_, err := validateDefinition(b, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
