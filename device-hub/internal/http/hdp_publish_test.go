package http

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	model "github.com/PetoAdam/homenavi/device-hub/internal/devices"
	dbinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMetadataAndRemovalEventsPropagateTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	mqtt := &pairingTestMQTTClient{}
	server := NewServer(nil, mqtt)
	ctx, root := provider.Tracer("test").Start(t.Context(), "request")
	server.publishHDPMeta(ctx, &model.Device{Protocol: "mock", ExternalID: "device"})
	server.publishHDPEvent(ctx, "mock/device", "device_removed", nil)
	if len(mqtt.published) != 2 || !mqtt.published[0].retain || mqtt.published[1].retain {
		t.Fatal("metadata and event retain settings changed")
	}
	_, metadataConsumer := observability.StartMessageConsumer(t.Context(), mqtt.published[0].payload, true, time.Now())
	metadataConsumer.End()
	_, eventConsumer := observability.StartMessageConsumer(t.Context(), mqtt.published[1].payload, false, time.Now())
	eventConsumer.End()
	root.End()
	spans := recorder.Ended()
	if len(spans) != 5 || spans[0].Parent().SpanID() != root.SpanContext().SpanID() || spans[1].Parent().SpanID() != root.SpanContext().SpanID() || spans[3].Parent().SpanID() != spans[1].SpanContext().SpanID() {
		t.Fatal("device events did not continue the request trace")
	}
	if spans[0].Name() != "mqtt publish HDP metadata" || spans[1].Name() != "mqtt publish HDP event" || spans[3].Name() != "mqtt process HDP event" {
		t.Fatalf("unexpected HDP span names: %q, %q, %q", spans[0].Name(), spans[1].Name(), spans[3].Name())
	}
	attributes := make(map[string]string)
	for _, value := range spans[1].Attributes() {
		attributes[string(value.Key)] = value.Value.Emit()
	}
	if attributes["messaging.destination.name"] != hdpEventPrefix+"mock/device" || attributes["homenavi.hdp.event"] != "device_removed" || attributes["homenavi.hdp.device_id"] != "mock/device" {
		t.Fatalf("unexpected HDP event attributes: %v", attributes)
	}
	if spans[2].SpanContext().TraceID() == root.SpanContext().TraceID() || len(spans[2].Links()) != 1 || spans[2].Links()[0].SpanContext.SpanID() != spans[0].SpanContext().SpanID() {
		t.Fatal("retained metadata did not link to the publishing trace")
	}
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "false")
	server.publishHDPEvent(ctx, "mock/device", "device_removed", nil)
	var envelope map[string]any
	if err := json.Unmarshal(mqtt.published[2].payload, &envelope); err != nil || envelope["trace"] != nil {
		t.Fatalf("disabled propagation changed event payload: %v %v", envelope, err)
	}
}

func TestMetadataAndRemovalConsumersContinueTrace(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Device{}, &dbinfra.DeviceState{}); err != nil {
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
	server := NewServer(dbinfra.New(database), nil)
	ctx, producer := observability.StartMessageProducer(t.Context())
	metadata := map[string]any{"schema": "hdp.v1", "type": "metadata", "device_id": "thread/one", "protocol": "thread", "manufacturer": "Test", "ts": time.Now().UnixMilli()}
	observability.InjectMessageContext(ctx, metadata)
	payload, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	server.handleHDPMetadataEvent(pairingFakeMessage{topic: hdpMetadataPrefix + "thread/one", payload: payload})
	device, err := server.repo.GetByExternal(t.Context(), "thread", "one")
	if err != nil || device == nil {
		t.Fatalf("metadata was not stored: %v", err)
	}
	event := map[string]any{"schema": "hdp.v1", "type": "event", "device_id": "thread/one", "event": "device_removed", "ts": time.Now().UnixMilli()}
	observability.InjectMessageContext(ctx, event)
	payload, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	server.handleHDPEvent(pairingFakeMessage{topic: hdpEventPrefix + "thread/one", payload: payload})
	device, err = server.repo.GetByExternal(t.Context(), "thread", "one")
	if err != nil || device != nil {
		t.Fatalf("device removal failed: %v", err)
	}
	producer.End()
	spans := recorder.Ended()
	if len(spans) != 3 || spans[0].Parent().SpanID() != producer.SpanContext().SpanID() || spans[1].Parent().SpanID() != producer.SpanContext().SpanID() || spans[0].SpanContext().TraceID() != producer.SpanContext().TraceID() {
		t.Fatal("device-hub consumers did not continue the MQTT trace")
	}
}

func TestCommandTimeoutContinuesStoredTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	mqtt := &pairingTestMQTTClient{}
	server := NewServer(nil, mqtt)
	ctx, root := provider.Tracer("test").Start(t.Context(), "request")
	server.beginCommandLifecycleWithTimeout(ctx, "mock/device", "cmd-1", nil, nil, time.Minute)
	stored, ok, err := server.commandStore.Get(context.Background(), "mock/device", "cmd-1")
	if err != nil || !ok || stored.Trace == nil {
		t.Fatalf("pending trace was not stored: %v", err)
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	stored.Trace = nil
	if err := json.Unmarshal(encoded, &stored); err != nil || stored.Trace == nil {
		t.Fatalf("stored trace did not survive serialization: %v", err)
	}
	stored.ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	if accepted, err := server.commandStore.Put(context.Background(), stored, false); err != nil || !accepted {
		t.Fatalf("unable to prepare expired command: %v", err)
	}
	root.End()
	server.reapExpiredCommands()
	if len(mqtt.published) != 1 {
		t.Fatalf("expected timeout event, got %d", len(mqtt.published))
	}
	var payload map[string]any
	if err := json.Unmarshal(mqtt.published[0].payload, &payload); err != nil || payload["corr"] != "cmd-1" || payload["status"] != "timeout" {
		t.Fatalf("unexpected timeout event: %v %v", payload, err)
	}
	_, consumer := observability.StartMessageConsumer(t.Context(), mqtt.published[0].payload, false, time.Now())
	consumer.End()
	spans := recorder.Ended()
	if len(spans) != 4 || spans[2].Name() != "command.timeout" || spans[2].Parent().SpanID() != root.SpanContext().SpanID() || spans[1].Parent().SpanID() != spans[2].SpanContext().SpanID() || spans[3].Parent().SpanID() != spans[1].SpanContext().SpanID() || spans[3].SpanContext().TraceID() != root.SpanContext().TraceID() {
		t.Fatal("timeout lifecycle did not continue the stored command trace")
	}
	if _, ok, err := server.commandStore.Get(context.Background(), "mock/device", "cmd-1"); err != nil || ok {
		t.Fatal("claimed timeout entry was not removed")
	}
}

func TestUninstrumentedAdapterResultUsesStoredCorrelation(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	mqtt := &pairingTestMQTTClient{}
	server := NewServer(nil, mqtt)
	ctx, root := provider.Tracer("test").Start(t.Context(), "request")
	server.beginCommandLifecycleWithTimeout(ctx, "mock/device", "cmd-1", nil, nil, time.Minute)
	root.End()
	result := map[string]any{"device_id": "mock/device", "corr": "cmd-1", "status": "rejected", "success": false, "ts": time.Now().UnixMilli()}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	server.handleHDPCommandResultEvent(pairingFakeMessage{topic: hdpCommandResultPrefix + "mock/device", payload: payload})
	if len(mqtt.published) != 1 {
		t.Fatalf("expected one lifecycle result, got %d", len(mqtt.published))
	}
	_, consumer := observability.StartMessageConsumer(t.Context(), mqtt.published[0].payload, false, time.Now())
	consumer.End()
	spans := recorder.Ended()
	if len(spans) != 4 || spans[2].Parent().SpanID() != root.SpanContext().SpanID() || spans[1].Parent().SpanID() != spans[2].SpanContext().SpanID() || spans[3].Parent().SpanID() != spans[1].SpanContext().SpanID() || spans[3].SpanContext().TraceID() != root.SpanContext().TraceID() {
		t.Fatal("uninstrumented adapter result did not continue the correlated command trace")
	}
	var lifecycle map[string]any
	if err := json.Unmarshal(mqtt.published[0].payload, &lifecycle); err != nil || lifecycle["corr"] != "cmd-1" {
		t.Fatalf("result correlation was not preserved: %v %v", lifecycle, err)
	}
}

func TestLegacyAdapterResultIncludesTraceDetails(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	mqtt := &pairingTestMQTTClient{}
	server := NewServer(nil, mqtt)
	ctx, root := provider.Tracer("test").Start(t.Context(), "request")
	server.beginCommandLifecycleWithTimeout(ctx, "mock/device", "cmd-1", nil, nil, time.Minute)
	result := map[string]any{"schema": "hdp.v1", "type": "command_result", "device_id": "mock/device", "correlation_id": "cmd-1", "status": "rejected", "success": "false", "ts": time.Now().UnixMilli()}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	server.handleHDPCommandResultEvent(pairingFakeMessage{topic: hdpCommandResultPrefix + "mock/device", payload: payload})
	root.End()
	if len(mqtt.published) != 1 {
		t.Fatalf("expected one lifecycle result, got %d", len(mqtt.published))
	}
	for _, span := range recorder.Ended() {
		if span.Name() != "mqtt process HDP command result" {
			continue
		}
		attributes := make(map[string]string)
		for _, value := range span.Attributes() {
			attributes[string(value.Key)] = value.Value.Emit()
		}
		if span.Parent().SpanID() != root.SpanContext().SpanID() || attributes["homenavi.hdp.correlation_id"] != "cmd-1" || attributes["homenavi.hdp.success"] != "false" || attributes["homenavi.hdp.status"] != "rejected" {
			t.Fatalf("unexpected result span: parent %s, attributes %v", span.Parent().SpanID(), attributes)
		}
		return
	}
	t.Fatal("legacy result span not found")
}

