package observability

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestAutomaticMessagePropagation(t *testing.T) {
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "auto")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	if MessagePropagationEnabled() || TraceMetadataEnabled() {
		t.Fatal("automatic tracing should be disabled without an exporter")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://collector:4318/v1/traces")
	if !MessagePropagationEnabled() || !TraceMetadataEnabled() {
		t.Fatal("automatic tracing should be enabled with an exporter")
	}
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "false")
	if MessagePropagationEnabled() {
		t.Fatal("explicit propagation setting was ignored")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	if !MessagePropagationEnabled() || !TraceMetadataEnabled() {
		t.Fatal("explicit propagation should work without an exporter")
	}
}

func TestDeferredSpanFreshStaleAndInvalidContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	now := time.Now()
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	metadata := CaptureTraceContext(ctx)
	_, fresh := StartDeferredSpan(metadata, now, now, "deferred")
	fresh.End()
	_, stale := StartDeferredSpan(metadata, now.Add(-time.Hour), now, "deferred")
	stale.End()
	_, invalid := StartDeferredSpan(&hdp.TraceContext{Traceparent: strings.Repeat("a", 257)}, now, now, "deferred")
	invalid.End()
	_, missing := StartDeferredSpan(nil, now, now, "deferred")
	missing.End()
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 5 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() || spans[0].SpanContext().TraceID() != parent.SpanContext().TraceID() {
		t.Fatal("fresh deferred span did not continue its parent")
	}
	if spans[1].Parent().IsValid() || spans[1].SpanContext().TraceID() == parent.SpanContext().TraceID() || len(spans[1].Links()) != 1 || spans[1].Links()[0].SpanContext.SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("stale deferred span did not start a linked trace")
	}
	for _, span := range spans[2:4] {
		if span.Parent().IsValid() || len(span.Links()) != 0 {
			t.Fatal("invalid or missing metadata must start an independent trace")
		}
	}
}

func TestCorrelatedConsumerRejectsAmbiguousParentage(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	now := time.Now()
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	metadata := CaptureTraceContext(ctx)
	for _, test := range []struct {
		payload  map[string]any
		retained bool
	}{
		{payload: map[string]any{"ts": now.UnixMilli()}, retained: true},
		{payload: map[string]any{"ts": now.Add(-time.Hour).UnixMilli()}},
		{payload: map[string]any{"ts": now.Add(-2 * time.Minute).UnixMilli()}},
		{payload: map[string]any{}},
		{payload: map[string]any{"ts": now.UnixMilli(), "trace": "invalid"}},
	} {
		payload, err := json.Marshal(test.payload)
		if err != nil {
			t.Fatal(err)
		}
		_, span := StartCorrelatedMessageConsumer(payload, test.retained, now, metadata, now)
		span.End()
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 6 {
		t.Fatalf("expected five consumers and parent, got %d", len(spans))
	}
	for _, span := range spans[:5] {
		if span.Parent().IsValid() || span.SpanContext().TraceID() == parent.SpanContext().TraceID() {
			t.Fatal("ambiguous result continued the pending command trace")
		}
	}
}

func TestCorrelatedConsumerDescribesRecoveredContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	now := time.Now()
	ctx, parent := provider.Tracer("test").Start(t.Context(), "command")
	payload, err := json.Marshal(map[string]any{"schema": "hdp.v1", "type": "command_result", "ts": now.UnixMilli(), "corr": "cmd-1"})
	if err != nil {
		t.Fatal(err)
	}
	_, span := StartCorrelatedMessageConsumer(payload, false, now, CaptureTraceContext(ctx), now, "homenavi/hdp/device/command_result/mock/device")
	span.End()
	parent.End()
	got := recorder.Ended()[0]
	if got.Name() != "mqtt process HDP command result" || got.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("correlated result lost its name or parent")
	}
	attributes := make(map[string]string)
	for _, value := range got.Attributes() {
		attributes[string(value.Key)] = value.Value.Emit()
	}
	if attributes["homenavi.hdp.correlation_id"] != "cmd-1" || attributes["messaging.destination.name"] != "homenavi/hdp/device/command_result/mock/device" {
		t.Fatalf("correlated result missing HDP details: %v", attributes)
	}
}

