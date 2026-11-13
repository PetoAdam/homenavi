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