func TestPublishHDPCommandTracePropagation(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	mqtt := &pairingTestMQTTClient{}
	server := NewServer(nil, mqtt)
	ctx, root := provider.Tracer("test").Start(t.Context(), "request")
	if err := server.publishHDPCommand(ctx, "test/device", map[string]any{"command": "set_state", "corr": "cmd-1"}); err != nil {
		t.Fatal(err)
	}
	if len(mqtt.published) != 1 {
		t.Fatalf("expected one message, got %d", len(mqtt.published))
	}
	var payload map[string]any
	if err := json.Unmarshal(mqtt.published[0].payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["corr"] != "cmd-1" || payload["trace"] == nil {
		t.Fatalf("missing correlation or trace: %v", payload)
	}
	_, consumer := observability.StartMessageConsumer(t.Context(), mqtt.published[0].payload, false, time.Now())
	consumer.End()
	root.End()
	spans := recorder.Ended()
	if len(spans) != 3 || spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() || spans[1].SpanContext().TraceID() != root.SpanContext().TraceID() {
		t.Fatal("command consumer is not connected to the HTTP trace")
	}
}

func TestPublishHDPCommandOmitsTraceByDefault(t *testing.T) {
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "false")
	provider := trace.NewTracerProvider()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	ctx, span := provider.Tracer("test").Start(t.Context(), "request")
	defer span.End()
	mqtt := &pairingTestMQTTClient{}
	server := NewServer(nil, mqtt)
	if err := server.publishHDPCommand(ctx, "mock/device", map[string]any{"command": "refresh", "corr": "cmd-1"}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(mqtt.published[0].payload, &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["trace"]; exists {
		t.Fatal("trace metadata must be opt-in")
	}
}

func TestUntracedStateDoesNotCreateSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	server := NewServer(nil, &pairingTestMQTTClient{})
	server.handleHDPStateEvent(pairingFakeMessage{topic: hdpStatePrefix + "mock/device", payload: []byte(`{"device_id":"mock/device","state":{"on":true}}`)})
	if got := len(recorder.Ended()); got != 0 {
		t.Fatalf("expected no span for untraced state, got %d", got)
	}
}

func TestCommandResultContinuesTraceToLifecycleEvent(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	mqtt := &pairingTestMQTTClient{}
	server := NewServer(nil, mqtt)
	ctx, root := provider.Tracer("test").Start(t.Context(), "command")
	producerCtx, producer := observability.StartMessageProducer(ctx)
	result := map[string]any{"device_id": "mock/device", "corr": "cmd-1", "status": "rejected", "success": false, "ts": time.Now().UnixMilli()}
	observability.InjectMessageContext(producerCtx, result)
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	producer.End()
	server.handleHDPCommandResultEvent(pairingFakeMessage{topic: hdpCommandResultPrefix + "mock/device", payload: payload})
	if len(mqtt.published) != 1 {
		t.Fatalf("expected one lifecycle event, got %d", len(mqtt.published))
	}
	_, eventConsumer := observability.StartMessageConsumer(t.Context(), mqtt.published[0].payload, false, time.Now())
	eventConsumer.End()
	root.End()
	spans := recorder.Ended()
	if len(spans) != 5 || spans[2].Parent().SpanID() != spans[0].SpanContext().SpanID() || spans[1].Parent().SpanID() != spans[2].SpanContext().SpanID() || spans[3].Parent().SpanID() != spans[1].SpanContext().SpanID() || spans[3].SpanContext().TraceID() != root.SpanContext().TraceID() {
		t.Fatal("result and lifecycle events do not form a connected trace")
	}
	var lifecycle map[string]any
	if err := json.Unmarshal(mqtt.published[0].payload, &lifecycle); err != nil || lifecycle["corr"] != "cmd-1" {
		t.Fatalf("lifecycle correlation not preserved: %v, %v", lifecycle, err)
	}
}