func TestMessageTraceParentAndReplay(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	now := time.Now()
	ctx, root := provider.Tracer("test").Start(t.Context(), "command")
	producerCtx, producer := StartMessageProducer(ctx)
	envelope := map[string]any{"ts": now.UnixMilli(), "corr": "cmd-1", "extra": "preserved"}
	InjectMessageContext(producerCtx, envelope)
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	producer.End()
	_, consumer := StartMessageConsumer(t.Context(), payload, false, now)
	consumer.End()
	_, replay := StartMessageConsumer(t.Context(), payload, true, now)
	replay.End()
	_, stale := StartMessageConsumer(t.Context(), payload, false, now.Add(time.Hour))
	stale.End()
	root.End()
	spans := recorder.Ended()
	if len(spans) != 5 {
		t.Fatalf("expected five spans, got %d", len(spans))
	}
	if spans[0].SpanKind() != oteltrace.SpanKindProducer || spans[1].SpanKind() != oteltrace.SpanKindConsumer || spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() || spans[1].SpanContext().TraceID() != root.SpanContext().TraceID() {
		t.Fatal("fresh message did not retain producer parent and trace ID")
	}
	for _, span := range spans[2:4] {
		if span.Parent().IsValid() || span.SpanContext().TraceID() == root.SpanContext().TraceID() || len(span.Links()) != 1 || span.Links()[0].SpanContext.SpanID() != spans[0].SpanContext().SpanID() {
			t.Fatal("replay or stale message did not start a linked trace")
		}
	}
	if envelope["corr"] != "cmd-1" || envelope["extra"] != "preserved" {
		t.Fatal("injection modified unrelated fields")
	}
}

func TestMessageConsumerIgnoresInvalidMetadata(t *testing.T) {
	provider := trace.NewTracerProvider()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	for _, payload := range []string{``, `[]`, `{"trace":123}`, `{"trace":{"traceparent":"invalid"}}`} {
		_, span := StartMessageConsumer(t.Context(), []byte(payload), false, time.Now())
		if !span.SpanContext().IsValid() {
			t.Fatalf("invalid span for payload %q", payload)
		}
		span.End()
	}
}

func TestMessageConsumerDescribesHDPWithoutPayloadData(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	_, span := StartMessageConsumer(t.Context(), []byte(`{"schema":"hdp.v1","type":"command_result","device_id":"zigbee/device","corr":"cmd-1","status":"completed","success":true,"terminal":true,"error":"secret","args":{"password":"secret"}}`), true, time.Now(), "homenavi/hdp/device/command_result/zigbee/device")
	span.End()
	got := recorder.Ended()[0]
	if got.Name() != "mqtt process HDP command result" {
		t.Fatalf("unexpected span name %q", got.Name())
	}
	attributes := make(map[string]string)
	for _, value := range got.Attributes() {
		attributes[string(value.Key)] = value.Value.Emit()
	}
	for key, want := range map[string]string{"messaging.operation.name": "process", "messaging.mqtt.retain": "true", "messaging.destination.name": "homenavi/hdp/device/command_result/zigbee/device", "homenavi.hdp.type": "command_result", "homenavi.hdp.device_id": "zigbee/device", "homenavi.hdp.correlation_id": "cmd-1", "homenavi.hdp.status": "completed", "homenavi.hdp.success": "true", "homenavi.hdp.terminal": "true"} {
		if attributes[key] != want {
			t.Errorf("%s = %q, want %q", key, attributes[key], want)
		}
	}
	for key, value := range attributes {
		if strings.Contains(key, "error") || strings.Contains(value, "secret") {
			t.Fatalf("sensitive payload data recorded: %s=%s", key, value)
		}
	}
}

func TestHDPDescriptionIgnoresInvalidAndOversizedFields(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	for _, test := range []struct {
		payload string
		topic   string
		name    string
	}{
		{payload: `not-json`, topic: "other/private", name: "messaging.process"},
		{payload: `{"schema":"other","type":"command","device_id":"private"}`, topic: "other/private", name: "messaging.process"},
		{payload: `{"schema":"hdp.v1","type":"unknown","device_id":"private"}`, topic: "other/private", name: "messaging.process"},
		{payload: `{"schema":"hdp.v1","type":"command","device_id":"` + strings.Repeat("a", 129) + `"}`, topic: "other/private", name: "mqtt process HDP command"},
	} {
		_, span := StartMessageConsumer(t.Context(), []byte(test.payload), false, time.Now(), test.topic)
		span.End()
		got := recorder.Ended()[len(recorder.Ended())-1]
		if got.Name() != test.name {
			t.Errorf("span name = %q, want %q", got.Name(), test.name)
		}
		for _, value := range got.Attributes() {
			if value.Key == "messaging.destination.name" || value.Key == "homenavi.hdp.device_id" {
				t.Errorf("unexpected attribute %s=%s", value.Key, value.Value.Emit())
			}
		}
	}
}

func TestHDPDescriptionCompatibleFields(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	for _, test := range []struct {
		name        string
		fields      string
		correlation string
		success     string
	}{
		{name: "legacy correlation", fields: `"correlation_id":"legacy","success":"true"`, correlation: "legacy", success: "true"},
		{name: "preferred correlation", fields: `"corr":" primary ","correlation_id":"legacy","success":1`, correlation: "primary", success: "true"},
		{name: "blank correlation", fields: `"corr":"  ","correlation_id":" legacy ","success":"OFF"`, correlation: "legacy", success: "false"},
		{name: "invalid correlation", fields: `"corr":42,"correlation_id":"legacy","success":0`, correlation: "legacy", success: "false"},
		{name: "invalid optional fields", fields: `"corr":"primary","command":{},"terminal":[],"success":"unknown"`, correlation: "primary"},
		{name: "absent success", fields: `"corr":"primary","success":null`, correlation: "primary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := []byte(`{"schema":"hdp.v1","type":"command_result","device_id":"mock/device","status":"applied",` + test.fields + `}`)
			_, span := StartMessageConsumer(t.Context(), payload, false, time.Now())
			span.End()
			got := recorder.Ended()[len(recorder.Ended())-1]
			if got.Name() != "mqtt process HDP command result" {
				t.Fatalf("unexpected span name %q", got.Name())
			}
			attributes := make(map[string]string)
			for _, value := range got.Attributes() {
				attributes[string(value.Key)] = value.Value.Emit()
			}
			for key, want := range map[string]string{"homenavi.hdp.correlation_id": test.correlation, "homenavi.hdp.device_id": "mock/device", "homenavi.hdp.status": "applied"} {
				if attributes[key] != want {
					t.Errorf("%s = %q, want %q", key, attributes[key], want)
				}
			}
			if value, ok := attributes["homenavi.hdp.success"]; value != test.success || ok != (test.success != "") {
				t.Errorf("success = %q (present %t), want %q", value, ok, test.success)
			}
			if _, ok := attributes["homenavi.hdp.terminal"]; ok {
				t.Error("invalid or absent terminal value was recorded")
			}
		})
	}
}

func TestMessageBoolCompatibility(t *testing.T) {
	for _, test := range []struct {
		raw   string
		value bool
		valid bool
	}{
		{raw: `true`, value: true, valid: true},
		{raw: `false`, valid: true},
		{raw: `" YES "`, value: true, valid: true},
		{raw: `"on"`, value: true, valid: true},
		{raw: `"enabled"`, value: true, valid: true},
		{raw: `"1"`, value: true, valid: true},
		{raw: `"no"`, valid: true},
		{raw: `"off"`, valid: true},
		{raw: `"disabled"`, valid: true},
		{raw: `"standby"`, valid: true},
		{raw: `"0"`, valid: true},
		{raw: `-1`, value: true, valid: true},
		{raw: `0`, valid: true},
		{raw: `null`},
		{raw: `[]`},
		{raw: `"unknown"`},
		{raw: `"` + strings.Repeat("a", 129) + `"`},
	} {
		t.Run(test.raw, func(t *testing.T) {
			value, valid := messageBool(json.RawMessage(test.raw))
			if value != test.value || valid != test.valid {
				t.Errorf("messageBool(%s) = %t, %t; want %t, %t", test.raw, value, valid, test.value, test.valid)
			}
		})
	}
}
